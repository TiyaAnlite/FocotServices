GOOS ?= $(shell go env GOOS 2>/dev/null || echo "unknown")
GOARCH ?= $(shell go env GOARCH 2>/dev/null || echo "noarch")
GOCACHE ?= $(shell go env GOCACHE 2>/dev/null || echo "")

BINARY_NAME ?= io-net-check
OUTPUT ?= bin/$(BINARY_NAME).$(GOOS).$(GOARCH)

.PHONY: build test clean

build:
	mkdir -p bin
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(OUTPUT) .

test:
	GOCACHE=$(GOCACHE) go test ./...

clean:
	rm -f $(OUTPUT)
