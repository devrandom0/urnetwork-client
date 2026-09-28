// Package config parses, loads and validates the CLI's configuration.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"gopkg.in/yaml.v3"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/socks"
)

const DefaultAPIURL = "https://api.bringyour.com"
const DefaultConnectURL = "wss://connect.bringyour.com"

// LocationConfig holds provider location selection options.
type LocationConfig struct {
	LocationID      string
	LocationGroupID string
	LocationQuery   string
}

// VPNConfig holds all configuration for session.Run and session.runCore.
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
	SOCKSAuth           socks.SocksAuth
	AllowDomains        []string
	ExcludeDomains      []string
	AllowInboundSrcList string
	AllowInboundLocal   bool
	EnableIPv6          bool
	EnableKillSwitch    bool
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
	Auth           socks.SocksAuth
	AllowDomains   []string
	ExcludeDomains []string
	Debug          bool
}

// ParseLocationConfig extracts location-related CLI flags into a LocationConfig.
func ParseLocationConfig(opts docopt.Opts) LocationConfig {
	return LocationConfig{
		LocationID:      strings.TrimSpace(StringOr(opts, "--location_id", "")),
		LocationGroupID: strings.TrimSpace(StringOr(opts, "--location_group_id", "")),
		LocationQuery:   strings.TrimSpace(StringOr(opts, "--location_query", "")),
	}
}

// ParseVPNConfig extracts the full VPN configuration from parsed docopt options and a
// pre-resolved JWT string. This is the only place docopt.Opts is read for VPN settings.
func ParseVPNConfig(opts docopt.Opts, jwt string) VPNConfig {
	defRoute, _ := opts.Bool("--default_route")
	dbg, _ := opts.Bool("--debug")
	allowLocal, _ := opts.Bool("--allow_inbound_local")
	enableIPv6, _ := opts.Bool("--enable_ipv6")
	killSwitch, _ := opts.Bool("--kill_switch")
	socksListen := strings.TrimSpace(StringOr(opts, "--socks", StringOr(opts, "--socks_listen", "")))
	return VPNConfig{
		APIURL:              StringOr(opts, "--api_url", DefaultAPIURL),
		ConnectURL:          StringOr(opts, "--connect_url", DefaultConnectURL),
		TunName:             StringOr(opts, "--tun", ""),
		IPCIDR:              StringOr(opts, "--ip_cidr", "10.255.0.2/24"),
		MTU:                 IntOr(opts, "--mtu", 1420),
		DefaultRoute:        defRoute,
		ExtraRoutes:         StringOr(opts, "--route", ""),
		ExcludeRoutes:       StringOr(opts, "--exclude_route", ""),
		DNSList:             StringOr(opts, "--dns", ""),
		DNSService:          strings.TrimSpace(StringOr(opts, "--dns_service", "")),
		DNSBootstrap:        strings.TrimSpace(StringOr(opts, "--dns_bootstrap", "bypass")),
		SOCKSListen:         socksListen,
		SOCKSAuth:           socks.ResolveSocksAuth(StringOr(opts, "--socks_user", ""), StringOr(opts, "--socks_pass", ""), os.Getenv),
		AllowDomains:        SplitCSV(StringOr(opts, "--domain", "")),
		ExcludeDomains:      SplitCSV(StringOr(opts, "--exclude_domain", "")),
		AllowInboundSrcList: strings.TrimSpace(StringOr(opts, "--allow_inbound_src", "")),
		AllowInboundLocal:   allowLocal,
		EnableIPv6:          enableIPv6,
		EnableKillSwitch:    killSwitch,
		Debug:               dbg,
		StatsInterval:       time.Duration(IntOr(opts, "--stats_interval", 5)) * time.Second,
		JWT:                 jwt,
		Location:            ParseLocationConfig(opts),
	}
}

// ParseSOCKSConfig extracts SOCKS proxy configuration from parsed docopt options.
func ParseSOCKSConfig(opts docopt.Opts) SOCKSConfig {
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
		Auth:           socks.ResolveSocksAuth(StringOr(opts, "--socks_user", ""), StringOr(opts, "--socks_pass", ""), os.Getenv),
		AllowDomains:   SplitCSV(StringOr(opts, "--domain", "")),
		ExcludeDomains: SplitCSV(StringOr(opts, "--exclude_domain", "")),
		Debug:          dbg,
	}
}

// ---------------------------------------------------------------------------
// Config file (--config)
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
	SOCKSListenAlias  string   `yaml:"socks_listen"` // older docs used this key; "socks" wins when both are set
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
	if cf.SOCKSListen == "" {
		cf.SOCKSListen = cf.SOCKSListenAlias
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

// resolveLogLevel applies precedence: --log_level, then --debug, then the config file.
func resolveLogLevel(flagLevel string, flagDebug bool, fileLevel string, fileDebug bool) (string, bool) {
	if strings.TrimSpace(flagLevel) != "" {
		return flagLevel, flagDebug
	}
	if flagDebug {
		return "debug", true
	}
	return fileLevel, fileDebug
}

// ResolveVPNConfig is the single entry point for vpn and quick-connect: CLI flags, then
// the --config file, then defaults. It applies the effective log level and leaves JWT empty.
func ResolveVPNConfig(opts docopt.Opts) (VPNConfig, error) {
	cfg := ParseVPNConfig(opts, "")
	cf, err := loadConfigFile(StringOr(opts, "--config", ""))
	if err != nil {
		return VPNConfig{}, err
	}
	cfg = applyConfigFile(cfg, cf)
	if err := ValidateEndpointURL("api_url", cfg.APIURL, "https", "http"); err != nil {
		return VPNConfig{}, err
	}
	if err := ValidateEndpointURL("connect_url", cfg.ConnectURL, "wss", "ws"); err != nil {
		return VPNConfig{}, err
	}
	if cfg.SOCKSListen != "" {
		if err := cfg.SOCKSAuth.Validate(); err != nil {
			return VPNConfig{}, fmt.Errorf("socks: %w", err)
		}
	}
	logx.SetLogLevel(resolveLogLevel(StringOr(opts, "--log_level", ""), MustBool(opts, "--debug"), cf.LogLevel, cf.Debug))
	return cfg, nil
}
