# Cross-platform build Makefile for urnet-client

MODULE_PATH := github.com/urnetwork/connect
CMD_PATH := $(MODULE_PATH)/cmd/urnet-client
BINARY := urnet-client
DIST := dist

# Image naming
# Base repo/name without tag (override on invocation):
IMAGE_BASENAME ?= moghaddas/urnetwork-client
# Local dev tag used by docker-build and compose builds
IMAGE ?= $(IMAGE_BASENAME):local

# Try to derive a version; fallback to dev
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X 'main.Version=$(VERSION)'

.PHONY: help
help:
	@echo "Targets:"
	@echo "  build                 Build for host platform -> $(DIST)/$(BINARY)"
	@echo "  build-linux-amd64     Build Linux/amd64 -> $(DIST)/linux_amd64/$(BINARY)"
	@echo "  build-linux-arm64     Build Linux/arm64 -> $(DIST)/linux_arm64/$(BINARY)"
	@echo "  build-darwin-amd64    Build macOS/amd64 -> $(DIST)/darwin_amd64/$(BINARY)"
	@echo "  build-darwin-arm64    Build macOS/arm64 -> $(DIST)/darwin_arm64/$(BINARY)"
	@echo "  build-all             Build all four targets"
	@echo "  docker-build          Build Docker image urnet-client:local"
	@echo "  dockerx-setup         Create/use a buildx builder for multi-arch builds"
	@echo "  dockerx-build         Build multi-arch image (no push)"
	@echo "  dockerx-push          Build and push multi-arch image to registry"
	@echo "  dockerx-release       Build and push with :$(VERSION) and :latest tags"
	@echo "  docker-mint-client    Run mint-client inside Docker"
	@echo "  docker-vpn            Run vpn in Docker (Linux only; needs TUN+NET_ADMIN)"
	@echo "  compose-build         Build with docker-compose"
	@echo "  compose-help          Show compose service help"
	@echo "  clean                 Remove dist/"

$(DIST):
	@mkdir -p $(DIST)

.PHONY: build
build: $(DIST)
	GOFLAGS=-trimpath go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) ./

.PHONY: build-linux-amd64
build-linux-amd64:
	@mkdir -p $(DIST)/linux_amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/linux_amd64/$(BINARY) .

.PHONY: build-linux-arm64
build-linux-arm64:
	@mkdir -p $(DIST)/linux_arm64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/linux_arm64/$(BINARY) .

.PHONY: build-darwin-amd64
build-darwin-amd64:
	@mkdir -p $(DIST)/darwin_amd64
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/darwin_amd64/$(BINARY) .

.PHONY: build-darwin-arm64
build-darwin-arm64:
	@mkdir -p $(DIST)/darwin_arm64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/darwin_arm64/$(BINARY) .

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64
	@echo "Built into $(DIST)/{linux, darwin}_*/$(BINARY)"

.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build -f Dockerfile -t $(IMAGE) ../../..

# --- Multi-arch (buildx) ---
.PHONY: dockerx-setup
dockerx-setup:
	@docker buildx inspect urnetx >/dev/null 2>&1 || docker buildx create --name urnetx --use
	@docker buildx use urnetx
	@docker buildx inspect --bootstrap

.PHONY: dockerx-build
dockerx-build: dockerx-setup
	DOCKER_BUILDKIT=1 docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  -f Dockerfile \
	  -t $(IMAGE_BASENAME):$(VERSION) \
	  -t $(IMAGE_BASENAME):latest \
	  ../../.. \
	  --load

.PHONY: dockerx-push
dockerx-push: dockerx-setup
	DOCKER_BUILDKIT=1 docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  -f Dockerfile \
	  -t $(IMAGE_BASENAME):$(VERSION) \
	  -t $(IMAGE_BASENAME):latest \
	  ../../.. \
	  --push

.PHONY: dockerx-release
dockerx-release: dockerx-push

.PHONY: docker-mint-client
docker-mint-client:
	@mkdir -p $$HOME/.urnetwork
	docker run --rm \
	  -e URNETWORK_HOME=/data \
	  -v $$HOME/.urnetwork:/data \
	  $(IMAGE) mint-client

.PHONY: docker-vpn
docker-vpn:
	@mkdir -p $$HOME/.urnetwork
	# Requires Linux host or Linux VM. On macOS, this runs but only tunnels container traffic.
	docker run --rm -it \
	  --cap-add NET_ADMIN \
	  --device /dev/net/tun \
	  -e URNETWORK_HOME=/data \
	  -v $$HOME/.urnetwork:/data \
	  $(IMAGE) vpn --tun urnet0

.PHONY: compose-build
compose-build:
	docker compose -f docker-compose.yml build

.PHONY: compose-help
compose-help:
	docker compose -f docker-compose.yml run --rm urnet-client --help

.PHONY: clean
clean:
	rm -rf $(DIST)
