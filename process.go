package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docopt/docopt-go"
)

type secretArg struct {
	Flag  string // canonical long option
	Env   string
	Value string
}

// secretFlags leave argv when re-executing with --background, because a long-lived
// process's argv is readable by every local user via ps; the environment is not.
var secretFlags = []secretArg{
	{Flag: "--password", Env: "URNETWORK_PASSWORD"},
	{Flag: "--jwt", Env: "URNETWORK_JWT"},
	{Flag: "--socks_pass", Env: "URNETWORK_SOCKS_PASS"},
}

func backgroundSecrets(opts docopt.Opts) []secretArg {
	out := make([]secretArg, 0, len(secretFlags))
	for _, s := range secretFlags {
		s.Value = getStringOr(opts, s.Flag, "")
		out = append(out, s)
	}
	return out
}

// applySecretEnvFallbacks lets a background child (and any caller) supply secret flags
// through the environment. Explicit flags always win. It reports the flags whose value
// came from the environment alone; a redacted "--flag=" left on argv by scrubSecretArgs
// counts as set on the CLI, because the parent got it from its own command line.
func applySecretEnvFallbacks(opts docopt.Opts, getenv func(string) string) (fromEnv map[string]bool) {
	fromEnv = map[string]bool{}
	for _, s := range secretFlags {
		if getStringOr(opts, s.Flag, "") != "" {
			continue
		}
		if v := getenv(s.Env); v != "" {
			fromEnv[s.Flag] = opts[s.Flag] == nil
			opts[s.Flag] = v
		}
	}
	return fromEnv
}

// scrubSecretArgs drops the program name and --background from argv and redacts every
// secret flag to an empty value, returning env entries that carry the real values
// instead. A matched flag is redacted rather than removed outright because docopt usage
// groups require it to appear alongside another flag (e.g. --user_auth needs --password);
// dropping it entirely would fail docopt parsing in the re-exec'd child. Matching requires
// the value too, because docopt accepts unambiguous prefixes (--pass) and --socks is
// itself a prefix of --socks_pass.
func scrubSecretArgs(argv []string, secrets []secretArg) (args, env []string) {
	for _, s := range secrets {
		if s.Value != "" {
			env = append(env, s.Env+"="+s.Value)
		}
	}
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if isBackgroundFlag(a) {
			continue
		}
		name, val, hasEq := strings.Cut(a, "=")
		redacted := false
		for _, s := range secrets {
			if s.Value == "" || len(name) <= 2 || !strings.HasPrefix(name, "--") || !strings.HasPrefix(s.Flag, name) {
				continue
			}
			if hasEq && val == s.Value {
				redacted = true
				break
			}
			if !hasEq && i+1 < len(argv) && argv[i+1] == s.Value {
				i++
				redacted = true
				break
			}
		}
		if redacted {
			args = append(args, name+"=")
		} else {
			args = append(args, a)
		}
	}
	return args, env
}

// isBackgroundFlag also matches the abbreviations docopt accepts (--back); leaving one in
// the child's argv would make every child spawn another child.
func isBackgroundFlag(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	return len(name) > 2 && strings.HasPrefix("--background", name)
}

// loadSecretEnv applies the secret env fallbacks, then removes those variables from this
// process's environment so route, ip, ifconfig and networksetup children do not inherit them.
func loadSecretEnv(opts docopt.Opts) map[string]bool {
	fromEnv := applySecretEnvFallbacks(opts, os.Getenv)
	for _, s := range secretFlags {
		_ = os.Unsetenv(s.Env)
	}
	return fromEnv
}

// startBackground resolves the VPN config first because the child's stderr is /dev/null:
// a config error found only there would leave the user with a "started" message and no VPN.
func startBackground(opts docopt.Opts, argv []string, spawn func([]string, []secretArg) (int, error)) (int, error) {
	if _, err := resolveVPNConfig(opts); err != nil {
		return 0, err
	}
	return spawn(argv, backgroundSecrets(opts))
}

// spawnBackground detaches a child copy of this process (dropping --background) and returns its PID.
func spawnBackground(argv []string, secrets []secretArg) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	args, secretEnv := scrubSecretArgs(argv, secrets)
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), secretEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// setupLogFile redirects os.Stdout and os.Stderr to the given file path, appending if it exists.
// The file is created with 0o600 permissions to protect potentially sensitive log content.
func setupLogFile(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	os.Stdout = f
	os.Stderr = f
	return nil
}
