# libgowebrtc Makefile
# State-of-the-art dependency management and build system

.PHONY: all build test test-v test-race bench clean deps deps-update deps-verify \
        lint vet fmt shim shim-clean install help

# Go settings
GO := go
GOFLAGS := -v
GOTEST := $(GO) test
GOBUILD := $(GO) build
GOVET := $(GO) vet
GOFMT := gofmt

# Version info
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Directories
BUILD_DIR := build
LIB_DIR := lib
SHIM_DIR := shim
SHIM_BUILD_DIR := $(SHIM_DIR)/build

# Platform detection
GOOS := $(shell $(GO) env GOOS)
GOARCH := $(shell $(GO) env GOARCH)
PLATFORM := $(GOOS)_$(GOARCH)

# Shim library name based on platform
ifeq ($(GOOS),darwin)
    SHIM_LIB := libwebrtc_shim.dylib
else ifeq ($(GOOS),linux)
    SHIM_LIB := libwebrtc_shim.so
else ifeq ($(GOOS),windows)
    SHIM_LIB := webrtc_shim.dll
endif

# libwebrtc path (set via environment or here)
LIBWEBRTC_DIR ?= $(HOME)/libwebrtc

# Packages
PACKAGES := ./...
TEST_PACKAGES := ./...

#-----------------------------------------------------------------------------
# Default target
#-----------------------------------------------------------------------------

all: deps build test

#-----------------------------------------------------------------------------
# Dependency management
#-----------------------------------------------------------------------------

# Install/update all dependencies with pinned versions
deps:
	@echo "==> Installing dependencies..."
	$(GO) mod download
	$(GO) mod verify
	@echo "==> Dependencies installed"

# Update dependencies to latest versions
deps-update:
	@echo "==> Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "==> Dependencies updated"

# Verify dependencies haven't been tampered with
deps-verify:
	@echo "==> Verifying dependencies..."
	$(GO) mod verify
	@echo "==> Dependencies verified"

# Tidy go.mod and go.sum
deps-tidy:
	@echo "==> Tidying dependencies..."
	$(GO) mod tidy
	@echo "==> Dependencies tidied"

# Download specific dependency versions (for reproducible builds)
deps-vendor:
	@echo "==> Vendoring dependencies..."
	$(GO) mod vendor
	@echo "==> Dependencies vendored to ./vendor"

#-----------------------------------------------------------------------------
# Build targets
#-----------------------------------------------------------------------------

build:
	@echo "==> Building $(PLATFORM)..."
	$(GOBUILD) $(GOFLAGS) $(LDFLAGS) $(PACKAGES)
	@echo "==> Build complete"

# Build for all supported platforms
build-all: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64

build-darwin-arm64:
	@echo "==> Building darwin/arm64..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) $(PACKAGES)

build-darwin-amd64:
	@echo "==> Building darwin/amd64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) $(PACKAGES)

build-linux-amd64:
	@echo "==> Building linux/amd64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) $(PACKAGES)

build-linux-arm64:
	@echo "==> Building linux/arm64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(GOFLAGS) $(LDFLAGS) $(PACKAGES)

#-----------------------------------------------------------------------------
# Test targets
#-----------------------------------------------------------------------------

test:
	@echo "==> Running tests..."
	$(GOTEST) $(TEST_PACKAGES)
	@echo "==> Tests passed"

test-v:
	@echo "==> Running tests (verbose)..."
	$(GOTEST) -v $(TEST_PACKAGES)

test-race:
	@echo "==> Running tests with race detector..."
	$(GOTEST) -race $(TEST_PACKAGES)

test-cover:
	@echo "==> Running tests with coverage..."
	$(GOTEST) -coverprofile=coverage.out $(TEST_PACKAGES)
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "==> Coverage report: coverage.html"

test-interop:
	@echo "==> Running interop tests..."
	$(GOTEST) -v ./test/interop/...

test-e2e:
	@echo "==> Running end-to-end tests..."
	$(GOTEST) -v ./pkg/encoder/e2e_test.go ./pkg/decoder/e2e_test.go ./pkg/track/e2e_test.go ./pkg/pc/e2e_test.go

#-----------------------------------------------------------------------------
# Benchmarks
#-----------------------------------------------------------------------------

bench:
	@echo "==> Running benchmarks..."
	$(GOTEST) -bench=. -benchmem $(TEST_PACKAGES)

bench-cpu:
	@echo "==> Running benchmarks with CPU profile..."
	$(GOTEST) -bench=. -cpuprofile=cpu.prof $(TEST_PACKAGES)
	@echo "==> CPU profile: cpu.prof"

bench-mem:
	@echo "==> Running benchmarks with memory profile..."
	$(GOTEST) -bench=. -memprofile=mem.prof $(TEST_PACKAGES)
	@echo "==> Memory profile: mem.prof"

#-----------------------------------------------------------------------------
# Code quality
#-----------------------------------------------------------------------------

lint:
	@echo "==> Running linter..."
	@which golangci-lint > /dev/null || (echo "Install golangci-lint: brew install golangci-lint" && exit 1)
	golangci-lint run $(PACKAGES)

vet:
	@echo "==> Running go vet..."
	$(GOVET) $(PACKAGES)

fmt:
	@echo "==> Formatting code..."
	$(GOFMT) -s -w .
	@echo "==> Code formatted"

fmt-check:
	@echo "==> Checking code formatting..."
	@test -z "$$($(GOFMT) -s -l . | tee /dev/stderr)" || (echo "Run 'make fmt' to fix" && exit 1)

check: vet fmt-check
	@echo "==> All checks passed"

#-----------------------------------------------------------------------------
# Shim library (C++ wrapper around libwebrtc)
#-----------------------------------------------------------------------------

shim: shim-configure shim-build

shim-configure:
	@echo "==> Configuring shim build..."
	@mkdir -p $(SHIM_BUILD_DIR)
	cd $(SHIM_BUILD_DIR) && cmake .. \
		-DCMAKE_BUILD_TYPE=Release \
		-DLIBWEBRTC_DIR=$(LIBWEBRTC_DIR) \
		-DBUILD_SHARED_LIBS=ON
	@echo "==> Shim configured"

shim-build:
	@echo "==> Building shim..."
	@if [ ! -d "$(SHIM_BUILD_DIR)" ]; then \
		echo "Run 'make shim-configure' first"; \
		exit 1; \
	fi
	cd $(SHIM_BUILD_DIR) && cmake --build . --config Release
	@echo "==> Shim built"

shim-install:
	@echo "==> Installing shim to $(LIB_DIR)/$(PLATFORM)..."
	@mkdir -p $(LIB_DIR)/$(PLATFORM)
	cp $(SHIM_BUILD_DIR)/$(SHIM_LIB) $(LIB_DIR)/$(PLATFORM)/
	@echo "==> Shim installed"

shim-clean:
	@echo "==> Cleaning shim build..."
	rm -rf $(SHIM_BUILD_DIR)
	@echo "==> Shim cleaned"

#-----------------------------------------------------------------------------
# Development helpers
#-----------------------------------------------------------------------------

# Generate code (if we have any codegen)
generate:
	@echo "==> Running go generate..."
	$(GO) generate $(PACKAGES)

# Watch for changes and run tests
watch:
	@which reflex > /dev/null || (echo "Install reflex: go install github.com/cespare/reflex@latest" && exit 1)
	reflex -r '\.go$$' -s -- sh -c 'make test'

# Run examples
example-encode:
	$(GO) run ./examples/encode_decode/main.go

example-pion:
	$(GO) run ./examples/pion_interop/main.go

#-----------------------------------------------------------------------------
# CI/CD targets
#-----------------------------------------------------------------------------

ci: deps-verify check test-race

ci-full: ci test-cover bench

#-----------------------------------------------------------------------------
# Release helpers
#-----------------------------------------------------------------------------

release-check:
	@echo "==> Checking release readiness..."
	@test -n "$(VERSION)" || (echo "VERSION not set" && exit 1)
	make check
	make test
	@echo "==> Ready for release $(VERSION)"

#-----------------------------------------------------------------------------
# Clean
#-----------------------------------------------------------------------------

clean:
	@echo "==> Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f cpu.prof mem.prof
	$(GO) clean -cache -testcache
	@echo "==> Cleaned"

clean-all: clean shim-clean
	rm -rf $(LIB_DIR)
	rm -rf vendor

#-----------------------------------------------------------------------------
# Install development tools
#-----------------------------------------------------------------------------

tools:
	@echo "==> Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/cespare/reflex@latest
	@echo "==> Tools installed"

#-----------------------------------------------------------------------------
# Help
#-----------------------------------------------------------------------------

help:
	@echo "libgowebrtc - Pion-compatible libwebrtc wrapper"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Dependency Management:"
	@echo "  deps          Install dependencies"
	@echo "  deps-update   Update to latest versions"
	@echo "  deps-verify   Verify dependency checksums"
	@echo "  deps-tidy     Tidy go.mod"
	@echo "  deps-vendor   Vendor dependencies locally"
	@echo ""
	@echo "Build:"
	@echo "  build         Build for current platform"
	@echo "  build-all     Build for all platforms"
	@echo ""
	@echo "Test:"
	@echo "  test          Run tests"
	@echo "  test-v        Run tests (verbose)"
	@echo "  test-race     Run tests with race detector"
	@echo "  test-cover    Run tests with coverage"
	@echo "  test-interop  Run Pion interop tests"
	@echo "  test-e2e      Run end-to-end tests"
	@echo ""
	@echo "Benchmarks:"
	@echo "  bench         Run benchmarks"
	@echo "  bench-cpu     Run benchmarks with CPU profile"
	@echo "  bench-mem     Run benchmarks with memory profile"
	@echo ""
	@echo "Code Quality:"
	@echo "  lint          Run linter"
	@echo "  vet           Run go vet"
	@echo "  fmt           Format code"
	@echo "  check         Run all checks"
	@echo ""
	@echo "Shim Library:"
	@echo "  shim          Build shim (requires LIBWEBRTC_DIR)"
	@echo "  shim-install  Install shim to lib/"
	@echo "  shim-clean    Clean shim build"
	@echo ""
	@echo "CI/CD:"
	@echo "  ci            Run CI checks"
	@echo "  ci-full       Run full CI (with coverage)"
	@echo ""
	@echo "Other:"
	@echo "  clean         Clean build artifacts"
	@echo "  clean-all     Clean everything"
	@echo "  tools         Install dev tools"
	@echo "  help          Show this help"
	@echo ""
	@echo "Environment Variables:"
	@echo "  LIBWEBRTC_DIR  Path to libwebrtc (default: ~/libwebrtc)"
	@echo ""
	@echo "Examples:"
	@echo "  make deps build test"
	@echo "  make LIBWEBRTC_DIR=/opt/libwebrtc shim shim-install"
	@echo "  make test-interop"
