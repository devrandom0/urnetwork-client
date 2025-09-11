# urnet-client (experimental)

[![CI](https://github.com/devrandom0/urnetwork-client/actions/workflows/ci.yml/badge.svg)](https://github.com/devrandom0/urnetwork-client/actions/workflows/ci.yml)

A minimal CLI for BringYour that can:

- Authenticate and mint a client-scoped JWT
- Discover and connect to providers
- Run a real VPN dataplane (userspace TUN) on macOS and Linux
  - Optionally run a SOCKS5 proxy that sends only proxy traffic through the VPN
  - Discover locations and select providers by country/region/group
  - Run in the background and optionally log to a file
  - Manage JWTs: reuse existing client JWTs by default, optionally force mint, and auto-renew on an interval

## Build

Requires Go 1.24+ (module uses toolchain go1.24.5).

Build the CLI (from this directory):

```bash
# Build for your current OS/arch
go build -o dist/urnet-client ./

# Cross-build examples
GOOS=darwin GOARCH=amd64   go build -o dist/darwin_amd64/urnet-client   ./
GOOS=darwin GOARCH=arm64   go build -o dist/darwin_arm64/urnet-client   ./
GOOS=linux  GOARCH=amd64   go build -o dist/linux_amd64/urnet-client    ./
GOOS=linux  GOARCH=arm64   go build -o dist/linux_arm64/urnet-client    ./

# Or use the Makefile
make build                  # host
make build-all              # linux+macOS (amd64/arm64)
make docker-build           # build Docker image (urnetwork/urnet-client:local)
make compose-build          # build via docker-compose
```

Tip: building the module root without specifying `./cmd/urnet-client` can produce an archive, not a runnable binary.

## Usage

```bash
./urnet-client --help
./urnet-client --version   # print version
```

- Login and save JWT:

```bash
./urnet-client login --user_auth me@example.com --password 'secret'
```

- Verify code (if required):

```bash
./urnet-client verify --user_auth me@example.com --code 123456
```

- Save an existing JWT:
- Quick connect (login -> mint client -> start vpn in one go):

```bash
./urnet-client quick-connect \
  --user_auth me@example.com \
  --password 'secret' \
  --location_query "country:Germany" \
  --default_route \
  --tun utun10 \
  --log_level warn \
  --stats_interval 0
```

- Quick connect extras:

```bash
# force mint a fresh client JWT even if one exists
./urnet-client quick-connect --force_jwt ...

# renew client JWT periodically while running
./urnet-client quick-connect --jwt_renew_interval=12h ...

# background quick-connect (spawns itself detached so renewals continue)
./urnet-client quick-connect --background ...
```


```bash
./urnet-client save-jwt --jwt "<JWT>"
```

- Find providers:

```bash
./urnet-client find-providers [--count=8] [--rank_mode=quality|speed] \
  [--location_query="country:Germany" | --location_id=<id> | --location_group_id=<id>]
```

- Open control-plane transports (connectivity test only):

```bash
./urnet-client open [--transports=4]
```

- Mint a client-scoped JWT (needed for `open`, includes client_id):

```bash
./urnet-client mint-client
```

- Start VPN dataplane (userspace TUN):

```bash
sudo ./urnet-client vpn [--tun=utun10]
```

Defaults:

- API: <https://api.bringyour.com>
- Connect: <wss://connect.bringyour.com>
- JWT path: `~/.urnetwork/jwt` (override with `URNETWORK_HOME=/path`)

### Complete flag reference

All currently supported flags (aggregated from the CLI `--help`). Some flags only apply to specific subcommands (noted). Aliases or behavior notes are included where helpful.

Identity / auth:

- `--user_auth=<email-or-phone>` (login, verify, quick-connect)
- `--password=<password>` (login, quick-connect)
- `--code=<code>` (verify, quick-connect when verification required)
- `--jwt=<jwt>` (most commands; falls back to `~/.urnetwork/jwt` if unset)
- `--force_jwt` (quick-connect only: always mint a fresh client JWT)
- `--jwt_renew_interval=<dur>` (quick-connect only: periodically re‑mint client JWT; Go duration like `30m`, `12h`; `0` disables)

Endpoints:

- `--api_url=<url>` (default: <https://api.bringyour.com>)
- `--connect_url=<wss-url>` (default: wss://connect.bringyour.com)

Transport / control plane:

- `--transports=<n>` (open)
- `--count=<n>` (find-providers)
- `--rank_mode=quality|speed` (find-providers)

VPN / interface:

- `--tun=<name>` (vpn, quick-connect) pass `none` or omit for SOCKS-only
- `--ip_cidr=<cidr>` (vpn, quick-connect) default `10.255.0.2/24`
- `--mtu=<mtu>` (vpn, quick-connect) default `1420`

Routing:

- `--default_route` (vpn, quick-connect) enable full tunnel (split defaults)
- `--route=<list>` comma-separated host/CIDR routes via tunnel
- `--exclude_route=<list>` when full tunnel: keep these off VPN

Location selection (also valid for find-providers even if not shown in its usage line):

- `--location_query=<q>` (e.g. `country:Germany`, `region:Europe`, `group:Western Europe`, `country_code:DE`)
- `--location_id=<id>`
- `--location_group_id=<id>`

DNS:

- `--dns=<list>` comma-separated resolvers while VPN up
- `--dns_service=<name>` macOS network service to modify (e.g. `"Wi-Fi"`)
- `--dns_bootstrap=bypass|cache|none` (vpn, quick-connect) strategy for keeping DNS during route flip

SOCKS (inline with vpn / quick-connect):

- `--socks=<addr>` start local SOCKS5 bound to VPN
- `--socks_listen=<addr>` alias for `--socks`
- `--domain=<list>` domains forced through VPN when SOCKS mode used
- `--exclude_domain=<list>` domains explicitly bypassing VPN (SOCKS mode)

Standalone SOCKS subcommand:

- `--listen=<addr>` required listen address
- `--extender_ip=<ip>` extender IP
- `--extender_port=<port>` extender TLS port
- `--extender_sni=<sni>` SNI host name
- `--extender_secret=<secret>` optional pre-shared secret

Inbound filtering (vpn / quick-connect):

- `--allow_inbound_src=<list>` comma-separated IPv4 CIDRs/hosts allowed to initiate new inbound TCP
- `--allow_inbound_local` allow only local ranges + tunnel subnet (can combine with above)

Logging / diagnostics:

- `--background` (vpn, quick-connect) run detached and print child PID
- `--log_file=<path>` append logs to file instead of console
- `--log_level=quiet|error|warn|info|debug` default `info` (`--debug` implies debug unless level set)
- `--debug` verbose per-packet logging
- `--stats_interval=<sec>` interval to print counters (`0` disables; default `5`)
- `--version` print version and exit
- `-h`, `--help` show help

Environment fallbacks:

- `URNETWORK_HOME` override directory containing `jwt` file.
- `URNETWORK_USERNAME` and `URNETWORK_PASSWORD` can supply credentials for `quick-connect` (used if corresponding flags are omitted).

If a flag accepts a list, separate values with commas (no spaces). Duration values use Go syntax (e.g., `15m`, `1h30m`).

## VPN flags (quick reference)

Common flags for `vpn`:

- Identity and endpoints
  - `--api_url`, `--connect_url`, `--jwt`
- TUN and interface
  - `--tun=<name>` (macOS uses utun; pass utunN; invalid names auto-fallback)
  - `--ip_cidr=<cidr>` (assign IP on interface) and `--mtu=<mtu>`
- Routing
  - `--default_route` full-tunnel (adds split defaults)
  - `--route=<list>` comma-separated host or CIDR to send via VPN
  - `--exclude_route=<list>` prefixes to keep off VPN when full-tunnel is on
- Location selection (provider choice)
  - `--location_query=<q>` search for locations (e.g., `country:Germany`, `region:Europe`, `group:Western Europe`, `country_code:DE`)
  - `--location_id=<id>` use a specific location id (from `locations` command)
  - `--location_group_id=<id>` use a specific location group id (from `locations`)
- DNS
  - `--dns=<list>` DNS servers to prefer while VPN is up
  - `--dns_service=<name>` macOS network service to modify (e.g., "Wi-Fi")
  - `--dns_bootstrap=bypass|cache|none` behavior during default-route switch
    - bypass (default): add host routes to resolvers via current gateway
    - cache: same, but removed after tunnel is active (then DNS goes via VPN)
    - none: don’t add any temporary DNS routes
- SOCKS proxy
  - `--socks=<addr>` (or `--socks_listen`) start a local SOCKS5 proxy (TCP + UDP)
  - `--domain=<list>` only these domains go via VPN (SOCKS only)
  - `--exclude_domain=<list>` these domains bypass VPN (SOCKS only)
- Inbound connection control
  - `--allow_inbound_local` allow only “local” sources to initiate new inbound TCP connections from the tunnel:
    - 127.0.0.0/8, 169.254.0.0/16, 10/8, 172.16/12, 192.168/16, 100.64/10, and your `--ip_cidr` subnet
  - `--allow_inbound_src=<list>` allow new inbound TCP connections only from the specified IPv4 CIDRs/hosts (comma-separated)
  - Behavior: specifying either flag automatically blocks new inbound TCP SYNs by default, allowing only the listed sources. TCP segments without ACK (e.g., unsolicited RST) are also dropped. No OS firewall changes are made; filtering happens in userspace.
- Debug
  - `--debug`, `--stats_interval=<sec>`
  - `--background` run vpn detached and print child PID
  - `--log_file=/path/to/file` append logs to file instead of console
  - `--log_level=quiet|error|warn|info|debug` control verbosity (default: info). `--debug` implies debug unless you set a level.
  - Tip: set `--stats_interval=0` to disable periodic stats output.

macOS specifics:

- Requires `sudo` to create utun and manage routes/DNS.
- Full-tunnel uses split defaults and tracks variants for accurate cleanup.
- Excludes are routed back to your current default gateway (auto-detected).
- In SOCKS-only mode, adds interface-scoped split defaults for utun so only SOCKS-bound sockets prefer the VPN. They don’t affect system routing and are removed on exit.

Background/logging notes:

- `--background` is supported for `vpn`. It forks a detached child, prints its PID, then exits.
- By default, logs go to the console. If you pass `--log_file=/path/to/file`, logs are appended to that file instead of the console.
- On macOS/Linux you still need `sudo` for `vpn` even in background mode.

Linux specifics:

- Requires `sudo` and TUN support. We configure IP/MTU and routes.
- Full-tunnel now uses macOS-style split defaults on Linux as well: adds `0.0.0.0/1` and `128.0.0.0/1` via the TUN, leaving your original `default` route untouched.
- Control-plane endpoints (API/connect) get temporary host routes via your current default gateway so the tunnel can establish and stay reachable.
- `--exclude_route` prefixes are sent via your original default path when known (gateway/dev), otherwise marked `unreachable`. All added routes are removed on exit.
- SOCKS binds to the TUN interface; if your kernel ignores binding for some flows, you may need policy routing (not yet enabled by default).
  (Router/forwarding iptables controls were removed; use application-level or host firewall if needed.)

## Examples

- SOCKS-only (no global route/DNS changes):

```bash
sudo ./urnet-client vpn \
  --socks=127.0.0.1:1080 \
  --debug
# Configure your app/browser to use 127.0.0.1:1080 (SOCKS5)
```

- Full tunnel with excludes:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  # on Linux use urnet0 (or your chosen TUN name)
  --default_route \
  --exclude_route=10.0.0.0/8,169.254.0.0/16,1.1.1.1/32 \
  --debug --stats_interval=5
```

### Inbound allowlist examples

- Allow new inbound only from local ranges and your TUN subnet:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --allow_inbound_local
```

- Allow new inbound only from specific CIDRs/hosts:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --allow_inbound_src=192.168.1.50/32,10.0.0.0/8
```

- Combine local and explicit sources:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --allow_inbound_local \
  --allow_inbound_src=203.0.113.7/32
```

- Full tunnel plus DNS via VPN, with temporary DNS bootstrap:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --dns=1.1.1.1,1.0.0.1 \
  --dns_service="Wi-Fi" \
  --dns_bootstrap=cache \
  --debug --stats_interval=2
```

- Route a specific host through the tunnel (macOS):

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --route 1.1.1.1 \
  --debug --stats_interval=2
```

- Select Germany by name using a query (works for find-providers and vpn):

```bash
# show the location first
./urnet-client locations --query="country:Germany"

# connect using a query
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_query="country:Germany"
```

- Run VPN in the background and log to a file (macOS):

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_query="country:Germany" \
  --background \
  --log_file=/tmp/urnet-client.log
# prints: started in background pid=<PID>
# tail the log
tail -f /tmp/urnet-client.log
```

- Use a location id or group id from the `locations` command:

```bash
# list active locations/groups (with ids and provider counts)
./urnet-client locations --query="country:*"

# connect by id
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_id=018bab1d-5b38-1698-e335-d5ad3a486f25
```

- Region/group examples:

```bash
./urnet-client locations --query="region:Europe"
./urnet-client locations --query="group:Western Europe"
./urnet-client find-providers --location_query="region:Europe"
```

## Docker

Build the image (multi-stage, small runtime):

## SOCKS-only mode (no TUN) — RouterOS-friendly

Use the new `socks` subcommand to run a local SOCKS5 proxy that sends TCP via the overlay extender without creating a TUN device. This works on MikroTik RouterOS containers where `/dev/net/tun` and `NET_ADMIN` are unavailable.

Limitations:

- TCP CONNECT only; UDP ASSOCIATE is not supported in this mode.
- Only common TLS ports are allowed by default (443, 853, 993, 995, 465, 2376, 3269, 4460).

Run directly:

```bash
./urnet-client socks \
  --listen=0.0.0.0:1080 \
  --extender_ip=<IP> \
  --extender_port=443 \
  --extender_sni=<hostname> \
  --extender_secret='<optional-psk>' \
  --domain='example.org,*.example.net' \
  --exclude_domain='internal.local'
# Point your app/browser to SOCKS5 at 0.0.0.0:1080
```

Docker Compose variant (no NET_ADMIN, no /dev/net/tun):

```bash
# Edit docker-compose.socks.yml and set your extender_ip/extender_sni/port (and optional secret)
docker compose -f docker-compose.socks.yml up -d
```

Notes for RouterOS:

- RouterOS does not support exposing `/dev/net/tun` or `cap-add NET_ADMIN` into containers; VPN mode won’t work.
- This SOCKS-only mode routes only proxied TCP via the overlay; system-wide routing is unchanged.

```bash
# from repo root
DOCKER_BUILDKIT=1 docker build -f connect/cmd/urnet-client/Dockerfile -t moghaddas/urnetwork-client:local .
```

Run the CLI in a container. Mount a host directory to persist the JWT across runs (the Makefile has shortcuts: `make docker-build`, `make docker-mint-client`, `make docker-vpn`). This repo also includes multi-arch buildx targets to publish to Docker Hub under `moghaddas/urnetwork-client`.

```bash
# create a local dir to hold JWT
mkdir -p ~/.urnetwork

# show help
docker run --rm \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local --help

# login
docker run --rm \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local login --user_auth me@example.com --password 'secret'

# mint a client-scoped JWT (includes client_id), saved to /data/jwt
docker run --rm \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local mint-client

# find providers
docker run --rm \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local find-providers

# open (press Ctrl+C to stop)
# Note: this only validates control-plane connectivity; it is not a full VPN.
docker run --rm -it \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local open

# start VPN dataplane (Linux only; needs TUN and NET_ADMIN)
docker run --rm -it \
  --cap-add NET_ADMIN \
  --device /dev/net/tun \
  -e URNETWORK_HOME=/data \
  -v ~/.urnetwork:/data \
  moghaddas/urnetwork-client:local vpn --tun urnet0

# then on the host (outside container), set IP and routes, e.g.:
# ip addr add 10.0.0.2/24 dev urnet0
# ip link set urnet0 up
# ip route add <your-destination-cidr> dev urnet0
# configure DNS if your provider offers resolvers
```

Notes:

- Container networking changes do not alter the host’s routing on macOS/Windows Docker Desktop; only the container’s traffic is affected.
- For host VPN on macOS/Linux, run the binary directly on the host with `sudo`.

## macOS notes

- `vpn` is implemented and creates a utun interface.
- If a specific utun name fails, the client retries with an auto-assigned utun.
- Excludes route back to your original default gateway when full-tunnel is on.
- DNS can be adjusted with `networksetup` if `--dns_service` is provided; otherwise it’s left unchanged.
- Location queries: if the server-side search returns no results for a specific query (e.g., `country:Germany`), the client falls back to the active locations list and filters locally, so queries by country/region/group names still work.

## Linux VPN quick test (inside the container)

If you run `vpn` in a container and want to quickly verify packets flow, exec into the container and configure the TUN and a test route:

```bash
docker exec -it <container-name> sh
ip addr add 10.255.0.2/24 dev urnet0
ip link set urnet0 up
ip route add 1.1.1.1/32 dev urnet0
ping -c 3 -I urnet0 1.1.1.1
# optional: curl over interface
apk add --no-cache curl 2>/dev/null || true
curl --interface urnet0 https://icanhazip.com
```

## docker-compose

A basic compose file is provided at `docker-compose.yml` in this directory. It builds the image, enables NET_ADMIN and /dev/net/tun, uses host networking, and persists JWTs to a named volume. By default it tags the local image as `moghaddas/urnetwork-client:local`. To use a published image, set the `image:` to `moghaddas/urnetwork-client:<version>`. Examples:

```bash
# build
docker compose -f docker-compose.yml build

# show help via compose
docker compose -f docker-compose.yml run --rm urnet-client --help

# run vpn (Linux host)
docker compose -f docker-compose.yml run --rm \
  --cap-add=NET_ADMIN --device=/dev/net/tun \
  urnet-client vpn --tun urnet0

### OS-specific overrides: VPN + SOCKS together

Two overrides are included to run VPN (TUN) and a SOCKS5 proxy together with sensible networking for each OS:

- `docker-compose.linux.yml` (Linux host): uses host networking so `--socks=0.0.0.0:1080` is reachable directly; keeps NET_ADMIN and `/dev/net/tun`.
- `docker-compose.macos.yml` (Docker Desktop macOS): uses bridge networking with `ports: ["1080:1080"]` because host networking isn’t supported; keeps NET_ADMIN and `/dev/net/tun` in the Linux VM.

Usage:

```bash
# Linux host: starts quick-connect with TUN+SOCKS (SOCKS on host:1080)
docker compose -f docker-compose.yml -f docker-compose.linux.yml up -d

# macOS host (Docker Desktop): starts quick-connect with TUN+SOCKS (SOCKS on localhost:1080)
docker compose -f docker-compose.yml -f docker-compose.macos.yml up -d

# optional: login once to persist a JWT in the volume instead of passing creds every time
docker compose run --rm urnet-client login --user_auth "$URNETWORK_USERNAME" --password "$URNETWORK_PASSWORD"
```

Notes:

- The VPN inside Docker only affects the container’s traffic. Use the SOCKS proxy from your host/apps to route selected flows through the tunnel.
- You can edit the override `command` array to add flags (e.g., `--location_query`, `--log_level`, `--default_route`).

## Publish a multi-arch image (amd64+arm64)

You can build and push multi-architecture images to Docker Hub `moghaddas/urnetwork-client` using Docker Buildx. Ensure you're logged in (`docker login`). The Makefile will tag with the current version inferred from Git and also tag `latest`.

Option A: with Makefile helpers

```bash
# Build locally for both platforms (loads into local Docker; no push)
make dockerx-build IMAGE_BASENAME=moghaddas/urnetwork-client

# Build and push to Docker Hub with tags :$(git describe) and :latest
make dockerx-release IMAGE_BASENAME=moghaddas/urnetwork-client
```

Option B: raw docker buildx commands

```bash
# one-time setup
docker buildx create --name urnetx --use
docker buildx inspect --bootstrap

# set a version tag
VER=$(git describe --tags --always --dirty 2>/dev/null || echo dev)

# build and push (amd64+arm64)
DOCKER_BUILDKIT=1 docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -f connect/cmd/urnet-client/Dockerfile \
  -t moghaddas/urnetwork-client:${VER} \
  -t moghaddas/urnetwork-client:latest \
  . \
  --push
```

## Notes and limitations

- UDP over SOCKS (UDP ASSOCIATE) is supported for QUIC/DNS, but not all apps use SOCKS for UDP.
- IPv6 routing and binding are not enabled yet; IPv4 is the primary path.
- On some Linux setups, strict interface binding may require policy routing rules.
