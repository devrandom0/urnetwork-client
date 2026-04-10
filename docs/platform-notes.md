# Platform Notes

## macOS

- Requires `sudo` for TUN, routes, and DNS changes.
- If requested utun name fails, client retries with auto-assigned utun.
- Full tunnel uses split defaults (`0.0.0.0/1` and `128.0.0.0/1`).
- Exclude routes are sent to current default gateway.
- DNS changes are applied via `networksetup` when `--dns_service` is provided.

## Linux

- Requires `sudo` and TUN support.
- Full tunnel also uses split defaults.
- Control-plane endpoints get temporary host routes via current default gateway.
- `--exclude_route` uses original default path when known.
- Some environments may need policy routing for strict interface binding.

## Current limitations

- UDP over SOCKS is available, but app support varies.
- IPv6 inbound packet filtering is supported.
- IPv6 system-level interface binding is now supported on macOS and Linux.

## IPv6 support

SOCKS proxy now supports IPv6 with automatic IPv4 fallback:

- When you request IPv6 (e.g., `curl -6`), the client will attempt to connect via IPv6 first
- If the VPN provider doesn't support IPv6 and the connection fails, it automatically falls back to IPv4 for domain names
- **Recommended:** For best compatibility, omit the `-6` flag when using SOCKS to allow dual-stack resolution with IPv4 preference

Example:

```bash
# Automatic fallback (recommended):
curl --socks5 127.0.0.1:1080 https://ifconfig.co/country

# Force IPv6 (may use IPv4 fallback if provider doesn't support IPv6):
curl --socks5 127.0.0.1:1080 -6 https://ifconfig.co/country
```
