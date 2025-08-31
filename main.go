package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/urnetwork/connect"
)

const DefaultApiUrl = "https://api.bringyour.com"
const DefaultConnectUrl = "wss://connect.bringyour.com"

const Version = "0.1.0"

func main() {
	usage := fmt.Sprintf(`urnet-client (experimental)

Usage:
    urnet-client login --user_auth=<user_auth> --password=<password> [--api_url=<api_url>]
    urnet-client verify --user_auth=<user_auth> --code=<code> [--api_url=<api_url>]
    urnet-client save-jwt --jwt=<jwt>
    urnet-client mint-client [--api_url=<api_url>] [--jwt=<jwt>]
	urnet-client quick-connect [--user_auth=<user_auth> --password=<password> [--code=<code>] | --jwt=<jwt>] [--api_url=<api_url>] [--connect_url=<connect_url>] [--tun=<name>] [--ip_cidr=<cidr>] [--mtu=<mtu>] [--default_route] [--route=<list>] [--exclude_route=<list>] [--domain=<list>] [--exclude_domain=<list>] [--dns=<list>] [--dns_service=<name>] [--dns_bootstrap=<mode>] [--location_query=<q>] [--location_id=<id>] [--location_group_id=<id>] [--socks=<addr>] [--socks_listen=<addr>] [--local_only] [--allow_forward_src=<list>] [--deny_forward_src=<list>] [--no_fw_rules] [--background] [--log_file=<path>] [--log_level=<level>] [--debug] [--stats_interval=<sec>] [--force_jwt] [--jwt_renew_interval=<dur>]
    urnet-client socks --listen=<addr> --extender_ip=<ip> --extender_port=<port> --extender_sni=<sni> [--extender_secret=<secret>] [--domain=<list>] [--exclude_domain=<list>] [--debug]
    urnet-client find-providers [--count=<count>] [--rank_mode=<rank_mode>] [--api_url=<api_url>] [--jwt=<jwt>]
    urnet-client open [--transports=<n>] [--connect_url=<connect_url>] [--api_url=<api_url>] [--jwt=<jwt>]
    urnet-client locations [--query=<q>] [--api_url=<api_url>] [--jwt=<jwt>]
			urnet-client vpn [--tun=<name>] [--connect_url=<connect_url>] [--api_url=<api_url>] [--jwt=<jwt>] [--ip_cidr=<cidr>] [--mtu=<mtu>] [--default_route] [--route=<list>] [--exclude_route=<list>] [--domain=<list>] [--exclude_domain=<list>] [--dns=<list>] [--dns_service=<name>] [--dns_bootstrap=<mode>] [--location_query=<q>] [--location_id=<id>] [--location_group_id=<id>] [--socks=<addr>] [--socks_listen=<addr>] [--local_only] [--allow_forward_src=<list>] [--deny_forward_src=<list>] [--no_fw_rules] [--background] [--log_file=<path>] [--log_level=<level>] [--debug] [--stats_interval=<sec>]

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
	--local_only                 Block acting as exit node: drop non-local TUN traffic and disable forwarding (best-effort)
	--allow_forward_src=<list>   Linux only: comma-separated source CIDRs to allow forwarding via the VPN (others dropped)
	--deny_forward_src=<list>    Linux only: comma-separated source CIDRs to block from forwarding via the VPN (e.g., 192.168.2.0/24)
	--no_fw_rules                Don’t modify iptables/route; enforce filters in userspace only
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
`, DefaultApiUrl, DefaultConnectUrl)

	opts, err := docopt.ParseArgs(usage, os.Args[1:], Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	if v, _ := opts.Bool("--version"); v {
		fmt.Println(Version)
		return
	}

	switch {
	case mustBool(opts, "login"):
		cmdLogin(opts)
	case mustBool(opts, "verify"):
		cmdVerify(opts)
	case mustBool(opts, "save-jwt"):
		cmdSaveJWT(opts)
	case mustBool(opts, "mint-client"):
		cmdMintClient(opts)
	case mustBool(opts, "quick-connect"):
		cmdQuickConnect(opts)
	case mustBool(opts, "find-providers"):
		cmdFindProviders(opts)
	case mustBool(opts, "open"):
		cmdOpen(opts)
	case mustBool(opts, "locations"):
		cmdLocations(opts)
	case mustBool(opts, "socks"):
		cmdSocks(opts)
	case mustBool(opts, "vpn"):
		// If requested, spawn a detached child and print its PID
		if bg, _ := opts.Bool("--background"); bg {
			pid, err := spawnBackground(os.Args)
			if err != nil {
				fatal(fmt.Errorf("background start failed: %w", err))
			}
			fmt.Printf("started in background pid=%d\n", pid)
			return
		}
		// Set up optional log file redirection
		if logPath := strings.TrimSpace(getStringOr(opts, "--log_file", "")); logPath != "" {
			if err := setupLogFile(logPath); err != nil {
				fatal(fmt.Errorf("log setup failed: %w", err))
			}
		}
		// Configure log level
		lvl := strings.TrimSpace(getStringOr(opts, "--log_level", ""))
		dbg, _ := opts.Bool("--debug")
		setLogLevel(lvl, dbg)
		cmdVpn(opts)
	default:
		fmt.Println(usage)
	}
}

func mustBool(opts docopt.Opts, key string) bool { b, _ := opts.Bool(key); return b }

func jwtPath() string {
	if base := strings.TrimSpace(os.Getenv("URNETWORK_HOME")); base != "" {
		return filepath.Join(base, "jwt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".urnetwork", "jwt")
}

func loadJWT(maybe string) (string, error) {
	if strings.TrimSpace(maybe) != "" {
		return strings.TrimSpace(maybe), nil
	}
	path := jwtPath()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no jwt provided and failed to read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func saveJWT(jwt string) error {
	path := jwtPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(jwt)+"\n"), 0o600)
}

func cmdSaveJWT(opts docopt.Opts) {
	jwt, _ := opts.String("--jwt")
	if strings.TrimSpace(jwt) == "" {
		fmt.Fprintln(os.Stderr, "--jwt is required")
		os.Exit(2)
	}
	if err := saveJWT(jwt); err != nil {
		fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("saved to %s\n", jwtPath())
}

// cmdQuickConnect performs: login (and optional verify) -> mint client JWT -> start vpn.
func cmdQuickConnect(opts docopt.Opts) {
	// If requested, spawn a detached child of quick-connect itself, so renewal can run alongside vpn
	if bg, _ := opts.Bool("--background"); bg {
		pid, err := spawnBackground(os.Args)
		if err != nil {
			fatal(fmt.Errorf("background start failed: %w", err))
		}
		fmt.Printf("started in background pid=%d\n", pid)
		return
	}

	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	// Configure logging early so login/mint messages also go to the target
	logPath := strings.TrimSpace(getStringOr(opts, "--log_file", ""))
	if logPath != "" {
		if err := setupLogFile(logPath); err != nil {
			fatal(fmt.Errorf("log setup failed: %w", err))
		}
	}
	lvl := strings.TrimSpace(getStringOr(opts, "--log_level", ""))
	dbg, _ := opts.Bool("--debug")
	setLogLevel(lvl, dbg)

	userAuth := strings.TrimSpace(getStringOr(opts, "--user_auth", ""))
	password := strings.TrimSpace(getStringOr(opts, "--password", ""))
	if userAuth == "" {
		userAuth = strings.TrimSpace(os.Getenv("URNETWORK_USERNAME"))
	}
	if password == "" {
		password = strings.TrimSpace(os.Getenv("URNETWORK_PASSWORD"))
	}
	codeOpt := strings.TrimSpace(getStringOr(opts, "--code", ""))
	jwtOpt, _ := opts.String("--jwt")
	forceJWT, _ := opts.Bool("--force_jwt")
	renewStr := strings.TrimSpace(getStringOr(opts, "--jwt_renew_interval", ""))
	var renewInterval time.Duration
	if renewStr != "" {
		if d, err := time.ParseDuration(renewStr); err == nil {
			renewInterval = d
		} else {
			logWarn("invalid --jwt_renew_interval=%q; ignoring\n", renewStr)
		}
	}
	// If not provided via flag, we will attempt to read from default path later

	// 1) Login if credentials provided
	if userAuth != "" || password != "" {
		if userAuth == "" || password == "" {
			fatal(errors.New("--user_auth and --password must be provided together"))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		strat := connect.NewClientStrategyWithDefaults(ctx)
		api := connect.NewBringYourApi(ctx, strat, apiUrl)

		done := make(chan struct{})
		api.AuthLoginWithPassword(&connect.AuthLoginWithPasswordArgs{UserAuth: userAuth, Password: password}, connect.NewApiCallback(func(res *connect.AuthLoginWithPasswordResult, err error) {
			defer close(done)
			if err != nil {
				logError("login error: %v\n", err)
				return
			}
			if res.Error != nil {
				logError("login error: %s\n", res.Error.Message)
				return
			}
			if res.VerificationRequired != nil {
				if codeOpt == "" {
					logError("verification required for %s (re-run with --code=<code> or run 'verify')\n", res.VerificationRequired.UserAuth)
					return
				}
			}
			if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
				logError("login succeeded but no by_jwt returned\n")
				return
			}
			if err := saveJWT(res.Network.ByJwt); err != nil {
				logError("save jwt failed: %v\n", err)
				return
			}
			logInfo("saved JWT for network %s -> %s\n", res.Network.NetworkName, jwtPath())
		}))
		<-done

		// If verification required and code was provided, verify now
		if codeOpt != "" {
			done2 := make(chan struct{})
			api2 := connect.NewBringYourApi(ctx, strat, apiUrl)
			api2.AuthVerify(&connect.AuthVerifyArgs{UserAuth: userAuth, VerifyCode: codeOpt}, connect.NewApiCallback(func(res *connect.AuthVerifyResult, err error) {
				defer close(done2)
				if err != nil {
					logError("verify error: %v\n", err)
					return
				}
				if res.Error != nil {
					logError("verify error: %s\n", res.Error.Message)
					return
				}
				if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
					logError("verify succeeded but no by_jwt returned\n")
					return
				}
				if err := saveJWT(res.Network.ByJwt); err != nil {
					logError("save jwt failed: %v\n", err)
					return
				}
				logInfo("verified and saved JWT -> %s\n", jwtPath())
			}))
			<-done2
		}
	}

	// 2) Ensure we have a working client-scoped JWT; if current JWT is network-scoped, mint a client JWT.
	//    If an existing client JWT is present but not working, try to fetch a fresh one and retry within an interval.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		strat := connect.NewClientStrategyWithDefaults(ctx)
		api := connect.NewBringYourApi(ctx, strat, apiUrl)
		// Load JWT from flag or saved file
		jwt, err := loadJWT(jwtOpt)
		if err != nil {
			fatal(errors.New("no JWT available; provide --user_auth/--password to login or --jwt to use an existing token"))
		}
		// If already client-scoped and not forced, validate and reuse if working; otherwise try to refresh.
	if id := parseClientID(jwt); id != "" && !forceJWT {
			if validateClientJWT(apiUrl, jwt) {
				api.SetByJwt(jwt)
		logInfo("using existing client JWT (client_id=%s)\n", id)
			} else {
				// Try to refresh by obtaining a new client token; prefer credentials when available
				retryEvery := renewInterval
				if retryEvery <= 0 {
					retryEvery = time.Minute
				}
				for {
					refreshed := false
					// Credentials path: login -> BY jwt -> mint client jwt
					if userAuth != "" && password != "" {
						ctx2, cancel2 := context.WithTimeout(context.Background(), 40*time.Second)
						strat2 := connect.NewClientStrategyWithDefaults(ctx2)
						api2 := connect.NewBringYourApi(ctx2, strat2, apiUrl)
						done := make(chan struct{})
						var byJwt string
						api2.AuthLoginWithPassword(&connect.AuthLoginWithPasswordArgs{UserAuth: userAuth, Password: password}, connect.NewApiCallback(func(lr *connect.AuthLoginWithPasswordResult, err error) {
							defer close(done)
							if err != nil || lr == nil || lr.Error != nil || lr.Network == nil || strings.TrimSpace(lr.Network.ByJwt) == "" {
								if err != nil {
									logWarn("jwt refresh: login failed: %v\n", err)
								} else if lr != nil && lr.Error != nil {
									logWarn("jwt refresh: login failed: %s\n", lr.Error.Message)
								} else {
									logWarn("jwt refresh: login failed\n")
								}
								return
							}
							byJwt = lr.Network.ByJwt
						}))
						<-done
						if strings.TrimSpace(byJwt) != "" {
							api2.SetByJwt(byJwt)
							if mres, merr := api2.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""}); merr == nil && mres != nil && mres.Error == nil && strings.TrimSpace(mres.ByClientJwt) != "" {
								if err := saveJWT(mres.ByClientJwt); err != nil {
									logWarn("jwt refresh: save failed: %v\n", err)
								} else {
									newJWT := mres.ByClientJwt
									if validateClientJWT(apiUrl, newJWT) {
										if id2 := parseClientID(newJWT); id2 != "" {
											_ = id2 // not used further; kept for potential future logging
										}
										logInfo("obtained new client JWT; proceeding\n")
										refreshed = true
									}
								}
							} else if merr != nil {
								logWarn("jwt refresh: mint failed: %v\n", merr)
							} else if mres != nil && mres.Error != nil {
								logWarn("jwt refresh: mint failed: %s\n", mres.Error.Message)
							}
						}
						cancel2()
					}
					if refreshed {
						break
					}
					if userAuth == "" || password == "" {
						fatal(errors.New("existing client JWT appears invalid; provide --user_auth and --password or a BY token via --jwt to refresh"))
					}
					logWarn("jwt still not usable; retrying in %s\n", retryEvery.String())
					time.Sleep(retryEvery)
				}
			}
		} else {
			// Either forced or we have a BY network-scoped token: mint client token now
			api.SetByJwt(jwt)
			res, err := api.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""})
			if err != nil {
				fatal(err)
			}
			if res.Error != nil {
				fatal(fmt.Errorf("auth-client error: %s", res.Error.Message))
			}
			if strings.TrimSpace(res.ByClientJwt) == "" {
				fatal(errors.New("auth-client succeeded but no by_client_jwt returned"))
			}
			if err := saveJWT(res.ByClientJwt); err != nil {
				fatal(err)
			}
			if id := parseClientID(res.ByClientJwt); id != "" {
				logInfo("saved client JWT (client_id=%s) -> %s\n", id, jwtPath())
			} else {
				logInfo("saved client JWT -> %s\n", jwtPath())
			}
		}
	}

	// 3) Optionally start a JWT renewal loop while VPN runs
	stopRenew := make(chan struct{})
	if renewInterval > 0 {
		go func() {
			ticker := time.NewTicker(renewInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Attempt to mint a fresh client JWT using current token; fallback to credentials if available
					// Load current JWT (may be client-scoped)
					currentJwt, err := loadJWT("")
					if err != nil {
						logWarn("jwt renew: no jwt available: %v\n", err)
						continue
					}
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					strat := connect.NewClientStrategyWithDefaults(ctx)
					api := connect.NewBringYourApi(ctx, strat, apiUrl)
					api.SetByJwt(currentJwt)
					res, err := api.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""})
					if err == nil && res != nil && res.Error == nil && strings.TrimSpace(res.ByClientJwt) != "" {
						if err := saveJWT(res.ByClientJwt); err != nil {
							logWarn("jwt renew: save failed: %v\n", err)
						} else {
							if id := parseClientID(res.ByClientJwt); id != "" {
								logInfo("jwt renewed (client_id=%s)\n", id)
							} else {
								logInfo("jwt renewed\n")
							}
						}
						cancel()
						continue
					}
					cancel()
					// Fallback: if we have credentials, attempt login -> mint
					if userAuth != "" && password != "" {
						ctx2, cancel2 := context.WithTimeout(context.Background(), 40*time.Second)
						strat2 := connect.NewClientStrategyWithDefaults(ctx2)
						api2 := connect.NewBringYourApi(ctx2, strat2, apiUrl)
						done := make(chan struct{})
						var byJwt string
						api2.AuthLoginWithPassword(&connect.AuthLoginWithPasswordArgs{UserAuth: userAuth, Password: password}, connect.NewApiCallback(func(lr *connect.AuthLoginWithPasswordResult, err error) {
							defer close(done)
							if err != nil || lr == nil || lr.Error != nil || lr.Network == nil || strings.TrimSpace(lr.Network.ByJwt) == "" {
								if err != nil {
									logWarn("jwt renew: login failed: %v\n", err)
								} else if lr != nil && lr.Error != nil {
									logWarn("jwt renew: login failed: %s\n", lr.Error.Message)
								} else {
									logWarn("jwt renew: login failed\n")
								}
								return
							}
							byJwt = lr.Network.ByJwt
						}))
						<-done
						if strings.TrimSpace(byJwt) != "" {
							api2.SetByJwt(byJwt)
							if mres, merr := api2.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""}); merr == nil && mres != nil && mres.Error == nil && strings.TrimSpace(mres.ByClientJwt) != "" {
								if err := saveJWT(mres.ByClientJwt); err != nil {
									logWarn("jwt renew: save failed: %v\n", err)
								} else {
									if id := parseClientID(mres.ByClientJwt); id != "" {
										logInfo("jwt renewed (client_id=%s)\n", id)
									} else {
										logInfo("jwt renewed\n")
									}
								}
							}
						}
						cancel2()
					}
				case <-stopRenew:
					return
				}
			}
		}()
	}

	// 4) Start VPN in the foreground of this process
	cmdVpn(opts)
	// When VPN exits, stop renewal if running
	close(stopRenew)
}

// buildVpnArgsFromOpts constructs argv to run the vpn subcommand with flags from opts.
func buildVpnArgsFromOpts(opts docopt.Opts, apiUrl, connectUrl string) []string {
	argv := []string{"vpn"}
	add := func(flag, val string) {
		val = strings.TrimSpace(val)
		if val != "" {
			argv = append(argv, fmt.Sprintf("%s=%s", flag, val))
		}
	}
	addBool := func(flag string, on bool) {
		if on {
			argv = append(argv, flag)
		}
	}
	// Core endpoints
	add("--api_url", apiUrl)
	add("--connect_url", connectUrl)
	// Interface and routing
	add("--tun", getStringOr(opts, "--tun", ""))
	add("--ip_cidr", getStringOr(opts, "--ip_cidr", ""))
	if mtu := getIntOr(opts, "--mtu", 0); mtu > 0 {
		add("--mtu", fmt.Sprintf("%d", mtu))
	}
	addBool("--default_route", mustBool(opts, "--default_route"))
	add("--route", getStringOr(opts, "--route", ""))
	add("--exclude_route", getStringOr(opts, "--exclude_route", ""))
	// Locations
	add("--location_query", getStringOr(opts, "--location_query", ""))
	add("--location_id", getStringOr(opts, "--location_id", ""))
	add("--location_group_id", getStringOr(opts, "--location_group_id", ""))
	// DNS
	add("--dns", getStringOr(opts, "--dns", ""))
	add("--dns_service", getStringOr(opts, "--dns_service", ""))
	add("--dns_bootstrap", getStringOr(opts, "--dns_bootstrap", ""))
	// SOCKS
	add("--socks", getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	add("--domain", getStringOr(opts, "--domain", ""))
	add("--exclude_domain", getStringOr(opts, "--exclude_domain", ""))
	// Forwarding and filtering controls
	addBool("--local_only", mustBool(opts, "--local_only"))
	add("--allow_forward_src", getStringOr(opts, "--allow_forward_src", ""))
	add("--deny_forward_src", getStringOr(opts, "--deny_forward_src", ""))
	addBool("--no_fw_rules", mustBool(opts, "--no_fw_rules"))
	// Logging and stats
	add("--log_file", getStringOr(opts, "--log_file", ""))
	add("--log_level", getStringOr(opts, "--log_level", ""))
	if si := getIntOr(opts, "--stats_interval", -1); si >= 0 {
		add("--stats_interval", fmt.Sprintf("%d", si))
	}
	if dbg, _ := opts.Bool("--debug"); dbg {
		argv = append(argv, "--debug")
	}
	return argv
}

// spawnProcessDetached starts this program with provided args in a new session and returns child pid.
// If logPath is non-empty, stdout/stderr are redirected to that file.
func spawnProcessDetached(args []string, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// stdio
	if strings.TrimSpace(logPath) != "" {
		// ensure dir exists
		if dir := filepath.Dir(logPath); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, err
		}
		cmd.Stdout = f
		cmd.Stderr = f
	} else {
		devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		cmd.Stdin = devnull
		cmd.Stdout = devnull
		cmd.Stderr = devnull
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func cmdLogin(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	userAuth := mustString(opts, "--user_auth")
	password := mustString(opts, "--password")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)

	done := make(chan struct{})
	api.AuthLoginWithPassword(&connect.AuthLoginWithPasswordArgs{UserAuth: userAuth, Password: password},
		connect.NewApiCallback(func(res *connect.AuthLoginWithPasswordResult, err error) {
			defer close(done)
			if err != nil {
				fmt.Fprintf(os.Stderr, "login error: %v\n", err)
				return
			}
			if res.Error != nil {
				fmt.Fprintf(os.Stderr, "login error: %s\n", res.Error.Message)
				return
			}
			if res.VerificationRequired != nil {
				fmt.Printf("verification required for %s\n", res.VerificationRequired.UserAuth)
				return
			}
			if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
				fmt.Fprintf(os.Stderr, "login succeeded but no by_jwt returned\n")
				return
			}
			if err := saveJWT(res.Network.ByJwt); err != nil {
				fmt.Fprintf(os.Stderr, "save jwt failed: %v\n", err)
				return
			}
			fmt.Printf("saved JWT for network %s -> %s\n", res.Network.NetworkName, jwtPath())
		}))
	<-done
}

func cmdVerify(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	userAuth := mustString(opts, "--user_auth")
	code := mustString(opts, "--code")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)

	done := make(chan struct{})
	api.AuthVerify(&connect.AuthVerifyArgs{UserAuth: userAuth, VerifyCode: code}, connect.NewApiCallback(func(res *connect.AuthVerifyResult, err error) {
		defer close(done)
		if err != nil {
			fmt.Fprintf(os.Stderr, "verify error: %v\n", err)
			return
		}
		if res.Error != nil {
			fmt.Fprintf(os.Stderr, "verify error: %s\n", res.Error.Message)
			return
		}
		if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
			fmt.Fprintf(os.Stderr, "verify succeeded but no by_jwt returned\n")
			return
		}
		if err := saveJWT(res.Network.ByJwt); err != nil {
			fmt.Fprintf(os.Stderr, "save jwt failed: %v\n", err)
			return
		}
		fmt.Printf("saved JWT -> %s\n", jwtPath())
	}))
	<-done
}

func cmdMintClient(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	res, err := api.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""})
	if err != nil {
		fatal(err)
	}
	if res.Error != nil {
		fatal(fmt.Errorf("auth-client error: %s", res.Error.Message))
	}
	if strings.TrimSpace(res.ByClientJwt) == "" {
		fatal(errors.New("auth-client succeeded but no by_client_jwt returned"))
	}
	if err := saveJWT(res.ByClientJwt); err != nil {
		fatal(err)
	}
	clientID := parseClientID(res.ByClientJwt)
	if clientID != "" {
		fmt.Printf("saved client JWT (client_id=%s) -> %s\n", clientID, jwtPath())
	} else {
		fmt.Printf("saved client JWT -> %s\n", jwtPath())
	}
}

func cmdFindProviders(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	// try to extract client_id from JWT for display
	if clientID := parseClientID(jwt); clientID != "" {
		fmt.Printf("client_id: %s\n", clientID)
	}

	count := getIntOr(opts, "--count", 8)
	rankMode := getStringOr(opts, "--rank_mode", "quality")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	// Build specs: prefer explicit location flags
	specs := []*connect.ProviderSpec{}
	if id := strings.TrimSpace(getStringOr(opts, "--location_id", "")); id != "" {
		if loc, err := connect.ParseId(id); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationId: &loc})
		}
	}
	if gid := strings.TrimSpace(getStringOr(opts, "--location_group_id", "")); gid != "" {
		if lg, err := connect.ParseId(gid); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationGroupId: &lg})
		}
	}
	if len(specs) == 0 {
		if q := strings.TrimSpace(getStringOr(opts, "--location_query", "")); q != "" {
			// Try server-side search first
			if httpRes, err := httpFindLocations(ctx, apiUrl, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
				specs = httpRes.Specs
				fmt.Printf("using %d specs from location query: %s\n", len(specs), q)
			}
			// Fallback: client-side filter of provider-locations (handles country names like 'country:Germany')
			if len(specs) == 0 {
				fb := findSpecsByQueryFallback(context.Background(), strat, apiUrl, jwt, q)
				if len(fb) > 0 {
					specs = fb
					fmt.Printf("using %d specs from provider-locations (fallback) for: %s\n", len(specs), q)
				}
			}
		}
	}
	if len(specs) == 0 {
		specs = []*connect.ProviderSpec{{BestAvailable: true}}
		fmt.Println("using best-available providers")
	}
	res, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: count, RankMode: rankMode})
	if err != nil {
		fatal(err)
	}
	for _, p := range res.Providers {
		fmt.Printf("provider client_id=%s tier=%d est_bps=%d intermediaries=%v\n", p.ClientId.String(), p.Tier, p.EstimatedBytesPerSecond, idsToStrings(p.IntermediaryIds))
	}
}

func cmdLocations(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	q := getStringOr(opts, "--query", "")
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	var res *findLocationsHTTPResult
	if q == "" || q == "*" || q == "country:*" || q == "region:*" || q == "group:*" {
		httpRes, err := httpProviderLocations(ctx, apiUrl, jwt)
		if err != nil {
			fatal(err)
		}
		res = httpRes
	} else {
		// Try server-side first via direct HTTP
		if httpRes, err := httpFindLocations(ctx, apiUrl, jwt, q); err == nil && httpRes != nil {
			res = httpRes
		}
		// If nothing came back, fallback to provider-locations and filter locally
		if res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0) {
			fbSpecs, fbRes := filterLocationsFallback(context.Background(), strat, apiUrl, jwt, q)
			if fbRes != nil {
				res = fbRes
			}
			if len(fbSpecs) == 0 && (res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0)) {
				fmt.Println("no results")
				return
			}
		}
	}
	if res == nil {
		fmt.Println("no results")
		return
	}

	if len(res.Groups) > 0 {
		fmt.Println("Groups:")
		for _, g := range res.Groups {
			fmt.Printf("  %-30s id=%s providers=%d\n", g.Name, g.LocationGroupId, g.ProviderCount)
		}
	}
	if len(res.Locations) > 0 {
		fmt.Println("Locations:")
		for _, l := range res.Locations {
			id := l.LocationId
			// Prefer specific country/region ids if provided
			if l.CountryLocationId != "" {
				id = l.CountryLocationId
			}
			if l.RegionLocationId != "" {
				id = l.RegionLocationId
			}
			extra := l.Country
			if l.Region != "" {
				extra = l.Region
			}
			if extra != "" {
				fmt.Printf("  %-18s %-24s id=%s providers=%d\n", l.LocationType, l.Name+" ("+extra+")", id, l.ProviderCount)
			} else {
				fmt.Printf("  %-18s %-24s id=%s providers=%d\n", l.LocationType, l.Name, id, l.ProviderCount)
			}
		}
	}
}

func cmdOpen(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	clientIDStr := parseClientID(jwt)
	if clientIDStr == "" {
		fatal(errors.New("JWT missing client_id (run 'urnet-client mint-client' to mint a client-scoped JWT)"))
	}
	clientID, err := connect.ParseId(clientIDStr)
	if err != nil {
		fatal(err)
	}

	oob := connect.NewApiOutOfBandControlWithApi(api)
	client := connect.NewClientWithDefaults(ctx, clientID, oob)
	defer client.Close()

	auth := &connect.ClientAuth{
		ByJwt:      jwt,
		InstanceId: connect.NewId(),
		AppVersion: fmt.Sprintf("urnet-client %s", Version),
	}

	n := getIntOr(opts, "--transports", 4)
	for i := 0; i < n; i++ {
		pt := connect.NewPlatformTransportWithDefaults(ctx, strat, client.RouteManager(), fmt.Sprintf("%s/", connectUrl), auth)
		defer pt.Close()
	}

	fmt.Println("transports opened; press Ctrl-C to exit")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

// helpers
func getStringOr(opts docopt.Opts, key, def string) string {
	if v, err := opts.String(key); err == nil && v != "" {
		return v
	}
	return def
}
func getIntOr(opts docopt.Opts, key string, def int) int {
	if v, err := opts.Int(key); err == nil {
		return v
	}
	return def
}
func mustString(opts docopt.Opts, key string) string {
	v, _ := opts.String(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", key)
		os.Exit(2)
	}
	return v
}
func fatal(err error) { fmt.Fprintf(os.Stderr, "error: %v\n", err); os.Exit(1) }

func parseClientID(jwt string) string {
	claims := gojwt.MapClaims{}
	_, _, _ = gojwt.NewParser().ParseUnverified(jwt, claims)
	if v, ok := claims["client_id"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func idsToStrings(ids []connect.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func cmdSocks(opts docopt.Opts) {
	listenAddr, _ := opts.String("--listen")
	extenderIP, _ := opts.String("--extender_ip")
	extenderPort, _ := opts.String("--extender_port")
	extenderSNI, _ := opts.String("--extender_sni")
	extenderSecret, _ := opts.String("--extender_secret")
	debugOn, _ := opts.Bool("--debug")

	if listenAddr == "" {
		fatal(errors.New("--listen is required for socks command"))
	}
	if extenderIP == "" {
		fatal(errors.New("--extender_ip is required for socks command"))
	}
	if extenderPort == "" {
		fatal(errors.New("--extender_port is required for socks command"))
	}
	if extenderSNI == "" {
		fatal(errors.New("--extender_sni is required for socks command"))
	}

	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For now, we'll log the extender details but the actual connection logic
	// would need to be implemented to use these parameters
	_ = extenderSecret // Acknowledge we have the secret but don't use it yet
	logInfo("Extender details: IP=%s Port=%s SNI=%s\n", extenderIP, extenderPort, extenderSNI)

	// Start SOCKS5 proxy without binding to a VPN interface
	// This is a standalone SOCKS proxy that connects to the specified extender
	stopSocks, err := StartSocks5(ctx, listenAddr, "", debugOn, allowDomains, excludeDomains)
	if err != nil {
		fatal(fmt.Errorf("failed to start SOCKS5 proxy: %w", err))
	}
	defer stopSocks()

	logInfo("SOCKS5 proxy listening at %s\n", listenAddr)
	logInfo("Connecting to extender at %s:%s (SNI: %s)\n", extenderIP, extenderPort, extenderSNI)

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logInfo("Shutting down SOCKS5 proxy...\n")
}

// cmdVpn is implemented per-OS in vpn_linux.go and vpn_darwin.go

// spawnBackground detaches a child copy of this process without the --background flag and returns its PID.
func spawnBackground(argv []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	// Drop the --background flag (both forms --background and --background=true)
	args := make([]string, 0, len(argv)-1)
	for i, a := range argv {
		if i == 0 {
			continue
		} // skip program name
		if a == "--background" || strings.HasPrefix(a, "--background=") {
			continue
		}
		args = append(args, a)
	}
	cmd := exec.Command(exe, args...)
	// Detach: start new session
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Disconnect stdio
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// setupLogFile redirects stdout and stderr to the given file, creating it if needed and appending.
func setupLogFile(path string) error {
	// Ensure directory exists
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	// Duplicate file descriptors
	os.Stdout = f
	os.Stderr = f
	return nil
}

// findSpecsByQueryFallback queries ProviderLocations and builds ProviderSpecs by client-side filtering
// for simple queries like 'country:Germany', 'country_code:DE', 'region:Europe', 'group:Western Europe'.
func findSpecsByQueryFallback(ctx context.Context, strat *connect.ClientStrategy, apiUrl, jwt, q string) []*connect.ProviderSpec {
	_, res := filterLocationsFallback(ctx, strat, apiUrl, jwt, q)
	if res == nil {
		return nil
	}
	key, val, ok := parseKV(q)
	if !ok {
		key = "name"
		val = strings.TrimSpace(q)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	valNorm := strings.ToLower(strings.TrimSpace(val))
	// Dedup by id string
	seen := map[string]bool{}
	specs := []*connect.ProviderSpec{}
	// Groups -> LocationGroupId
	if key == "group" || key == "name" {
		for _, g := range res.Groups {
			if matchValueFold(g.Name, valNorm) {
				if id, err := connect.ParseId(g.LocationGroupId); err == nil {
					if !seen[id.String()] {
						seen[id.String()] = true
						specs = append(specs, &connect.ProviderSpec{LocationGroupId: &id})
					}
				}
			}
		}
	}
	// Locations -> LocationId (prefer region/country specific ids if present)
	for _, l := range res.Locations {
		var match bool
		switch key {
		case "country":
			match = matchValueFold(l.Country, valNorm) || matchValueFold(l.Name, valNorm)
		case "country_code":
			match = strings.EqualFold(strings.TrimSpace(l.CountryCode), val)
		case "region":
			match = matchValueFold(l.Region, valNorm) || (l.LocationType == "region" && matchValueFold(l.Name, valNorm))
		case "name":
			match = matchValueFold(l.Name, valNorm)
		default:
			match = matchValueFold(l.Name, valNorm) || matchValueFold(l.Country, valNorm) || matchValueFold(l.Region, valNorm)
		}
		if !match {
			continue
		}
		idStr := l.LocationId
		if key == "country" && l.CountryLocationId != "" {
			idStr = l.CountryLocationId
		}
		if key == "region" && l.RegionLocationId != "" {
			idStr = l.RegionLocationId
		}
		if id, err := connect.ParseId(idStr); err == nil {
			if !seen[id.String()] {
				seen[id.String()] = true
				specs = append(specs, &connect.ProviderSpec{LocationId: &id})
			}
		}
	}
	return specs
}

// filterLocationsFallback returns specs (unused by locations printing) and a filtered
// provider-locations result for the given query q.
func filterLocationsFallback(ctx context.Context, strat *connect.ClientStrategy, apiUrl, jwt, q string) ([]*connect.ProviderSpec, *findLocationsHTTPResult) {
	res, err := httpProviderLocations(ctx, apiUrl, jwt)
	if err != nil || res == nil {
		return nil, nil
	}
	key, val, ok := parseKV(q)
	if !ok {
		key = "name"
		val = strings.TrimSpace(q)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	valNorm := strings.ToLower(strings.TrimSpace(val))
	out := &findLocationsHTTPResult{}
	// Filter groups
	if key == "group" || key == "name" {
		for _, g := range res.Groups {
			if matchValueFold(g.Name, valNorm) {
				out.Groups = append(out.Groups, g)
			}
		}
	}
	// Filter locations
	for _, l := range res.Locations {
		var match bool
		switch key {
		case "country":
			match = matchValueFold(l.Country, valNorm) || matchValueFold(l.Name, valNorm)
		case "country_code":
			match = strings.EqualFold(strings.TrimSpace(l.CountryCode), val)
		case "region":
			match = matchValueFold(l.Region, valNorm) || (l.LocationType == "region" && matchValueFold(l.Name, valNorm))
		case "name":
			match = matchValueFold(l.Name, valNorm)
		default:
			match = matchValueFold(l.Name, valNorm) || matchValueFold(l.Country, valNorm) || matchValueFold(l.Region, valNorm)
		}
		if match {
			out.Locations = append(out.Locations, l)
		}
	}
	// Build specs for convenience (not used by locations printing directly)
	specs := []*connect.ProviderSpec{}
	seen := map[string]bool{}
	for _, g := range out.Groups {
		if id, err := connect.ParseId(g.LocationGroupId); err == nil {
			if !seen[id.String()] {
				seen[id.String()] = true
				specs = append(specs, &connect.ProviderSpec{LocationGroupId: &id})
			}
		}
	}
	for _, l := range out.Locations {
		idStr := l.LocationId
		if key == "country" && l.CountryLocationId != "" {
			idStr = l.CountryLocationId
		}
		if key == "region" && l.RegionLocationId != "" {
			idStr = l.RegionLocationId
		}
		if id, err := connect.ParseId(idStr); err == nil {
			if !seen[id.String()] {
				seen[id.String()] = true
				specs = append(specs, &connect.ProviderSpec{LocationId: &id})
			}
		}
	}
	return specs, out
}

func parseKV(q string) (string, string, bool) {
	if q == "" {
		return "", "", false
	}
	i := strings.Index(q, ":")
	if i < 0 {
		return "", "", false
	}
	return q[:i], q[i+1:], true
}

func matchValueFold(s, valNorm string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if valNorm == "*" {
		return s != ""
	}
	return strings.Contains(s, valNorm)
}

// validateClientJWT performs a quick authenticated API call with the provided client-scoped JWT
// to check if it is currently accepted by the backend. Returns true if usable.
func validateClientJWT(apiUrl, clientJwt string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(clientJwt)
	// Use a tiny call: FindProviders2 with BestAvailable spec and Count=1
	specs := []*connect.ProviderSpec{{BestAvailable: true}}
	_, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: 1, RankMode: "quality"})
	return err == nil
}
