# Command Reference

## Commands

| Command | Description |
|---------|-------------|
| `login` | Authenticate with email/phone and save JWT |
| `verify` | Submit verification code |
| `save-jwt` | Save an existing JWT token |
| `mint-client` | Mint a client-scoped JWT |
| `quick-connect` | Login + mint + connect in one command |
| `find-providers` | List providers, optionally filtered by location |
| `locations` | List active locations and groups |
| `open` | Open control-plane transports (connectivity test) |
| `vpn` | Start VPN dataplane (userspace TUN) |
| `socks` | Start standalone SOCKS5 proxy (no TUN required) |

Global help:

```bash
./urnet-client --help
./urnet-client --version
```

## Flags

### Identity and auth

- `--user_auth=<email-or-phone>`
- `--password=<password>`
- `--code=<code>`
- `--jwt=<jwt>`
- `--force_jwt`
- `--jwt_renew_interval=<dur>`

### Endpoints

- `--api_url=<url>`
- `--connect_url=<wss-url>`

### VPN and interface

- `--tun=<name>`
- `--ip_cidr=<cidr>`
- `--mtu=<mtu>`
- `--config=<path>`

### Routing

- `--default_route`
- `--route=<list>`
- `--exclude_route=<list>`

### Location selection

- `--location_query=<q>`
- `--location_id=<id>`
- `--location_group_id=<id>`

### DNS

- `--dns=<list>`
- `--dns_service=<name>`
- `--dns_bootstrap=bypass|cache|none`

### SOCKS

When using `vpn` subcommand:

- `--socks=<addr>` (alias: `--socks_listen`) — Start SOCKS5 proxy. **Requires `--tun` to bind traffic through VPN.**
- `--domain=<list>` — Comma-separated domains that must route through VPN (SOCKS-only mode)
- `--exclude_domain=<list>` — Comma-separated domains to exclude from VPN routing

For a standalone SOCKS proxy (without TUN/VPN), use the separate `socks` command: `./urnet-client socks --listen=... --extender_ip=... --extender_port=... --extender_sni=...`

- `--listen=<addr>`
- `--extender_ip=<ip>`
- `--extender_port=<port>`
- `--extender_sni=<sni>`
- `--extender_secret=<secret>`

### Inbound filtering

- `--allow_inbound_local`
- `--allow_inbound_src=<list>`

### IPv6 control

- `--enable_ipv6` — Allow IPv6 traffic through the VPN (disabled by default). Only use if your provider supports IPv6.

### Diagnostics

- `--log_level=quiet|error|warn|info|debug`
- `--debug`
- `--stats_interval=<sec>`
- `--log_file=<path>`
- `--background`
- `--version`
- `-h`, `--help`

Notes:

- Lists are comma-separated with no spaces.
- Durations use Go format, e.g. `15m`, `1h30m`.