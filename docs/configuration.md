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
  - 9.9.9.9
dns_bootstrap: none
enable_ipv6: false
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

- A CLI value equal to the built-in default counts as "not set". For example `--mtu=1420` together with `mtu: 1380` in the file gives 1380. The same applies to `--ip_cidr=10.255.0.2/24`, `--stats_interval=5`, `--dns_bootstrap=none`, and the default `--api_url` / `--connect_url`.
- Booleans in the file (`default_route`, `allow_inbound_local`, `enable_ipv6`, `debug`) can only turn a feature on. `false` in the file has no effect.
- `stats_interval: 0` in the file does not disable stats. Use `--stats_interval=0`.

## DNS and IPv6

With `--default_route`, DNS and IPv6 follow the tunnel. Without `--default_route` (split routing or SOCKS only) the client does not touch system DNS or IPv6 routes; `--dns` servers are routed through the TUN as before.

### DNS

- A DNS stub listens on port 53 over UDP and TCP. On Linux it uses the TUN address from `--ip_cidr`, so `10.255.0.2:53` by default. On macOS it uses `127.0.0.1:53`, because macOS sends queries for a resolver on the TUN address into the tunnel instead of to the local stub; the client does not start if another program already listens there.
- For the session the system resolver points at the stub:
  - macOS: a `scutil` dynamic-store entry, `State:/Network/Service/urnet-client`, on `lo0`. Saved network settings are not changed and the entry is gone after a reboot. At startup the client checks that `scutil --dns` lists the stub first, and refuses to start otherwise.
  - Linux with systemd-resolved (`/etc/resolv.conf` is resolved's `stub-resolv.conf`): `resolvectl dns`, `resolvectl domain <tun> '~.'` and `resolvectl default-route <tun> true` on the TUN link, reverted on exit.
  - Other Linux, including containers: `/etc/resolv.conf` is copied to `/etc/resolv.conf.urnet-client.bak`, replaced with `nameserver <stub>`, and restored on exit. If `/etc/resolv.conf` is a symlink managed by another tool (NetworkManager, resolvconf, resolved without its stub file), the client refuses to start.
- The process that points system DNS at its stub records its pid: the `UrnetPID` key in the `scutil` entry, a `# urnet-client pid=<pid>` line in `/etc/resolv.conf`, or `/run/urnet-client-dns.owner` with systemd-resolved. While that process is alive, a second `--default_route` run refuses to start with "another urnet-client (pid N) owns system DNS". Leftovers of a process that is gone are cleaned up at the next start, in every mode.
- With systemd-resolved, `~.` only catches names no other link claims. Search and routing domains on other links (for example a DHCP search domain such as `lan`) still go to that link's DNS servers outside the tunnel, and so do single-label names resolved over LLMNR or mDNS. Startup logs a WARN naming the links and domains `resolvectl domain` reports.
- The stub forwards to `--dns`, or to `1.1.1.1` and `9.9.9.9` when `--dns` is not set. UDP queries go upstream over UDP, TCP queries over TCP, so truncated answers still work. It does not cache.
- `--dns_bootstrap` picks the path of the stub's upstream queries:
  - `none` (default): upstream sockets are bound to the TUN. If binding fails, the query fails; nothing goes out your normal connection.
  - `cache`: outside the tunnel through host routes until the tunnel carries traffic both ways or 3 seconds pass, then bound to the TUN.
  - `bypass`: outside the tunnel for the whole session. The host routes carry all traffic to the upstream IPs, not only port 53, so DNS over HTTPS to them leaves outside the tunnel too. Your network sees every lookup; startup logs a WARN.
- The API and connect hostnames are answered by the stub with the addresses resolved at startup, which are routed around the tunnel. Reconnecting never depends on the tunnel it is rebuilding.
- If no upstream answers within 1 second in total, the stub answers SERVFAIL, so macOS keeps asking the stub instead of falling back to the LAN resolver while the tunnel is down.
- The stub only answers queries from this machine. Queries from other hosts are dropped without a reply, their TCP connections are closed at once, and at most 64 TCP connections are open at a time.
- With `--socks` and `--default_route`, SOCKS hostname lookups go to the stub too. SOCKS-only mode (no `--tun`) keeps its own resolver.
- `--dns_service` is ignored; setting it logs a WARN.
- Browsers with DNS over HTTPS resolve names themselves. That traffic goes through the tunnel like any other HTTPS, but the stub cannot filter it.

### IPv6

- IPv6 is off by default. In every TUN mode the TUN gets no IPv6 address and IPv6 packets are never sent into the tunnel.
- With `--default_route`, reject routes for `::/1` and `8000::/1` make IPv6 connections fail at once with "no route to host", so apps use IPv4 right away. Link-local and connected LAN IPv6 prefixes keep working. AAAA queries get an empty answer, AAAA records and the `ipv6hint` of HTTPS and SVCB records are removed from other answers, and SOCKS resolves A records only.
- The reject routes are always attempted. If the Linux kernel refuses them because IPv6 is disabled (common in containers), there is nothing to block and startup goes on.
- An existing `::/1` or `8000::/1` route from another tool is replaced, and startup logs a WARN naming it. More specific IPv6 routes that already exist, for example a `/64` from another VPN, still win over the `/1` reject routes.
- `--enable_ipv6` (or `enable_ipv6: true`) gives the TUN `fd00::2/120`, routes `::/1` and `8000::/1` into the TUN and passes AAAA answers.
- `--kill_switch` also installs an IPv6 blackhole default and keeps it after exit, like the IPv4 one.

### Failing closed

The VPN does not start, and whatever was already applied is undone, when the stub cannot bind its port, when system DNS cannot be pointed at the stub, or when an IPv6 reject, IPv6 tunnel or IPv6 kill switch route cannot be installed. After a crash, system DNS keeps pointing at the stopped stub until the next start or `sudo urnet-client recover` cleans it up. Whether lookups then fail or leak depends on the platform; `docs/platform-notes.md` has the details and the manual commands.

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
