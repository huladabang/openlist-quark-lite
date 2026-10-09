# Quark WebDAV Lite for OpenWrt

A standalone, read-only Quark cloud drive WebDAV gateway for small OpenWrt
routers. It does **not** embed OpenList, its database, frontend or plugin system.

## First release scope

- Browse Quark folders through WebDAV or a basic browser page
- Read, stream and seek files (HTTP Range proxy)
- Three concurrent 10 MiB CDN ranges by default (configurable from 1–4)
- Optional WebDAV Basic authentication
- Select a Quark root folder ID
- Mount a folder by path, for example `/视频`
- Minimal LuCI configuration page and procd service
- OpenWrt 24.10 / `aarch64_cortex-a53`

Writing, uploading, deleting and renaming are intentionally disabled in the
first release. This keeps the package small and prevents accidental cloud-drive
changes while the API implementation is being tested.

## Configuration

Install the IPK, then open **Services → Quark WebDAV**. Enter the complete
Cookie copied from an authenticated request at `pan.quark.cn`, choose a WebDAV
username/password, enable the service and save. Restart the service after a
configuration change.

Default WebDAV URL: `http://router-address:5244/`

Use **挂载目录路径** to expose only one folder. The default `/` exposes the
whole drive. Version 0.2.1 also preserves Quark authorization headers across
cross-domain CDN redirects to avoid the severely throttled fallback path.
Version 0.3.0 adds ordered parallel range fetching. The default temporary
memory ceiling for an active transfer is about 30 MiB.

Version 0.3.1 changes the downloader from stop-and-go batches to a sliding
pipeline: while one ordered chunk is sent to the WebDAV client, later chunks
continue downloading from the CDN.

Version 0.4.0 streams the current range while it is still arriving, starts at
two workers and adapts between one and the configured maximum according to
client wait time. The default part size is 2 MiB. Interrupted parts resume from
the exact received offset and retry up to three times.

## Origin and license

The Quark API request flow is derived from the `quark_uc` driver in
[OpenList](https://github.com/OpenListTeam/OpenList), version 4.2.6. OpenList
and this derivative are licensed under AGPL-3.0-only. No OpenList executable,
database, web frontend or framework code is included in the IPK.

This software calls an unofficial cloud API that may change at any time. Use it
only with your own account and do not expose the service directly to the public
Internet.
