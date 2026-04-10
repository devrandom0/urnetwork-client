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

## Kill switch

Use `--kill_switch` with `--default_route` to prevent IP leaks when the VPN drops:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --kill_switch \
  --location_query="country:Germany"
```

How it works:
- On startup, a **blackhole default route** is installed before the VPN routes
- While VPN is running, the more-specific `/1` split routes win → traffic goes through VPN
- If the VPN drops or the TUN goes down, the split routes disappear and the blackhole catches **all** remaining traffic → no leaks
- On graceful VPN exit: blackhole is preserved, all traffic blocked until you manually restore

To restore connectivity after kill switch activates:

```bash
# macOS
sudo route delete default
sudo route add default <your-router-ip>   # e.g. 192.168.1.1

# Linux
sudo ip route del blackhole default
sudo ip route add default via <your-router-ip> dev <interface>  # e.g. eth0
```

**Requires `--default_route`**. Kill switch has no effect in SOCKS-only mode.

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
