package main

import (
	"strings"
	"time"

	"github.com/docopt/docopt-go"
)

// LocationConfig holds provider location selection options.
type LocationConfig struct {
	LocationID      string
	LocationGroupID string
	LocationQuery   string
}

// VPNConfig holds all configuration for cmdVpn and vpnRunCore.
type VPNConfig struct {
	APIURL              string
	ConnectURL          string
	TunName             string
	IPCIDR              string
	MTU                 int
	DefaultRoute        bool
	ExtraRoutes         string
	ExcludeRoutes       string
	DNSList             string
	DNSService          string
	DNSBootstrap        string
	SOCKSListen         string
	AllowDomains        []string
	ExcludeDomains      []string
	AllowInboundSrcList string
	AllowInboundLocal   bool
	Debug               bool
	StatsInterval       time.Duration
	JWT                 string
	Location            LocationConfig
}

// SOCKSConfig holds all configuration for the standalone socks subcommand.
type SOCKSConfig struct {
	ListenAddr     string
	ExtenderIP     string
	ExtenderPort   string
	ExtenderSNI    string
	ExtenderSecret string
	AllowDomains   []string
	ExcludeDomains []string
	Debug          bool
}

// parseLocationConfig extracts location-related CLI flags into a LocationConfig.
func parseLocationConfig(opts docopt.Opts) LocationConfig {
	return LocationConfig{
		LocationID:      strings.TrimSpace(getStringOr(opts, "--location_id", "")),
		LocationGroupID: strings.TrimSpace(getStringOr(opts, "--location_group_id", "")),
		LocationQuery:   strings.TrimSpace(getStringOr(opts, "--location_query", "")),
	}
}

// parseVPNConfig extracts the full VPN configuration from parsed docopt options and a
// pre-resolved JWT string. This is the only place docopt.Opts is read for VPN settings.
func parseVPNConfig(opts docopt.Opts, jwt string) VPNConfig {
	defRoute, _ := opts.Bool("--default_route")
	dbg, _ := opts.Bool("--debug")
	allowLocal, _ := opts.Bool("--allow_inbound_local")
	socksListen := strings.TrimSpace(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	return VPNConfig{
		APIURL:              getStringOr(opts, "--api_url", DefaultAPIURL),
		ConnectURL:          getStringOr(opts, "--connect_url", DefaultConnectURL),
		TunName:             getStringOr(opts, "--tun", ""),
		IPCIDR:              getStringOr(opts, "--ip_cidr", "10.255.0.2/24"),
		MTU:                 getIntOr(opts, "--mtu", 1420),
		DefaultRoute:        defRoute,
		ExtraRoutes:         getStringOr(opts, "--route", ""),
		ExcludeRoutes:       getStringOr(opts, "--exclude_route", ""),
		DNSList:             getStringOr(opts, "--dns", ""),
		DNSService:          strings.TrimSpace(getStringOr(opts, "--dns_service", "")),
		DNSBootstrap:        strings.TrimSpace(getStringOr(opts, "--dns_bootstrap", "bypass")),
		SOCKSListen:         socksListen,
		AllowDomains:        splitCSV(getStringOr(opts, "--domain", "")),
		ExcludeDomains:      splitCSV(getStringOr(opts, "--exclude_domain", "")),
		AllowInboundSrcList: strings.TrimSpace(getStringOr(opts, "--allow_inbound_src", "")),
		AllowInboundLocal:   allowLocal,
		Debug:               dbg,
		StatsInterval:       time.Duration(getIntOr(opts, "--stats_interval", 5)) * time.Second,
		JWT:                 jwt,
		Location:            parseLocationConfig(opts),
	}
}

// parseSOCKSConfig extracts SOCKS proxy configuration from parsed docopt options.
func parseSOCKSConfig(opts docopt.Opts) SOCKSConfig {
	dbg, _ := opts.Bool("--debug")
	listen, _ := opts.String("--listen")
	extIP, _ := opts.String("--extender_ip")
	extPort, _ := opts.String("--extender_port")
	extSNI, _ := opts.String("--extender_sni")
	extSec, _ := opts.String("--extender_secret")
	return SOCKSConfig{
		ListenAddr:     strings.TrimSpace(listen),
		ExtenderIP:     strings.TrimSpace(extIP),
		ExtenderPort:   strings.TrimSpace(extPort),
		ExtenderSNI:    strings.TrimSpace(extSNI),
		ExtenderSecret: strings.TrimSpace(extSec),
		AllowDomains:   splitCSV(getStringOr(opts, "--domain", "")),
		ExcludeDomains: splitCSV(getStringOr(opts, "--exclude_domain", "")),
		Debug:          dbg,
	}
}
