FROM golang:alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /go/src/app

COPY api-one-action/go.mod ./api-one-action/go.mod
COPY api-one-action/go.sum ./api-one-action/go.sum
RUN go work init api-one-action
RUN go mod download

COPY api-one-action ./api-one-action
RUN go build -tags timetzdata -o /go/bin/app github.com/TiyaAnlite/FocotServices/api-one-action

FROM alpine

WORKDIR /app

ENTRYPOINT ["./go"]

COPY --from=builder /go/bin/app go
