# syntax=docker/dockerfile:1

# ---- dev ----
# Go toolchain + the runtime tools the app shells out to (ffmpeg/ffprobe). Not
# part of the shipped image: integration tests use it (`target: dev` in
# docker-compose.test.yml) to run the ffmpeg-dependent Go tests against the same
# ffmpeg the runtime stage ships — which holds only while both stages pin the SAME
# Alpine release (ALPINE below); bump them together.
ARG ALPINE=3.24

FROM golang:1.25-alpine${ALPINE} AS dev
RUN apk add --no-cache ffmpeg curl
WORKDIR /app

# ---- builder ----
FROM golang:1.25-alpine${ALPINE} AS builder
WORKDIR /src

# Copy go.mod/go.sum first for layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG GIT_SHA=dev
# Cache mounts keep module + build caches across builds, so the rebuild after a
# code change (`task up`) only recompiles what changed.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${GIT_SHA}" \
    -o /out/server ./cmd/server

# ---- runtime ----
# Last stage = default build target (what docker-compose.yml runs).
FROM alpine:${ALPINE}
WORKDIR /app

# ffmpeg/ffprobe for preview/metadata; curl for healthcheck; ca-certificates for TLS
RUN apk add --no-cache ffmpeg ca-certificates curl

COPY --from=builder /out/server /app/server
# casbin is loaded at runtime from the relative path casbin/ (main.go: casbin.NewEnforcer("casbin/model.conf", ...))
COPY casbin /app/casbin

EXPOSE 8080
ENTRYPOINT ["/app/server"]
