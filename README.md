# OpenList Quark Lite for OpenWrt

A space-saving OpenList derivative for OpenWrt. It keeps the ordinary Quark
cloud-drive driver, the official lite frontend, WebDAV support and a minimal
LuCI service page.

## Design

- OpenList backend: v4.2.6
- Registered storage driver: `quark_uc`
- Frontend: official OpenList lite frontend
- Target: OpenWrt 24.10, `aarch64_cortex-a53`
- One installable package containing the backend, init script and LuCI page

The package is experimental. Back up your router before installation and
check that the overlay has enough free space.

## License

OpenList is licensed under AGPL-3.0. This derivative and its complete build
source are distributed under the same license. The small LuCI integration is
also released under AGPL-3.0 for a single, unambiguous project license.

Upstream projects:

- https://github.com/OpenListTeam/OpenList
- https://github.com/OpenListTeam/OpenList-Frontend
- https://github.com/OpenListTeam/OpenList-OpenWRT
