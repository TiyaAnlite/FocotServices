FROM golang:alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /go/src/app

COPY client-http-proxy ./client-http-proxy

RUN go work init client-http-proxy
RUN go build -tags timetzdata -o /go/bin/app github.com/TiyaAnlite/FocotServices/client-http-proxy

FROM alpine

WORKDIR /app

ENTRYPOINT ["./go"]

COPY --from=builder /go/bin/app go
