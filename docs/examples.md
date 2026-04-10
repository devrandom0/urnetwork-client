# Examples

## Full tunnel by location query

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_query="country:Germany"
```

## Full tunnel with excludes

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --exclude_route=10.0.0.0/8,169.254.0.0/16
```

## SOCKS proxy (bound to VPN)

To use a SOCKS proxy with VPN routing, specify both `--tun` and `--socks`. SOCKS connections will be bound to the VPN interface:

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --socks=127.0.0.1:1080 \
  --location_query="country:Germany"
```

Then connect through it:

```bash
curl --socks5 127.0.0.1:1080 https://example.com
```

**Note:** Without `--tun`, SOCKS connections use the system's default routes. For a standalone SOCKS proxy without VPN, use the separate `socks` subcommand (see below).

## DNS over VPN

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --dns=1.1.1.1,1.0.0.1 \
  --dns_service="Wi-Fi" \
  --dns_bootstrap=cache
```

## Inbound allowlist

```bash
sudo ./urnet-client vpn --tun utun10 --default_route --allow_inbound_local

sudo ./urnet-client vpn --tun utun10 --default_route \
  --allow_inbound_src=192.168.1.50/32,10.0.0.0/8
```

## Background mode

```bash
sudo ./urnet-client vpn \
  --tun utun10 \
  --default_route \
  --background \
  --log_file=/tmp/urnet-client.log
```

## Standalone SOCKS subcommand

```bash
./urnet-client socks \
  --listen=0.0.0.0:1080 \
  --extender_ip=<IP> \
  --extender_port=443 \
  --extender_sni=<hostname>
```
