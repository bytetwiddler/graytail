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

.PHONY: all build build-all test zip deb clean

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

clean:
	rm -rf $(OUTDIR) $(COVERFILE) dist $(DEB_PKG)-*.zip $(DEB_PKG)_*.deb
