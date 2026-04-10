# Quick Start

## 1) Build

```bash
go build -o dist/urnet-client ./
```

## 2) Login

```bash
./dist/urnet-client login --user_auth me@example.com --password 'secret'
```

If verification is required:

```bash
./dist/urnet-client verify --user_auth me@example.com --code 123456
```

## 3) Start VPN

macOS example:

```bash
sudo ./dist/urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_query="country:Germany"
```

Linux example:

```bash
sudo ./dist/urnet-client vpn \
  --tun urnet0 \
  --default_route \
  --location_query="country:Germany"
```

## One-command flow

```bash
sudo ./dist/urnet-client quick-connect \
  --user_auth me@example.com \
  --password 'secret' \
  --default_route \
  --location_query="country:Germany" \
  --tun utun10
```
