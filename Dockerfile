# syntax=docker/dockerfile:1

# Multi-stage build for urnet-client (repo-local layout)
FROM golang:1.25-alpine AS build

# Dependencies and certs for go tooling
RUN apk add --no-cache git ca-certificates && update-ca-certificates

# Honor go toolchain directives in go.mod if present
ENV GOTOOLCHAIN=auto

# Build args for cross-compilation (used by BuildKit/buildx)
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Leverage build cache for modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy only Go source at repo root to avoid cache busts from docs/CI edits.
# If you later add subpackages or assets, adjust this list or use a more
# selective .dockerignore include strategy.
COPY *.go ./

# Build the CLI from repository root (main.go is at root)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
    -o /out/urnet-client ./

# Runtime image
FROM alpine:3.22

# Add CA certs, tzdata and networking tools needed by vpn_linux.go (iproute2 provides `ip`)
RUN apk add --no-cache ca-certificates tzdata iproute2 && adduser -D -u 10001 appuser

# Binary
COPY --from=build /out/urnet-client /usr/local/bin/urnet-client

# App runtime
## 'root' is needed for tunnel in Mikrotik containers
USER root
WORKDIR /home/appuser
ENV URNETWORK_HOME=/home/appuser/.urnetwork
RUN mkdir -p "$URNETWORK_HOME"

# Default to showing help; override with subcommands
# To run VPN in Docker on Linux, you’ll need: --cap-add NET_ADMIN --device /dev/net/tun
ENTRYPOINT ["/usr/local/bin/urnet-client"]
CMD ["--help"]
