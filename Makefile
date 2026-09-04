SHELL := /bin/bash

BINARY      := aiusagemonitor
MODULE      := github.com/kawaiipantsu/aiusagemonitor
CMD         := ./cmd/aiusagemonitor
DIST        := dist
BIN         := bin

VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
               -X '$(MODULE)/internal/version.Version=$(VERSION)' \
               -X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
               -X '$(MODULE)/internal/version.Date=$(DATE)'

PREFIX      ?= /usr/local
DESTDIR     ?=

# modernc.org/sqlite is a pure-Go SQLite driver, so build/install/dist/deb all
# force CGO_ENABLED=0 explicitly (cross-compiling needs no C toolchain for the
# target). It is deliberately NOT exported globally: `go test -race` requires
# cgo to be enabled on the *host*, so `test`/`cover`/`run`/`demo` leave it alone.

# GOOS/GOARCH/GOARM targets. "Intel/Arm/Apple Silicon, 32/64-bit" maps to:
#   linux:   amd64 (Intel/AMD 64), 386 (Intel 32), arm64 (ARM 64), armv7 (ARM 32)
#   windows: amd64, 386, arm64
#   darwin:  amd64 (Intel Mac), arm64 (Apple Silicon) — macOS has no 32-bit target
PLATFORMS := \
	linux/amd64/ \
	linux/386/ \
	linux/arm64/ \
	linux/arm/7 \
	windows/amd64/ \
	windows/386/ \
	windows/arm64/ \
	darwin/amd64/ \
	darwin/arm64/

DEB_ARCHES  := amd64/amd64 386/i386 arm64/arm64 arm/armhf

.PHONY: all
all: build

.PHONY: build
build: ## Build a binary for the host OS/arch into ./bin
	@mkdir -p $(BIN)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/$(BINARY) $(CMD)
	@echo "-> $(BIN)/$(BINARY)"

.PHONY: run
run: ## Run the TUI directly (go run)
	go run $(CMD)

.PHONY: demo
demo: ## Run with synthetic demo data, isolated from your real config/db
	go run $(CMD) -demo -config /tmp/aium-demo-config.yaml -db /tmp/aium-demo.db

.PHONY: test
test: ## Run unit tests with race detection
	go test -race -count=1 ./...

.PHONY: cover
cover: ## Run tests with coverage summary
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

.PHONY: fmt
fmt: ## Format all Go source
	gofmt -w .

.PHONY: vet
vet: ## Static analysis
	go vet ./...

.PHONY: tidy
tidy: ## Sync go.mod/go.sum
	go mod tidy

.PHONY: lint
lint: fmt vet ## fmt + vet, plus golangci-lint if installed
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; skipping"; \
	fi

.PHONY: install
install: build ## Install into $(DESTDIR)$(PREFIX)/bin
	install -Dm755 $(BIN)/$(BINARY) $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	install -Dm644 packaging/man/$(BINARY).1 $(DESTDIR)$(PREFIX)/share/man/man1/$(BINARY).1

.PHONY: uninstall
uninstall: ## Remove a previous `make install`
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BINARY)
	rm -f $(DESTDIR)$(PREFIX)/share/man/man1/$(BINARY).1

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN) $(DIST) coverage.out

# --- cross-compilation ------------------------------------------------------

.PHONY: dist
dist: clean $(PLATFORMS) ## Cross-compile every supported platform into ./dist
	@$(MAKE) --no-print-directory checksums

# Pattern target driven by the PLATFORMS list: "os/arch/goarm"
.PHONY: $(PLATFORMS)
$(PLATFORMS):
	$(eval GOOS_=$(word 1,$(subst /, ,$@)))
	$(eval GOARCH_=$(word 2,$(subst /, ,$@)))
	$(eval GOARM_=$(word 3,$(subst /, ,$@)))
	$(eval EXT_=$(if $(filter windows,$(GOOS_)),.exe,))
	$(eval OUTDIR_=$(DIST)/$(BINARY)_$(VERSION)_$(GOOS_)_$(GOARCH_)$(if $(GOARM_),v$(GOARM_),))
	@mkdir -p $(OUTDIR_)
	@echo "==> $(GOOS_)/$(GOARCH_)$(if $(GOARM_), GOARM=$(GOARM_),)"
	CGO_ENABLED=0 GOOS=$(GOOS_) GOARCH=$(GOARCH_) $(if $(GOARM_),GOARM=$(GOARM_),) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(OUTDIR_)/$(BINARY)$(EXT_) $(CMD)
	@cp README.md LICENSE $(OUTDIR_)/ 2>/dev/null || true
	@tar -C $(DIST) -czf $(OUTDIR_).tar.gz $(notdir $(OUTDIR_))
	@rm -rf $(OUTDIR_)

.PHONY: checksums
checksums: ## Write dist/SHA256SUMS for every archive/package produced
	@cd $(DIST) && sha256sum *.tar.gz *.deb 2>/dev/null > SHA256SUMS || true
	@echo "-> $(DIST)/SHA256SUMS"

# --- Debian packaging --------------------------------------------------------

.PHONY: deb
deb: $(addprefix deb-,$(subst /,-,$(DEB_ARCHES))) ## Build .deb packages for amd64/i386/arm64/armhf

deb-%:
	@$(MAKE) --no-print-directory _deb GOARCH_PAIR=$(subst -,/,$*)

.PHONY: _deb
_deb:
	@bash packaging/build-deb.sh "$(word 1,$(subst /, ,$(GOARCH_PAIR)))" "$(word 2,$(subst /, ,$(GOARCH_PAIR)))" "$(VERSION)" "$(BINARY)" "$(LDFLAGS)" "$(DIST)"

.PHONY: release
release: dist deb ## Full release build: all archives + all .deb packages
	@$(MAKE) --no-print-directory checksums
	@echo "release artifacts in $(DIST)/"

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
