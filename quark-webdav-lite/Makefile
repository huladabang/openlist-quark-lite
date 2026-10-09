include $(TOPDIR)/rules.mk

PKG_NAME:=quark-webdav-lite
PKG_VERSION:=0.3.1
PKG_RELEASE:=1
PKG_LICENSE:=AGPL-3.0-only
PKG_MAINTAINER:=huladabang
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

GO_PKG:=github.com/huladabang/openlist-quark-lite
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/quark-webdav
GO_PKG_LDFLAGS:=-s -w -X main.version=v$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/quark-webdav-lite
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=Web Servers/Proxies
  TITLE:=Tiny read-only Quark WebDAV server with LuCI
  URL:=https://github.com/huladabang/openlist-quark-lite
  DEPENDS:=$(GO_ARCH_DEPENDS) +ca-bundle +luci-base
  CONFLICTS:=openlist-quark-lite
endef

define Package/quark-webdav-lite/description
  A small standalone, read-only Quark cloud drive WebDAV gateway. It reuses
  the OpenList Quark API approach without embedding the OpenList core.
endef

define Package/quark-webdav-lite/conffiles
/etc/config/quark-webdav
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	$(CP) ./src/. $(PKG_BUILD_DIR)/
endef

define Package/quark-webdav-lite/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/quark-webdav $(1)/usr/bin/quark-webdav
	$(INSTALL_DIR) $(1)/etc/config $(1)/etc/init.d
	$(INSTALL_CONF) ./files/quark-webdav.config $(1)/etc/config/quark-webdav
	$(INSTALL_BIN) ./files/quark-webdav.init $(1)/etc/init.d/quark-webdav
	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/quark-webdav
	$(INSTALL_DATA) ./files/basic.js $(1)/www/luci-static/resources/view/quark-webdav/basic.js
	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d $(1)/usr/share/rpcd/acl.d $(1)/usr/share/ucitrack
	$(INSTALL_DATA) ./files/luci-app-quark-webdav.menu.json $(1)/usr/share/luci/menu.d/luci-app-quark-webdav.json
	$(INSTALL_DATA) ./files/luci-app-quark-webdav.acl.json $(1)/usr/share/rpcd/acl.d/luci-app-quark-webdav.json
	$(INSTALL_DATA) ./files/luci-app-quark-webdav.ucitrack.json $(1)/usr/share/ucitrack/luci-app-quark-webdav.json
endef

$(eval $(call BuildPackage,quark-webdav-lite))
