package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docopt/docopt-go"
)

const DefaultAPIURL = "https://api.bringyour.com"
const DefaultConnectURL = "wss://connect.bringyour.com"

const Version = "0.1.0"

func main() {
	usage := fmt.Sprintf(`urnet-client (experimental)

Usage:
    urnet-client login --user_auth=<user_auth> --password=<password> [--api_url=<api_url>]
    urnet-client verify --user_auth=<user_auth> --code=<code> [--api_url=<api_url>]
    urnet-client save-jwt --jwt=<jwt>
    urnet-client mint-client [--api_url=<api_url>] [--jwt=<jwt>]
	urnet-client quick-connect [--user_auth=<user_auth> --password=<password> [--code=<code>] | --jwt=<jwt>] [--api_url=<api_url>] [--connect_url=<connect_url>] [--tun=<name>] [--ip_cidr=<cidr>] [--mtu=<mtu>] [--default_route] [--route=<list>] [--exclude_route=<list>] [--domain=<list>] [--exclude_domain=<list>] [--dns=<list>] [--dns_service=<name>] [--dns_bootstrap=<mode>] [--location_query=<q>] [--location_id=<id>] [--location_group_id=<id>] [--socks=<addr>] [--socks_listen=<addr>] [--allow_inbound_src=<list>] [--allow_inbound_local] [--background] [--log_file=<path>] [--log_level=<level>] [--debug] [--stats_interval=<sec>] [--force_jwt] [--jwt_renew_interval=<dur>]
    urnet-client socks --listen=<addr> --extender_ip=<ip> --extender_port=<port> --extender_sni=<sni> [--extender_secret=<secret>] [--domain=<list>] [--exclude_domain=<list>] [--debug]
    urnet-client find-providers [--count=<count>] [--rank_mode=<rank_mode>] [--api_url=<api_url>] [--jwt=<jwt>]
    urnet-client open [--transports=<n>] [--connect_url=<connect_url>] [--api_url=<api_url>] [--jwt=<jwt>]
    urnet-client locations [--query=<q>] [--api_url=<api_url>] [--jwt=<jwt>]
			urnet-client vpn [--tun=<name>] [--connect_url=<connect_url>] [--api_url=<api_url>] [--jwt=<jwt>] [--ip_cidr=<cidr>] [--mtu=<mtu>] [--default_route] [--route=<list>] [--exclude_route=<list>] [--domain=<list>] [--exclude_domain=<list>] [--dns=<list>] [--dns_service=<name>] [--dns_bootstrap=<mode>] [--location_query=<q>] [--location_id=<id>] [--location_group_id=<id>] [--socks=<addr>] [--socks_listen=<addr>] [--allow_inbound_src=<list>] [--allow_inbound_local] [--background] [--log_file=<path>] [--log_level=<level>] [--debug] [--stats_interval=<sec>]

Options:
    --api_url=<api_url>          API base URL [default: %s]
    --connect_url=<connect_url>  Connect URL (WS) [default: %s]
    --count=<count>              Number of providers to return [default: 8]
    --rank_mode=<rank_mode>      quality|speed [default: quality]
    --transports=<n>             Number of transports to open [default: 4]
    --jwt=<jwt>                  BY network JWT (falls back to ~/.urnetwork/jwt)
	--tun=<name>                 TUN interface name (omit or use 'none' to disable; SOCKS-only)
        --ip_cidr=<cidr>             Assign CIDR to TUN [default: 10.255.0.2/24]
    --mtu=<mtu>                  Set MTU on TUN [default: 1420]
    --default_route              Route all traffic via TUN (disabled by default)
    --route=<list>               Comma-separated extra routes (IP or CIDR) via TUN
        --exclude_route=<list>       Comma-separated routes to keep off the TUN when --default_route is set
    --dns=<list>                 Comma-separated DNS servers to prefer while VPN is up
        --dns_service=<name>         macOS only: Network Service name to modify DNS (e.g., "Wi-Fi"); optional
    --dns_bootstrap=<mode>       How to keep DNS working during default-route switch: bypass|cache|none [default: bypass]
    --location_query=<q>         Search for locations (e.g., "country:Germany" or "region:Europe") to select providers
    --location_id=<id>           Select providers in a specific location id (use with find-locations)
    --location_group_id=<id>     Select providers in a specific location group id
    --domain=<list>              Comma-separated domains that should go via VPN (SOCKS only). If set, non-matching domains bypass VPN
    --exclude_domain=<list>      Comma-separated domains that should bypass VPN (SOCKS only)
    --listen=<addr>              socks: listen address (e.g., 0.0.0.0:1080)
    --extender_ip=<ip>           socks: extender IP to connect through
    --extender_port=<port>       socks: extender TCP port (e.g., 443)
    --extender_sni=<sni>         socks: TLS SNI for extender (domain-like)
    --extender_secret=<secret>   socks: optional pre-shared secret for extender auth
    --socks=<addr>               Start a SOCKS5 proxy (e.g., 127.0.0.1:1080) and bind traffic to the VPN
    --socks_listen=<addr>        Alias for --socks
	--allow_inbound_src=<list>   Comma-separated CIDRs to allow for new inbound connections (e.g., 10.0.0.0/8,192.168.0.0/16)
	--allow_inbound_local        Also allow from local ranges (RFC1918, loopback, CGNAT, link-local) and the TUN subnet from --ip_cidr
    --background                 Run vpn in the background and print the child process id
    --log_file=<path>            If set, write logs to this file (default: console)
    --log_level=<level>          quiet|error|warn|info|debug (default: info). --debug implies debug unless a level is set
    --debug                      Verbose per-packet logs for vpn
    --stats_interval=<sec>       Interval (seconds) to print vpn counters [default: 5]
    --force_jwt                  quick-connect: force mint a fresh client JWT even if one exists
    --jwt_renew_interval=<dur>   quick-connect: periodically renew client JWT while running (e.g., 12h, 30m); 0 disables
    --user_auth=<user_auth>      Email or phone
    --password=<password>        Password
    --code=<code>                Verification code
    -h --help                    Show help
    --version                    Show version
`, DefaultAPIURL, DefaultConnectURL)

	opts, err := docopt.ParseArgs(usage, os.Args[1:], Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if v, _ := opts.Bool("--version"); v {
		fmt.Println(Version)
		return
	}

	// Set up log file and level for commands that support it.
	if logPath := strings.TrimSpace(getStringOr(opts, "--log_file", "")); logPath != "" {
		if err := setupLogFile(logPath); err != nil {
			fmt.Fprintf(os.Stderr, "log setup failed: %v\n", err)
			os.Exit(1)
		}
	}
	lvl := strings.TrimSpace(getStringOr(opts, "--log_level", ""))
	dbg, _ := opts.Bool("--debug")
	setLogLevel(lvl, dbg)

	// Handle --background for commands that support it before creating context.
	if bg, _ := opts.Bool("--background"); bg {
		if mustBool(opts, "quick-connect") || mustBool(opts, "vpn") {
			pid, err := spawnBackground(os.Args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "background start failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("started in background pid=%d\n", pid)
			return
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runErr error
	switch {
	case mustBool(opts, "login"):
		runErr = cmdLogin(ctx, opts)
	case mustBool(opts, "verify"):
		runErr = cmdVerify(ctx, opts)
	case mustBool(opts, "save-jwt"):
		runErr = cmdSaveJWT(opts)
	case mustBool(opts, "mint-client"):
		runErr = cmdMintClient(ctx, opts)
	case mustBool(opts, "quick-connect"):
		runErr = cmdQuickConnect(ctx, opts)
	case mustBool(opts, "find-providers"):
		runErr = cmdFindProviders(ctx, opts)
	case mustBool(opts, "open"):
		runErr = cmdOpen(ctx, opts)
	case mustBool(opts, "locations"):
		runErr = cmdLocations(ctx, opts)
	case mustBool(opts, "socks"):
		runErr = cmdSocks(ctx, opts)
	case mustBool(opts, "vpn"):
		jwt, _ := loadJWT(getStringOr(opts, "--jwt", ""))
		cfg := parseVPNConfig(opts, jwt)
		runErr = cmdVpn(ctx, cfg)
	default:
		fmt.Println(usage)
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
		os.Exit(1)
	}
}
