# Docker Guide

## Build image

```bash
make docker-build
# or
DOCKER_BUILDKIT=1 docker build -t moghaddas/urnetwork-client:local .
```

## Basic container usage

```bash
mkdir -p ~/.urnetwork

docker run --rm \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local --help
```

## VPN in container (Linux host)

```bash
docker run --rm -it \
  --cap-add NET_ADMIN \
  --device /dev/net/tun \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local vpn --tun urnet0
```

## docker-compose

```bash
docker compose -f docker-compose.yml build
docker compose -f docker-compose.yml run --rm urnet-client --help
```

OS-specific overrides:

- `docker-compose.linux.yml`: host networking
- `docker-compose.macos.yml`: bridge networking with `1080:1080` port mapping

```bash
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d
docker compose -f docker-compose.yml -f docker-compose.macos.yml up -d
```

## Multi-arch buildx

```bash
make dockerx-build IMAGE_BASENAME=moghaddas/urnetwork-client
make dockerx-release IMAGE_BASENAME=moghaddas/urnetwork-client
```
