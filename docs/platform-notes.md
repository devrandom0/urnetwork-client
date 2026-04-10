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
