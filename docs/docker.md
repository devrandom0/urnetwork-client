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

- `docker-compose.linux.yml`: host networking; the proxy listens on `127.0.0.1:1080` of the host only.
- `docker-compose.macos.yml`: bridge networking; the proxy listens on `0.0.0.0:1080` inside the container and the port is published on `127.0.0.1:1080` of the host only. The startup WARN about a non-loopback bind refers to the container address and is expected here.

To offer the proxy to your LAN, change the bind (Linux: `--socks=0.0.0.0:1080`; macOS: `ports: ["1080:1080"]`) and set `URNETWORK_SOCKS_USER` / `URNETWORK_SOCKS_PASS` in `environment:`. See "SOCKS proxy for your LAN" in examples.md.

```bash
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d
docker compose -f docker-compose.yml -f docker-compose.macos.yml up -d
```

## Multi-arch buildx

```bash
make dockerx-build IMAGE_BASENAME=moghaddas/urnetwork-client
make dockerx-release IMAGE_BASENAME=moghaddas/urnetwork-client
```
