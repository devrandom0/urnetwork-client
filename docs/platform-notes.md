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

## IPv6 support

IPv6 is **disabled by default** since many VPN providers don't support it yet.

To enable IPv6 routing through the VPN, use the `--enable_ipv6` flag:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --enable_ipv6 \
  --location_query="country:Germany"
```

When IPv6 is disabled (default):
- IPv6 packets from the TUN are silently dropped
- IPv6 clients will timeout or fail, encouraging them to retry with IPv4
- All traffic continues to work with IPv4-only

When IPv6 is enabled:
- Both IPv4 and IPv6 traffic route through the VPN
- **Only works if your VPN provider supports IPv6**
