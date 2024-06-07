FROM golang:alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /go/src/app

COPY io-bilive-recorder/go.mod ./io-bilive-recorder/go.mod
COPY client-http-proxy/go.mod ./client-http-proxy/go.mod
COPY io-bilive-recorder/go.sum ./io-bilive-recorder/go.sum
COPY client-http-proxy/go.sum ./client-http-proxy/go.sum
RUN go work init io-bilive-recorder client-http-proxy
RUN go mod download

COPY io-bilive-recorder ./io-bilive-recorder
COPY client-http-proxy ./client-http-proxy
RUN go build -tags timetzdata -o /go/bin/app github.com/TiyaAnlite/FocotServices/io-bilive-recorder

FROM alpine

WORKDIR /app

ENTRYPOINT ["./go"]

COPY --from=builder /go/bin/app go
