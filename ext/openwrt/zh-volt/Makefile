# This is free software, licensed under the GNU General Public License v2.
# See /LICENSE for more information.
#

include $(TOPDIR)/rules.mk

PKG_NAME:=zh_volt
PKG_VERSION:=0.1.1
PKG_RELEASE:=2

PKG_SOURCE:=$(PKG_NAME)-$(PKG_VERSION).tar.gz
PKG_SOURCE_PROTO=git
PKG_SOURCE_URL=https://sirherobrine23.com.br/Sirherobrine23/zh-volt.git
PKG_SOURCE_VERSION=main

PKG_MAINTAINER:=Matheus Sampaio Queiroga <srherobrine20@gmail.com>
PKG_LICENSE:=MIT
PKG_LICENSE_FILES:=LICENSE

PKG_BUILD_DEPENDS:= rust/host libpcap
PKG_BUILD_PARALLEL:=1

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/rust/rust-package.mk

define Package/zh_volt
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=Cli to manager AINOPOL OLTs
  URL:=https://sirherobrine23.com.br/Sirherobrine23/zh-volt.git
  DEPENDS:=$(RUST_ARCH_DEPENDS) +libpcap
endef

define Package/zh_volt/description
  Manage Ainopol gpon OLTs as an alternative to their closed program
endef

$(eval $(call RustBinPackage,zh_volt))
$(eval $(call BuildPackage,zh_volt))