# Phase 1 Package Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the single `package main` (60 files, ~7.5k LOC) into `cmd/urnet-client` plus focused `internal/*` packages with no behavior change, add a macOS CI job, make release assets and images carry the real release version, and harden privileged command lookup and file writes.

**Architecture:** Three streams. The split stream is serial on `sina/phase1_package_split`: one package is extracted per task, leaf-first along the real dependency graph (logx, socks, netcfg, tunnel, config, urapi, auth, session, then cmd), each task always green. The CI stream runs in parallel in its own worktree on `sina/phase1_ci` and only touches `.github/workflows/ci.yml`, `.releaserc.json` and two new scripts, so it merges without conflicts. The hygiene stream runs after the split and the CI merge, inside the new packages (`internal/netcfg`, a new `internal/safefile`, `internal/auth`, `internal/logx`).

**Tech Stack:** Go 1.26.3 module (`go 1.26.3`, toolchain go1.27.1), `github.com/urnetwork/connect`, `github.com/docopt/docopt-go`, `github.com/songgao/water`, `gopkg.in/yaml.v3`, `github.com/golang-jwt/jwt/v5`, golangci-lint v2.14.0, GitHub Actions, semantic-release via `cycjimmy/semantic-release-action@v6` plus `@semantic-release/exec`.

**Spec:** The Phase 1 owner brief (scope items 1 to 4, execution constraints, deferred list) as restated in "Scope and rulings" below, plus the "Deferred to Phase 1+" section of `docs/superpowers/plans/2026-09-28-phase0-critical-fixes.md`. There is no separate spec file.

## Global Constraints

- No behavior change in the split (Tasks 1 to 9): CLI flags, usage text, stdout/stderr text, exit codes, config keys, env vars and documented behavior stay identical. The golden CLI transcript (Task 1) must diff clean after every split task.
- Module path: `github.com/devrandom0/urnetwork-client`. New code lives under `internal/` and `cmd/urnet-client/`.
- Only `internal/urapi` may import `github.com/urnetwork/connect` or `github.com/urnetwork/connect/protocol` (enforced by `make arch-check` from Task 9 on, including test imports).
- No import cycles. Allowed edges: `logx` <- everything; `socks -> logx`; `netcfg -> logx`; `tunnel -> logx`; `config -> logx, socks`; `urapi -> config, logx`; `auth -> urapi, logx` (and `safefile` from Task 16); `session -> config, netcfg, tunnel, urapi, socks, logx`; `cmd/urnet-client -> all`.
- `Version` stays `var Version = "dev"` in `package main` (now `cmd/urnet-client/main.go`). The ldflags string `-X main.Version=<v>` does not change; only the build path changes (`.` becomes `./cmd/urnet-client`).
- Unexported-to-exported renames only where another package calls the symbol. Rename tables per task are authoritative.
- Tests move with their code, stay in-package (white-box) unless a task says `_test` package. Test count never drops: baseline is 214 on darwin (`go test -race -count=1 -v ./...`, counting `--- PASS`/`--- SKIP` lines including subtests) and 217 on linux (golang:1.27 container).
- Use `git mv` for every moved file so history follows.
- Build-tagged files keep their tags: `//go:build darwin`, `//go:build linux`, `//go:build !linux && !darwin`.
- Modernize hints folded in only where code moves anyway: `slices.Contains` in `socks_auth.go`, `sync.WaitGroup.Go` in `socks.go` (Go >= 1.25; module is 1.26.3). No other modernize changes.
- File ownership: split stream owns `Makefile`, `Dockerfile`, `README.md`, `docs/*.md` (except `docs/superpowers`), all `*.go`. CI stream owns `.github/workflows/ci.yml`, `.releaserc.json`, `scripts/stage-release-assets.sh`, `scripts/test-release-prepare.sh`. All Makefile/Dockerfile build-path edits happen in Task 9 and nowhere else. The CI stream calls only existing make targets (`build-all`) and must not edit the Makefile; Task 9 must not rename `build-all` or change its `dist/<os>_<arch>/urnet-client` outputs.
- Conventional Commits, imperative, subject under 72 characters, no ticket IDs. `refactor:` for moves, `ci:` for workflow changes, `fix:` for hygiene.
- Comments: none by default; only a non-obvious WHY, one line. No em dashes, straight quotes, in code, docs and commit bodies.
- TDD for new behavior (Tasks 4 dataplane seam, 11, 14, 15, 16). For pure moves the existing tests are the safety net.
- Never `git push` without explicit user approval.
- Verification gate after every task (`$BASE/gate.sh <min-count>` from Task 1 runs exactly this, plus the count and golden checks):

```bash
go build ./... && CGO_ENABLED=0 GOOS=linux go build ./... && GOOS=darwin go vet ./... && CGO_ENABLED=0 GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run && CGO_ENABLED=0 GOOS=linux golangci-lint run && CGO_ENABLED=0 GOOS=linux go test -count=1 -exec /usr/bin/true ./...
```

  `CGO_ENABLED=0` is required for every `GOOS=linux` command on a macOS host: with cgo on, `runtime/cgo` fails to cross-compile with the macOS clang (`implicit declaration of function 'setresgid'`, verified on this machine). The last command compiles the linux test binaries without running them. From Task 9 on also run `make version-check` and `make arch-check`.
- Import fixing uses goimports: `GOIMPORTS="go run golang.org/x/tools/cmd/goimports@latest -local github.com/devrandom0/urnetwork-client -w"`. If a file still reports `undefined: <pkg>` after it, add the import line by hand.

---

## Scope and rulings

### Owner scope (restated)

1. Package split, full target layout, no behavior change, tests move with code, build files updated, darwin and linux verified.
2. macOS CI job running `go test -race ./...` and golangci-lint.
3. Release assets and Docker image built with the version semantic-release computes.
4. Root hygiene: absolute paths for privileged tools with a fixed search order, never `$PATH`; symlink-safe writes for the JWT file and `--log_file`.

Out of scope: removing `--background`, tri-state config and everything in "Deferred to Phase 2+".

### Dependency graph that fixed the package order

Measured with a `go/ast` pass over every root file (package-level identifiers defined per file and cross-file references), cross-checked with gopls `findReferences` on `Version`, `SocksAuth` and `setLogLevel`:

- `logger.go` depends on nothing and is referenced from 23 places in 10 files (every other cluster). It must be extracted first, as `internal/logx`.
- `socks.go`, `socks_auth.go`, `socks_limits.go` depend only on the logger. Leaf. `config.go` depends on `SocksAuth` and `resolveSocksAuth`, so socks must precede config.
- `routes_darwin.go`/`routes_linux.go` depend on the logger, `util.go` (`runCapture`, `extractHost`) and on `runSudo`/`run` defined in `vpn_darwin.go`/`vpn_linux.go`. `configureDarwinTUN`, `configureLinuxTUN`, `getDefaultGateway`, `getSystemDNSResolvers`, `linuxListDefaultRoutes` and their fake-runner tests share `cmdRunner`. They must move together with the route managers into `internal/netcfg`, otherwise netcfg and session would import each other.
- The packet filter (`shouldDropInbound*`, `parseCIDRHost`) and the dataplane loop inside `vpnRunCore` use only the logger and `water`. Leaf, `internal/tunnel`.
- `config.go` depends on the logger, socks, `urlcheck.go`, the docopt helpers in `util.go` and `DefaultAPIURL`/`DefaultConnectURL` from `main.go`.
- `api_http.go`, `specs.go`, `auth.go`, `jwt.go` (`validateClientJWT`), `cmd_open.go`, `cmd_providers.go`, `vpn_core.go` hold every `connect.*` use. `specs.go` needs `config.LocationConfig`; `api_http.go` needs `validateEndpointURL`. So urapi comes after config.
- `jwt.go` store functions and `ensureClientJWT` (in `cmd_quickconnect.go`) call login/mint/validate, so auth comes after urapi.
- `vpn_core.go` plus the per-OS `cmdVpn` depend on config, netcfg, tunnel, urapi, socks: `internal/session`, second to last.
- `main.go`, `process.go`, `cmd_*.go` depend on everything: `cmd/urnet-client`, last.

### Target layout

```
cmd/urnet-client/     main.go, process.go (secrets env, --background), cmd_*.go
internal/logx         leveled logger, SetupLogFile
internal/socks        SOCKS5 server, auth, UDP relay, limits
internal/netcfg       cmdRunner seam, tool path lookup, route managers, TUN address config, gateway/DNS discovery, DNS bootstrap
internal/tunnel       TUN device open, inbound filter, dataplane loop, counters, stats
internal/config       VPNConfig/SOCKSConfig/LocationConfig, YAML file, precedence, endpoint URL checks, docopt helpers, defaults
internal/urapi        every connect call: HTTP location API, provider specs, login/verify/mint/validate, find-providers, transports, multi-client
internal/auth         JWT store (path/load/save/client_id), EnsureClientJWT, RenewLoop
internal/session      VPN session lifecycle (Run per OS, runCore, SOCKS-only mode, startup log)
internal/safefile     symlink-safe WriteFile and OpenAppend (Task 15)
```

### Rulings

1. `internal/logx` added: the logger is used by every package, so it cannot live in any of the owner's packages without cycles.
2. URL checks (`validateEndpointURL`, `validateEndpointFlags`), docopt helpers (`getStringOr`, `getIntOr`, `mustBool`, `splitCSV`) and `DefaultAPIURL`/`DefaultConnectURL` go into `internal/config`. urapi imports config for `ValidateEndpointURL` and `LocationConfig`.
3. TUN address configuration (ifconfig/ip) and gateway/resolver discovery go into `internal/netcfg`, not `internal/tunnel`: they run OS commands through `cmdRunner`, and their tests use the unexported fake runner. `internal/tunnel` is device open plus dataplane.
4. The connect RPCs (login, verify, mint, validate) go into `internal/urapi`, not `internal/auth`: the anti-corruption rule forbids connect types outside urapi, and those functions are thin connect adapters. `internal/auth` owns the JWT store and the token lifecycle flows (`EnsureClientJWT`, `RenewLoop`).
5. `SocksAuth` stays in `internal/socks` (it owns validation and constant-time matching); config imports socks for the type.
6. The unused `Authenticator`, `apiAuthenticator` and `DefaultAuthenticator` in `auth.go` are deleted in Task 6 (zero references, verified with grep and gopls). Moving dead code into a new exported API would be worse.
7. `configureLinuxTUN(name, cfg VPNConfig)` becomes `ConfigureLinuxTUN(name, ipCIDR string, mtu int, enableIPv6 bool)`, matching the darwin signature, so netcfg does not import config.
8. `Version` stays in package main; `session.Run` and `urapi.OpenTransports`/`urapi.GeneratorConfig` take the app version explicitly. The `-X main.Version` string in Makefile/Dockerfile/CI does not change, which also removes a merge hazard with the CI stream. `make version-check` still changes, to build `./cmd/urnet-client`.
9. Test helpers are duplicated instead of exported: `vpnTestOpts`/`writeTempConfig` exist in `internal/config` tests and in `cmd/urnet-client` tests; `captureStderr` exists in `internal/socks` and `internal/session` tests. Each copy is under 25 lines.
10. `TestParseConfigs_SocksAuthFromFlagsAndEnv` spans config and session, so its `socksOptionsFromVPN` assertion becomes `TestSocksOptionsFromVPN_KeepsAuth` (count +1).
11. Test-count baseline is 214 on darwin and 217 on linux (measured on 93f01f8), not ~211.
12. Release fix uses `@semantic-release/exec` `prepareCmd` inside the existing release job. semantic-release docs (Context7, `/semantic-release/semantic-release`): the `prepare` step runs after `analyzeCommits`/`generateNotes` and before tag creation and `publish`, and `nextRelease.version` is available to it; `@semantic-release/github` uploads assets in `publish`. This computes the version once, unlike a dry-run job plus a build job (dry-run skips `prepare`, and two runs could disagree). Docker images are built after the release job and take `needs.release.outputs.version`.
13. Tool search dirs: `/sbin`, `/usr/sbin`, `/bin`, `/usr/bin`, then `/run/current-system/sw/bin` (NixOS keeps `ip` only there; it is a root-owned profile link, not user-controlled). Flagged for owner review. `getDefaultGateway` moves from `exec.Command` to the runner (it was the only privileged call bypassing the seam); its output becomes combined stdout+stderr, parsed the same way.
14. Symlink-safe writes: refuse a symlink or non-regular target; refuse a directory that a user other than the current user or root owns, or that is group/world-writable without the sticky bit (keeps `/tmp` usable); resolve symlinks in the directory path and write into the resolved directory. When euid is 0 and the directory belongs to another user (sudo with a preserved `HOME`, the common macOS case), the write is allowed but safe: new files are created with `O_EXCL`/`O_NOFOLLOW` and chowned to the directory owner, appends refuse hard-linked files and files not owned by the current user or the directory owner. This is the "document" option of the brief; refusing would break `sudo urnet-client quick-connect`. Documented in `docs/configuration.md`.
15. `make test-integration` points at `./` until Task 9 moves it to `./internal/urapi/`; between Task 7 and Task 9 the target runs no integration test. Acceptable because CI only sees the finished branch.
16. `make arch-check` (new Makefile target, run by `make lint`) enforces ruling 2 of the Global Constraints on darwin and linux.

---

## Shared setup (run once, before Task 1)

- [ ] **Setup 1: Confirm branch and base**

```bash
cd /Users/sinamoghaddas/projects/my-projects/devrandom0/urnetwork-client
git switch sina/phase1_package_split
git log --oneline -1   # expect 93f01f8 docs(release): 2.0.0 [skip ci]
git status --short     # expect empty
```

- [ ] **Setup 2: Create the CI worktree (for the parallel CI stream, Tasks 10 to 12)**

```bash
git worktree add ../urnetwork-client-phase1_ci -b sina/phase1_ci main
```

Expected: `Preparing worktree (new branch 'sina/phase1_ci')`. `main` is 93f01f8, the same base.

---

### Task 1: [split] Baseline tooling and extract internal/logx

**Files:**
- Create (untracked, inside `.git`): `$BASE/gate.sh`, `$BASE/golden.sh`, `$BASE/linux-test.sh`, `$BASE/golden.base.txt`
- Move: `logger.go` -> `internal/logx/logx.go`, `logger_test.go` -> `internal/logx/logx_test.go`
- Modify: `process.go` (remove `setupLogFile`), every root `*.go` that logs

**Interfaces:**
- Consumes: nothing.
- Produces (package `logx`, import `github.com/devrandom0/urnetwork-client/internal/logx`):
  - `type LogLevel int32`; `LevelQuiet, LevelError, LevelWarn, LevelInfo, LevelDebug`
  - `func SetLogLevel(level string, debugFlag bool)`
  - `func IsDebugEnabled() bool`, `IsInfoEnabled`, `IsWarnEnabled`, `IsErrorEnabled`
  - `func Info(format string, args ...any)`, `Warn`, `Error`, `Debug`
  - `func SetupLogFile(path string) error`

Rename table: `setLogLevel`->`SetLogLevel`, `isDebugEnabled`->`IsDebugEnabled`, `isInfoEnabled`->`IsInfoEnabled`, `isWarnEnabled`->`IsWarnEnabled`, `isErrorEnabled`->`IsErrorEnabled`, `logInfo`->`Info`, `logWarn`->`Warn`, `logError`->`Error`, `logDebug`->`Debug`, `setupLogFile`->`SetupLogFile`. Unchanged: `LogLevel`, `Level*`, `currentLogLevel`, `logf`, `init`.

- [ ] **Step 1: Create the gate, golden and linux scripts**

```bash
BASE="$(cd "$(git rev-parse --git-common-dir)" && pwd)/phase1-baseline"
mkdir -p "$BASE"
echo "export BASE=$BASE"   # paste this export into every later shell
cat > "$BASE/gate.sh" <<'EOF'
#!/usr/bin/env bash
# Usage: gate.sh <min test count>. Runs the Phase 1 verification gate from the repo root.
set -euo pipefail
want="${1:?usage: gate.sh <min test count>}"
base="$(cd "$(git rev-parse --git-common-dir)" && pwd)/phase1-baseline"
go build ./...
CGO_ENABLED=0 GOOS=linux go build ./...
GOOS=darwin go vet ./...
CGO_ENABLED=0 GOOS=linux go vet ./...
go test -race -count=1 ./...
golangci-lint run
CGO_ENABLED=0 GOOS=linux golangci-lint run
CGO_ENABLED=0 GOOS=linux go test -count=1 -exec /usr/bin/true ./... >/dev/null
got=$(go test -race -count=1 -v ./... 2>&1 | grep -cE '^\s*--- (PASS|SKIP)')
echo "test count: $got (want >= $want)"
[ "$got" -ge "$want" ]
pkg=.
[ -f main.go ] || pkg=./cmd/urnet-client
go build -o "$base/urnet-client.cur" "$pkg"
bash "$base/golden.sh" "$base/urnet-client.cur" | diff "$base/golden.base.txt" - && echo "golden: identical"
EOF
cat > "$BASE/golden.sh" <<'EOF'
#!/usr/bin/env bash
# Usage: golden.sh <binary>. Prints a timestamp-normalized transcript of CLI output and exit codes.
set -uo pipefail
bin="$1"
home="$(mktemp -d)"
run() {
  echo "## $*"
  URNETWORK_HOME="$home" URNETWORK_JWT= URNETWORK_PASSWORD= URNETWORK_SOCKS_PASS= "$bin" "$@" >"$home/out" 2>"$home/err"
  echo "exit=$?"
  sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:]{8}Z /TS /' "$home/out" "$home/err"
}
run --help
run --version
run bogus-cmd
run vpn --tun=none
run vpn --tun=none --api_url=http://example.com
run vpn --tun=none --config=/nonexistent.yaml
run vpn --tun=none --socks=127.0.0.1:1080 --socks_user=alice
run socks
run save-jwt --jwt=
run open --jwt=x
run locations --jwt=x --api_url=http://127.0.0.1:9
rm -rf "$home"
EOF
cat > "$BASE/linux-test.sh" <<'EOF'
#!/usr/bin/env bash
# Usage: linux-test.sh <min test count>. Runs the race tests in a linux container.
set -euo pipefail
want="${1:?usage: linux-test.sh <min test count>}"
got=$(docker run --rm -v "$PWD":/src -v "$HOME/go/pkg/mod":/go/pkg/mod -w /src golang:1.27 \
  sh -c 'go test -race -count=1 -v ./... 2>&1 | tee /tmp/out >/dev/null; grep -q -- "--- FAIL" /tmp/out && exit 1; grep -cE "^\s*--- (PASS|SKIP)" /tmp/out')
echo "linux test count: $got (want >= $want)"
[ "$got" -ge "$want" ]
EOF
chmod +x "$BASE"/*.sh
```

- [ ] **Step 2: Record the baseline**

```bash
go build -o "$BASE/urnet-client.base" .
bash "$BASE/golden.sh" "$BASE/urnet-client.base" > "$BASE/golden.base.txt"
grep -c '^## ' "$BASE/golden.base.txt"      # expect 11
go test -race -count=1 -v ./... 2>&1 | grep -cE '^\s*--- (PASS|SKIP)'   # expect 214
bash "$BASE/linux-test.sh" 217               # expect "linux test count: 217"
```

- [ ] **Step 3: Move the logger**

```bash
mkdir -p internal/logx
git mv logger.go internal/logx/logx.go
git mv logger_test.go internal/logx/logx_test.go
perl -pi -e 's/^package main$/package logx/; s/\bsetLogLevel\b/SetLogLevel/g; s/\bis(Debug|Info|Warn|Error)Enabled\b/Is$1Enabled/g; s/\blogInfo\b/Info/g; s/\blogWarn\b/Warn/g; s/\blogError\b/Error/g; s/\blogDebug\b/Debug/g' internal/logx/*.go
```

- [ ] **Step 4: Move `setupLogFile` into logx**

Delete the whole `setupLogFile` function (its doc comment and body, the last function in `process.go`) and append to `internal/logx/logx.go`:

```go
// SetupLogFile redirects os.Stdout and os.Stderr to the given file path, appending if it exists.
// The file is created with 0o600 permissions to protect potentially sensitive log content.
func SetupLogFile(path string) error {
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
```

- [ ] **Step 5: Rewrite root call sites and imports**

```bash
perl -pi -e 's/\blogInfo\(/logx.Info(/g; s/\blogWarn\(/logx.Warn(/g; s/\blogError\(/logx.Error(/g; s/\blogDebug\(/logx.Debug(/g; s/\bsetLogLevel\(/logx.SetLogLevel(/g; s/\bis(Debug|Info|Warn|Error)Enabled\(/logx.Is$1Enabled(/g; s/\bsetupLogFile\(/logx.SetupLogFile(/g' *.go
$GOIMPORTS *.go internal/logx
grep -nE '\b(logInfo|logWarn|logError|logDebug|setLogLevel|setupLogFile|is(Debug|Info|Warn|Error)Enabled)\b' *.go internal/logx/*.go   # expect no output
```

- [ ] **Step 6: Run the gate**

Run: `bash "$BASE/gate.sh" 214`
Expected: every command passes, `test count: 214 (want >= 214)`, `golden: identical`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move logger into internal/logx"
```

---

### Task 2: [split] Extract internal/socks

**Files:**
- Move: `socks.go`, `socks_auth.go`, `socks_limits.go` -> `internal/socks/`; tests `socks_accept_test.go`, `socks_auth_test.go`, `socks_bind_test.go`, `socks_exposure_test.go`, `socks_helpers_test.go`, `socks_limits_test.go`, `socks_proto_test.go`, `socks_resolver_test.go`, `socks_test.go`, `socks_udp_test.go` -> `internal/socks/`
- Modify: `config.go`, `config_test.go`, `cmd_socks.go`, `vpn_core.go`, `vpn_core_socks_test.go`

**Interfaces:**
- Consumes: `logx.*` (Task 1).
- Produces (package `socks`): `type SocksOptions struct{...}` (unchanged fields), `func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error)`, `type SocksAuth struct{ User, Pass string }`, `func (a SocksAuth) Enabled() bool`, `func (a SocksAuth) Validate() error`, `func ResolveSocksAuth(flagUser, flagPass string, getenv func(string) string) SocksAuth`.

Rename table: `validate` (method on `SocksAuth`) -> `Validate`, `resolveSocksAuth` -> `ResolveSocksAuth`. Everything else keeps its name.

- [ ] **Step 1: Move the config-parsing test out of the socks test file**

Cut `TestParseConfigs_SocksAuthFromFlagsAndEnv` (the last function in `socks_auth_test.go`) and append it to `config_test.go`, replacing the two unexported constants with literals so it no longer depends on the socks package internals:

```go
func TestParseConfigs_SocksAuthFromFlagsAndEnv(t *testing.T) {
	t.Setenv("URNETWORK_SOCKS_USER", "")
	t.Setenv("URNETWORK_SOCKS_PASS", "from-env")
	opts := docopt.Opts{"--socks_user": "alice"}
	if got := parseVPNConfig(opts, "").SOCKSAuth; got != (SocksAuth{User: "alice", Pass: "from-env"}) {
		t.Fatalf("vpn SOCKSAuth = %+v", got)
	}
	if got := parseSOCKSConfig(opts).Auth; got != (SocksAuth{User: "alice", Pass: "from-env"}) {
		t.Fatalf("socks Auth = %+v", got)
	}
	if got := socksOptionsFromVPN(VPNConfig{SOCKSAuth: SocksAuth{User: "a", Pass: "b"}}, "").Auth; got.User != "a" {
		t.Fatalf("socksOptionsFromVPN dropped auth: %+v", got)
	}
}
```

- [ ] **Step 2: Copy `captureStderr` for the root test that still needs it**

Append to `vpn_core_socks_test.go` (identical to the one in `socks_bind_test.go`, which moves away):

```go
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	s := <-out
	_ = r.Close()
	return s
}
```

- [ ] **Step 3: Move the files and rename**

```bash
mkdir -p internal/socks
git mv socks.go socks_auth.go socks_limits.go internal/socks/
git mv socks_accept_test.go socks_auth_test.go socks_bind_test.go socks_exposure_test.go socks_helpers_test.go socks_limits_test.go socks_proto_test.go socks_resolver_test.go socks_test.go socks_udp_test.go internal/socks/
perl -pi -e 's/^package main$/package socks/; s/\bvalidate\(\)/Validate()/g; s/\bresolveSocksAuth\b/ResolveSocksAuth/g' internal/socks/*.go
perl -pi -e 's/\bSocksAuth\b/socks.SocksAuth/g; s/\bSocksOptions\b/socks.SocksOptions/g; s/\bStartSocks5\(/socks.StartSocks5(/g; s/\bresolveSocksAuth\(/socks.ResolveSocksAuth(/g; s/\.validate\(\)/.Validate()/g' *.go
$GOIMPORTS *.go internal/socks
grep -n 'socks\.socks\.' *.go   # expect no output (no double prefix)
```

- [ ] **Step 4: Run the gate**

Run: `bash "$BASE/gate.sh" 214`
Expected: PASS, count 214, golden identical.

- [ ] **Step 5: Commit the move**

```bash
git add -A
git commit -m "refactor: move SOCKS5 server into internal/socks"
```

- [ ] **Step 6: Apply the two modernize hints**

In `internal/socks/socks_auth.go` replace the loop in `selectSocksMethod`:

```go
func selectSocksMethod(offered []byte, authRequired bool) byte {
	want := byte(socksMethodNoAuth)
	if authRequired {
		want = socksMethodUserPass
	}
	if slices.Contains(offered, want) {
		return want
	}
	return socksMethodNoAcceptable
}
```

In `internal/socks/socks.go`, `handleConnect`, replace the `wg.Add(2)` block and change `relayTCP` to drop the WaitGroup parameter:

```go
	var wg sync.WaitGroup
	wg.Go(func() { relayTCP(rc, c) })
	wg.Go(func() { relayTCP(c, rc) })
	wg.Wait()
}
```

```go
func relayTCP(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if tc, ok := dst.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
```

In `runUDPAssociate` replace the three `wg.Add(1); go func() { defer wg.Done(); ... }()` blocks:

```go
	wg.Go(func() { assoc.closeWhenIdle(ctrl, s.opts.UDPIdleTimeout, done) })
	wg.Go(func() { s.relayFromClient(ctx, pcClient, pcVPN, pcSys, assoc) })
	for _, pc := range conns[1:] {
		wg.Go(func() { relayToClient(pc, pcClient, assoc) })
	}
```

```bash
$GOIMPORTS internal/socks
go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest ./internal/socks/ 2>&1 | grep -E 'slices.Contains|WaitGroup.Go'   # expect no output
```

- [ ] **Step 7: Run the gate and commit**

Run: `bash "$BASE/gate.sh" 214`
Expected: PASS.

```bash
git add -A
git commit -m "refactor: use slices.Contains and WaitGroup.Go in socks"
```

---

### Task 3: [split] Extract internal/netcfg

**Files:**
- Move: `cmdrunner.go`, `cmdrunner_test.go`, `routes.go`, `routes_darwin.go`, `routes_darwin_test.go`, `routes_linux.go`, `routes_linux_test.go`, `dnsbootstrap.go`, `dnsbootstrap_test.go`, `tun_darwin_test.go`, `tun_linux_test.go` -> `internal/netcfg/`
- Create: `internal/netcfg/tun_darwin.go`, `internal/netcfg/tun_linux.go`, `internal/netcfg/host.go`
- Modify: `vpn_darwin.go`, `vpn_linux.go` (keep only `cmdVpn`), `util.go` (remove `runCapture`, `extractHost`)

**Interfaces:**
- Consumes: `logx.*`.
- Produces (package `netcfg`):
  - `type RouteManager interface{...}` (unchanged)
  - darwin: `type DarwinRouteManager struct{...}`, `func NewDarwinRouteManager(tunName, peerIP, defGw string) *DarwinRouteManager` (methods unchanged: `AddBypassEndpoint`, `AddSplitDefault`, `AddScopedDefault`, `AddScopedExclude`, `AddExclude`, `AddExtraRoute`, `AddDNSServerRoutes`, `SetDNS`, `RemoveDNSBypass`, `AddKillSwitchRoute`, `Cleanup`), `func ConfigureDarwinTUN(name, ipCIDR string, mtu int, enableIPv6 bool) (string, error)`, `func DefaultGateway() (string, string, error)`, `func SystemDNSResolvers() ([]string, error)`, `func RemoveDNSBypassWhenWarm(ctx context.Context, rm dnsBypassRemover, pktsIn, pktsOut *uint64, maxWait, tick time.Duration)`
  - linux: `type LinuxRouteManager struct{...}`, `func NewLinuxRouteManager(tunName, origGw, origDev string) *LinuxRouteManager`, `func ConfigureLinuxTUN(name, ipCIDR string, mtu int, enableIPv6 bool) error`, `type DefaultRoute struct{ Gw, Dev string; Metric int }`, `func ListDefaultRoutes() ([]DefaultRoute, error)`
  - unexported, same package: `cmdRunner`, `commandRunner`, `execRunner`, `runCapture`, `runSudo` (darwin), `run` (linux), `extractHost`, `tunCIDRParts`

Rename table: `newDarwinRouteManager`->`NewDarwinRouteManager`, `darwinRouteManager`->`DarwinRouteManager`, `newLinuxRouteManager`->`NewLinuxRouteManager`, `linuxRouteManager`->`LinuxRouteManager`, `removeDNSBypassWhenWarm`->`RemoveDNSBypassWhenWarm`, `configureDarwinTUN`->`ConfigureDarwinTUN`, `configureLinuxTUN`->`ConfigureLinuxTUN`, `getDefaultGateway`->`DefaultGateway`, `getSystemDNSResolvers`->`SystemDNSResolvers`, `linuxListDefaultRoutes`->`ListDefaultRoutes`, `defaultRoute` (type) -> `DefaultRoute`.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/netcfg
git mv cmdrunner.go cmdrunner_test.go routes.go routes_darwin.go routes_darwin_test.go routes_linux.go routes_linux_test.go dnsbootstrap.go dnsbootstrap_test.go tun_darwin_test.go tun_linux_test.go internal/netcfg/
```

- [ ] **Step 2: Move the darwin TUN helpers**

Cut from `vpn_darwin.go` the functions `tunCIDRParts`, `runSudo`, `configureDarwinTUN`, `getDefaultGateway`, `getSystemDNSResolvers` (everything after `cmdVpn`) into a new `internal/netcfg/tun_darwin.go` that starts with:

```go
//go:build darwin

package netcfg
```

followed by the cut functions verbatim.

- [ ] **Step 3: Move the linux TUN helpers with the new signature**

Cut from `vpn_linux.go` the functions `run`, `configureLinuxTUN`, the `defaultRoute` type and `linuxListDefaultRoutes` into `internal/netcfg/tun_linux.go` (`//go:build linux`, `package netcfg`). Replace `configureLinuxTUN` with:

```go
// ConfigureLinuxTUN must succeed before any route points at the TUN; a half-configured
// device would blackhole all routed traffic.
func ConfigureLinuxTUN(name, ipCIDR string, mtu int, enableIPv6 bool) error {
	steps := [][]string{
		{"ip", "addr", "add", ipCIDR, "dev", name},
		{"ip", "link", "set", "dev", name, "mtu", strconv.Itoa(mtu)},
		{"ip", "link", "set", name, "up"},
	}
	for _, s := range steps {
		if err := run(s[0], s[1:]...); err != nil {
			return fmt.Errorf("configure TUN %s (%s): %w", name, strings.Join(s, " "), err)
		}
	}
	if err := run("ip", "addr", "add", "fd00::2/120", "dev", name); err != nil {
		if enableIPv6 {
			return fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logx.Debug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return nil
}
```

In `internal/netcfg/tun_linux_test.go` change the three calls:

```go
	err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, false)
```
```go
	if err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, false); err != nil {
```
```go
	if err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, true); err == nil {
```

- [ ] **Step 4: Move `runCapture` and `extractHost`**

Cut `runCapture` from `util.go` and append it to `internal/netcfg/cmdrunner.go`. Cut `extractHost` from `util.go` into `internal/netcfg/host.go` (`package netcfg`, import `neturl "net/url"` and `strings`).

- [ ] **Step 5: Rename inside netcfg and fix root call sites**

```bash
perl -pi -e 's/^package main$/package netcfg/; s/\bnewDarwinRouteManager\b/NewDarwinRouteManager/g; s/\bdarwinRouteManager\b/DarwinRouteManager/g; s/\bnewLinuxRouteManager\b/NewLinuxRouteManager/g; s/\blinuxRouteManager\b/LinuxRouteManager/g; s/\bremoveDNSBypassWhenWarm\b/RemoveDNSBypassWhenWarm/g; s/\bconfigureDarwinTUN\b/ConfigureDarwinTUN/g; s/\bconfigureLinuxTUN\b/ConfigureLinuxTUN/g; s/\bgetDefaultGateway\b/DefaultGateway/g; s/\bgetSystemDNSResolvers\b/SystemDNSResolvers/g; s/\blinuxListDefaultRoutes\b/ListDefaultRoutes/g; s/\bdefaultRoute\b/DefaultRoute/g' internal/netcfg/*.go
perl -pi -e 's/\bnewDarwinRouteManager\(/netcfg.NewDarwinRouteManager(/g; s/\bnewLinuxRouteManager\(/netcfg.NewLinuxRouteManager(/g; s/\bremoveDNSBypassWhenWarm\(/netcfg.RemoveDNSBypassWhenWarm(/g; s/\bconfigureDarwinTUN\(/netcfg.ConfigureDarwinTUN(/g; s/\bgetDefaultGateway\(/netcfg.DefaultGateway(/g; s/\bgetSystemDNSResolvers\(/netcfg.SystemDNSResolvers(/g; s/\blinuxListDefaultRoutes\(/netcfg.ListDefaultRoutes(/g' vpn_darwin.go vpn_linux.go
perl -pi -e 's/\bconfigureLinuxTUN\(tunName, cfg\)/netcfg.ConfigureLinuxTUN(tunName, cfg.IPCIDR, cfg.MTU, cfg.EnableIPv6)/' vpn_linux.go
$GOIMPORTS *.go internal/netcfg
GOOS=linux $GOIMPORTS vpn_linux.go internal/netcfg/tun_linux.go internal/netcfg/routes_linux.go
grep -nE 'cmdRunner|runCapture|extractHost|runSudo|\brun\(' *.go   # expect no output
```

- [ ] **Step 6: Run the gate and the linux container tests**

Run: `bash "$BASE/gate.sh" 214 && bash "$BASE/linux-test.sh" 217`
Expected: PASS on both, darwin count 214, linux count 217.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move routes and TUN setup into internal/netcfg"
```

---

### Task 4: [split] Extract internal/tunnel

**Files:**
- Move: `vpn_core_test.go` -> `internal/tunnel/filter_test.go`
- Create: `internal/tunnel/device.go`, `internal/tunnel/device_darwin.go`, `internal/tunnel/device_linux.go`, `internal/tunnel/filter.go`, `internal/tunnel/dataplane.go`, `internal/tunnel/dataplane_test.go`
- Modify: `vpn_core.go` (remove filter functions, rewrite `vpnRunCore`), `vpn_darwin.go`, `vpn_linux.go`

**Interfaces:**
- Consumes: `logx.*`, `netcfg.RemoveDNSBypassWhenWarm(ctx, rm, pktsIn, pktsOut *uint64, maxWait, tick)`.
- Produces (package `tunnel`):
  - `type Device interface{ io.ReadWriteCloser; Name() string }`
  - `func Open(name string) (Device, error)` (darwin retries with an auto utun name; linux does not)
  - `type InboundPolicy struct{ ... }`, `func NewInboundPolicy(allowSrcList string, allowLocal bool, tunCIDR string) InboundPolicy`
  - `type Counters struct{ PktsIn, PktsOut, BytesIn, BytesOut uint64 }` (accessed with `sync/atomic`)
  - `func NewDataplane(dev io.ReadWriter, policy InboundPolicy, enableIPv6 bool, c *Counters) *Dataplane`
  - `func (d *Dataplane) Receive(packet []byte)`, `func (d *Dataplane) PumpOutbound(send func(packet []byte))`
  - `func LogStats(ctx context.Context, interval time.Duration, c *Counters)`
  - Root after this task: `func vpnRunCore(ctx context.Context, dev tunnel.Device, tunIfName string, cfg VPNConfig, counters *tunnel.Counters, onBeforeExit func())`

- [ ] **Step 1: Move the filter tests and write the failing dataplane tests**

```bash
mkdir -p internal/tunnel
git mv vpn_core_test.go internal/tunnel/filter_test.go
perl -pi -e 's/^package main$/package tunnel/' internal/tunnel/filter_test.go
```

Create `internal/tunnel/dataplane_test.go`:

```go
package tunnel

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeDev struct {
	mu     sync.Mutex
	writes [][]byte
	reads  chan []byte
}

func (d *fakeDev) Read(p []byte) (int, error) {
	pkt, ok := <-d.reads
	if !ok {
		return 0, io.EOF
	}
	return copy(p, pkt), nil
}

func (d *fakeDev) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = append(d.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (d *fakeDev) written() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.writes)
}

func TestDataplane_ReceiveDropsIPv6WhenDisabled(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c)
	dp.Receive(buildIPv6TCPPacket(net.ParseIP("2001:db8::2"), 0x10))
	if dev.written() != 0 || atomic.LoadUint64(&c.PktsIn) != 0 {
		t.Fatalf("IPv6 packet reached the TUN with --enable_ipv6 off (writes=%d)", dev.written())
	}
}

func TestDataplane_ReceiveAppliesInboundPolicy(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("10.0.0.0/8", false, ""), false, &c)
	dp.Receive(buildTCPPacket(net.ParseIP("8.8.8.8"), 0x02))
	allowed := buildTCPPacket(net.ParseIP("10.1.2.3"), 0x02)
	dp.Receive(allowed)
	if dev.written() != 1 {
		t.Fatalf("writes = %d, want only the allowlisted SYN", dev.written())
	}
	if atomic.LoadUint64(&c.PktsIn) != 1 || atomic.LoadUint64(&c.BytesIn) != uint64(len(allowed)) {
		t.Fatalf("counters = %+v", &c)
	}
}

func TestNewInboundPolicy_DisabledPassesInboundSYN(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c)
	dp.Receive(buildTCPPacket(net.ParseIP("8.8.8.8"), 0x02))
	if dev.written() != 1 {
		t.Fatal("inbound control is off without --allow_inbound_*; the SYN must pass")
	}
}

func TestDataplane_PumpOutboundForwardsUntilReadFails(t *testing.T) {
	dev := &fakeDev{reads: make(chan []byte, 2)}
	dev.reads <- []byte{0x45, 1, 2}
	dev.reads <- []byte{0x45, 1, 2, 3}
	close(dev.reads)
	var c Counters
	var sent [][]byte
	NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c).PumpOutbound(func(p []byte) { sent = append(sent, p) })
	if len(sent) != 2 || atomic.LoadUint64(&c.PktsOut) != 2 || atomic.LoadUint64(&c.BytesOut) != 7 {
		t.Fatalf("sent=%d counters=%+v", len(sent), &c)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go test ./internal/tunnel/`
Expected: FAIL to compile with `undefined: Counters`, `undefined: NewDataplane`, `undefined: NewInboundPolicy`, `undefined: shouldDropInbound`, `undefined: parseCIDRHost`.

- [ ] **Step 3: Move the filter**

Create `internal/tunnel/filter.go` (`package tunnel`, imports `net`, `strings`, `logx`). Cut `shouldDropInbound`, `shouldDropInboundIPv4`, `shouldDropInboundIPv6` and `parseCIDRHost` from `vpn_core.go` into it verbatim, then add:

```go
// InboundPolicy decides which provider-to-TUN TCP packets are dropped before they reach the host.
type InboundPolicy struct {
	enabled bool
	allow   []*net.IPNet
}

// NewInboundPolicy builds the allowlist from --allow_inbound_src and --allow_inbound_local.
func NewInboundPolicy(allowSrcList string, allowLocal bool, tunCIDR string) InboundPolicy {
	p := InboundPolicy{enabled: allowLocal || allowSrcList != ""}
	add := func(cidr string) {
		if n := parseCIDRHost(cidr); n != nil {
			p.allow = append(p.allow, n)
		}
	}
	if allowSrcList != "" {
		for _, s := range strings.Split(allowSrcList, ",") {
			add(s)
		}
	}
	if allowLocal {
		for _, cidr := range []string{"127.0.0.0/8", "169.254.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"} {
			add(cidr)
		}
		if tunCIDR != "" {
			add(tunCIDR)
		}
	}
	if p.enabled && logx.IsInfoEnabled() {
		logx.Info("inbound-control: enabled (allowlist=%d entries); policy: drop new inbound SYN not in allowlist, and drop inbound TCP without ACK\n", len(p.allow))
	}
	return p
}
```

`strings.Split` without trimming is equivalent to the old `splitCSV` here because `parseCIDRHost` trims and returns nil for empty input.

- [ ] **Step 4: Create the device and dataplane files**

`internal/tunnel/device.go`:

```go
package tunnel

import "io"

// Device is the TUN interface the dataplane reads from and writes to.
type Device interface {
	io.ReadWriteCloser
	Name() string
}
```

`internal/tunnel/device_darwin.go`:

```go
//go:build darwin

package tunnel

import (
	"fmt"
	"strings"

	"github.com/songgao/water"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Open creates a utun device named name and falls back to an auto-assigned utun name.
func Open(name string) (Device, error) {
	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = name
	dev, err := water.New(cfg)
	if err != nil {
		if strings.TrimSpace(cfg.Name) != "" {
			logx.Warn("failed to create %s (%v); retrying with auto utun name\n", cfg.Name, err)
			cfg.Name = ""
			dev, err = water.New(cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("create utun failed: %w", err)
		}
	}
	return dev, nil
}
```

`internal/tunnel/device_linux.go`:

```go
//go:build linux

package tunnel

import (
	"fmt"

	"github.com/songgao/water"
)

// Open creates the named TUN device.
func Open(name string) (Device, error) {
	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = name
	dev, err := water.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create TUN failed: %w", err)
	}
	return dev, nil
}
```

`internal/tunnel/dataplane.go`:

```go
package tunnel

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Counters are updated atomically by the dataplane and read by stats and the DNS bootstrap.
type Counters struct {
	PktsIn, PktsOut, BytesIn, BytesOut uint64
}

// Dataplane moves packets between the TUN device and the provider connection.
type Dataplane struct {
	dev        io.ReadWriter
	policy     InboundPolicy
	enableIPv6 bool
	counters   *Counters
}

func NewDataplane(dev io.ReadWriter, policy InboundPolicy, enableIPv6 bool, c *Counters) *Dataplane {
	return &Dataplane{dev: dev, policy: policy, enableIPv6: enableIPv6, counters: c}
}

// Receive filters one packet from the provider and writes it to the TUN.
func (d *Dataplane) Receive(packet []byte) {
	if len(packet) > 0 && packet[0]>>4 == 6 && !d.enableIPv6 {
		if logx.IsDebugEnabled() {
			logx.Debug("dropped IPv6 packet (--enable_ipv6 not set)\n")
		}
		return
	}
	if d.policy.enabled && shouldDropInbound(packet, d.policy.allow) {
		if logx.IsDebugEnabled() {
			logDroppedInbound(packet)
		}
		return
	}
	_, _ = d.dev.Write(packet)
	atomic.AddUint64(&d.counters.PktsIn, 1)
	atomic.AddUint64(&d.counters.BytesIn, uint64(len(packet)))
}

func logDroppedInbound(packet []byte) {
	version := byte(0)
	if len(packet) > 0 {
		version = packet[0] >> 4
	}
	if version == 4 && len(packet) >= 20 {
		ihl := int(packet[0]&0x0F) * 4
		if ihl >= 20 && len(packet) >= ihl+20 && packet[9] == 6 {
			srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
			dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
			srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
			dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
			tcpFlags := packet[ihl+13]
			logx.Debug("dropped inbound TCP %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
		}
	} else if version == 6 && len(packet) >= 60 && packet[6] == 6 {
		srcIP := net.IP(packet[8:24])
		dstIP := net.IP(packet[24:40])
		srcPort := binary.BigEndian.Uint16(packet[40:42])
		dstPort := binary.BigEndian.Uint16(packet[42:44])
		tcpFlags := packet[53]
		logx.Debug("dropped inbound TCP6 %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
	}
}

// PumpOutbound copies packets from the TUN to send until a device read fails.
func (d *Dataplane) PumpOutbound(send func(packet []byte)) {
	buf := make([]byte, 65536)
	for {
		n, err := d.dev.Read(buf)
		if err != nil {
			return
		}
		if n <= 0 {
			continue
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		logx.Debug("-> provider len=%d\n", len(pkt))
		send(pkt)
		atomic.AddUint64(&d.counters.PktsOut, 1)
		atomic.AddUint64(&d.counters.BytesOut, uint64(len(pkt)))
	}
}

// LogStats prints the counters every interval until ctx ends.
func LogStats(ctx context.Context, interval time.Duration, c *Counters) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			inP := atomic.LoadUint64(&c.PktsIn)
			inB := atomic.LoadUint64(&c.BytesIn)
			outP := atomic.LoadUint64(&c.PktsOut)
			outB := atomic.LoadUint64(&c.BytesOut)
			logx.Info("[stats] in=%d pkts / %d bytes, out=%d pkts / %d bytes\n", inP, inB, outP, outB)
		}
	}
}
```

- [ ] **Step 5: Run the tunnel tests to see them pass**

Run: `go test -race ./internal/tunnel/`
Expected: PASS (existing filter tests plus the 4 new ones).

- [ ] **Step 6: Rewrite `vpnRunCore` on top of tunnel**

Replace `vpnRunCore` in `vpn_core.go` from its doc comment down to (not including) `// Optional SOCKS5 proxy bound to the VPN interface` with:

```go
// vpnRunCore selects providers, runs the dataplane between dev and the provider, prints stats
// and serves the optional SOCKS proxy until ctx ends, then runs onBeforeExit.
func vpnRunCore(
	ctx context.Context,
	dev tunnel.Device,
	tunIfName string,
	cfg VPNConfig,
	counters *tunnel.Counters,
	onBeforeExit func(),
) {
	strat, specs := buildProviderSpecs(ctx, cfg.APIURL, cfg.JWT, cfg.Location)
	appVer := fmt.Sprintf("urnet-client %s", Version)
	gen := connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, cfg.APIURL, cfg.JWT, fmt.Sprintf("%s/", cfg.ConnectURL), "", "", appVer, nil,
	)

	policy := tunnel.NewInboundPolicy(cfg.AllowInboundSrcList, cfg.AllowInboundLocal, cfg.IPCIDR)
	dp := tunnel.NewDataplane(dev, policy, cfg.EnableIPv6, counters)
	receive := func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		logx.Debug("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		dp.Receive(packet)
	}
	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen, receive, protocol.ProvideMode_Network)
	go dp.PumpOutbound(func(pkt []byte) {
		mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, pkt, -1)
	})

	if cfg.StatsInterval > 0 && logx.IsInfoEnabled() {
		go tunnel.LogStats(ctx, cfg.StatsInterval, counters)
	}

```

The rest of the function (SOCKS start, "VPN dataplane running" line, wait, cleanup) stays as is.

- [ ] **Step 7: Use `tunnel.Open` and `tunnel.Counters` in the per-OS `cmdVpn`**

`vpn_darwin.go`: replace the block from `// Create TUN device.` through `defer func() { _ = dev.Close() }()` with:

```go
	devName := tunName
	if tunLikelyMissingArg {
		devName = ""
		logx.Warn("--tun provided without a valid name (got %q); using auto utun\n", rawTun)
	}
	dev, err := tunnel.Open(devName)
	if err != nil {
		return err
	}
	defer func() { _ = dev.Close() }()
```

and replace `var pktsIn, bytesIn, pktsOut, bytesOut uint64` with `var counters tunnel.Counters`, the bootstrap line with `go netcfg.RemoveDNSBypassWhenWarm(ctx, rm, &counters.PktsIn, &counters.PktsOut, 3*time.Second, 200*time.Millisecond)`, and the core call with `vpnRunCore(ctx, dev, actualName, cfg, &counters, func() {})`.

`vpn_linux.go`: replace the block from `// Create TUN device.` through `defer func() { _ = dev.Close() }()` with:

```go
	dev, err := tunnel.Open(tunName)
	if err != nil {
		return err
	}
	defer func() { _ = dev.Close() }()
```

and replace `var pktsIn, bytesIn, pktsOut, bytesOut uint64` plus the core call with:

```go
	var counters tunnel.Counters
	vpnRunCore(ctx, dev, tunName, cfg, &counters, func() {})
```

```bash
$GOIMPORTS *.go internal/tunnel
GOOS=linux $GOIMPORTS vpn_linux.go internal/tunnel/device_linux.go
grep -n 'water' *.go   # expect no output: only internal/tunnel imports water
```

- [ ] **Step 8: Run the gate and the linux container tests**

Run: `bash "$BASE/gate.sh" 218 && bash "$BASE/linux-test.sh" 221`
Expected: PASS, darwin count 218, linux count 221, golden identical.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: move TUN device and dataplane into internal/tunnel"
```

---

### Task 5: [split] Extract internal/config

**Files:**
- Move: `config.go` -> `internal/config/config.go`, `config_test.go` -> `internal/config/config_test.go`, `urlcheck.go` -> `internal/config/endpoint.go`, `urlcheck_test.go` -> `internal/config/endpoint_test.go`, `util.go` -> `internal/config/opts.go`, `util_test.go` -> `internal/config/opts_test.go`
- Create: `testhelpers_test.go` (root, later moved to cmd)
- Modify: `main.go` (drop the two URL constants), `cmd_providers.go` (receives `idsToStrings`), `vpn_core_socks_test.go`, every root file using config symbols

**Interfaces:**
- Consumes: `logx.SetLogLevel`, `socks.SocksAuth`, `socks.ResolveSocksAuth`, `(socks.SocksAuth).Validate`.
- Produces (package `config`): `const DefaultAPIURL = "https://api.bringyour.com"`, `const DefaultConnectURL = "wss://connect.bringyour.com"`, `type VPNConfig`, `type SOCKSConfig`, `type LocationConfig`, `type ConfigFile` (fields unchanged), `func ParseLocationConfig(opts docopt.Opts) LocationConfig`, `func ParseVPNConfig(opts docopt.Opts, jwt string) VPNConfig`, `func ParseSOCKSConfig(opts docopt.Opts) SOCKSConfig`, `func ResolveVPNConfig(opts docopt.Opts) (VPNConfig, error)`, `func ValidateEndpointURL(name, raw, secureScheme, devScheme string) error`, `func ValidateEndpointFlags(opts docopt.Opts) error`, `func StringOr(opts docopt.Opts, key, def string) string`, `func IntOr(opts docopt.Opts, key string, def int) int`, `func MustBool(opts docopt.Opts, key string) bool`, `func SplitCSV(s string) []string`.

Rename table: `parseLocationConfig`->`ParseLocationConfig`, `parseVPNConfig`->`ParseVPNConfig`, `parseSOCKSConfig`->`ParseSOCKSConfig`, `resolveVPNConfig`->`ResolveVPNConfig`, `validateEndpointURL`->`ValidateEndpointURL`, `validateEndpointFlags`->`ValidateEndpointFlags`, `getStringOr`->`StringOr`, `getIntOr`->`IntOr`, `mustBool`->`MustBool`, `splitCSV`->`SplitCSV`. Unexported and unchanged: `loadConfigFile`, `applyConfigFile`, `resolveLogLevel`, `isLoopbackHost`.

- [ ] **Step 1: Split the cross-package test**

In `config_test.go`, delete the third `if` block (the `socksOptionsFromVPN` assertion) from `TestParseConfigs_SocksAuthFromFlagsAndEnv`, and add to `vpn_core_socks_test.go`:

```go
func TestSocksOptionsFromVPN_KeepsAuth(t *testing.T) {
	if got := socksOptionsFromVPN(VPNConfig{SOCKSAuth: socks.SocksAuth{User: "a", Pass: "b"}}, "").Auth; got.User != "a" {
		t.Fatalf("socksOptionsFromVPN dropped auth: %+v", got)
	}
}
```

- [ ] **Step 2: Create the root copy of the docopt test helpers**

`testhelpers_test.go` (root, `package main`) with the exact bodies of `vpnTestOpts` and `writeTempConfig` from `config_test.go`, using `config.DefaultAPIURL` and `config.DefaultConnectURL`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

// vpnTestOpts mimics what docopt returns for vpn/quick-connect, including its defaults.
func vpnTestOpts(overrides map[string]interface{}) docopt.Opts {
	o := docopt.Opts{
		"--api_url":             config.DefaultAPIURL,
		"--connect_url":         config.DefaultConnectURL,
		"--ip_cidr":             "10.255.0.2/24",
		"--mtu":                 "1420",
		"--dns_bootstrap":       "bypass",
		"--stats_interval":      "5",
		"--default_route":       false,
		"--debug":               false,
		"--allow_inbound_local": false,
		"--enable_ipv6":         false,
		"--kill_switch":         false,
	}
	for k, v := range overrides {
		o[k] = v
	}
	return o
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
```

- [ ] **Step 3: Move `idsToStrings` out of util.go and the URL constants out of main.go**

Cut `idsToStrings` from `util.go` and paste it at the end of `cmd_providers.go` (temporary home until Task 6). Cut `const DefaultAPIURL = ...` and `const DefaultConnectURL = ...` from `main.go` and paste them at the top of `config.go` after the imports.

- [ ] **Step 4: Move the files and rename**

```bash
mkdir -p internal/config
git mv config.go internal/config/config.go
git mv config_test.go internal/config/config_test.go
git mv urlcheck.go internal/config/endpoint.go
git mv urlcheck_test.go internal/config/endpoint_test.go
git mv util.go internal/config/opts.go
git mv util_test.go internal/config/opts_test.go
perl -pi -e 's/^package main$/package config/; s/\bparseLocationConfig\b/ParseLocationConfig/g; s/\bparseVPNConfig\b/ParseVPNConfig/g; s/\bparseSOCKSConfig\b/ParseSOCKSConfig/g; s/\bresolveVPNConfig\b/ResolveVPNConfig/g; s/\bvalidateEndpointURL\b/ValidateEndpointURL/g; s/\bvalidateEndpointFlags\b/ValidateEndpointFlags/g; s/\bgetStringOr\b/StringOr/g; s/\bgetIntOr\b/IntOr/g; s/\bmustBool\b/MustBool/g; s/\bsplitCSV\b/SplitCSV/g' internal/config/*.go
perl -pi -e 's/\bresolveVPNConfig\(/config.ResolveVPNConfig(/g; s/\bparseVPNConfig\(/config.ParseVPNConfig(/g; s/\bparseSOCKSConfig\(/config.ParseSOCKSConfig(/g; s/\bparseLocationConfig\(/config.ParseLocationConfig(/g; s/\bvalidateEndpointFlags\(/config.ValidateEndpointFlags(/g; s/\bvalidateEndpointURL\(/config.ValidateEndpointURL(/g; s/\bgetStringOr\(/config.StringOr(/g; s/\bgetIntOr\(/config.IntOr(/g; s/\bmustBool\(/config.MustBool(/g; s/\bsplitCSV\(/config.SplitCSV(/g; s/(?<![.\w])(DefaultAPIURL|DefaultConnectURL|VPNConfig|SOCKSConfig|LocationConfig)\b/config.$1/g' *.go
$GOIMPORTS *.go internal/config
GOOS=linux $GOIMPORTS vpn_linux.go
grep -nE 'config\.config\.' *.go   # expect no output
```

`opts.go` keeps its `neturl`/`connect` imports only if still used; goimports drops them (after Task 3 and Step 3 it only needs `strings` and docopt).

- [ ] **Step 5: Run the gate and the linux container tests**

Run: `bash "$BASE/gate.sh" 219 && bash "$BASE/linux-test.sh" 222`
Expected: PASS, darwin count 219 (+1 from Step 1), golden identical.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: move config loading into internal/config"
```

---

### Task 6: [split] Extract internal/urapi (connect anti-corruption layer)

**Files:**
- Move: `api_http.go` -> `internal/urapi/http.go`, `api_http_test.go` -> `internal/urapi/http_test.go`, `specs.go` -> `internal/urapi/specs.go`, `specs_test.go` -> `internal/urapi/specs_test.go`, `main_test.go` -> `internal/urapi/kv_test.go`, `locations_fallback_test.go` -> `internal/urapi/locations_fallback_test.go`, `auth.go` -> `internal/urapi/client.go`
- Create: `internal/urapi/providers.go`, `internal/urapi/transports.go`, `internal/urapi/multiclient.go`
- Modify: `jwt.go` (validateClientJWT leaves), `cmd_open.go`, `cmd_providers.go`, `cmd_locations.go`, `cmd_login.go`, `cmd_auth.go`, `cmd_quickconnect.go`, `vpn_core.go`, `integration_test.go`

**Interfaces:**
- Consumes: `config.LocationConfig`, `config.ValidateEndpointURL`, `logx.*`.
- Produces (package `urapi`):
  - `type LocationsResult struct{ Specs; Groups; Locations }` (was `findLocationsHTTPResult`, fields unchanged)
  - `func FindLocations(ctx context.Context, apiURL, jwt, q string) (*LocationsResult, error)`
  - `func ProviderLocations(ctx context.Context, apiURL, jwt string) (*LocationsResult, error)`
  - `func LocationsFallback(ctx context.Context, apiURL, jwt, q string) (hasSpecs bool, res *LocationsResult)`
  - `type LoginResult struct{ ByJwt, NetworkName string; VerificationRequired bool }`
  - `func LoginWithPassword(ctx context.Context, apiURL, userAuth, password string) (*LoginResult, error)`
  - `func VerifyCode(ctx context.Context, apiURL, userAuth, code string) (string, error)`
  - `func MintClientJWT(ctx context.Context, apiURL, byJwt string) (string, error)`
  - `func ValidateClientJWT(ctx context.Context, apiURL, clientJwt string) bool`
  - `type ProviderSpecs struct{...}`, `func SelectProviders(ctx context.Context, apiURL, jwt string, loc config.LocationConfig) ProviderSpecs`, `func (p ProviderSpecs) BestAvailableOnly() bool`
  - `type Provider struct{ ClientID string; Tier int; EstimatedBytesPerSecond int64; IntermediaryIDs []string }`, `func FindProviders(ctx context.Context, apiURL, jwt string, specs ProviderSpecs, count int, rankMode string) ([]Provider, error)`
  - `func OpenTransports(ctx context.Context, apiURL, connectURL, jwt, clientID string, n int, appVersion string) (func(), error)`
  - `type GeneratorConfig struct{ APIURL, ConnectURL, JWT, AppVersion string; Location config.LocationConfig }`, `type Generator struct{...}`, `func NewGenerator(ctx context.Context, c GeneratorConfig) *Generator`
  - `type MultiClient struct{...}`, `func NewMultiClient(ctx context.Context, gen *Generator, receive func(packet []byte)) *MultiClient`, `func (m *MultiClient) SendPacket(packet []byte)`

Rename table: `findLocationsHTTPResult`->`LocationsResult`, `httpFindLocations`->`FindLocations`, `httpProviderLocations`->`ProviderLocations`, `loginWithPassword`->`LoginWithPassword`, `verifyCode`->`VerifyCode`, `mintClientJWT`->`MintClientJWT`, `validateClientJWT`->`ValidateClientJWT`. Unexported and unchanged: `newByAPI`, `buildProviderSpecs`, `findSpecsByQueryFallback`, `filterLocationsFallback`, `parseKV`, `matchValueFold`, `providerLookupTimeout`, `defaultHTTPClient`, `httpDoer`, `checkAPIRedirect`, `doAPIRequest`, `idsToStrings`.

- [ ] **Step 1: Move the files and delete the dead Authenticator**

```bash
mkdir -p internal/urapi
git mv api_http.go internal/urapi/http.go
git mv api_http_test.go internal/urapi/http_test.go
git mv specs.go internal/urapi/specs.go
git mv specs_test.go internal/urapi/specs_test.go
git mv main_test.go internal/urapi/kv_test.go
git mv locations_fallback_test.go internal/urapi/locations_fallback_test.go
git mv auth.go internal/urapi/client.go
```

In `internal/urapi/client.go` delete everything from `// Authenticator abstracts network authentication operations.` through `var DefaultAuthenticator Authenticator = &apiAuthenticator{}` (the interface, `apiAuthenticator`, its three methods and `DefaultAuthenticator`). Then cut `validateClientJWT` (with its doc comment) from `jwt.go` and append it to `client.go`.

- [ ] **Step 2: Add the new adapters**

`internal/urapi/providers.go`:

```go
package urapi

import (
	"context"

	"github.com/urnetwork/connect"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

// ProviderSpecs is an opaque provider selection built from location options.
type ProviderSpecs struct {
	specs []*connect.ProviderSpec
}

// SelectProviders resolves loc into provider specs, falling back to best-available.
func SelectProviders(ctx context.Context, apiURL, jwt string, loc config.LocationConfig) ProviderSpecs {
	_, specs := buildProviderSpecs(ctx, apiURL, jwt, loc)
	return ProviderSpecs{specs: specs}
}

func (p ProviderSpecs) BestAvailableOnly() bool {
	return len(p.specs) == 1 && p.specs[0].BestAvailable
}

// Provider is one entry of a find-providers response.
type Provider struct {
	ClientID                string
	Tier                    int
	EstimatedBytesPerSecond int64
	IntermediaryIDs         []string
}

func FindProviders(ctx context.Context, apiURL, jwt string, specs ProviderSpecs, count int, rankMode string) ([]Provider, error) {
	api := newByAPI(ctx, apiURL, jwt)
	res, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs.specs, Count: count, RankMode: rankMode})
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(res.Providers))
	for _, p := range res.Providers {
		out = append(out, Provider{
			ClientID:                p.ClientId.String(),
			Tier:                    p.Tier,
			EstimatedBytesPerSecond: int64(p.EstimatedBytesPerSecond),
			IntermediaryIDs:         idsToStrings(p.IntermediaryIds),
		})
	}
	return out, nil
}

// LocationsFallback filters /network/provider-locations client-side for q. hasSpecs reports
// whether any matching group or location id parsed.
func LocationsFallback(ctx context.Context, apiURL, jwt, q string) (hasSpecs bool, res *LocationsResult) {
	specs, res := filterLocationsFallback(ctx, apiURL, jwt, q)
	return len(specs) > 0, res
}
```

Cut `idsToStrings` from the end of `cmd_providers.go` and append it to `providers.go`.

`internal/urapi/transports.go`:

```go
package urapi

import (
	"context"
	"fmt"

	"github.com/urnetwork/connect"
)

// OpenTransports opens n platform transports for clientID. The returned func closes the
// transports in reverse order, then the client.
func OpenTransports(ctx context.Context, apiURL, connectURL, jwt, clientID string, n int, appVersion string) (func(), error) {
	api := newByAPI(ctx, apiURL, jwt)
	strat := connect.NewClientStrategyWithDefaults(ctx)
	id, err := connect.ParseId(clientID)
	if err != nil {
		return nil, err
	}
	oob := connect.NewApiOutOfBandControlWithApi(api)
	client := connect.NewClientWithDefaults(ctx, id, oob)
	clientAuth := &connect.ClientAuth{
		ByJwt:      jwt,
		InstanceId: connect.NewId(),
		AppVersion: fmt.Sprintf("urnet-client %s", appVersion),
	}
	var transports []*connect.PlatformTransport
	for range n {
		transports = append(transports, connect.NewPlatformTransportWithDefaults(ctx, strat, client.RouteManager(), fmt.Sprintf("%s/", connectURL), clientAuth))
	}
	return func() {
		for i := len(transports) - 1; i >= 0; i-- {
			transports[i].Close()
		}
		client.Close()
	}, nil
}
```

`internal/urapi/multiclient.go`:

```go
package urapi

import (
	"context"
	"fmt"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
)

type GeneratorConfig struct {
	APIURL     string
	ConnectURL string
	JWT        string
	AppVersion string
	Location   config.LocationConfig
}

// Generator picks providers for a MultiClient.
type Generator struct {
	g *connect.ApiMultiClientGenerator
}

func NewGenerator(ctx context.Context, c GeneratorConfig) *Generator {
	strat, specs := buildProviderSpecs(ctx, c.APIURL, c.JWT, c.Location)
	appVer := fmt.Sprintf("urnet-client %s", c.AppVersion)
	return &Generator{g: connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, c.APIURL, c.JWT, fmt.Sprintf("%s/", c.ConnectURL), "", "", appVer, nil,
	)}
}

// MultiClient carries IP packets to and from the selected providers.
type MultiClient struct {
	mc *connect.RemoteUserNatMultiClient
}

func NewMultiClient(ctx context.Context, gen *Generator, receive func(packet []byte)) *MultiClient {
	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen.g, func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		logx.Debug("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		receive(packet)
	}, protocol.ProvideMode_Network)
	return &MultiClient{mc: mc}
}

func (m *MultiClient) SendPacket(packet []byte) {
	m.mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, packet, -1)
}
```

- [ ] **Step 3: Rename inside urapi**

```bash
perl -pi -e 's/^package main$/package urapi/; s/\bfindLocationsHTTPResult\b/LocationsResult/g; s/\bhttpFindLocations\b/FindLocations/g; s/\bhttpProviderLocations\b/ProviderLocations/g; s/\bloginWithPassword\b/LoginWithPassword/g; s/\bverifyCode\b/VerifyCode/g; s/\bmintClientJWT\b/MintClientJWT/g; s/\bvalidateClientJWT\b/ValidateClientJWT/g' internal/urapi/*.go
```

- [ ] **Step 4: Rewrite `vpnRunCore` to go through urapi**

In `vpn_core.go` replace the lines from `strat, specs := buildProviderSpecs(...)` through the `go dp.PumpOutbound(...)` block with:

```go
	gen := urapi.NewGenerator(ctx, urapi.GeneratorConfig{
		APIURL:     cfg.APIURL,
		ConnectURL: cfg.ConnectURL,
		JWT:        cfg.JWT,
		AppVersion: Version,
		Location:   cfg.Location,
	})
	policy := tunnel.NewInboundPolicy(cfg.AllowInboundSrcList, cfg.AllowInboundLocal, cfg.IPCIDR)
	dp := tunnel.NewDataplane(dev, policy, cfg.EnableIPv6, counters)
	mc := urapi.NewMultiClient(ctx, gen, dp.Receive)
	go dp.PumpOutbound(mc.SendPacket)
```

The order (provider selection logs, then the inbound-control log, then the multi-client) matches the old code.

- [ ] **Step 5: Rewrite `cmd_providers.go`, `cmd_open.go` and the fallback call in `cmd_locations.go`**

`cmd_providers.go`:

```go
func cmdFindProviders(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	if clientID := parseClientID(jwt); clientID != "" {
		fmt.Printf("client_id: %s\n", clientID)
	}

	count := config.IntOr(opts, "--count", 8)
	rankMode := config.StringOr(opts, "--rank_mode", "quality")

	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	specs := urapi.SelectProviders(qCtx, apiURL, jwt, config.ParseLocationConfig(opts))
	if specs.BestAvailableOnly() {
		fmt.Println("using best-available providers")
	}

	providers, err := urapi.FindProviders(qCtx, apiURL, jwt, specs, count, rankMode)
	if err != nil {
		return err
	}
	for _, p := range providers {
		fmt.Printf("provider client_id=%s tier=%d est_bps=%d intermediaries=%v\n",
			p.ClientID, p.Tier, p.EstimatedBytesPerSecond, p.IntermediaryIDs)
	}
	return nil
}
```

`cmd_open.go`:

```go
func cmdOpen(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	connectURL := config.StringOr(opts, "--connect_url", config.DefaultConnectURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	clientIDStr := parseClientID(jwt)
	if clientIDStr == "" {
		return errors.New("JWT missing client_id (run 'urnet-client mint-client' to mint a client-scoped JWT)")
	}
	closeAll, err := urapi.OpenTransports(ctx, apiURL, connectURL, jwt, clientIDStr, config.IntOr(opts, "--transports", 4), Version)
	if err != nil {
		return err
	}
	defer closeAll()

	fmt.Println("transports opened; press Ctrl-C to exit")
	<-ctx.Done()
	return nil
}
```

`cmd_locations.go`: replace

```go
			fbSpecs, fbRes := filterLocationsFallback(qCtx, apiURL, jwt, q)
```
with
```go
			hasSpecs, fbRes := urapi.LocationsFallback(qCtx, apiURL, jwt, q)
```
and `if len(fbSpecs) == 0 && (` with `if !hasSpecs && (`.

- [ ] **Step 6: Rewrite the remaining root call sites**

```bash
perl -pi -e 's/\b(loginWithPassword|verifyCode|mintClientJWT|validateClientJWT)\(/urapi.\u$1(/g; s/\bhttpFindLocations\(/urapi.FindLocations(/g; s/\bhttpProviderLocations\(/urapi.ProviderLocations(/g; s/\*findLocationsHTTPResult\b/*urapi.LocationsResult/g' *.go
$GOIMPORTS *.go internal/urapi
go list -f '{{.ImportPath}}: {{join .Imports " "}}' . ./internal/... | grep 'urnetwork/connect' | grep -v '/internal/urapi:'   # expect no output
```

The root `integration_test.go` still imports connect directly; it moves in Task 7.

- [ ] **Step 7: Run the gate and the linux container tests**

Run: `bash "$BASE/gate.sh" 219 && bash "$BASE/linux-test.sh" 222`
Expected: PASS, golden identical (the `open --jwt=x` and `locations` transcripts prove the reshaped commands kept their output).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: route all connect usage through internal/urapi"
```

---

### Task 7: [split] Extract internal/auth

**Files:**
- Move: `jwt.go` -> `internal/auth/store.go`, `integration_test.go` -> `internal/urapi/integration_test.go`
- Create: `internal/auth/ensure.go`, `internal/auth/ensure_test.go`
- Modify: `cmd_quickconnect.go`, `cmd_quickconnect_test.go`, `cmd_auth.go`, `cmd_login.go`, `cmd_locations.go`, `cmd_open.go`, `cmd_providers.go`, `cmd_vpn.go`

**Interfaces:**
- Consumes: `urapi.LoginWithPassword`, `urapi.MintClientJWT`, `urapi.ValidateClientJWT`, `logx.*`.
- Produces (package `auth`): `func Path() string`, `func Load(maybe string) (string, error)`, `func Save(jwt string) error`, `func ParseClientID(jwt string) string`, `func EnsureClientJWT(ctx context.Context, apiURL, jwt string, forceJWT bool, userAuth, password string) (string, error)`, `func RenewLoop(ctx context.Context, apiURL, userAuth, password string, interval time.Duration, stop <-chan struct{})`. Unexported: `loginRetryMin`, `loginRetryMax`, `loginRetryBackoff`, `renewOnce`.

Rename table: `jwtPath`->`Path`, `loadJWT`->`Load`, `saveJWT`->`Save`, `parseClientID`->`ParseClientID`, `ensureClientJWT`->`EnsureClientJWT`. `jwtLoadArgForStep2` stays in the CLI.

- [ ] **Step 1: Move the store**

```bash
mkdir -p internal/auth
git mv jwt.go internal/auth/store.go
perl -pi -e 's/^package main$/package auth/; s/\bjwtPath\b/Path/g; s/\bloadJWT\b/Load/g; s/\bsaveJWT\b/Save/g; s/\bparseClientID\b/ParseClientID/g' internal/auth/store.go
```

- [ ] **Step 2: Move `ensureClientJWT` and the retry backoff**

Cut `loginRetryMin`/`loginRetryMax` (the const block), `loginRetryBackoff` and `ensureClientJWT` from `cmd_quickconnect.go` into `internal/auth/ensure.go` (`package auth`), then:

```bash
perl -pi -e 's/\bensureClientJWT\b/EnsureClientJWT/g; s/\bsaveJWT\(/Save(/g; s/\bjwtPath\(/Path(/g; s/\bparseClientID\(/ParseClientID(/g' internal/auth/ensure.go
```

(`urapi.*` and `logx.*` calls are already qualified.)

- [ ] **Step 3: Extract the renewal goroutine into `RenewLoop`**

Append to `internal/auth/ensure.go`:

```go
// RenewLoop re-mints the client JWT every interval until stop is closed. When minting from
// the stored JWT fails and credentials are set, it logs in again first.
func RenewLoop(ctx context.Context, apiURL, userAuth, password string, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renewOnce(ctx, apiURL, userAuth, password)
		case <-stop:
			return
		}
	}
}

func renewOnce(ctx context.Context, apiURL, userAuth, password string) {
	currentJwt, err := Load("")
	if err != nil {
		logx.Warn("jwt renew: no jwt available: %v\n", err)
		return
	}
	clientJwt, mintErr := urapi.MintClientJWT(ctx, apiURL, currentJwt)
	if mintErr == nil {
		saveRenewed(clientJwt)
		return
	}
	if userAuth == "" || password == "" {
		return
	}
	loginRes, loginErr := urapi.LoginWithPassword(ctx, apiURL, userAuth, password)
	if loginErr != nil {
		logx.Warn("jwt renew: login failed: %v\n", loginErr)
		return
	}
	if loginRes.VerificationRequired || loginRes.ByJwt == "" {
		logx.Warn("jwt renew: login requires verification or returned no JWT\n")
		return
	}
	clientJwt2, mintErr2 := urapi.MintClientJWT(ctx, apiURL, loginRes.ByJwt)
	if mintErr2 != nil {
		logx.Warn("jwt renew: mint failed: %v\n", mintErr2)
		return
	}
	saveRenewed(clientJwt2)
}

func saveRenewed(clientJwt string) {
	if err := Save(clientJwt); err != nil {
		logx.Warn("jwt renew: save failed: %v\n", err)
	} else if id := ParseClientID(clientJwt); id != "" {
		logx.Info("jwt renewed (client_id=%s)\n", id)
	} else {
		logx.Info("jwt renewed\n")
	}
}
```

In `cmd_quickconnect.go` replace the whole `if renewInterval > 0 { go func() { ... }() }` block with:

```go
	if renewInterval > 0 {
		go auth.RenewLoop(ctx, apiURL, userAuth, password, renewInterval, stopRenew)
	}
```

- [ ] **Step 4: Move the auth tests**

Cut `TestLoginRetryBackoff`, `fakeClientJWT` and `TestEnsureClientJWT_KeepsValidJWTFromFlag` from `cmd_quickconnect_test.go` into `internal/auth/ensure_test.go` (`package auth`), then:

```bash
perl -pi -e 's/\bensureClientJWT\(/EnsureClientJWT(/g; s/\bsaveJWT\(/Save(/g; s/\bparseClientID\(/ParseClientID(/g' internal/auth/ensure_test.go
```

- [ ] **Step 5: Move and rewrite the integration test**

```bash
git mv integration_test.go internal/urapi/integration_test.go
```

Replace its content with:

```go
package urapi_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

// Integration test (opt-in): requires URNETWORK_TEST_INTEGRATION=1 and a valid JWT in URNETWORK_JWT or ~/.urnetwork/jwt
func TestIntegration_FindLocations_And_FindProviders(t *testing.T) {
	if os.Getenv("URNETWORK_TEST_INTEGRATION") != "1" {
		t.Skip("integration test disabled; set URNETWORK_TEST_INTEGRATION=1 to enable")
	}
	apiURL := config.DefaultAPIURL
	jwt, err := auth.Load(os.Getenv("URNETWORK_JWT"))
	if err != nil {
		t.Skipf("no jwt available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := urapi.FindLocations(ctx, apiURL, jwt, "country:*"); err != nil {
		t.Fatalf("find-locations failed: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel2()
	specs := urapi.SelectProviders(ctx2, apiURL, jwt, config.LocationConfig{})
	if _, err := urapi.FindProviders(ctx2, apiURL, jwt, specs, 1, "quality"); err != nil {
		t.Fatalf("FindProviders2 failed: %v", err)
	}
}
```

- [ ] **Step 6: Rewrite root call sites**

```bash
perl -pi -e 's/\bjwtPath\(/auth.Path(/g; s/\bloadJWT\(/auth.Load(/g; s/\bsaveJWT\(/auth.Save(/g; s/\bparseClientID\(/auth.ParseClientID(/g; s/\bensureClientJWT\(/auth.EnsureClientJWT(/g' *.go
$GOIMPORTS *.go internal/auth internal/urapi
URNETWORK_TEST_INTEGRATION=1 URNETWORK_HOME="$(mktemp -d)" go test -run '^TestIntegration_' -v ./internal/urapi/ 2>&1 | grep -E 'SKIP|PASS|FAIL'   # expect SKIP "no jwt available"
```

- [ ] **Step 7: Run the gate**

Run: `bash "$BASE/gate.sh" 219 && bash "$BASE/linux-test.sh" 222`
Expected: PASS, golden identical.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: move JWT store and refresh into internal/auth"
```

---

### Task 8: [split] Extract internal/session

**Files:**
- Move: `vpn_core.go` -> `internal/session/core.go`, `vpn_core_socks_test.go` -> `internal/session/core_socks_test.go`, `vpn_darwin.go` -> `internal/session/vpn_darwin.go`, `vpn_linux.go` -> `internal/session/vpn_linux.go`, `vpn_stub.go` -> `internal/session/vpn_stub.go`
- Modify: `cmd_vpn.go`, `cmd_quickconnect.go`

**Interfaces:**
- Consumes: `config.VPNConfig`, `config.SplitCSV`, `netcfg.*`, `tunnel.*`, `urapi.NewGenerator`, `urapi.NewMultiClient`, `socks.StartSocks5`, `socks.SocksOptions`, `logx.*`.
- Produces (package `session`): `func Run(ctx context.Context, cfg config.VPNConfig, appVersion string) error` (darwin, linux, stub), `func IsTUNDisabled(name string) bool`. Unexported: `runCore`, `runSocksOnly`, `socksOptionsFromVPN`, `warnIfSocksDNSUnset`, `logStartupConfig`.

Rename table: `cmdVpn`->`Run` (gains `appVersion string`), `vpnRunCore`->`runCore` (gains `appVersion string` after `cfg`), `isTUNDisabled`->`IsTUNDisabled`.

- [ ] **Step 1: Move and rename**

```bash
mkdir -p internal/session
git mv vpn_core.go internal/session/core.go
git mv vpn_core_socks_test.go internal/session/core_socks_test.go
git mv vpn_darwin.go internal/session/vpn_darwin.go
git mv vpn_linux.go internal/session/vpn_linux.go
git mv vpn_stub.go internal/session/vpn_stub.go
perl -pi -e 's/^package main$/package session/; s/\bcmdVpn\b/Run/g; s/\bvpnRunCore\b/runCore/g; s/\bisTUNDisabled\b/IsTUNDisabled/g' internal/session/*.go
```

- [ ] **Step 2: Thread the app version**

In `internal/session/core.go` change the signature and the generator config:

```go
func runCore(
	ctx context.Context,
	dev tunnel.Device,
	tunIfName string,
	cfg config.VPNConfig,
	appVersion string,
	counters *tunnel.Counters,
	onBeforeExit func(),
) {
```
```go
		AppVersion: appVersion,
```

In `vpn_darwin.go` and `vpn_linux.go`: `func Run(ctx context.Context, cfg config.VPNConfig, appVersion string) error {` and `runCore(ctx, dev, actualName, cfg, appVersion, &counters, func() {})` (linux: `tunName`). In `vpn_stub.go`:

```go
func Run(_ context.Context, _ config.VPNConfig, _ string) error {
	return errors.New("vpn is currently supported on Linux only (container) with --cap-add NET_ADMIN and /dev/net/tun")
}
```

- [ ] **Step 3: Update the CLI callers**

```bash
perl -pi -e 's/\bcmdVpn\(ctx, cfg\)/session.Run(ctx, cfg, Version)/; s/\bcmdVpn\(ctx, vpnCfg\)/session.Run(ctx, vpnCfg, Version)/; s/\bisTUNDisabled\(/session.IsTUNDisabled(/g' cmd_vpn.go cmd_quickconnect.go
$GOIMPORTS *.go internal/session
GOOS=linux $GOIMPORTS internal/session/vpn_linux.go
GOOS=windows $GOIMPORTS internal/session/vpn_stub.go
grep -n 'Version' internal/session/*.go   # expect only appVersion / AppVersion
ls *.go   # expect: cmd_*.go main.go process.go process_test.go testhelpers_test.go
```

- [ ] **Step 4: Run the gate**

Run: `bash "$BASE/gate.sh" 219 && bash "$BASE/linux-test.sh" 222`
Expected: PASS, golden identical.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: move VPN session lifecycle into internal/session"
```

---

### Task 9: [split] Move main to cmd/urnet-client and update build paths

**Files:**
- Move: `main.go`, `process.go`, `process_test.go`, `testhelpers_test.go`, `cmd_auth.go`, `cmd_locations.go`, `cmd_login.go`, `cmd_open.go`, `cmd_providers.go`, `cmd_quickconnect.go`, `cmd_quickconnect_test.go`, `cmd_socks.go`, `cmd_vpn.go`, `cmd_vpn_test.go` -> `cmd/urnet-client/`
- Modify: `Makefile`, `Dockerfile`, `README.md`, `docs/quick-start.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `package main` at `github.com/devrandom0/urnetwork-client/cmd/urnet-client` with `var Version = "dev"`; Makefile variable `PKG := ./cmd/urnet-client`; new target `arch-check`. Unchanged targets and outputs: `build`, `build-all`, `build-{linux,darwin}-{amd64,arm64}` -> `dist/<os>_<arch>/urnet-client`, `version-check`, `test-race`, `lint`, `test-integration`, `ci-docker-build`.

- [ ] **Step 1: Move the CLI**

```bash
mkdir -p cmd/urnet-client
git mv main.go process.go process_test.go testhelpers_test.go cmd_*.go cmd/urnet-client/
ls *.go 2>/dev/null   # expect no output
go build ./... && go test ./cmd/urnet-client/
```

- [ ] **Step 2: Update the Makefile**

After `LDFLAGS := ...` add `PKG := ./cmd/urnet-client`. Then:

- `build`: `-o $(DIST)/$(BINARY) ./` -> `-o $(DIST)/$(BINARY) $(PKG)`
- `version-check`: `-o $$tmp/$(BINARY) .` -> `-o $$tmp/$(BINARY) $(PKG)`
- `build-linux-amd64`, `build-linux-arm64`, `build-darwin-amd64`, `build-darwin-arm64`: trailing ` .` -> ` $(PKG)`
- `test-integration`: `go test -v -run '^TestIntegration_' ./` -> `go test -v -run '^TestIntegration_' ./internal/urapi/`
- `lint`: add a last line `$(MAKE) arch-check`
- help: add `@echo "  arch-check            Verify only internal/urapi imports github.com/urnetwork/connect"`
- new target:

```make
.PHONY: arch-check
arch-check:
	@for os in darwin linux; do \
	  bad=$$(CGO_ENABLED=0 GOOS=$$os go list -f '{{.ImportPath}}: {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... \
	    | grep 'github.com/urnetwork/connect' | grep -v '^github.com/devrandom0/urnetwork-client/internal/urapi:'); \
	  if [ -n "$$bad" ]; then echo "arch-check ($$os): only internal/urapi may import github.com/urnetwork/connect:"; echo "$$bad"; exit 1; fi; \
	done; \
	echo "arch-check: ok"
```

`LDFLAGS` keeps `-X 'main.Version=$(VERSION)'`.

- [ ] **Step 3: Update the Dockerfile**

Replace:

```dockerfile
# Copy only Go source at repo root to avoid cache busts from docs/CI edits.
# If you later add subpackages or assets, adjust this list or use a more
# selective .dockerignore include strategy.
COPY *.go ./

# Build the CLI from repository root (main.go is at root)
```

with:

```dockerfile
# Copy only Go source to avoid cache busts from docs/CI edits.
COPY cmd ./cmd
COPY internal ./internal

# Build the CLI (main package lives in cmd/urnet-client)
```

change `-o /out/urnet-client ./` to `-o /out/urnet-client ./cmd/urnet-client`, and in the runtime comment `needed by vpn_linux.go` to `needed by internal/netcfg`.

- [ ] **Step 4: Update the docs**

In `README.md` and `docs/quick-start.md`: `go build -o dist/urnet-client ./` -> `go build -o dist/urnet-client ./cmd/urnet-client`.

```bash
grep -rn 'go build\|\*\.go\|main\.go' README.md docs/*.md Dockerfile Makefile   # every hit must use ./cmd/urnet-client or $(PKG)
```

- [ ] **Step 5: Verify build paths, version injection, architecture and image**

```bash
make version-check          # expect "version-check: ok"
make arch-check             # expect "arch-check: ok"
make lint                   # expect clean, ends with "arch-check: ok"
make build-all VERSION=9.9.9 && dist/darwin_arm64/urnet-client --version   # expect 9.9.9 (host arm64)
make ci-docker-build        # expect a successful image build
docker run --rm test/urnetwork-client:ci --version   # expect a version string, exit 0
make clean
```

- [ ] **Step 6: Run the gate and the linux container tests**

Run: `bash "$BASE/gate.sh" 219 && bash "$BASE/linux-test.sh" 222`
Expected: PASS; the gate builds `./cmd/urnet-client` now and the golden transcript is identical to the one from `93f01f8`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move CLI entrypoint to cmd/urnet-client"
```

---

### Task 10: [ci] Add macOS test and lint job

Runs in `../urnetwork-client-phase1_ci` on `sina/phase1_ci`. Parallel with Tasks 1 to 9.

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: nothing from the split stream (uses `go test ./...`, independent of layout).
- Produces: job id `test-macos`; the `release` job needs it.

- [ ] **Step 1: Add the job**

In `.github/workflows/ci.yml`, after the `test` job, add:

```yaml
  test-macos:
    runs-on: macos-latest
    needs: lint
    name: Unit tests and lint (macOS)
    steps:
      - name: Checkout
        uses: actions/checkout@v7
      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
          cache: true
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.14.0
      - name: Test
        run: go test -race -count=1 ./...
```

and change the `release` job's `needs: [test, build-cli]` to `needs: [test, test-macos, build-cli]`.

- [ ] **Step 2: Lint the workflow**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: run tests and lint on macOS"
```

---

### Task 11: [ci] Build release assets after the version is known

**Files:**
- Create: `scripts/stage-release-assets.sh`, `scripts/test-release-prepare.sh`
- Modify: `.releaserc.json`, `.github/workflows/ci.yml` (`release` and `build-cli` jobs)

**Interfaces:**
- Consumes: `make build-all VERSION=<v>` producing `dist/{linux,darwin}_{amd64,arm64}/urnet-client` (exists on both branches; Task 9 keeps it).
- Produces: `.releaserc.json` `@semantic-release/exec` entry with `prepareCmd`; `scripts/stage-release-assets.sh <version>` staging `dist/urnet-client_<os>_<arch>`.

Root cause: `build-cli` runs before `release`, uses `git describe` (the previous tag) for `VERSION`, and `release` uploads those artifacts, so every released binary reports the previous version. Building inside semantic-release's `prepare` step uses `nextRelease.version` and runs before tag creation and the GitHub `publish` upload.

- [ ] **Step 1: Write the staging script**

`scripts/stage-release-assets.sh`:

```bash
#!/usr/bin/env bash
# Stage per-platform release assets from dist/<os>_<arch>/ and prove the host binary reports the release version.
set -euo pipefail
version="${1:?usage: stage-release-assets.sh <version>}"
host="$(go env GOOS)_$(go env GOARCH)"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  src="dist/$p/urnet-client"
  dst="dist/urnet-client_$p"
  [ -f "$src" ] || { echo "missing $src" >&2; exit 1; }
  cp "$src" "$dst"
  if [ "$p" = "$host" ]; then
    got="$("./$dst" --version)"
    [ "$got" = "$version" ] || { echo "$dst reports '$got', want '$version'" >&2; exit 1; }
  fi
done
echo "staged release assets for $version"
```

- [ ] **Step 2: Write the test for the release prepare command**

`scripts/test-release-prepare.sh`:

```bash
#!/usr/bin/env bash
# Runs the semantic-release prepareCmd from .releaserc.json with a fake version and checks the staged assets.
set -euo pipefail
cd "$(dirname "$0")/.."
version="0.0.0-prepare-test"
cmd=$(jq -r '.plugins[] | select(type == "array" and .[0] == "@semantic-release/exec") | .[1].prepareCmd' .releaserc.json)
if [ -z "$cmd" ] || [ "$cmd" = "null" ]; then
  echo "no @semantic-release/exec prepareCmd in .releaserc.json" >&2
  exit 1
fi
rm -rf dist
bash -c "$(printf '%s' "$cmd" | sed "s/\${nextRelease.version}/$version/g")"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  [ -f "dist/urnet-client_$p" ] || { echo "missing dist/urnet-client_$p" >&2; exit 1; }
done
rm -rf dist
echo "release prepare: ok"
```

```bash
chmod +x scripts/stage-release-assets.sh scripts/test-release-prepare.sh
```

- [ ] **Step 3: Run the test to see it fail**

Run: `scripts/test-release-prepare.sh`
Expected: FAIL with `no @semantic-release/exec prepareCmd in .releaserc.json`.

- [ ] **Step 4: Add the exec plugin**

In `.releaserc.json`, insert before the `@semantic-release/git` entry:

```json
    ["@semantic-release/exec", {
      "prepareCmd": "make build-all VERSION=${nextRelease.version} && scripts/stage-release-assets.sh ${nextRelease.version}"
    }],
```

- [ ] **Step 5: Run the test to see it pass**

Run: `scripts/test-release-prepare.sh`
Expected: `staged release assets for 0.0.0-prepare-test` then `release prepare: ok`.

- [ ] **Step 6: Update the workflow**

In the `release` job: delete the `Download CLI artifacts` step; add before `Setup Node.js`:

```yaml
      - name: Set up Go
        if: steps.skip_check.outputs.skip != 'true'
        uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
          cache: true
```

and add `@semantic-release/exec` as the first line of `extra_plugins`. In the `build-cli` job add a last step:

```yaml
      - name: Release prepare command builds versioned assets
        run: scripts/test-release-prepare.sh
```

(`build-cli` keeps uploading its own artifacts for `get-jwt`; they are no longer release assets.)

- [ ] **Step 7: Lint and commit**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml && jq . .releaserc.json >/dev/null`
Expected: exit 0, no output.

```bash
git add scripts/stage-release-assets.sh scripts/test-release-prepare.sh .releaserc.json .github/workflows/ci.yml
git commit -m "ci: build release assets after the version is known"
```

---

### Task 12: [ci] Build Docker images with the release version

**Files:**
- Modify: `.github/workflows/ci.yml` (`docker-build`, `docker` jobs)

**Interfaces:**
- Consumes: `release` job outputs `released`, `version`, `tag` (existing).
- Produces: images whose binary reports the release version; `publish-images-on-release` keeps retagging the digest from `docker`.

Root cause: `docker-build` runs in parallel with `release` and passes `VERSION=${{ steps.meta.outputs.version }}`, which is the branch name (`main`) on a main push. `publish-images-on-release` then retags that digest as `2.x.y`, so the released image prints `main`.

- [ ] **Step 1: Make `docker-build` wait for `release` and use its version**

```yaml
  docker-build:
    ...
    needs: [test, release]
    if: always() && github.event_name != 'pull_request' && needs.test.result == 'success' && (needs.release.result == 'success' || needs.release.result == 'skipped')
```

and in its `Build and push by digest` step:

```yaml
          build-args: |
            BUILDKIT_INLINE_CACHE=1
            VERSION=${{ needs.release.outputs.version || steps.meta.outputs.version }}
```

`release` is skipped on `workflow_dispatch` runs (and when HEAD is already tagged it succeeds with empty outputs); `always()` plus the explicit result checks keep `docker-build` running there with the metadata version, as today. Tag pushes are unchanged: `lint` and therefore `test` are skipped for them, so `docker-build` stays skipped exactly as before.

- [ ] **Step 2: Keep the manifest job running when `release` was skipped**

In the `docker` job replace `if: github.event_name != 'pull_request'` with:

```yaml
    if: always() && github.event_name != 'pull_request' && needs.docker-build.result == 'success'
```

A skipped ancestor (`release` on tag pushes) otherwise skips every downstream job that uses the default `success()` condition.

- [ ] **Step 3: Lint and check the expressions**

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml
grep -n 'needs.release.outputs.version || steps.meta.outputs.version' .github/workflows/ci.yml   # expect 1 hit
```

Expected: actionlint exit 0.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: build docker images with the release version"
```

---

### Task 13: [integration] Merge the CI stream into the split branch

**Files:**
- Merge only. No file owned by both streams, so no conflicts are expected.

**Interfaces:**
- Consumes: `sina/phase1_ci` (Tasks 10 to 12), `sina/phase1_package_split` (Tasks 1 to 9).
- Produces: integration branch with both.

- [ ] **Step 1: Merge**

```bash
cd /Users/sinamoghaddas/projects/my-projects/devrandom0/urnetwork-client
git switch sina/phase1_package_split
git merge --no-ff sina/phase1_ci -m "ci: merge macOS job and release version fixes"
git diff --name-only HEAD^1 HEAD   # expect only ci.yml, .releaserc.json, scripts/stage-release-assets.sh, scripts/test-release-prepare.sh
```

If git reports a conflict, stop: a file-ownership rule was broken. Resolve by taking the owner stream's version of that file.

- [ ] **Step 2: Verify the combined tree**

```bash
bash "$BASE/gate.sh" 219
scripts/test-release-prepare.sh          # now builds ./cmd/urnet-client via make build-all; expect "release prepare: ok"
make version-check && make arch-check
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml
git worktree remove ../urnetwork-client-phase1_ci
```

Expected: all pass.

---

### Task 14: [hygiene] Run privileged tools from fixed system paths

**Files:**
- Create: `internal/netcfg/toolpath.go`, `internal/netcfg/toolpath_test.go`
- Modify: `internal/netcfg/cmdrunner.go` (`execRunner`), `internal/netcfg/tun_darwin.go` (`DefaultGateway`), `internal/netcfg/tun_darwin_test.go`, `docs/platform-notes.md`

**Interfaces:**
- Consumes: `cmdRunner`, `runCapture`, test helper `useFakeRunner` (all in netcfg).
- Produces (unexported): `var systemToolDirs = []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}`, `type toolLookup struct{ dirs []string; stat func(string) (fs.FileInfo, error); mu sync.Mutex; cache map[string]toolResult }`, `func (l *toolLookup) resolve(name string) (string, error)`, `var tools *toolLookup`.

Root cause: `exec.Command("ip", ...)` resolves the name through the caller's `$PATH`. Under `sudo -E`, or any root run with a user-controlled `PATH`, a planted `ip`/`route` binary runs as root. The fix resolves the bare tool name once against a fixed list of root-owned directories inside `execRunner`, so every call site (and the fake runner used in tests) keeps passing bare names.

- [ ] **Step 1: Write the failing tests**

`internal/netcfg/toolpath_test.go`:

```go
package netcfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeTool(t *testing.T, dir, name string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func newTestLookup(dirs ...string) *toolLookup {
	return &toolLookup{dirs: dirs, stat: os.Stat, cache: map[string]toolResult{}}
}

func TestSystemToolDirs_Order(t *testing.T) {
	want := []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}
	if !slices.Equal(systemToolDirs, want) {
		t.Fatalf("systemToolDirs = %v, want %v", systemToolDirs, want)
	}
}

func TestToolLookup_FirstDirWins(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTool(t, a, "route", 0o755)
	writeTool(t, b, "route", 0o755)
	got, err := newTestLookup(a, b).resolve("route")
	if err != nil || got != filepath.Join(a, "route") {
		t.Fatalf("resolve = %q, %v; want %s", got, err, filepath.Join(a, "route"))
	}
}

func TestToolLookup_NeverUsesPATH(t *testing.T) {
	onPath := t.TempDir()
	writeTool(t, onPath, "ip", 0o755)
	t.Setenv("PATH", onPath)
	if got, err := newTestLookup(t.TempDir()).resolve("ip"); err == nil {
		t.Fatalf("resolve found %q via PATH; privileged tools must come from fixed dirs only", got)
	}
}

func TestToolLookup_SkipsNonExecutableAndDirectories(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	writeTool(t, a, "scutil", 0o644)
	if err := os.Mkdir(filepath.Join(b, "scutil"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTool(t, c, "scutil", 0o755)
	got, err := newTestLookup(a, b, c).resolve("scutil")
	if err != nil || got != filepath.Join(c, "scutil") {
		t.Fatalf("resolve = %q, %v; want the executable regular file in %s", got, err, c)
	}
}

func TestToolLookup_ResolvesOnce(t *testing.T) {
	a := t.TempDir()
	writeTool(t, a, "ifconfig", 0o755)
	n := 0
	l := &toolLookup{dirs: []string{a}, stat: func(p string) (fs.FileInfo, error) { n++; return os.Stat(p) }, cache: map[string]toolResult{}}
	for range 3 {
		if _, err := l.resolve("ifconfig"); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("stat called %d times, want 1", n)
	}
}

func TestToolLookup_RejectsNamesWithSlash(t *testing.T) {
	if _, err := newTestLookup("/bin").resolve("/bin/sh"); err == nil {
		t.Fatal("a path must not bypass the fixed directory list")
	}
}
```

Append to `internal/netcfg/tun_darwin_test.go`:

```go
func TestDefaultGateway_UsesCommandRunner(t *testing.T) {
	f := useFakeRunner(t)
	f.results["route -n get default"] = fakeResult{out: "   route to: default\ndestination: default\n    gateway: 192.168.1.1\n  interface: en0\n"}
	gw, iface, err := DefaultGateway()
	if err != nil || gw != "192.168.1.1" || iface != "en0" {
		t.Fatalf("DefaultGateway = %q, %q, %v; want the scripted gateway via the command runner", gw, iface, err)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go test ./internal/netcfg/ -run 'ToolLookup|SystemToolDirs|DefaultGateway'`
Expected: FAIL to compile (`undefined: toolLookup`, `undefined: systemToolDirs`). After Step 3, `TestDefaultGateway_UsesCommandRunner` still fails until Step 4 (it runs the real `route`).

- [ ] **Step 3: Implement the lookup**

`internal/netcfg/toolpath.go`:

```go
package netcfg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// systemToolDirs are searched in order; $PATH is never consulted, so the caller's environment
// cannot substitute the binary a root-run command executes. The last entry is NixOS's system profile.
var systemToolDirs = []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}

type toolResult struct {
	path string
	err  error
}

type toolLookup struct {
	dirs  []string
	stat  func(string) (fs.FileInfo, error)
	mu    sync.Mutex
	cache map[string]toolResult
}

var tools = &toolLookup{dirs: systemToolDirs, stat: os.Stat, cache: map[string]toolResult{}}

func (l *toolLookup) resolve(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '/') {
		return "", fmt.Errorf("tool name %q must be a bare command name", name)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.cache[name]; ok {
		return r.path, r.err
	}
	r := toolResult{err: fmt.Errorf("%s not found in %s", name, strings.Join(l.dirs, ", "))}
	for _, d := range l.dirs {
		p := filepath.Join(d, name)
		if fi, err := l.stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			r = toolResult{path: p}
			break
		}
	}
	l.cache[name] = r
	return r.path, r.err
}
```

In `internal/netcfg/cmdrunner.go` make `execRunner` resolve first:

```go
func (execRunner) Run(name string, args ...string) error {
	path, err := tools.resolve(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (execRunner) Capture(name string, args ...string) (string, error) {
	path, err := tools.resolve(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	return string(out), err
}
```

- [ ] **Step 4: Route `DefaultGateway` through the runner**

In `internal/netcfg/tun_darwin.go` replace the first lines of `DefaultGateway`:

```go
func DefaultGateway() (string, string, error) {
	out, err := runCapture("route", "-n", "get", "default")
	if err != nil {
		return "", "", fmt.Errorf("route get default failed: %w", err)
	}
	var gw, iface string
	for _, line := range strings.Split(out, "\n") {
```

(the rest of the parsing is unchanged; drop the `os/exec` import).

- [ ] **Step 5: Run the tests to see them pass**

```bash
go test -race ./internal/netcfg/
grep -rn 'exec.Command' internal/ | grep -v '_test.go'   # expect only the two in cmdrunner.go, both using path
```

Expected: PASS.

- [ ] **Step 6: Document**

Append to `docs/platform-notes.md`:

```markdown
## Privileged tools

`route`, `ifconfig`, `networksetup`, `scutil` (macOS) and `ip` (Linux) are run by absolute path. Each is looked up once, in this order, and `PATH` is never used: `/sbin`, `/usr/sbin`, `/bin`, `/usr/bin`, `/run/current-system/sw/bin`. If a tool is in none of them the command fails with `<tool> not found in ...`.
```

- [ ] **Step 7: Run the gate and commit**

Run: `bash "$BASE/gate.sh" 226 && bash "$BASE/linux-test.sh" 228`
Expected: PASS (darwin +7, linux +6). The golden transcript stays identical (no golden command runs a tool).

```bash
git add -A
git commit -m "fix: run privileged tools from fixed system paths"
```

---

### Task 15: [hygiene] Add internal/safefile for symlink-safe writes

**Files:**
- Create: `internal/safefile/safefile.go`, `internal/safefile/safefile_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (package `safefile`): `func WriteFile(path string, data []byte) error` (atomic replace, mode 0600), `func OpenAppend(path string) (*os.File, error)` (append, create 0600). Test seams (unexported vars): `geteuid func() int`, `fileOwner func(fs.FileInfo) (uid, gid uint32, ok bool)`, `fchown func(f *os.File, uid, gid int) error`.

Rules (ruling 14): the directory path is resolved with `filepath.EvalSymlinks` and used from then on; the resolved directory must be owned by the current euid or root, unless euid is 0; it must not be group/world-writable unless sticky; the target must not be a symlink or non-regular file. WriteFile uses `os.CreateTemp` (O_EXCL, 0600) plus rename. OpenAppend opens with `O_NOFOLLOW` and refuses hard-linked files and files not owned by the euid or the directory owner. When euid is 0 and the directory owner is not root, newly created files are chowned to the directory owner.

- [ ] **Step 1: Write the failing tests**

`internal/safefile/safefile_test.go`:

```go
package safefile

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func useEUID(t *testing.T, uid int) {
	t.Helper()
	old := geteuid
	geteuid = func() int { return uid }
	t.Cleanup(func() { geteuid = old })
}

func useOwner(t *testing.T, owner func(fs.FileInfo) (uint32, uint32, bool)) {
	t.Helper()
	old := fileOwner
	fileOwner = owner
	t.Cleanup(func() { fileOwner = old })
}

func recordChown(t *testing.T) *[][2]int {
	t.Helper()
	var calls [][2]int
	old := fchown
	fchown = func(_ *os.File, uid, gid int) error { calls = append(calls, [2]int{uid, gid}); return nil }
	t.Cleanup(func() { fchown = old })
	return &calls
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteFile_CreatesWith0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt")
	if err := WriteFile(path, []byte("tok\n")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 || mustRead(t, path) != "tok\n" {
		t.Fatalf("mode=%v err=%v", fi.Mode(), err)
	}
}

func TestWriteFile_ReplacesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	fi, _ := os.Stat(path)
	if mustRead(t, path) != "new" || fi.Mode().Perm() != 0o600 || len(entries) != 1 {
		t.Fatalf("content=%q mode=%v entries=%d", mustRead(t, path), fi.Mode(), len(entries))
	}
}

func TestWriteFile_RefusesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "jwt")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(link, []byte("pwned")); err == nil {
		t.Fatal("WriteFile replaced a symlinked target")
	}
	if mustRead(t, victim) != "keep" {
		t.Fatal("symlink target was modified")
	}
}

func TestWriteFile_RefusesDirOwnedByAnotherUser(t *testing.T) {
	useEUID(t, 4242)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 4343, 4343, true })
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err == nil {
		t.Fatal("wrote into a directory owned by another non-root user")
	}
}

func TestWriteFile_RefusesGroupWritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(dir, "jwt"), []byte("x")); err == nil {
		t.Fatal("wrote into a group-writable directory without the sticky bit")
	}
}

func TestWriteFile_AllowsRootOwnedStickyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o1777); err != nil {
		t.Fatal(err)
	}
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 0, 0, true })
	if err := WriteFile(filepath.Join(dir, "log"), []byte("x")); err != nil {
		t.Fatalf("a root-owned sticky dir like /tmp must be allowed: %v", err)
	}
}

func TestWriteFile_FollowsSymlinkedParentDir(t *testing.T) {
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(linkDir, "jwt"), []byte("x")); err != nil {
		t.Fatalf("a symlinked directory such as macOS /tmp must work: %v", err)
	}
	if mustRead(t, filepath.Join(realDir, "jwt")) != "x" {
		t.Fatal("file not written into the resolved directory")
	}
}

func TestWriteFile_RootChownsToDirOwner(t *testing.T) {
	useEUID(t, 0)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 1234, 5678, true })
	calls := recordChown(t)
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != [2]int{1234, 5678} {
		t.Fatalf("chown calls = %v; a sudo run must leave the file owned by the directory owner", *calls)
	}
}

func TestWriteFile_RootInRootDirDoesNotChown(t *testing.T) {
	useEUID(t, 0)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 0, 0, true })
	calls := recordChown(t)
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("chown calls = %v, want none", *calls)
	}
}

func TestOpenAppend_CreatesAndAppends0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "urnet.log")
	for _, s := range []string{"a\n", "b\n"} {
		f, err := OpenAppend(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	fi, _ := os.Stat(path)
	if mustRead(t, path) != "a\nb\n" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("content=%q mode=%v", mustRead(t, path), fi.Mode())
	}
}

func TestOpenAppend_RefusesSymlink(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(link); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend followed a symlink")
	}
}

func TestOpenAppend_RefusesHardLinkedFile(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig")
	if err := os.WriteFile(orig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "urnet.log")
	if err := os.Link(orig, link); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(link); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a file with two hard links")
	}
}

func TestOpenAppend_RefusesNonRegularFile(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "urnet.log")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(fifo); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a FIFO")
	}
}

func TestOpenAppend_RefusesFileOwnedByAnotherUser(t *testing.T) {
	euid := uint32(os.Geteuid())
	useOwner(t, func(fi fs.FileInfo) (uint32, uint32, bool) {
		if fi.IsDir() {
			return euid, euid, true
		}
		return 4343, 4343, true
	})
	path := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(path); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a file owned by another user")
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go test ./internal/safefile/`
Expected: FAIL to compile (`undefined: WriteFile`, `undefined: OpenAppend`, `undefined: geteuid`).

- [ ] **Step 3: Implement**

`internal/safefile/safefile.go`:

```go
// Package safefile writes files that may sit in directories another user controls, such as
// a user's home during a sudo run, without following symlinks planted there.
package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

var (
	geteuid   = os.Geteuid
	fileOwner = statOwner
	fchown    = func(f *os.File, uid, gid int) error { return f.Chown(uid, gid) }
)

func statOwner(fi fs.FileInfo) (uid, gid uint32, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Uid, st.Gid, true
}

type dirInfo struct {
	path     string
	uid, gid uint32
}

// safeDir refuses a directory in which someone other than the current user or root could
// swap the target between our checks and our write.
func safeDir(dir string) (dirInfo, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dirInfo{}, err
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return dirInfo{}, err
	}
	if !fi.IsDir() {
		return dirInfo{}, fmt.Errorf("%s is not a directory", dir)
	}
	uid, gid, ok := fileOwner(fi)
	if !ok {
		return dirInfo{}, fmt.Errorf("%s: cannot determine owner", dir)
	}
	euid := geteuid()
	if euid != 0 && uid != 0 && int(uid) != euid {
		return dirInfo{}, fmt.Errorf("%s is owned by uid %d, not the current user (uid %d); refusing to write there", dir, uid, euid)
	}
	if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&fs.ModeSticky == 0 {
		return dirInfo{}, fmt.Errorf("%s is writable by group or others; refusing to write there", dir)
	}
	return dirInfo{path: resolved, uid: uid, gid: gid}, nil
}

// chownForRoot keeps a file created by a sudo run usable by the owner of the directory it is in.
func chownForRoot(f *os.File, d dirInfo) error {
	if geteuid() != 0 || d.uid == 0 {
		return nil
	}
	return fchown(f, int(d.uid), int(d.gid))
}

// WriteFile replaces path with data (mode 0600) through a temp file and rename, so readers
// never see a partial file and an existing symlink at path is never followed.
func WriteFile(path string, data []byte) error {
	d, err := safeDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	target := filepath.Join(d.path, filepath.Base(path))
	if fi, err := os.Lstat(target); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is a symlink or special file; refusing to replace it", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(d.path, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := fillTemp(tmp, data, d); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func fillTemp(f *os.File, data []byte, d dirInfo) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := chownForRoot(f, d); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// OpenAppend opens path for appending and creates it with mode 0600. It refuses symlinks,
// special files, hard-linked files and files owned by anyone but the current user or the
// directory owner.
func OpenAppend(path string) (*os.File, error) {
	d, err := safeDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	target := filepath.Join(d.path, filepath.Base(path))
	fi, err := os.Lstat(target)
	existed := err == nil
	if existed && !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is a symlink or special file; refusing to open it", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, path, d, existed); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func checkOpened(f *os.File, path string, d dirInfo, existed bool) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return fmt.Errorf("%s has %d hard links; refusing to append to it", path, st.Nlink)
	}
	uid, _, ok := fileOwner(fi)
	if !ok {
		return fmt.Errorf("%s: cannot determine owner", path)
	}
	if int(uid) != geteuid() && uid != d.uid {
		return fmt.Errorf("%s is owned by uid %d; refusing to append to it", path, uid)
	}
	if !existed {
		return chownForRoot(f, d)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to see them pass**

Run: `go test -race -v ./internal/safefile/ 2>&1 | grep -cE '^--- PASS'`
Expected: `14`.

- [ ] **Step 5: Run the gate and the linux container tests, then commit**

Run: `bash "$BASE/gate.sh" 240 && bash "$BASE/linux-test.sh" 242`
Expected: PASS. In the linux container the tests run as root; the seams make them independent of the real euid.

```bash
git add -A
git commit -m "fix: add safefile helpers that refuse symlinked targets"
```

---

### Task 16: [hygiene] Write the JWT and --log_file through safefile

**Files:**
- Modify: `internal/auth/store.go` (`Save`), `internal/logx/logx.go` (`SetupLogFile`), `docs/configuration.md`
- Create: `internal/auth/store_test.go`
- Modify: `internal/logx/logx_test.go`

**Interfaces:**
- Consumes: `safefile.WriteFile`, `safefile.OpenAppend`.
- Produces: unchanged signatures `auth.Save(jwt string) error`, `logx.SetupLogFile(path string) error`; new edges `auth -> safefile`, `logx -> safefile`.

Root cause: `os.WriteFile(jwtPath, ...)` and `os.OpenFile(logPath, O_CREATE|O_APPEND|O_WRONLY, ...)` follow a symlink at the final path component. A root run (the normal case for `vpn`/`quick-connect`) writing into `~/.urnetwork` or a user-chosen log path would overwrite whatever file the symlink names.

- [ ] **Step 1: Write the failing tests**

`internal/auth/store_test.go`:

```go
package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSave_WritesTrimmedTokenWith0600(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	if err := Save("  tok  "); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "jwt")
	b, err := os.ReadFile(path)
	fi, _ := os.Stat(path)
	if err != nil || string(b) != "tok\n" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("content=%q mode=%v err=%v", b, fi.Mode(), err)
	}
}

func TestSave_RefusesSymlinkedJWTFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, "jwt")); err != nil {
		t.Fatal(err)
	}
	if err := Save("tok"); err == nil {
		t.Fatal("Save wrote through a symlinked jwt file")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("symlink target overwritten: %q", b)
	}
}
```

Append to `internal/logx/logx_test.go`:

```go
func TestSetupLogFile_RefusesSymlink(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr
	t.Cleanup(func() { os.Stdout, os.Stderr = origOut, origErr })
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := SetupLogFile(link); err == nil {
		t.Fatal("SetupLogFile opened a symlinked log path")
	}
	if os.Stdout != origOut {
		t.Fatal("stdout redirected despite the error")
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `go test ./internal/auth/ ./internal/logx/ -run 'Symlink|Trimmed'`
Expected: `TestSave_RefusesSymlinkedJWTFile` and `TestSetupLogFile_RefusesSymlink` FAIL (the write followed the symlink); `TestSave_WritesTrimmedTokenWith0600` passes.

- [ ] **Step 3: Implement**

`internal/auth/store.go`:

```go
func Save(jwt string) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return safefile.WriteFile(path, []byte(strings.TrimSpace(jwt)+"\n"))
}
```

`internal/logx/logx.go`, in `SetupLogFile` replace the `os.OpenFile(...)` line with:

```go
	f, err := safefile.OpenAppend(path)
```

```bash
$GOIMPORTS internal/auth internal/logx
```

- [ ] **Step 4: Run the tests to see them pass**

Run: `go test -race ./internal/auth/ ./internal/logx/`
Expected: PASS.

- [ ] **Step 5: Document**

In `docs/configuration.md`, after the `URNETWORK_HOME` line, add:

```markdown
### Files the client writes

- The JWT file (`$URNETWORK_HOME/jwt`, default `~/.urnetwork/jwt`) is replaced atomically through a temp file and rename, with mode 0600. `--log_file` is opened for append with mode 0600.
- Both refuse a target that is a symlink or not a regular file, and a directory that another user (other than root) owns or that is group/world-writable without the sticky bit. Symlinks in the directory path itself are resolved first, so paths such as `/tmp` on macOS work.
- `--log_file` also refuses a file with more than one hard link or owned by a different user.
- When run as root (sudo) against a directory owned by a normal user, for example your home, the write is allowed and new files are handed to that user so later non-sudo commands can read them.
```

- [ ] **Step 6: Run the gate and commit**

Run: `bash "$BASE/gate.sh" 243 && bash "$BASE/linux-test.sh" 245`
Expected: PASS; golden identical (`save-jwt --jwt=` fails before any write).

```bash
git add -A
git commit -m "fix: write jwt and log file without following symlinks"
```

---

### Task 17: [final] PM review and full verification

**Files:**
- None changed unless a check fails (fix in the owning task's package with a follow-up commit).

**Interfaces:**
- Consumes: the finished integration branch.
- Produces: a verified branch ready for review, and the deferred list below.

- [ ] **Step 1: Full verification**

```bash
bash "$BASE/gate.sh" 243
bash "$BASE/linux-test.sh" 245
make version-check && make arch-check && make lint
scripts/test-release-prepare.sh
make ci-docker-build && docker run --rm test/urnetwork-client:ci --version
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml
go list -f '{{.ImportPath}}' ./...   # expect exactly: cmd/urnet-client and internal/{auth,config,logx,netcfg,safefile,session,socks,tunnel,urapi}
git log --oneline 93f01f8..HEAD      # every subject Conventional, under 72 chars
git log --format=%s 93f01f8..HEAD | awk 'length > 72'   # expect no output
git grep -nP '\x{2014}' -- '*.go' '*.md' ':!CHANGELOG.md' ':!docs/superpowers'   # expect no em dashes in new text
```

- [ ] **Step 2: PM checklist**

- [ ] Layout matches "Target layout"; `ls *.go` at the repo root is empty.
- [ ] Only `internal/urapi` imports connect (`make arch-check`).
- [ ] No import cycles (`go build ./...` on darwin and linux).
- [ ] Golden CLI transcript identical to 93f01f8 (gate).
- [ ] Test count darwin >= 243 (baseline 214), linux >= 245 (baseline 217).
- [ ] `make version-check` passes with `./cmd/urnet-client`; `-X main.Version` unchanged in Makefile, Dockerfile and ci.yml.
- [ ] Makefile, Dockerfile, README, quick-start use `./cmd/urnet-client`; `.golangci.yml` needed no change (it has no paths).
- [ ] ci.yml: `test-macos` job exists and gates `release`; release assets built by `@semantic-release/exec` prepareCmd; `build-cli` runs `scripts/test-release-prepare.sh`; `docker-build` uses the release version.
- [ ] Tools resolved from `/sbin`, `/usr/sbin`, `/bin`, `/usr/bin`, `/run/current-system/sw/bin`, never `$PATH`; documented.
- [ ] JWT and `--log_file` written via safefile; documented.
- [ ] Modernize: only `slices.Contains` and `WaitGroup.Go` in socks.
- [ ] Owner manual checks still pending from Phase 0 (linux route survival, macOS kill switch with sudo) are not claimed by this phase.
- [ ] Nothing pushed.

- [ ] **Step 3: Record gaps**

Any unchecked item above becomes a follow-up entry under "Deferred to Phase 2+" in this file (commit `docs: record phase 1 follow-ups`).

---

## Deferred to Phase 2+

- Tri-state config: distinguish "flag not given" from "flag equals default", booleans that the file can turn off, `stats_interval: 0`.
- Session lifecycle with `errgroup`: tie SOCKS sessions, the dataplane goroutine, stats and JWT renewal to one cancellable group; decide whether a failed SOCKS start is fatal; drop the always-empty `onBeforeExit`.
- Structured logging with `log/slog` behind `internal/logx`.
- Firewall kill switch (pf on macOS, nftables on Linux) with an explicit unblock command.
- DNS and IPv6 leak policy with `--default_route` (block v6 or route it, force DNS through the tunnel); Linux `SetDNS` is still a no-op.
- WireGuard-go `tun` package instead of `songgao/water`.
- CLI framework (kong or cobra) to replace docopt.
- Evaluate a maintained SOCKS5 library (for example `go-socks5`) against `internal/socks`.
- macOS kill switch blackhole suspicion: verify whether `route -n add -blackhole default` without a gateway ever succeeds (Phase 0 manual check still pending).
- Dropping `--background`.
- `auth.Load` still reads the JWT through a symlink (only writes are hardened).
- `DefaultGateway` now parses combined stdout+stderr from `route -n get default`; revisit if a macOS version prints a `gateway:` line on stderr.
- `make lint` runs `go mod tidy`, which can change the tree in CI without failing.
- Standalone `socks` command still does not connect to the extender.
- Semantic-release pushes tags with `GITHUB_TOKEN`, so `build-binaries-for-tag`/`release-on-tag` only run for manually pushed tags; consider removing that duplicate path now that release assets are versioned correctly.
