# Configuration

Precedence:

1. CLI flags
2. Config file (`--config`)
3. Environment variables
4. Built-in defaults

## YAML config file

`vpn` and `quick-connect` accept `--config=<path>`.

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
socks_listen: 127.0.0.1:1080
location_query: "country:Germany"
log_level: info
stats_interval: 5
debug: false
```

## Environment variables

- `URNETWORK_HOME`: Override path containing `jwt`
- `URNETWORK_USERNAME`: Username for `quick-connect` when `--user_auth` omitted
- `URNETWORK_PASSWORD`: Password for `quick-connect` when `--password` omitted

## Security note

Avoid passing `--password` in shell history or process args when possible. Prefer `URNETWORK_PASSWORD`.
