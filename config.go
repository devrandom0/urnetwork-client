package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"gopkg.in/yaml.v3"
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
	EnableIPv6          bool
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
	enableIPv6, _ := opts.Bool("--enable_ipv6")
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
		EnableIPv6:          enableIPv6,
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

// ---------------------------------------------------------------------------
// Config file (--config / URNETWORK_CONFIG)
// ---------------------------------------------------------------------------

// ConfigFile holds all fields that can be set via a YAML config file.
// CLI flags take precedence over config file values; the file only supplies
// defaults for fields that are empty/zero after CLI parsing.
//
// Example (~/.urnetwork/config.yaml):
//
//	api_url: https://api.bringyour.com
//	connect_url: wss://connect.bringyour.com
//	tun: utun10
//	ip_cidr: 10.255.0.2/24
//	mtu: 1420
//	default_route: false
//	dns:
//	  - "1.1.1.1"
//	  - "8.8.8.8"
//	dns_service: "Wi-Fi"
//	dns_bootstrap: bypass
//	location_query: "country:Germany"
//	log_level: info
//	stats_interval: 5
type ConfigFile struct {
	APIURL            string   `yaml:"api_url"`
	ConnectURL        string   `yaml:"connect_url"`
	TunName           string   `yaml:"tun"`
	IPCIDR            string   `yaml:"ip_cidr"`
	MTU               int      `yaml:"mtu"`
	DefaultRoute      bool     `yaml:"default_route"`
	ExtraRoutes       string   `yaml:"route"`
	ExcludeRoutes     string   `yaml:"exclude_route"`
	DNS               []string `yaml:"dns"`
	DNSService        string   `yaml:"dns_service"`
	DNSBootstrap      string   `yaml:"dns_bootstrap"`
	SOCKSListen       string   `yaml:"socks"`
	AllowDomains      []string `yaml:"domain"`
	ExcludeDomains    []string `yaml:"exclude_domain"`
	AllowInboundSrc   string   `yaml:"allow_inbound_src"`
	AllowInboundLocal bool     `yaml:"allow_inbound_local"`
	LocationQuery     string   `yaml:"location_query"`
	LocationID        string   `yaml:"location_id"`
	LocationGroupID   string   `yaml:"location_group_id"`
	LogLevel          string   `yaml:"log_level"`
	StatsInterval     int      `yaml:"stats_interval"`
	Debug             bool     `yaml:"debug"`
}

// loadConfigFile reads and parses a YAML config file from path.
// Returns an empty ConfigFile (no error) when path is "".
func loadConfigFile(path string) (ConfigFile, error) {
	if strings.TrimSpace(path) == "" {
		return ConfigFile{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, fmt.Errorf("config file: %w", err)
	}
	var cf ConfigFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return ConfigFile{}, fmt.Errorf("config file parse: %w", err)
	}
	return cf, nil
}

// applyConfigFile merges cf into cfg: any cfg field that is zero/empty is
// filled from cf. CLI-parsed values (non-zero) are never overwritten.
func applyConfigFile(cfg VPNConfig, cf ConfigFile) VPNConfig {
	if cfg.APIURL == "" || cfg.APIURL == DefaultAPIURL {
		if cf.APIURL != "" {
			cfg.APIURL = cf.APIURL
		}
	}
	if cfg.ConnectURL == "" || cfg.ConnectURL == DefaultConnectURL {
		if cf.ConnectURL != "" {
			cfg.ConnectURL = cf.ConnectURL
		}
	}
	if cfg.TunName == "" && cf.TunName != "" {
		cfg.TunName = cf.TunName
	}
	if cfg.IPCIDR == "10.255.0.2/24" && cf.IPCIDR != "" {
		cfg.IPCIDR = cf.IPCIDR
	}
	if cfg.MTU == 1420 && cf.MTU != 0 {
		cfg.MTU = cf.MTU
	}
	if !cfg.DefaultRoute && cf.DefaultRoute {
		cfg.DefaultRoute = true
	}
	if cfg.ExtraRoutes == "" && cf.ExtraRoutes != "" {
		cfg.ExtraRoutes = cf.ExtraRoutes
	}
	if cfg.ExcludeRoutes == "" && cf.ExcludeRoutes != "" {
		cfg.ExcludeRoutes = cf.ExcludeRoutes
	}
	if cfg.DNSList == "" && len(cf.DNS) > 0 {
		cfg.DNSList = strings.Join(cf.DNS, ",")
	}
	if cfg.DNSService == "" && cf.DNSService != "" {
		cfg.DNSService = cf.DNSService
	}
	if (cfg.DNSBootstrap == "" || cfg.DNSBootstrap == "bypass") && cf.DNSBootstrap != "" {
		cfg.DNSBootstrap = cf.DNSBootstrap
	}
	if cfg.SOCKSListen == "" && cf.SOCKSListen != "" {
		cfg.SOCKSListen = cf.SOCKSListen
	}
	if len(cfg.AllowDomains) == 0 && len(cf.AllowDomains) > 0 {
		cfg.AllowDomains = cf.AllowDomains
	}
	if len(cfg.ExcludeDomains) == 0 && len(cf.ExcludeDomains) > 0 {
		cfg.ExcludeDomains = cf.ExcludeDomains
	}
	if cfg.AllowInboundSrcList == "" && cf.AllowInboundSrc != "" {
		cfg.AllowInboundSrcList = cf.AllowInboundSrc
	}
	if !cfg.AllowInboundLocal && cf.AllowInboundLocal {
		cfg.AllowInboundLocal = true
	}
	if cfg.Location.LocationQuery == "" && cf.LocationQuery != "" {
		cfg.Location.LocationQuery = cf.LocationQuery
	}
	if cfg.Location.LocationID == "" && cf.LocationID != "" {
		cfg.Location.LocationID = cf.LocationID
	}
	if cfg.Location.LocationGroupID == "" && cf.LocationGroupID != "" {
		cfg.Location.LocationGroupID = cf.LocationGroupID
	}
	if !cfg.Debug && cf.Debug {
		cfg.Debug = true
	}
	if cfg.StatsInterval == 5*time.Second && cf.StatsInterval > 0 {
		cfg.StatsInterval = time.Duration(cf.StatsInterval) * time.Second
	}
	return cfg
}
