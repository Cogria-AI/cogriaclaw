BINARY  := cogriaclaw
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath
# Cross-compile targets. CGO is off everywhere, so these are pure cross-builds.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

.DEFAULT_GOAL := build
.PHONY: build build-all universal package run fmt test tidy clean help

build: ## Build the binary for the host platform
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY) .

build-all: ## Cross-compile for all target platforms into dist/
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=dist/$(BINARY)-$$os-$$arch; \
		echo "building $$out ($(VERSION))"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $$out . || exit 1; \
	done

universal: ## Build a macOS universal (arm64+amd64) binary via lipo
	@mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 .
	lipo -create -output dist/$(BINARY)-darwin-universal dist/$(BINARY)-darwin-arm64 dist/$(BINARY)-darwin-amd64
	@echo "built dist/$(BINARY)-darwin-universal"

package: build-all ## Cross-compile, then tar.gz each binary with LICENSE + README + checksums
	@cd dist && for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name=$(BINARY)-$$os-$$arch; \
		rm -rf $$name.pkg && mkdir -p $$name.pkg; \
		cp $$name $$name.pkg/$(BINARY); \
		cp ../LICENSE ../README.md $$name.pkg/; \
		tar -czf $$name.tar.gz -C $$name.pkg .; \
		rm -rf $$name.pkg; \
		echo "packaged dist/$$name.tar.gz"; \
	done; \
	{ shasum -a 256 *.tar.gz 2>/dev/null || sha256sum *.tar.gz; } > SHA256SUMS; \
	echo "checksums: dist/SHA256SUMS"

run: build ## Build and run in the foreground
	./$(BINARY) run

fmt: ## Format (gofmt -s) and vet
	gofmt -s -w .
	go vet ./...

test: ## Run tests
	go test ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -rf dist

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
