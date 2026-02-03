GOOS ?= $(shell go env GOOS 2>/dev/null || echo "unknown")
GOARCH ?= $(shell go env GOARCH 2>/dev/null || echo "noarch")

.PHONY: build-tools
build-tools:
	rm -f bin/blive-tools.$(GOOS).$(GOARCH)
	CGO_ENABLED=0 \
	go build \
		-o bin/blive-tools.$(GOOS).$(GOARCH) \
		github.com/TiyaAnlite/FocotServices/io-bilive-damaku/cmd
	strip bin/blive-tools.$(GOOS).$(GOARCH)
	-upx -9 bin/blive-tools.$(GOOS).$(GOARCH)