// quark-webdav is a small, read-only Quark cloud drive WebDAV gateway.
// Its API interaction is derived from OpenList's quark_uc driver (AGPL-3.0).
package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiBase = "https://drive.quark.cn/1/clouddrive"
	referer = "https://pan.quark.cn"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
)

var version = "dev"

type file struct {
	FID string `json:"fid"`
	Name string `json:"file_name"`
	Size int64 `json:"size"`
	IsFile bool `json:"file"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

type apiEnvelope struct { Status int `json:"status"`; Code int `json:"code"`; Message string `json:"message"` }
type listResponse struct {
	apiEnvelope
	Data struct{ List []file `json:"list"` } `json:"data"`
	Metadata struct{ Total int `json:"_total"` } `json:"metadata"`
}
type downloadResponse struct { apiEnvelope; Data []struct{ URL string `json:"download_url"` } `json:"data"` }

type client struct { http *http.Client; mu sync.RWMutex; cookie string }

func (c *client) request(method, endpoint string, query url.Values, body io.Reader, out any) error {
	u, _ := url.Parse(apiBase + endpoint)
	q := u.Query(); q.Set("pr", "ucpro"); q.Set("fr", "pc")
	for key, values := range query { for _, value := range values { q.Add(key, value) } }
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(method, u.String(), body); if err != nil { return err }
	c.mu.RLock(); cookie := c.cookie; c.mu.RUnlock()
	req.Header.Set("Cookie", cookie); req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", referer); req.Header.Set("User-Agent", userAgent)
	if body != nil { req.Header.Set("Content-Type", "application/json") }
	resp, err := c.http.Do(req); if err != nil { return err }
	defer resp.Body.Close(); c.refreshCookie(resp.Cookies())
	if resp.StatusCode/100 != 2 { return fmt.Errorf("quark HTTP %d", resp.StatusCode) }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil { return err }
	var env apiEnvelope; b, _ := json.Marshal(out); _ = json.Unmarshal(b, &env)
	if env.Status >= 400 || env.Code != 0 { return fmt.Errorf("quark: %s (code %d)", env.Message, env.Code) }
	return nil
}

func (c *client) refreshCookie(cookies []*http.Cookie) {
	for _, fresh := range cookies {
		if fresh.Name != "__puus" && fresh.Name != "__pus" { continue }
		c.mu.Lock(); parts := strings.Split(c.cookie, ";"); found := false
		for i, part := range parts {
			if strings.TrimSpace(strings.SplitN(part, "=", 2)[0]) == fresh.Name { parts[i] = fresh.Name + "=" + fresh.Value; found = true }
		}
		if !found { parts = append(parts, fresh.Name+"="+fresh.Value) }
		c.cookie = strings.Join(parts, "; "); c.mu.Unlock()
	}
}

func (c *client) list(parent string) ([]file, error) {
	var all []file
	for pageNum := 1; ; pageNum++ {
		q := url.Values{"pdir_fid": {parent}, "_size": {"100"}, "_page": {strconv.Itoa(pageNum)}, "_fetch_total": {"1"}, "fetch_all_file": {"1"}, "fetch_risk_file_name": {"1"}, "_sort": {"file_type:asc,file_name:asc"}}
		var resp listResponse
		if err := c.request(http.MethodGet, "/file/sort", q, nil, &resp); err != nil { return nil, err }
		for i := range resp.Data.List { resp.Data.List[i].Name = html.UnescapeString(resp.Data.List[i].Name) }
		all = append(all, resp.Data.List...)
		if len(all) >= resp.Metadata.Total || len(resp.Data.List) == 0 { return all, nil }
	}
}

func (c *client) resolve(rootID, requestPath string) (file, error) {
	cur := file{FID: rootID}
	clean := strings.Trim(path.Clean("/"+requestPath), "/"); if clean == "" { return cur, nil }
	for _, segment := range strings.Split(clean, "/") {
		children, err := c.list(cur.FID); if err != nil { return file{}, err }
		found := false
		for _, child := range children { if child.Name == segment { cur = child; found = true; break } }
		if !found { return file{}, os.ErrNotExist }
	}
	return cur, nil
}

func (c *client) downloadURL(fid string) (string, error) {
	body, _ := json.Marshal(map[string]any{"fids": []string{fid}})
	var resp downloadResponse
	if err := c.request(http.MethodPost, "/file/download", nil, strings.NewReader(string(body)), &resp); err != nil { return "", err }
	if len(resp.Data) == 0 || resp.Data[0].URL == "" { return "", errors.New("quark returned no download URL") }
	return resp.Data[0].URL, nil
}

type server struct { client *client; rootID, username, password string; parallel int; chunkSize int64 }

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) { w.Header().Set("WWW-Authenticate", `Basic realm="Quark WebDAV"`); http.Error(w, "Unauthorized", http.StatusUnauthorized); return }
	switch r.Method {
	case http.MethodOptions: w.Header().Set("Allow", "OPTIONS, GET, HEAD, PROPFIND"); w.Header().Set("DAV", "1"); w.WriteHeader(http.StatusNoContent)
	case "PROPFIND": s.propfind(w, r)
	case http.MethodGet, http.MethodHead: s.get(w, r)
	default: w.Header().Set("Allow", "OPTIONS, GET, HEAD, PROPFIND"); http.Error(w, "read-only WebDAV", http.StatusMethodNotAllowed)
	}
}

func (s *server) authorized(r *http.Request) bool {
	if s.username == "" && s.password == "" { return true }
	u, p, ok := r.BasicAuth(); if !ok { return false }
	return subtle.ConstantTimeCompare([]byte(u), []byte(s.username)) == 1 && subtle.ConstantTimeCompare([]byte(p), []byte(s.password)) == 1
}

func (s *server) propfind(w http.ResponseWriter, r *http.Request) {
	obj, err := s.client.resolve(s.rootID, r.URL.Path)
	if errors.Is(err, os.ErrNotExist) { http.NotFound(w, r); return }; if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
	items := []davItem{{Href: escapedHref(r.URL.Path, !obj.IsFile), File: obj}}
	if !obj.IsFile && r.Header.Get("Depth") != "0" {
		children, err := s.client.list(obj.FID); if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
		base := strings.TrimSuffix(r.URL.Path, "/") + "/"
		for _, child := range children { items = append(items, davItem{Href: escapedHref(base+url.PathEscape(child.Name), !child.IsFile), File: child}) }
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8"); w.Header().Set("DAV", "1"); w.WriteHeader(207)
	_, _ = io.WriteString(w, xml.Header+`<D:multistatus xmlns:D="DAV:">`)
	for _, item := range items { _, _ = io.WriteString(w, item.xml()) }; _, _ = io.WriteString(w, `</D:multistatus>`)
}

type davItem struct { Href string; File file }
func (d davItem) xml() string {
	name := html.EscapeString(d.File.Name); if name == "" { name = "/" }
	modified := time.UnixMilli(d.File.UpdatedAt).UTC().Format(http.TimeFormat)
	created := time.UnixMilli(d.File.CreatedAt).UTC().Format(time.RFC3339)
	resource := ""; length := strconv.FormatInt(d.File.Size, 10)
	if !d.File.IsFile { resource = "<D:collection/>"; length = "0" }
	return `<D:response><D:href>`+html.EscapeString(d.Href)+`</D:href><D:propstat><D:prop><D:displayname>`+name+`</D:displayname><D:resourcetype>`+resource+`</D:resourcetype><D:getcontentlength>`+length+`</D:getcontentlength><D:getlastmodified>`+modified+`</D:getlastmodified><D:creationdate>`+created+`</D:creationdate></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`
}

func escapedHref(p string, directory bool) string {
	parts := strings.Split(p, "/"); for i := range parts { if decoded, err := url.PathUnescape(parts[i]); err == nil { parts[i] = url.PathEscape(decoded) } }
	result := strings.Join(parts, "/"); if result == "" { result = "/" }; if directory && !strings.HasSuffix(result, "/") { result += "/" }; return result
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	obj, err := s.client.resolve(s.rootID, r.URL.Path)
	if errors.Is(err, os.ErrNotExist) { http.NotFound(w, r); return }; if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
	if !obj.IsFile { s.directoryHTML(w, r, obj); return }
	download, err := s.client.downloadURL(obj.FID); if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
	start, end, partial, err := parseRange(r.Header.Get("Range"), obj.Size)
	if err != nil { w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", obj.Size)); http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable); return }
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(obj.Name)))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if partial { w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, obj.Size)); w.WriteHeader(http.StatusPartialContent) }
	if r.Method == http.MethodHead { return }
	if err := s.parallelCopy(r, w, download, start, end); err != nil { log.Printf("download %s: %v", obj.Name, err) }
}

func parseRange(value string, size int64) (int64, int64, bool, error) {
	if size <= 0 { return 0, 0, false, errors.New("invalid file size") }
	if value == "" { return 0, size-1, false, nil }
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") { return 0, 0, false, errors.New("unsupported range") }
	parts := strings.SplitN(strings.TrimPrefix(value, "bytes="), "-", 2)
	if len(parts) != 2 { return 0, 0, false, errors.New("invalid range") }
	var start, end int64; var err error
	if parts[0] == "" {
		suffix, e := strconv.ParseInt(parts[1], 10, 64); if e != nil || suffix <= 0 { return 0, 0, false, errors.New("invalid suffix range") }
		if suffix > size { suffix = size }; start, end = size-suffix, size-1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64); if err != nil || start < 0 || start >= size { return 0, 0, false, errors.New("range start outside file") }
		end = size-1
		if parts[1] != "" { end, err = strconv.ParseInt(parts[1], 10, 64); if err != nil || end < start { return 0, 0, false, errors.New("invalid range end") }; if end >= size { end = size-1 } }
	}
	return start, end, true, nil
}

type chunkResult struct { index int; data []byte; err error }

func (s *server) parallelCopy(r *http.Request, w io.Writer, download string, start, end int64) error {
	chunkCount := int((end-start)/s.chunkSize) + 1
	pending := make(map[int]<-chan chunkResult, s.parallel)
	launch := func(index int) {
		result := make(chan chunkResult, 1)
		pending[index] = result
		chunkStart := start + int64(index)*s.chunkSize
		chunkEnd := chunkStart+s.chunkSize-1; if chunkEnd > end { chunkEnd = end }
		go func() { data, err := s.fetchChunk(r, download, chunkStart, chunkEnd); result <- chunkResult{index: index, data: data, err: err} }()
	}
	for i := 0; i < s.parallel && i < chunkCount; i++ { launch(i) }
	for index := 0; index < chunkCount; index++ {
		result := <-pending[index]; delete(pending, index)
		if result.err != nil { return result.err }
		if next := index+s.parallel; next < chunkCount { launch(next) }
		if _, err := io.Copy(w, bytes.NewReader(result.data)); err != nil { return err }
	}
	return nil
}

func (s *server) fetchChunk(r *http.Request, download string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, download, nil); if err != nil { return nil, err }
	s.client.mu.RLock(); cookie := s.client.cookie; s.client.mu.RUnlock()
	req.Header.Set("Cookie", cookie); req.Header.Set("Referer", referer); req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := s.client.http.Do(req); if err != nil { return nil, err }; defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent { return nil, fmt.Errorf("CDN ignored range %d-%d: HTTP %d", start, end, resp.StatusCode) }
	expected := end-start+1; data, err := io.ReadAll(io.LimitReader(resp.Body, expected+1)); if err != nil { return nil, err }
	if int64(len(data)) != expected { return nil, fmt.Errorf("short CDN range %d-%d: got %d bytes", start, end, len(data)) }
	return data, nil
}

func (s *server) directoryHTML(w http.ResponseWriter, r *http.Request, dir file) {
	children, err := s.client.list(dir.FID); if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta name=viewport content='width=device-width'><title>Quark WebDAV</title><style>body{font:16px sans-serif;max-width:900px;margin:40px auto;padding:0 16px}a{display:block;padding:10px;border-bottom:1px solid #ddd;text-decoration:none}</style><h1>Quark WebDAV</h1>")
	if path.Clean(r.URL.Path) != "/" { _, _ = io.WriteString(w, `<a href="../">../</a>`) }
	for _, child := range children { suffix := ""; if !child.IsFile { suffix = "/" }; _, _ = io.WriteString(w, `<a href="`+url.PathEscape(child.Name)+suffix+`">`+html.EscapeString(child.Name)+suffix+`</a>`) }
}

func env(key, fallback string) string { if value := os.Getenv(key); value != "" { return value }; return fallback }
func main() {
	listen := env("QUARK_LISTEN", "127.0.0.1:5244"); cookie := os.Getenv("QUARK_COOKIE")
	if cookie == "" { log.Fatal("QUARK_COOKIE is required") }
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	transport.ForceAttemptHTTP2 = true
	httpClient := &http.Client{Transport: transport, Timeout: 0}
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 { return nil }
		previous := via[len(via)-1]
		for _, key := range []string{"Cookie", "Referer", "User-Agent", "Range"} {
			if value := previous.Header.Get(key); value != "" { req.Header.Set(key, value) }
		}
		return nil
	}
	apiClient := &client{http: httpClient, cookie: cookie}
	rootID := env("QUARK_ROOT_ID", "0")
	rootPath := env("QUARK_ROOT_PATH", "/")
	if strings.Trim(path.Clean("/"+rootPath), "/") != "" {
		selected, err := apiClient.resolve(rootID, rootPath)
		if err != nil { log.Fatalf("cannot resolve root path %q: %v", rootPath, err) }
		if selected.IsFile { log.Fatalf("root path %q is a file, not a directory", rootPath) }
		rootID = selected.FID
		log.Printf("mounted Quark folder %s (%s)", rootPath, rootID)
	}
	parallel, _ := strconv.Atoi(env("QUARK_PARALLEL", "3")); if parallel < 1 { parallel = 1 }; if parallel > 4 { parallel = 4 }
	chunkMB, _ := strconv.Atoi(env("QUARK_CHUNK_MB", "10")); if chunkMB < 1 { chunkMB = 1 }; if chunkMB > 32 { chunkMB = 32 }
	s := &server{client: apiClient, rootID: rootID, username: os.Getenv("QUARK_USERNAME"), password: os.Getenv("QUARK_PASSWORD"), parallel: parallel, chunkSize: int64(chunkMB)*1024*1024}
	log.Printf("quark-webdav %s listening on %s (read-only)", version, listen); log.Fatal(http.ListenAndServe(listen, s))
}
