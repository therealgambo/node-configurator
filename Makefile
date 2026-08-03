BINARY  := node-configurator
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test vet clean install

build: bin/$(BINARY)-linux-amd64 bin/$(BINARY)-linux-arm64

bin/$(BINARY)-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/node-configurator

bin/$(BINARY)-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $@ ./cmd/node-configurator

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin

# Installs the locally-built binary matching this machine's architecture,
# plus the systemd unit and a starter config. Meant for local iteration on
# a Linux host with this repo checked out -- for fleet rollout, ship the
# cross-compiled binary from `make build` via your AMI/user-data pipeline
# instead, since target nodes shouldn't need a Go toolchain at all.
install: bin/$(BINARY)-linux-$(shell go env GOARCH)
	install -m 0755 bin/$(BINARY)-linux-$(shell go env GOARCH) /usr/local/bin/$(BINARY)
	install -m 0644 systemd/$(BINARY).service /etc/systemd/system/$(BINARY).service
	mkdir -p /etc/$(BINARY)
	[ -f /etc/$(BINARY)/config.yaml ] || install -m 0644 configs/example.yaml /etc/$(BINARY)/config.yaml
	systemctl daemon-reload
