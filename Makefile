SHELL := /bin/bash

OUTDIR := bin
# Semantic version used to name release artifacts (zip/deb), derived from the
# nearest git tag (e.g. "v0.0.1", or "v0.0.1-3-gabcdef" / "-dirty" when the
# working tree has moved on from that tag). Override explicitly if needed:
#   make zip VERSION=v0.1.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
ifeq ($(VERSION),)
VERSION := 0.0.0-dev
endif
# Debian version fields must start with a digit; strip a leading "v".
DEB_VERSION := $(patsubst v%,%,$(VERSION))
DEB_ARCH ?= amd64

# Binaries are derived from the cmd/* directories (graytail, grayquery, ...).
CMDS := $(notdir $(wildcard cmd/*))

OS_ARCH := \
	linux:amd64 linux:arm64 \
	darwin:amd64 darwin:arm64 \
	windows:amd64 windows:arm64

COVERFILE := coverage.out

# Packages that actually have _test.go files. go test -coverprofile prints an
# unlabeled "coverage: 0.0%" line (no ok/FAIL marker) for untested packages
# like cmd/graytail, which reads like an error; excluding them keeps output clean.
TESTED_PKGS := $(shell go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./...)

DEB_PKG := graytail
DEB_STAGE := dist/deb/$(DEB_PKG)_$(DEB_VERSION)_$(DEB_ARCH)
DEB_FILE := $(DEB_PKG)_$(DEB_VERSION)_$(DEB_ARCH).deb

# Arch pkgver may not contain hyphens: v1.0.0-3-gabc1234-dirty becomes
# 1.0.0.r3.gabc1234.dirty. A bare commit hash (no tags) becomes 0.0.0.r0.g<hash>.
ARCH_PKGVER := $(shell echo '$(DEB_VERSION)' | \
	sed -E '/^[0-9]+\./!s/^/0.0.0-0-g/; s/-([0-9]+)-g/.r\1.g/; s/-/./g')
ARCH_PKGREL ?= 1
PKG_ARCH := $(shell uname -m)
ARCH_STAGE := dist/arch
ARCH_TARBALL := $(DEB_PKG)-$(ARCH_PKGVER).tar.gz
ARCH_FILE := $(DEB_PKG)-$(ARCH_PKGVER)-$(ARCH_PKGREL)-$(PKG_ARCH).pkg.tar.zst

# macOS package variables
OSX_ARCH := $(shell uname -m)
OSX_STAGE := dist/osx/stage
OSX_PAYLOAD_DIR := $(OSX_STAGE)/payload
OSX_PKG_NAME := $(DEB_PKG)-$(VERSION)-$(OSX_ARCH).pkg

.PHONY: all build build-all test zip deb arch osx clean

all: build

# Default build for the host platform (requires go to be available)
build:
	@echo "Building for host..."
	@set -e; \
	for cmd in $(CMDS); do \
		echo "Building $$cmd"; \
		go build -o $(OUTDIR)/$$cmd ./cmd/$$cmd; \
	done; \
	echo "Built binaries in $(OUTDIR)/"

# Build all OS/ARCH combinations for every binary (cross-compile via Go toolchain)
build-all:
	@set -e; \
	for cmd in $(CMDS); do \
		for pair in $(OS_ARCH); do \
			OS=$${pair%%:*}; ARCH=$${pair##*:}; \
			OUT=$(OUTDIR)/$${OS}/$${ARCH}; mkdir -p $$OUT; \
			EXT=; if [ "$$OS" = "windows" ]; then EXT=.exe; fi; \
			echo "Building $$cmd $$OS/$$ARCH"; \
			GOOS=$$OS GOARCH=$$ARCH go build -o $$OUT/$$cmd$$EXT ./cmd/$$cmd || exit 1; \
		done; \
	done; \
	echo "All builds placed in $(OUTDIR)/<os>/<arch>/"

# Run all tests with per-test output and a function-level coverage summary.
test:
	go test -v -coverprofile=$(COVERFILE) $(TESTED_PKGS)
	@echo ""
	@echo "Coverage summary:"
	go tool cover -func=$(COVERFILE)

# Cross-compile everything, then zip up bin/<os>/<arch>/ as one release archive.
zip: clean build-all
	@command -v zip >/dev/null || { echo "zip not found; install it (e.g. apt-get install zip)"; exit 1; }
	cd $(OUTDIR) && zip -r ../$(DEB_PKG)-$(VERSION).zip .
	@echo "Created $(DEB_PKG)-$(VERSION).zip"

# Build a linux/$(DEB_ARCH) .deb package. Edit packaging/debian/control.template
# (see packaging/debian/README.md) with your real maintainer/publishing info
# before shipping this to anyone.
deb:
	@command -v dpkg-deb >/dev/null || { echo "dpkg-deb not found; install dpkg"; exit 1; }
	@set -e; \
	echo "Building linux/$(DEB_ARCH) binaries..."; \
	mkdir -p $(OUTDIR)/linux/$(DEB_ARCH); \
	for cmd in $(CMDS); do \
		GOOS=linux GOARCH=$(DEB_ARCH) go build -o $(OUTDIR)/linux/$(DEB_ARCH)/$$cmd ./cmd/$$cmd; \
	done; \
	rm -rf $(DEB_STAGE); \
	mkdir -p $(DEB_STAGE)/DEBIAN $(DEB_STAGE)/usr/bin; \
	sed -e 's/__VERSION__/$(DEB_VERSION)/' -e 's/__ARCH__/$(DEB_ARCH)/' packaging/debian/control.template > $(DEB_STAGE)/DEBIAN/control; \
	cp $(OUTDIR)/linux/$(DEB_ARCH)/* $(DEB_STAGE)/usr/bin/; \
	dpkg-deb --build --root-owner-group $(DEB_STAGE) $(DEB_FILE); \
	echo "Created $(DEB_FILE)"

# Build an Arch Linux package with makepkg from packaging/arch/PKGBUILD.template
# (see packaging/arch/README.md). Binaries are static (CGO_ENABLED=0) and built
# for the host architecture. ARCH_NOCHECK=1 skips the test suite.
arch:
	@command -v makepkg >/dev/null || { echo "makepkg not found; this target requires Arch Linux (pacman)"; exit 1; }
	@command -v go >/dev/null || { echo "go not found; install it (sudo pacman -S go)"; exit 1; }
	@[ "$$(id -u)" != 0 ] || { echo "makepkg cannot be run as root; run make arch as a regular user"; exit 1; }
	@set -e; \
	rm -rf $(ARCH_STAGE); mkdir -p $(ARCH_STAGE); \
	nodeps=; if ! pacman -T go >/dev/null 2>&1; then \
		echo "note: go is not a pacman package here (e.g. mise/asdf); using $$(command -v go) with --nodeps"; \
		nodeps=--nodeps; \
	fi; \
	ref=$$(git stash create); ref=$${ref:-HEAD}; \
	git archive --format=tar.gz --prefix=$(DEB_PKG)-$(ARCH_PKGVER)/ -o $(ARCH_STAGE)/$(ARCH_TARBALL) $$ref; \
	sum=$$(sha256sum $(ARCH_STAGE)/$(ARCH_TARBALL) | cut -d' ' -f1); \
	sed -e 's/__PKGVER__/$(ARCH_PKGVER)/' -e 's/__PKGREL__/$(ARCH_PKGREL)/' -e "s/__SHA256__/$$sum/" \
		packaging/arch/PKGBUILD.template > $(ARCH_STAGE)/PKGBUILD; \
	cd $(ARCH_STAGE) && \
	BUILDDIR=$(CURDIR)/$(ARCH_STAGE)/build SRCDEST=$(CURDIR)/$(ARCH_STAGE) PKGDEST=$(CURDIR) \
	PKGEXT=.pkg.tar.zst \
		makepkg -f --cleanbuild --noconfirm $$nodeps $(if $(ARCH_NOCHECK),--nocheck); \
	echo ""; \
	echo "Created $(ARCH_FILE)"; \
	echo ""; \
	echo "Install:"; \
	echo "  sudo pacman -U $(CURDIR)/$(ARCH_FILE)"; \
	echo ""; \
	echo "Uninstall:"; \
	echo "  sudo pacman -R $(DEB_PKG)"; \
	echo ""; \
	echo "Files installed:"; \
	pacman -Qlpq $(CURDIR)/$(ARCH_FILE) | grep -v '/$$' | sed 's/^/  /'

# Build a macOS .pkg installer package. Auto-detects the host architecture (arm64 or amd64)
# and creates a native installer with both graytail and grayquery binaries.
osx:
	@command -v pkgbuild >/dev/null || { echo "pkgbuild not found; install Xcode Command Line Tools (xcode-select --install)"; exit 1; }
	@set -e; \
	echo "Detecting macOS architecture..."; \
	UNAME_ARCH=$$(uname -m); \
	echo "Building for $$UNAME_ARCH..."; \
	\
	case $$UNAME_ARCH in \
		arm64) GOARCH=arm64 ;; \
		x86_64) GOARCH=amd64 ;; \
		*) echo "Unsupported architecture: $$UNAME_ARCH"; exit 1 ;; \
	esac; \
	\
	echo "Building darwin/$$GOARCH binaries..."; \
	mkdir -p $(OUTDIR)/darwin/$$GOARCH; \
	for cmd in $(CMDS); do \
		echo "Building $$cmd"; \
		GOOS=darwin GOARCH=$$GOARCH go build -o $(OUTDIR)/darwin/$$GOARCH/$$cmd ./cmd/$$cmd; \
	done; \
	\
	echo "Creating macOS package..."; \
	rm -rf $(OSX_STAGE); \
	mkdir -p $(OSX_PAYLOAD_DIR)/usr/local/bin; \
	\
	cp $(OUTDIR)/darwin/$$GOARCH/* $(OSX_PAYLOAD_DIR)/usr/local/bin/; \
	chmod +x $(OSX_PAYLOAD_DIR)/usr/local/bin/*; \
	\
	pkgbuild \
		--root $(OSX_PAYLOAD_DIR) \
		--install-location / \
		--identifier com.graytail.pkg \
		--version $(DEB_VERSION) \
		--ownership preserve \
		$(OSX_PKG_NAME); \
	\
	echo ""; \
	echo "Created $(OSX_PKG_NAME)"; \
	echo ""; \
	echo "Install:"; \
	echo "  installer -pkg $(OSX_PKG_NAME) -target /"; \
	echo ""; \
	echo "Uninstall:"; \
	echo "  sudo rm /usr/local/bin/graytail"; \
	echo "  sudo rm /usr/local/bin/grayquery"; \
	echo ""; \
	echo "Files installed:"; \
	echo "  /usr/local/bin/graytail"; \
	echo "  /usr/local/bin/grayquery"; \
	echo ""; \
	echo "Verify installation:"; \
	echo "  graytail -h"; \
	echo "  grayquery -h"

clean:
	rm -rf $(OUTDIR) $(COVERFILE) dist $(DEB_PKG)-*.zip $(DEB_PKG)_*.deb $(DEB_PKG)-*.pkg.tar.* $(DEB_PKG)-*.pkg
