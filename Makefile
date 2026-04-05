# Build script for tpix-cli

# Use := to evaluate these only once
VERSION        := $(shell git describe --tags 2>/dev/null || echo "v0.0.0")
BUILD_TIME     := $(shell date +%s)
GO_VERSION     := $(shell go version | cut -d' ' -f3)
BINARY_NAME    := tpix
DIST_DIR       := dist
PKG_PATH       := github.com/typstify/tpix-cli/version

# Setup the -ldflags option
LDFLAGS := -s -w \
	-X "$(PKG_PATH).Version=$(VERSION)" \
	-X "$(PKG_PATH).BuildTime=$(BUILD_TIME)" \
	-X "$(PKG_PATH).BuildGoVersion=$(GO_VERSION)"

# Supported platforms for release
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build clean install release help

all: build

## build: Build the binary for the current architecture
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd

## clean: Remove build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(DIST_DIR)

## install: Build and install the binary to /usr/local/bin
install: build
	install -m 755 $(BINARY_NAME) /usr/local/bin/

## release: Build for all supported platforms and package them
release: clean
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		output_name=$(BINARY_NAME); \
		[ $$os = "windows" ] && output_name+=".exe"; \
		\
		echo "Building for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$$output_name ./cmd; \
		\
		tar_name="tpix-cli-$$os-$$arch.tar.gz"; \
		cp LICENSE $(DIST_DIR)/; \
		cd $(DIST_DIR) && tar -czvf $$tar_name $$output_name LICENSE > /dev/null; \
		rm $$output_name LICENSE; \
		cd ..; \
	done
	@cd $(DIST_DIR) && shasum -a 256 *.tar.gz > checksums.txt; 
	@echo "Release packages created in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/*.tar.gz

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'