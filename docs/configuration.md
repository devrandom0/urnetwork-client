# Configuration

## Precedence

For `vpn` and `quick-connect`:

1. CLI flags
2. Config file (`--config=<path>`)
3. Built-in defaults

Environment variables are fallbacks for specific flags only (listed below). They apply when the matching flag is not given.

## YAML config file

`vpn` and `quick-connect` accept `--config=<path>`. For `quick-connect`, the file's `api_url` is also used for login.

```yaml
api_url: https://api.bringyour.com
connect_url: wss://connect.bringyour.com
tun: utun10
ip_cidr: 10.255.0.2/24
mtu: 1420
default_route: true
dns:
  - 1.1.1.1
  - 1.0.0.1
dns_service: Wi-Fi
dns_bootstrap: bypass
socks: 127.0.0.1:1080
location_query: "country:Germany"
log_level: info
stats_interval: 5
debug: false
```

`socks` is the canonical key. `socks_listen` is still accepted for older files; if both are set, `socks` wins.

SOCKS credentials are not read from the file. Use `--socks_user` and `URNETWORK_SOCKS_PASS`.

### Log level

The effective level is the first one set among: `--log_level`, `--debug`, `log_level` in the file, `debug: true` in the file. The default is `info`.

### Known limitations

These are planned to go away with a config rework:

- A CLI value equal to the built-in default counts as "not set". For example `--mtu=1420` together with `mtu: 1380` in the file gives 1380. The same applies to `--ip_cidr=10.255.0.2/24`, `--stats_interval=5`, `--dns_bootstrap=bypass`, and the default `--api_url` / `--connect_url`.
- Booleans in the file (`default_route`, `allow_inbound_local`, `debug`) can only turn a feature on. `false` in the file has no effect.
- `stats_interval: 0` in the file does not disable stats. Use `--stats_interval=0`.

## Environment variables

- `URNETWORK_HOME`: directory containing `jwt` (default `~/.urnetwork`).
- `URNETWORK_USERNAME`: fallback for `--user_auth` in `quick-connect`.
- `URNETWORK_PASSWORD`: fallback for `--password`.
- `URNETWORK_JWT`: fallback for `--jwt`.
- `URNETWORK_SOCKS_USER`: fallback for `--socks_user`.
- `URNETWORK_SOCKS_PASS`: fallback for `--socks_pass`.

## Security note

Command-line arguments are visible in shell history and to every local user through `ps`. Prefer the environment variables above for passwords and tokens. With `--background`, `--password`, `--jwt` and `--socks_pass` are moved from the child's command line into its environment.

Only `https://` API URLs and `wss://` connect URLs are accepted, except `http://` / `ws://` to `localhost` for development.
