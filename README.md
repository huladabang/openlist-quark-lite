# Quark WebDAV Lite for OpenWrt

A standalone, read-only Quark cloud drive WebDAV gateway for small OpenWrt
routers. It does **not** embed OpenList, its database, frontend or plugin system.

## First release scope

- Browse Quark folders through WebDAV or a basic browser page
- Read, stream and seek files (HTTP Range proxy)
- Optional WebDAV Basic authentication
- Select a Quark root folder ID
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

## Origin and license

The Quark API request flow is derived from the `quark_uc` driver in
[OpenList](https://github.com/OpenListTeam/OpenList), version 4.2.6. OpenList
and this derivative are licensed under AGPL-3.0-only. No OpenList executable,
database, web frontend or framework code is included in the IPK.

This software calls an unofficial cloud API that may change at any time. Use it
only with your own account and do not expose the service directly to the public
Internet.
