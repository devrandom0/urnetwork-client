# urnet-client

[![CI](https://github.com/devrandom0/urnetwork-client/actions/workflows/ci.yml/badge.svg)](https://github.com/devrandom0/urnetwork-client/actions/workflows/ci.yml)

A minimal CLI for URnetwork (BringYour).

## Quick Start

1. Build:

```bash
go build -o dist/urnet-client ./
```

2. Login:

```bash
./dist/urnet-client login --user_auth me@example.com --password 'secret'
```

3. Verify (if required):

```bash
./dist/urnet-client verify --user_auth me@example.com --code 123456
```

4. Start VPN:

```bash
sudo ./dist/urnet-client vpn \
  --tun utun10 \
  --default_route \
  --location_query="country:Germany"
```

Or run all in one command:

```bash
sudo ./dist/urnet-client quick-connect \
  --user_auth me@example.com \
  --password 'secret' \
  --default_route \
  --location_query="country:Germany" \
  --tun utun10
```

## Commands

- `login`, `verify`, `save-jwt`, `mint-client`
- `quick-connect`, `vpn`, `socks`
- `find-providers`, `locations`, `open`

Run `./dist/urnet-client --help` for full command help.

## Common Defaults

- API URL: `https://api.bringyour.com`
- Connect URL: `wss://connect.bringyour.com`
- JWT path: `~/.urnetwork/jwt`

## Docs

Detailed documentation is in [docs/README.md](docs/README.md):

- [Quick Start](docs/quick-start.md)
- [Configuration](docs/configuration.md)
- [Command Reference](docs/command-reference.md)
- [Examples](docs/examples.md)
- [Docker Guide](docs/docker.md)
- [Platform Notes](docs/platform-notes.md)

## Security note

Prefer `URNETWORK_PASSWORD` over passing `--password` in command arguments, since args may appear in shell history and process listings.

## Support

If you liked this project, please use [this referral link](https://ur.io/app?bonus=4MT0ZB).
