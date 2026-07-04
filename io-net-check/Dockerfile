FROM golang:alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /go/src/app

COPY io-net-check .
RUN go build -tags timetzdata -o /go/bin/app github.com/TiyaAnlite/FocotServices/io-net-check

FROM alpine

WORKDIR /app

ENTRYPOINT ["./go"]

COPY --from=builder /go/bin/app go