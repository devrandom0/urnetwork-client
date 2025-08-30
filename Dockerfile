# syntax=docker/dockerfile:1

# Multi-stage build for urnet-client
FROM golang:1.24-alpine AS build

# Enable module caching and ensure certs are present during build
RUN apk add --no-cache git ca-certificates && update-ca-certificates

# Ensure the correct Go toolchain can be used if the module requests it
ENV GOTOOLCHAIN=auto

# Build args provided by BuildKit for cross-arch builds
ARG TARGETOS
ARG TARGETARCH

# Work inside repo root and copy full source (needed for replace ../.. in nested module)
WORKDIR /src
COPY . ./connect

# Build the urnet-client binary from nested module
WORKDIR /src/connect/cmd/urnet-client
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
    -o /out/urnet-client ./

# Runtime image
FROM alpine:3.20

# Add CA certs, tzdata and networking tools needed by vpn_linux.go (iproute2 provides `ip`)
RUN apk add --no-cache ca-certificates tzdata iproute2 && adduser -D -u 10001 appuser

# Binary
COPY --from=build /out/urnet-client /usr/local/bin/urnet-client

# App runtime
USER appuser
WORKDIR /home/appuser
ENV URNETWORK_HOME=/home/appuser/.urnetwork
RUN mkdir -p "$URNETWORK_HOME"

# Default to showing help; override with subcommands
# To run VPN in Docker on Linux, you’ll need: --cap-add NET_ADMIN --device /dev/net/tun
ENTRYPOINT ["/usr/local/bin/urnet-client"]
CMD ["--help"]
