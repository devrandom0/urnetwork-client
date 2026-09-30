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
| `recover` | Undo the DNS and route changes of a VPN run that died without cleaning up (needs root) |

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

- `--dns=<list>`: upstream servers for the DNS stub with `--default_route` (default `1.1.1.1,9.9.9.9`); without `--default_route` they are routed through the TUN.
- `--dns_bootstrap=none|cache|bypass`: path of the stub's upstream queries. `none` (default) goes through the tunnel; `cache` starts outside and switches after the tunnel is up; `bypass` stays outside and leaks DNS.
- `--dns_service=<name>`: ignored, kept so old command lines still parse.

See "DNS and IPv6" in configuration.md.

### SOCKS

`vpn` and `quick-connect`:

- `--socks=<addr>` (alias: `--socks_listen`): start a SOCKS5 proxy. With `--tun`, proxied traffic is bound to the VPN interface. If that binding fails, the request is refused instead of leaving through your normal connection.
- `--socks_user=<user>` and `--socks_pass=<pass>`: require RFC 1929 username/password auth. Env fallbacks: `URNETWORK_SOCKS_USER`, `URNETWORK_SOCKS_PASS`. Prefer the env var for the password, because command-line arguments are visible to every local user.
- `--domain=<list>`: comma-separated domains that must route through the VPN (SOCKS only).
- `--exclude_domain=<list>`: comma-separated domains that bypass the VPN (SOCKS only).

If the proxy listens on anything other than a loopback address and no auth is configured, the client logs a WARN at startup. It still starts, so existing LAN setups keep working.

With `--tun` and `--default_route`, SOCKS hostname lookups go to the DNS stub. With `--tun` but without `--default_route`, they go through the VPN interface and fail closed; if the system resolver is on the LAN or is Docker's `127.0.0.11`, set `--dns=<public resolver>` (e.g. `--dns=1.1.1.1`). In that case the client logs a WARN at startup. While IPv6 is off, SOCKS resolves A records only.

Supported SOCKS5 commands: CONNECT and UDP ASSOCIATE. BIND is answered with "command not supported".

Standalone proxy without TUN/VPN: `./urnet-client socks --listen=... --extender_ip=... --extender_port=... --extender_sni=...`

- `--listen=<addr>`
- `--extender_ip=<ip>`
- `--extender_port=<port>`
- `--extender_sni=<sni>`
- `--extender_secret=<secret>`
- `--socks_user=<user>`, `--socks_pass=<pass>` (same meaning and env fallbacks as above)

### Inbound filtering

- `--allow_inbound_local`
- `--allow_inbound_src=<list>`

### IPv6 control

- `--enable_ipv6`: route IPv6 through the VPN. Off by default; then IPv6 is blocked with `--default_route` (reject routes, empty AAAA answers) and never sent into the tunnel. Only use it if your provider supports IPv6.

### Kill switch

- `--kill_switch`: Block all traffic if the VPN connection drops. **Requires `--default_route`.** On graceful exit, the blackhole default route is preserved; traffic stays blocked until you manually restore your gateway. If startup fails after the blackhole route is installed, the host stays offline the same way: run the restore command printed in the error to bring connectivity back. It also installs an IPv6 blackhole default, kept after exit the same way.

### Recover

- `--force`: `recover` also undoes changes whose recorded owner process is still running. Without it, `recover` refuses and names that pid.

### Diagnostics

- `--log_level=quiet|error|warn|info|debug`
- `--debug`: per-packet logs. With `--default_route` they also hold one line per DNS query the stub handles: the asking address, the name and record type, and the path it took. They stay in the terminal or `--log_file` (created 0600) and carry no secrets, but they are a list of the names you looked up.
- `--stats_interval=<sec>`
- `--log_file=<path>`
- `--background`
- `--version`
- `-h`, `--help`

Notes:

- Lists are comma-separated with no spaces.
- Durations use Go format, e.g. `15m`, `1h30m`.