# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/futuretea/etcd-mcp-server/pkg/core/version.Version=${VERSION} \
      -X github.com/futuretea/etcd-mcp-server/pkg/core/version.Commit=${GIT_COMMIT} \
      -X github.com/futuretea/etcd-mcp-server/pkg/core/version.Date=${BUILD_DATE}" \
    -o /usr/local/bin/etcd-mcp-server ./cmd/etcd-mcp-server

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates \
    && addgroup -S mcp \
    && adduser -S -G mcp mcp

USER mcp

ENTRYPOINT ["/usr/local/bin/etcd-mcp-server", "mcp"]

FROM runtime AS release

COPY etcd-mcp-server /usr/local/bin/etcd-mcp-server

FROM runtime AS dev

COPY --from=builder /usr/local/bin/etcd-mcp-server /usr/local/bin/etcd-mcp-server
