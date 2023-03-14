#!/bin/sh
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -o run -tags timetzdata github.com/TiyaAnlite/FocotServices/client-http-proxy
