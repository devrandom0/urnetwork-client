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

### Files the client writes and reads

- The JWT file (`$URNETWORK_HOME/jwt`, default `~/.urnetwork/jwt`) is replaced atomically through a temp file and rename, with mode 0600. `--log_file` is opened for append with mode 0600. A missing log directory is created with mode 0700.
- Both refuse a target that is a symlink or not a regular file (a FIFO at the log path fails instead of blocking), a directory owned by another non-root user (when not run as root), and a directory that is group/world-writable unless it is root-owned with the sticky bit, like `/tmp`.
- Every parent directory up to `/` must be owned by root or by you (under sudo, by the `SUDO_UID` user) and must not be group/world-writable unless it is root-owned with the sticky bit. A group-writable `~/.urnetwork`, `--log_file` directory or parent of either is now refused; run `chmod g-w,o-w <dir>` to fix it.
- Symlinks in the directory path are resolved first, so paths such as `/tmp` on macOS work. When run as root, a symlink in the directory path is followed only if the link is owned by root and sits in a root-owned directory that is not group/world-writable (or has the sticky bit, like `/tmp`). System links such as `/tmp` and `/var` on macOS pass; a link you own, for example `~/.urnetwork` pointing elsewhere, is refused under sudo.
- `--log_file` also refuses a file with more than one hard link or owned by a different user. When run as root an existing log file must be owned by root or by the `SUDO_UID` user.
- When run with sudo against a directory owned by the invoking user (`SUDO_UID`), for example your home, new files are handed to that user so later non-sudo commands can read them. In any other directory they stay owned by root.
- The JWT file and `--config` file must be regular files, at most 64 KiB and 1 MiB respectively. A symlink to them (for example from a dotfile manager) is followed in normal runs and refused when run as root.

## Security note

Command-line arguments are visible in shell history and to every local user through `ps`. Prefer the environment variables above for passwords and tokens. With `--background`, `--password`, `--jwt` and `--socks_pass` are moved from the child's command line into its environment.

Only `https://` API URLs and `wss://` connect URLs are accepted, except `http://` / `ws://` to `localhost` for development.
