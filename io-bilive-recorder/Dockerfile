FROM golang:alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /go/src/app

COPY io-bilive-recorder ./io-bilive-recorder
COPY client-http-proxy ./client-http-proxy

RUN go work init io-bilive-recorder client-http-proxy
RUN go mod download
RUN go build -tags timetzdata -o /go/bin/app github.com/TiyaAnlite/FocotServices/io-bilive-recorder

FROM alpine

WORKDIR /app

ENTRYPOINT ["./go"]

COPY --from=builder /go/bin/app go
