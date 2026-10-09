// quark-webdav is a small, read-only Quark cloud drive WebDAV gateway.
// Its API interaction is derived from OpenList's quark_uc driver (AGPL-3.0).
package main

import (
	"crypto/subtle"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
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

type server struct { client *client; rootID, username, password string }

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
	req, err := http.NewRequestWithContext(r.Context(), r.Method, download, nil); if err != nil { http.Error(w, err.Error(), 500); return }
	s.client.mu.RLock(); cookie := s.client.cookie; s.client.mu.RUnlock()
	req.Header.Set("Cookie", cookie); req.Header.Set("Referer", referer); req.Header.Set("User-Agent", userAgent)
	if value := r.Header.Get("Range"); value != "" { req.Header.Set("Range", value) }
	resp, err := s.client.http.Do(req); if err != nil { http.Error(w, err.Error(), http.StatusBadGateway); return }; defer resp.Body.Close()
	for _, key := range []string{"Accept-Ranges", "Content-Length", "Content-Range", "Content-Type", "ETag", "Last-Modified"} { if value := resp.Header.Get(key); value != "" { w.Header().Set(key, value) } }
	w.WriteHeader(resp.StatusCode); if r.Method == http.MethodGet { _, _ = io.Copy(w, resp.Body) }
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
	s := &server{client: &client{http: &http.Client{Timeout: 0}, cookie: cookie}, rootID: env("QUARK_ROOT_ID", "0"), username: os.Getenv("QUARK_USERNAME"), password: os.Getenv("QUARK_PASSWORD")}
	log.Printf("quark-webdav %s listening on %s (read-only)", version, listen); log.Fatal(http.ListenAndServe(listen, s))
}
