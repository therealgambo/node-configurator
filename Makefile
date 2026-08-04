BINARY  := node-configurator
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help build clean test vet lint fmt fmt-check ci

##@ Build

build: bin/$(BINARY)-linux-amd64 bin/$(BINARY)-linux-arm64 ## Cross-compile linux/amd64 and linux/arm64 binaries into bin/

bin/$(BINARY)-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/node-configurator

bin/$(BINARY)-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/node-configurator

clean: ## Remove build artifacts (bin/)
	rm -rf bin

##@ Test & Lint

test: ## Run the unit test suite (go test ./...)
	go test ./...

vet: ## Run go vet ./...
	go vet ./...

lint: ## Run golangci-lint with the repo's .golangci.yml (same linters as CI)
	golangci-lint run

fmt: ## Reformat source in place with gofmt
	gofmt -l -w .

# Read-only counterpart to `fmt`, used by `ci`: fails like CI's `gofmt -l .`
# check instead of rewriting files, so `make ci` never mutates the tree.
fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; \
		echo "$$out"; \
		echo "run 'make fmt' to fix"; \
		exit 1; \
	fi

ci: fmt-check vet lint test ## Run the same checks CI runs: gofmt check, vet, lint, test

##@ Other

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9][a-zA-Z0-9 _-]*:.*?##/ { split($$1, targets, " "); for (i in targets) { printf "  \033[36m%-12s\033[0m %s\n", targets[i], $$2 } } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
