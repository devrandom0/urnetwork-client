# Platform Notes

## macOS

- Requires `sudo` for TUN, routes, and DNS changes.
- If requested utun name fails, client retries with auto-assigned utun.
- Full tunnel uses split defaults (`0.0.0.0/1` and `128.0.0.0/1`).
- Exclude routes are sent to current default gateway.
- With `--default_route`, DNS goes to the client's stub through a `scutil` dynamic-store entry (`State:/Network/Service/urnet-client`); saved network settings are not touched.

## Linux

- Requires `sudo` and TUN support.
- Full tunnel also uses split defaults.
- Control-plane endpoints get temporary host routes via current default gateway.
- `--exclude_route` uses original default path when known.
- Some environments may need policy routing for strict interface binding.
- With `--default_route`, DNS goes to the client's stub through systemd-resolved when `/etc/resolv.conf` is its stub file, otherwise by rewriting `/etc/resolv.conf` (backup in `/etc/resolv.conf.urnet-client.bak`).

## Current limitations

- UDP over SOCKS is available, but app support varies.
- If the TUN address, MTU or link-up step fails, the client exits with an error before touching any route. The IPv6 ULA address is only required when `--enable_ipv6` is set.
- If a kill switch route (IPv4 or IPv6) cannot be installed, the client exits with an error before the VPN starts.

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
sudo route delete -inet6 default   # then turn Wi-Fi or the interface off and on

# Linux
sudo ip route del blackhole default
sudo ip route add default via <your-router-ip> dev <interface>  # e.g. eth0
sudo ip -6 route del blackhole default metric 1
```

**Requires `--default_route`**. Kill switch has no effect in SOCKS-only mode.

## IPv6 support

IPv6 is off by default. With `--default_route` it is blocked, not leaked: reject routes for `::/1` and `8000::/1` make IPv6 connections fail at once, AAAA lookups return nothing and apps use IPv4. See "DNS and IPv6" in configuration.md.

To route IPv6 through the VPN (only if your provider supports it):

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --enable_ipv6 \
  --location_query="country:Germany"
```

## Crash recovery

If the client is killed (for example `kill -9` or a power cut) it cannot undo its changes. DNS then fails instead of leaking, and IPv6 stays blocked. The next start of the client cleans DNS up by itself. To restore by hand:

On macOS this fails safe for IPv6 but not for DNS: if the client crashes while its `scutil` entry is still published and the stub is no longer answering, macOS falls back to querying the primary LAN resolver until the entry is removed, which leaks DNS until the next start (or the manual removal below). Closing this fully needs a packet-filter firewall kill switch (deferred).

macOS:

```bash
printf 'remove State:/Network/Service/urnet-client/DNS\nremove State:/Network/Service/urnet-client/IPv4\n' | sudo scutil
sudo route -n delete -inet6 ::/1
sudo route -n delete -inet6 8000::/1
```

Linux:

```bash
sudo cp /etc/resolv.conf.urnet-client.bak /etc/resolv.conf && sudo rm /etc/resolv.conf.urnet-client.bak
sudo ip -6 route del unreachable ::/1
sudo ip -6 route del unreachable 8000::/1
```

With systemd-resolved nothing is needed for DNS: the settings disappear with the TUN interface. With `--enable_ipv6` the IPv6 routes also disappear with the TUN.

## Privileged tools

`route`, `ifconfig`, `scutil` (macOS), `ip` and `resolvectl` (Linux) are run by absolute path. Each is looked up once, in this order, and `PATH` is never used: `/sbin`, `/usr/sbin`, `/bin`, `/usr/bin`, and on Linux also `/run/current-system/sw/bin` (NixOS). A candidate is used only if the file, its directory and, for a symlink, the target's directory are owned by root and not group/world-writable; otherwise the next directory is tried. If no directory has a usable tool the command fails with `<tool> not found in ...`.
