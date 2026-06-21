# ---- builder ----
FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy go.mod/go.sum first for layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG GIT_SHA=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${GIT_SHA}" \
    -o /out/server ./cmd/server

# ---- runtime ----
FROM alpine:3.20
WORKDIR /app

# ffmpeg/ffprobe for preview/metadata; curl for healthcheck; ca-certificates for TLS
RUN apk add --no-cache ffmpeg ca-certificates curl

COPY --from=builder /out/server /app/server
# casbin is loaded at runtime from the relative path casbin/ (main.go: casbin.NewEnforcer("casbin/model.conf", ...))
COPY casbin /app/casbin

EXPOSE 8080
ENTRYPOINT ["/app/server"]
