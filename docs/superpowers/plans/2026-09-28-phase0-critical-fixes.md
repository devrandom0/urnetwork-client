# Phase 0 Critical Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the verified Phase 0 defects in urnet-client (SOCKS protocol and exposure, route/TUN safety, config wiring, build/release metadata, secret handling) without redesigning anything, in five file-disjoint streams that can run in parallel worktrees.

**Architecture:** Everything stays in `package main` at the repo root. Each stream owns a set of files and lands on its own sub-branch off `sina/phase0_critical_fixes`; shared files (`main.go`, `config.go`, `vpn_linux.go`, `vpn_darwin.go`) are split by hunk so merges are mechanical. Two small seams are introduced for testability: a swappable `commandRunner` for OS commands (stream B) and a `SocksOptions`/`socksServer` structure for the SOCKS server (stream A).

**Tech Stack:** Go 1.26 module with toolchain go1.27.1, `github.com/urnetwork/connect`, `github.com/docopt/docopt-go`, `github.com/songgao/water`, `gopkg.in/yaml.v3`, golangci-lint v2.14.0, GitHub Actions, Renovate, semantic-release.

**Spec:** The Phase 0 scope brief (findings A1..A8, B0..B5, C1..C5, D1..D6, E1..E3) from the prior reviews, as restated and verified in the "Findings verification" section below. There is no separate spec file; this plan is the source of truth for Phase 0.

## Global Constraints

- Do not redesign config. Tri-state config, package split and firewall kill switch are Phase 1+.
- Code default for SOCKS stays permissive: a non-loopback bind without auth is ALLOWED, but logs a loud WARN at startup (owner decision A3).
- New SOCKS auth flags: `--socks_user=<user>` and `--socks_pass=<pass>` on `vpn`, `quick-connect` and `socks`. Env fallbacks: `URNETWORK_SOCKS_USER`, `URNETWORK_SOCKS_PASS`.
- `docker-compose.linux.yml` binds `--socks=127.0.0.1:1080`. `docker-compose.macos.yml` keeps `--socks=0.0.0.0:1080` inside the container and publishes `"127.0.0.1:1080:1080"`.
- `main.go`: `var Version = "dev"`, settable with `-ldflags "-X main.Version=<v>"`.
- Renovate: no automerge for `github.com/urnetwork/connect`; one grouped PR for golangci-lint across `asdf` and `github-actions`; `github-actions` and `asdf` updates use semantic commit type `ci` (no release per `.releaserc.json`).
- TDD: every task writes the failing test first, runs it and sees it fail, then implements.
- Conventional Commits, imperative mood, subject under 72 characters. No ticket IDs in code or commits.
- Comments: default none. Only a non-obvious WHY, one line where possible, no em dashes.
- Never `git push` without explicit user approval.
- Verification after every task (all must pass):

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- Test code must satisfy errcheck: wrap ignored results as `_ = x.Close()` or `defer func() { _ = x.Close() }()`.
- Prose (docs, commit bodies): no em dashes, straight quotes only.

---

## Findings verification

Every finding was checked against the code on `sina/phase0_critical_fixes` at `2a11d8e`. None was false. Notes where the reality differs from the brief:

| ID | Verified | Note |
|----|----------|------|
| A1 | yes | `socks.go:101` always writes `{5, 0}`. |
| A2 | yes (feature) | No auth support at all. |
| A3 | yes | Both compose overrides use `--socks=0.0.0.0:1080`; the linux one runs with `network_mode: host`, so the proxy is open to the LAN. |
| A4 | yes | `bindFDToInterface` returns nil on every path. Also found: when the VPN-bound UDP socket fails to open, UDP traffic silently falls back to `pcSys` (`socks.go:319-327, 424-428`). Folded into A4/A5. |
| A5 | yes | DST.ADDR/PORT never read for UDP ASSOCIATE, so `ctrl.Read` at 486 consumes a byte of it and returns at once. Also found: the UDP relay always listens on `127.0.0.1`, so LAN clients (the MikroTik use case) can never reach it, and it accepts datagrams from any source. Folded into A5. |
| A6 | yes | `pcVPN`/`pcSys` never closed; both `sendBack` goroutines block forever. |
| A7 | yes | `socks.go:56-59`. |
| A8 | yes | `writeSocksReply` ignores `bindAddr`. Needed anyway for A5 (UDP relay address must be encoded, possibly IPv6). |
| B0 | n/a | Seam, not a defect. |
| B1 | yes | Plus: `AddExclude` without a gateway runs `ip route add <dest> unreachable`, which is invalid syntax (`unreachable` is a route TYPE and goes first), so it always failed and still got recorded. Fixed in B-2. |
| B2 | yes | Also suspected (needs manual check with sudo): `route -n add -blackhole default` without a gateway argument may always fail on macOS, which would make the B2 path the common path. Listed in Deferred. |
| B3 | yes | |
| B4 | yes | `RemoveDNSBypass` writes `m.addedDNSBypass = nil` on a goroutine while `Cleanup` ranges over it. |
| B5 | yes | Split: the "nothing to do" returns live inside the SOCKS-only block that stream A replaces, so that part moves to A-1. The discarded `loadJWT` error lives inside the `vpn` dispatch block that stream C rewrites, so that part moves to C-2. |
| C1 | yes | Also: quick-connect reads `--api_url` for login before any config is loaded, so a config `api_url` is ignored for login. Fixed by C-3. |
| C2 | yes | |
| C3 | yes | Docs use `socks_listen`, code tag is `yaml:"socks"`. |
| C4 | partly | "A file cannot turn default_route off" is true but harmless, because the CLI default is already off. The real limitations: file booleans can only enable, and a CLI value equal to the built-in default is treated as unset. Documented only (C-4), no code change. |
| C5 | yes | `cmd_quickconnect.go:86-89`. |
| D1 | yes | Confirmed by building with `-X main.Version=9.9.9`: binary still prints `0.1.0`. |
| D2 | yes | Workflow-level cancellation cancels the whole run, including the `release` job, regardless of the job-level `concurrency` on `release`. |
| D3 | yes | `MODULE_PATH`/`CMD_PATH` unused. |
| D4 | yes | Also: the global `commitMessagePrefix: "chore(deps): "` overrides every `semanticCommitType` (Renovate docs: "The semantic commits feature is overridden if a commitMessagePrefix is configured"), so it must be removed for the `ci` type to take effect. |
| D5 | yes | |
| D6 | yes | `GO_VERSION: '1.26.x'` vs `toolchain go1.27.1`. setup-go v6+ with `go-version-file: go.mod` uses the `toolchain` directive. The stale local branch `sina/bump_go_version_ci` is superseded by D-3. |
| E1 | yes | |
| E2 | yes | |
| E3 | small | Implemented for CLI flags in E-3; config-file URLs wired in Z-1 after the merge. |

Dropped as false: none.

---

## Parallel execution model

### Branches and worktrees

Existing sibling worktrees live next to the repo (`../urnetwork-client-<name>`). Follow that convention. From the main checkout:

```bash
cd /Users/sinamoghaddas/projects/my-projects/devrandom0/urnetwork-client
git fetch origin
git worktree add ../urnetwork-client-phase0_ci      -b sina/phase0_ci      sina/phase0_critical_fixes
git worktree add ../urnetwork-client-phase0_routes  -b sina/phase0_routes  sina/phase0_critical_fixes
git worktree add ../urnetwork-client-phase0_socks   -b sina/phase0_socks   sina/phase0_critical_fixes
git worktree add ../urnetwork-client-phase0_hygiene -b sina/phase0_hygiene sina/phase0_critical_fixes
git worktree add ../urnetwork-client-phase0_config  -b sina/phase0_config  sina/phase0_critical_fixes
```

### File ownership matrix

"Owns" means only that stream edits the file. "Shared" lists the exact hunk each stream may touch.

| File | A socks | B routes | C config | D ci | E hygiene |
|------|---------|----------|----------|------|-----------|
| socks.go | owns | | | | |
| socks_auth.go (new) | owns | | | | |
| socks_test.go, socks_*_test.go (new) | owns | | | | |
| vpn_core_socks_test.go (new) | owns | | | | |
| cmd_socks.go | owns | | | | |
| vpn_core.go | owns | | | | |
| docker-compose.linux.yml, docker-compose.macos.yml | owns | | | | |
| docs/command-reference.md, docs/examples.md, docs/docker.md | owns | | | | |
| cmdrunner.go, cmdrunner_test.go (new) | | owns | | | |
| dnsbootstrap.go, dnsbootstrap_test.go (new) | | owns | | | |
| util.go | | owns (`runCapture` body only) | | | |
| routes_linux.go, routes_darwin.go | | owns | | | |
| routes_linux_test.go, routes_darwin_test.go, tun_linux_test.go, tun_darwin_test.go (new) | | owns | | | |
| docs/platform-notes.md | | owns | | | |
| vpn_linux.go | shared: SOCKS-only `if` block (lines 27-41) | shared: imports, TUN config (59-66), `run` body (111-116) | | | |
| vpn_darwin.go | shared: SOCKS-only `if` block (lines 30-44) | shared: imports, TUN config (72-78), DNS goroutine (146-166), `runSudo` body (187-192) | | | |
| config.go | shared: add `SOCKSAuth` field to `VPNConfig`, `Auth` to `SOCKSConfig`, one line in each parse func | | shared: `ConfigFile` alias field, `loadConfigFile`, section comment line 123, new funcs appended at end | | |
| config_test.go, cmd_vpn.go, cmd_vpn_test.go, cmd_quickconnect_test.go (new) | | | owns | | |
| cmd_quickconnect.go | | | owns | | |
| docs/configuration.md | | | owns | | |
| main.go | shared: usage patterns lines 27, 28, 32 and Options block (insert after line 61) | | shared: `vpn` case lines 139-150 | shared: line 17 only | shared: insert after line 85, insert after line 101, line 106 |
| Makefile, Dockerfile, .github/workflows/ci.yml, .github/renovate.json | | | | owns | |
| .releaserc.json | | | | read-only check | |
| process.go, process_test.go (new) | | | | | owns |
| api_http.go, api_http_test.go | | | | | owns |
| specs.go, specs_test.go | | | | | owns |
| urlcheck.go, urlcheck_test.go (new) | | | | | owns |
| README.md | | | | | owns (Security note) |

### main.go hunk map

| Stream | Lines (at 2a11d8e) | Change |
|--------|--------------------|--------|
| D | 17 | `const Version = "0.1.0"` to `var Version = "dev"` |
| A | 27, 28, 32 | add `[--socks_user=<user>] [--socks_pass=<pass>]` to quick-connect, socks, vpn patterns |
| A | after 61 | two Options lines |
| E | after 85 | `applySecretEnvFallbacks(opts, os.Getenv)` |
| E | after 101 | endpoint validation |
| E | 106 | `spawnBackground(os.Args, backgroundSecrets(opts))` |
| C | 139-150 | `vpn` case becomes `runErr = cmdVpnFromOpts(ctx, opts)` |

### Recommended merge order into sina/phase0_critical_fixes

1. `sina/phase0_ci` (D): smallest, only line 17 of main.go; puts `go-version-file` and cross-OS vet in place before the rest lands.
2. `sina/phase0_routes` (B): self-contained; lands the runner seam and the vpn_linux/vpn_darwin TUN hunks first.
3. `sina/phase0_socks` (A): touches the SOCKS-only blocks of vpn_linux/vpn_darwin (above B's hunks), adds fields to config.go, usage lines in main.go.
4. `sina/phase0_hygiene` (E): main.go lines 85-106, process/api/specs files.
5. `sina/phase0_config` (C): last, because it rewrites the `vpn` dispatch, adds functions to config.go after A's field additions, and its docs describe the env vars A and E introduce.
6. Task Z-1 and Z-2 on `sina/phase0_critical_fixes` after all merges.

Merge command for each branch (from the main checkout):

```bash
cd /Users/sinamoghaddas/projects/my-projects/devrandom0/urnetwork-client
git checkout sina/phase0_critical_fixes
git merge --no-ff sina/phase0_ci
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

Repeat with `sina/phase0_routes`, `sina/phase0_socks`, `sina/phase0_hygiene`, `sina/phase0_config`, running the verification after each merge.

### Expected conflict points

- `vpn_linux.go` / `vpn_darwin.go` (B then A): A replaces the SOCKS-only `if` body; B changed imports and later functions. Hunks are 15+ lines apart, so git should merge cleanly. If it conflicts, keep A's `return runSocksOnly(ctx, cfg)` block and B's import list.
- `config.go` (A then C): A adds struct fields and one literal line per parse func; C edits `ConfigFile`, `loadConfigFile` and appends functions. No overlap expected. gofmt column alignment does not shift because no new field name is longer than the existing longest one.
- `main.go` (D, A, E, C): hunks are separated by unchanged lines (see map). Only textual risk is A's Options insertion vs nothing else, and E's post-parse insert vs A's usage string end (line 79), which are 6 lines apart.
- `socks_test.go` call sites: only A touches them.
- If E merges before A, `--socks_pass` in E's secret list is a no-op until A lands. That is intended.

---

## Root cause analysis

### Stream A (SOCKS)

The SOCKS server was written as a minimal "happy path for one local client" proxy: one function parses the greeting, the request and the command inline with a shared 262-byte buffer, and every error path is "return". That shape explains every defect:

- A1/A2: method negotiation was hardcoded to `{5, 0}` because the server was only ever meant for `127.0.0.1` with curl. There is no place to plug in RFC 1929, and no rejection path (0xFF).
- A3: compose files were copied from a LAN use case (MikroTik) where `0.0.0.0` is required, but without auth that turns the host into an open proxy into the user's VPN exit.
- A4: `bindFDToInterface` was written "best-effort, never break connectivity", which inverts the security property: when binding fails, the connection silently egresses via the normal default route, outside the VPN. The same philosophy made the UDP path fall back to the unbound system socket.
- A5: the CONNECT branch reads DST.ADDR/PORT after the `cmd` check, and UDP ASSOCIATE was added as an early branch before that read. The unread bytes then satisfy `ctrl.Read(tmp)` immediately, which tears the association down. The handshake deadline is only cleared on the CONNECT success path, so even a correct client would be cut at 10 s. The relay was hard-wired to `127.0.0.1`, so remote clients cannot use it at all.
- A6: sockets were created with no ownership model; only `pcClient` got a `defer Close`, and the two reader goroutines block on sockets nobody closes.
- A7: the accept loop only treats `Timeout()` as transient, so EMFILE (a routine condition under load) kills the listener for the life of the process.
- A8: `writeSocksReply` was stubbed to `0.0.0.0:0`.

The fix introduces a `socksServer` with explicit options, parses the request address for every command in one place (`readSocksAddr`), makes bind failures hard errors that map to reply 0x01, gives UDP ASSOCIATE an owner that closes everything when the control connection ends, and adds a proper negotiation step with RFC 1929.

### Stream B (routes/TUN)

Route mutation is done by shelling out and ignoring every result (`_ = run(...)`), then appending to "added" slices unconditionally. The code conflates "I asked for this route" with "I own this route". When `ip route add` fails with `File exists`, the route belongs to the user, but Cleanup deletes it anyway (B1). Darwin already learned this lesson (it records on success) but the kill-switch path forgot the restore half: it deletes the real default before knowing whether the blackhole will install (B2). TUN address/MTU/up errors are ignored for the same reason, so a half-configured TUN gets routes pointed at it and all traffic blackholes (B3). The macOS DNS bootstrap goroutine was bolted on with direct access to manager state and no lifecycle, so it races Cleanup and outlives the session (B4). None of this was caught because there was no way to run route code without root; the fix adds a command-runner seam so behavior can be asserted with a fake.

### Stream C (config)

Config-file support was added only at the `vpn` dispatch site in `main.go`, after log level was already applied and after quick-connect had gone its own way with `parseVPNConfig(opts, finalJWT)` at the end of its flow. So quick-connect advertises `--config` but never reads it (C1), `log_level` from the file is parsed but nothing consumes it (C2), and the docs were written against a key name (`socks_listen`) that matches the CLI alias rather than the YAML tag (C3). The merge rule "CLI value equal to default means unset" exists because docopt fills defaults and the code cannot tell "user typed 1420" from "default 1420" (C4); fixing that properly needs tri-state values, which is Phase 2. The login retry reuses `--jwt_renew_interval` as its sleep because it was the only duration in scope (C5). The fix centralizes "flags + file + log level" in one helper used by both commands.

### Stream D (build/release/CI)

`Version` is a `const`, and the Go linker's `-X` only sets string variables, so every `-X main.Version=...` in the Makefile has been a silent no-op and the Dockerfile never tried (D1). CI concurrency was set at workflow level to speed up PRs, which also applies to `main` pushes and can cancel a half-finished semantic-release (D2). The Makefile's buildx context `../../..` is left over from when the client lived inside the upstream monorepo (D3). Renovate's global `commitMessagePrefix` overrides semantic types, so every tooling bump is `chore(deps)`, which `.releaserc.json` maps to a patch release, and the upstream `connect` digest is automerged like any other patch (D4). Go version is pinned twice (env `GO_VERSION` and `go.mod`), so they drift (D6), and darwin-only files are never vetted in CI because runners are Linux (D5).

### Stream E (hygiene)

`--background` re-execs itself with the parent's argv, so secrets passed as flags become part of a long-lived process's command line, readable by any local user through `ps` (E1). HTTP helpers use `http.DefaultClient`, which has no timeout, and read bodies unbounded; `buildProviderSpecs` runs with the VPN's lifetime context, so a hung API blocks VPN startup forever (E2). URL flags are passed through unchecked, so a typo like `http://` sends JWTs in cleartext (E3).

---
## Stream D: build, release, CI (branch `sina/phase0_ci`)

Worktree: `../urnetwork-client-phase0_ci`. Owns `Makefile`, `Dockerfile`, `.github/workflows/ci.yml`, `.github/renovate.json`, and line 17 of `main.go`.

### Task D-1: Make Version injectable at build time

**Files:**
- Modify: `main.go:17`
- Modify: `Makefile` (new `version-check` target, help line, `--build-arg VERSION` on docker targets)
- Modify: `Dockerfile:12-34`

**Interfaces:**
- Consumes: nothing.
- Produces: `var Version string` in package main (default `"dev"`), Makefile target `version-check`, Dockerfile `ARG VERSION`.

- [ ] **Step 1: Write the failing check (Makefile target)**

Add to `Makefile` after the `build` target:

```make
.PHONY: version-check
version-check:
	@tmp=$$(mktemp -d); \
	go build -ldflags "-X main.Version=9.9.9" -o $$tmp/$(BINARY) . || { rm -rf $$tmp; exit 1; }; \
	out=$$($$tmp/$(BINARY) --version); rm -rf $$tmp; \
	if [ "$$out" != "9.9.9" ]; then echo "version-check: got '$$out', want 9.9.9"; exit 1; fi; \
	echo "version-check: ok"
```

Add to the `help` target, after the `test-race` line:

```make
	@echo "  version-check         Verify -ldflags -X main.Version reaches the binary"
```

- [ ] **Step 2: Run it to verify it fails**

Run: `make version-check`
Expected: FAIL with `version-check: got '0.1.0', want 9.9.9`

- [ ] **Step 3: Implement**

`main.go:17`, replace:

```go
const Version = "0.1.0"
```

with:

```go
var Version = "dev"
```

`Dockerfile`, replace:

```dockerfile
ARG TARGETOS
ARG TARGETARCH
```

with:

```dockerfile
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
```

and replace:

```dockerfile
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
```

with:

```dockerfile
    go build -trimpath -buildvcs=false -ldflags="-s -w -X main.Version=${VERSION}" \
```

`Makefile`, pass the version into every image build. Replace the `docker-build` and `ci-docker-build` recipes:

```make
.PHONY: docker-build
docker-build:
	DOCKER_BUILDKIT=1 docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

.PHONY: ci-docker-build
ci-docker-build:
	# Build only linux/amd64 for quick CI validation using repo root context
	DOCKER_BUILDKIT=1 docker build --build-arg VERSION=$(VERSION) -f Dockerfile -t test/urnetwork-client:ci .
```

- [ ] **Step 4: Run it to verify it passes**

Run: `make version-check`
Expected: `version-check: ok`

Optional, if Docker is available:

```bash
docker build --build-arg VERSION=9.9.9 -t urnet-client:vcheck . && docker run --rm urnet-client:vcheck --version
```

Expected: `9.9.9`

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add main.go Makefile Dockerfile
git commit -m "fix: allow setting Version with -ldflags and in Docker builds"
```

### Task D-2: Makefile docker context, stale vars, cross-OS vet

**Files:**
- Modify: `Makefile:3-4` (delete), `Makefile:59-67` (lint), `Makefile:122-140` (dockerx)

**Interfaces:**
- Consumes: `version-check` target from D-1.
- Produces: `make lint` now also runs `GOOS=linux go vet ./...` and `GOOS=darwin go vet ./...`.

- [ ] **Step 1: Write the failing check**

```bash
grep -n '\.\./\.\./\.\.' Makefile; grep -n 'MODULE_PATH\|CMD_PATH' Makefile; grep -c 'GOOS=darwin go vet' Makefile
```

Expected now (failing state): two `../../..` lines, two `MODULE_PATH`/`CMD_PATH` lines, and a count of `0`.

- [ ] **Step 2: Implement**

Delete these two lines at the top of `Makefile`:

```make
MODULE_PATH := github.com/urnetwork/connect
CMD_PATH := $(MODULE_PATH)/cmd/urnet-client
```

Replace the `lint` recipe with:

```make
.PHONY: lint
lint:
	go mod tidy
	@fmt_out=$$(gofmt -s -l .); \
	if [ -n "$$fmt_out" ]; then \
	  echo "gofmt found issues:" && echo "$$fmt_out" && exit 1; \
	fi
	go vet ./...
	GOOS=linux go vet ./...
	GOOS=darwin go vet ./...
	golangci-lint run ./...
```

Replace both `dockerx-build` and `dockerx-push` recipes with:

```make
.PHONY: dockerx-build
dockerx-build: dockerx-setup
	DOCKER_BUILDKIT=1 docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --build-arg VERSION=$(VERSION) \
	  -f Dockerfile \
	  -t $(IMAGE_BASENAME):$(VERSION) \
	  -t $(IMAGE_BASENAME):latest \
	  . \
	  --load

.PHONY: dockerx-push
dockerx-push: dockerx-setup
	DOCKER_BUILDKIT=1 docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --build-arg VERSION=$(VERSION) \
	  -f Dockerfile \
	  -t $(IMAGE_BASENAME):$(VERSION) \
	  -t $(IMAGE_BASENAME):latest \
	  . \
	  --push
```

- [ ] **Step 3: Verify**

```bash
grep -c '\.\./\.\./\.\.' Makefile; grep -c 'MODULE_PATH\|CMD_PATH' Makefile; grep -c 'GOOS=darwin go vet' Makefile
make lint
```

Expected: `0`, `0`, `1`, and `make lint` passes (note: `go mod tidy` must not change `go.mod`/`go.sum`; check `git status`).

- [ ] **Step 4: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "build: fix buildx context and vet darwin and linux in make lint"
```

### Task D-3: CI concurrency, Go version source, version build-args

**Files:**
- Modify: `.github/workflows/ci.yml:19-26` (concurrency, env), every `actions/setup-go` step (lines 38-41, 69-72, 111-114, 217-220, 804-807), `lint` job steps, `build-cli` checkout, `docker-build-test`, `docker-build`, `force-tag-build`, `build-binaries-for-tag`.

**Interfaces:**
- Consumes: `make version-check` (D-1), `make lint` with cross-OS vet (D-2), Dockerfile `ARG VERSION` (D-1).
- Produces: nothing code-level.

- [ ] **Step 1: Write the failing checks**

```bash
grep -c 'go-version-file: go.mod' .github/workflows/ci.yml
grep -c "cancel-in-progress: \${{ github.event_name == 'pull_request' }}" .github/workflows/ci.yml
grep -c 'VERSION=' .github/workflows/ci.yml
grep -c 'make version-check' .github/workflows/ci.yml
```

Expected (failing state): `0`, `0`, `0`, `0`.

- [ ] **Step 2: Implement**

Replace the top-level concurrency and env block:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read
env:
  GO_VERSION: '1.26.x'
```

with:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

permissions:
  contents: read
```

In all five `Set up Go` steps (jobs `lint`, `build-cli`, `test`, `integration-test`, `build-binaries-for-tag`), replace:

```yaml
          go-version: ${{ env.GO_VERSION }}
```

with:

```yaml
          go-version-file: go.mod
```

In the `lint` job, add after the `Lint` step:

```yaml
      - name: Version injection check
        run: make version-check
```

In the `build-cli` job, give `git describe` the tags it needs. Replace its checkout step:

```yaml
      - name: Checkout
        uses: actions/checkout@v7
```

(the first step under `build-cli`) with:

```yaml
      - name: Checkout
        uses: actions/checkout@v7
        with:
          fetch-depth: 0
```

In `docker-build-test`, add to the `Build Docker image (no push)` step's `with:`:

```yaml
          build-args: |
            VERSION=pr-${{ github.event.pull_request.number }}
```

In `docker-build`, replace:

```yaml
          build-args: |
            BUILDKIT_INLINE_CACHE=1
```

with:

```yaml
          build-args: |
            BUILDKIT_INLINE_CACHE=1
            VERSION=${{ steps.meta.outputs.version }}
```

In `force-tag-build`, replace the same `build-args` block with:

```yaml
          build-args: |
            BUILDKIT_INLINE_CACHE=1
            VERSION=${{ inputs.version }}
```

In `build-binaries-for-tag`, replace:

```yaml
      - name: Build all platforms
        run: make build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64
      - name: Stage artifacts
```

with:

```yaml
      - name: Build all platforms
        run: make build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 VERSION="${GITHUB_REF_NAME#v}"
      - name: Stage artifacts
```

- [ ] **Step 3: Verify**

```bash
grep -c 'go-version-file: go.mod' .github/workflows/ci.yml
grep -c "cancel-in-progress: \${{ github.event_name == 'pull_request' }}" .github/workflows/ci.yml
grep -c 'GO_VERSION' .github/workflows/ci.yml
grep -c 'VERSION=' .github/workflows/ci.yml
grep -c 'make version-check' .github/workflows/ci.yml
(command -v actionlint >/dev/null && actionlint .github/workflows/ci.yml) || python3 -c 'import yaml; yaml.safe_load(open(".github/workflows/ci.yml")); print("yaml ok")'
```

Expected: `5`, `1`, `0`, `4` (three build-args lines plus the `build-binaries-for-tag` make line), `1`, then `yaml ok` or a clean actionlint run.

- [ ] **Step 4: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: stop cancelling main runs and read Go version from go.mod"
```

Note for the PR description: the stale branch `sina/bump_go_version_ci` is superseded by this change and can be deleted after merge (ask the owner first).

### Task D-4: Renovate policy

**Files:**
- Modify: `.github/renovate.json` (full replacement below)
- Read-only check: `.releaserc.json:14` (`{ "type": "ci", "release": false }` already present)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing code-level.

- [ ] **Step 1: Write the failing check**

```bash
jq -e '
  (has("commitMessagePrefix") | not)
  and (.packageRules | any(.matchPackageNames == ["github.com/urnetwork/connect"] and .automerge == false))
  and (.packageRules | any(.groupName == "golangci-lint"))
  and (.packageRules | any(.matchManagers == ["github-actions", "asdf"] and .semanticCommitType == "ci"))
' .github/renovate.json
jq -e '.plugins[0][1].releaseRules | any(.type == "ci" and .release == false)' .releaserc.json
```

Expected: first command prints `false` and exits 1 (FAIL). Second prints `true` (release rule already correct, no change needed).

- [ ] **Step 2: Implement**

Replace `.github/renovate.json` with:

```json
{
  "$schema": "https://docs.renovatebot.com/renovate-schema.json",
  "extends": [
    "config:recommended",
    ":enableVulnerabilityAlerts"
  ],
  "labels": ["dependencies", "renovate"],
  "rebaseWhen": "auto",
  "timezone": "UTC",
  "schedule": [
    "after 02:00 and before 03:00 every day"
  ],
  "semanticCommits": "enabled",
  "semanticCommitType": "chore",
  "semanticCommitScope": "deps",
  "branchPrefix": "renovate/",
  "prHourlyLimit": 1,
  "prConcurrentLimit": 10,
  "dependencyDashboard": true,
  "enabledManagers": ["gomod", "dockerfile", "github-actions", "asdf"],
  "packageRules": [
    {
      "description": "Keep the previous chore(deps) prefix for everything unless a later rule says otherwise",
      "matchPackageNames": ["*"],
      "semanticCommitType": "chore"
    },
    {
      "matchManagers": ["gomod"],
      "postUpdateOptions": ["gomodTidy"]
    },
    {
      "matchManagers": ["gomod", "dockerfile"],
      "matchUpdateTypes": ["patch", "digest"],
      "automerge": true,
      "automergeType": "pr",
      "labels": ["dependencies", "automerge"]
    },
    {
      "matchManagers": ["github-actions"],
      "matchUpdateTypes": ["patch"],
      "automerge": true,
      "automergeType": "pr",
      "labels": ["dependencies", "automerge"]
    },
    {
      "matchManagers": ["github-actions"],
      "groupName": "github-actions (minor+patch)",
      "matchUpdateTypes": ["minor", "patch"]
    },
    {
      "description": "Tooling-only bumps use ci, which .releaserc.json does not release",
      "matchManagers": ["github-actions", "asdf"],
      "semanticCommitType": "ci"
    },
    {
      "description": "One PR for golangci-lint in .tool-versions and ci.yml",
      "matchDepNames": ["golangci-lint", "golangci/golangci-lint"],
      "groupName": "golangci-lint"
    },
    {
      "description": "Upstream protocol library changes are reviewed by a human",
      "matchPackageNames": ["github.com/urnetwork/connect"],
      "automerge": false,
      "labels": ["dependencies", "connect"]
    }
  ]
}
```

Why each rule sits where it does: Renovate applies `packageRules` in order and later rules win, so the `connect` rule must come after the gomod automerge rule, and the golangci-lint group must come after the github-actions group. The top-level `commitMessagePrefix` is removed because it overrides `semanticCommitType`.

- [ ] **Step 3: Verify**

```bash
jq -e '
  (has("commitMessagePrefix") | not)
  and (.packageRules | any(.matchPackageNames == ["github.com/urnetwork/connect"] and .automerge == false))
  and (.packageRules | any(.groupName == "golangci-lint"))
  and (.packageRules | any(.matchManagers == ["github-actions", "asdf"] and .semanticCommitType == "ci"))
' .github/renovate.json
```

Expected: `true`. If Node is available, also run:

```bash
npx --yes --package renovate -- renovate-config-validator .github/renovate.json
```

Expected: `Config validated successfully` (or equivalent success line).

- [ ] **Step 4: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 5: Commit**

```bash
git add .github/renovate.json
git commit -m "ci: stop automerging connect and release-free tooling bumps"
```

---
## Stream B: routes and TUN (branch `sina/phase0_routes`)

Worktree: `../urnetwork-client-phase0_routes`. Owns `cmdrunner.go`, `dnsbootstrap.go`, `util.go` (`runCapture` only), `routes_linux.go`, `routes_darwin.go`, their tests, `docs/platform-notes.md`, and the non-SOCKS hunks of `vpn_linux.go`/`vpn_darwin.go`. Does NOT touch the SOCKS-only `if` block in `cmdVpn` (stream A owns it).

Note on test coverage: `*_linux_test.go` runs in CI (ubuntu). `*_darwin_test.go` only runs on a Mac (`go test -race ./...` locally); CI still type-checks it through `GOOS=darwin go vet ./...` (vet includes test files).

### Task B-1: Command runner seam

**Files:**
- Create: `cmdrunner.go`
- Create: `cmdrunner_test.go`
- Modify: `util.go:3-9` (imports), `util.go:28-33` (`runCapture`)
- Modify: `vpn_linux.go:5-14` (imports), `vpn_linux.go:111-116` (`run`)
- Modify: `vpn_darwin.go:5-15` (imports), `vpn_darwin.go:187-192` (`runSudo`)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type commandRunner interface { Run(name string, args ...string) error; Capture(name string, args ...string) (string, error) }`
  - `var cmdRunner commandRunner` (production value `execRunner{}`)
  - Test helpers (package main, `_test.go`): `type fakeRunner`, `func useFakeRunner(t *testing.T) *fakeRunner`, `func (f *fakeRunner) failOn(line, out string)`, `func (f *fakeRunner) Calls() []string`, `func (f *fakeRunner) count(line string) int`, field `results map[string]fakeResult`. A recorded line is the command and args joined by single spaces, e.g. `"ip route add 0.0.0.0/1 dev tun0"`.

- [ ] **Step 1: Write the failing test**

Create `cmdrunner_test.go`:

```go
package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeResult struct {
	out string
	err error
}

// fakeRunner records every command and returns scripted results; safe for concurrent use.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
	results map[string]fakeResult
}

func useFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{results: map[string]fakeResult{}}
	old := cmdRunner
	cmdRunner = f
	t.Cleanup(func() { cmdRunner = old })
	return f
}

func (f *fakeRunner) failOn(line, out string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[line] = fakeResult{out: out, err: errors.New("exit status 2")}
}

func (f *fakeRunner) Run(name string, args ...string) error {
	_, err := f.Capture(name, args...)
	return err
}

func (f *fakeRunner) Capture(name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, line)
	r := f.results[line]
	return r.out, r.err
}

func (f *fakeRunner) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeRunner) count(line string) int {
	n := 0
	for _, c := range f.Calls() {
		if c == line {
			n++
		}
	}
	return n
}

func TestRunCaptureUsesCommandRunner(t *testing.T) {
	f := useFakeRunner(t)
	f.results["echo hi"] = fakeResult{out: "scripted"}
	out, err := runCapture("echo", "hi")
	if err != nil || out != "scripted" {
		t.Fatalf("runCapture = %q, %v; want scripted output from the fake", out, err)
	}
	if got := f.Calls(); len(got) != 1 || got[0] != "echo hi" {
		t.Fatalf("calls = %v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestRunCaptureUsesCommandRunner ./...`
Expected: FAIL to compile with `undefined: cmdRunner`.

- [ ] **Step 3: Implement**

Create `cmdrunner.go`:

```go
package main

import (
	"os"
	"os/exec"
)

// commandRunner is the single choke point for OS commands so route and TUN logic can be
// tested without root.
type commandRunner interface {
	Run(name string, args ...string) error
	Capture(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (execRunner) Capture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

var cmdRunner commandRunner = execRunner{}
```

`util.go`: replace the `runCapture` function with:

```go
// runCapture executes a command and returns its combined stdout+stderr output and any error.
func runCapture(name string, args ...string) (string, error) {
	return cmdRunner.Capture(name, args...)
}
```

and remove `"os/exec"` from the import block.

`vpn_linux.go`: replace `run` with:

```go
func run(name string, args ...string) error {
	return cmdRunner.Run(name, args...)
}
```

and remove `"os"` and `"os/exec"` from its imports.

`vpn_darwin.go`: replace `runSudo` with:

```go
func runSudo(name string, args ...string) error {
	return cmdRunner.Run(name, args...)
}
```

and remove `"os"` from its imports (keep `"os/exec"`, still used by `getDefaultGateway`).

- [ ] **Step 4: Run it to verify it passes**

Run: `go test -race -run TestRunCaptureUsesCommandRunner ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add cmdrunner.go cmdrunner_test.go util.go vpn_linux.go vpn_darwin.go
git commit -m "refactor: route OS commands through a swappable runner"
```

### Task B-2: Linux routes are recorded only when this session added them

**Files:**
- Modify: `routes_linux.go:17-21` (struct), `37-62` (bypass, split default), `90-135` (exclude, extra, DNS), `141-145` (Cleanup splits)
- Create: `routes_linux_test.go`

**Interfaces:**
- Consumes: `useFakeRunner`, `fakeRunner.failOn`, `fakeRunner.count`, `fakeRunner.Calls` (B-1).
- Produces: `func (m *linuxRouteManager) addRoute(args ...string) bool`; `linuxRouteManager.addedSplits` becomes `[]string`.

- [ ] **Step 1: Write the failing tests**

Create `routes_linux_test.go`:

```go
//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxRoutes_RecordOnlyOnSuccess(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add 128.0.0.0/1 dev tun0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 10.0.0.0/8 via 192.168.1.1 dev eth0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 172.16.0.0/12 dev tun0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 1.1.1.1 dev tun0", "RTNETLINK answers: File exists")

	m := newLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddSplitDefault()
	m.AddExclude("10.0.0.0/8")
	m.AddExclude("10.1.0.0/16")
	m.AddExtraRoute("172.16.0.0/12")
	m.AddExtraRoute("172.20.0.0/16")
	m.AddDNSServerRoutes([]string{"1.1.1.1", "8.8.8.8"}, false)
	m.AddBypassEndpoint("https://203.0.113.7")
	m.Cleanup()

	for _, owned := range []string{"0.0.0.0/1", "10.1.0.0/16", "172.20.0.0/16", "8.8.8.8", "203.0.113.7"} {
		if f.count("ip route del "+owned) != 1 {
			t.Errorf("route %s was added by us and must be deleted exactly once", owned)
		}
	}
	for _, foreign := range []string{"128.0.0.0/1", "10.0.0.0/8", "172.16.0.0/12", "1.1.1.1"} {
		if f.count("ip route del "+foreign) != 0 {
			t.Errorf("route %s already existed and must not be deleted by Cleanup", foreign)
		}
	}
}

func TestLinuxRoutes_CleanupDeletesEveryAddedRoute(t *testing.T) {
	f := useFakeRunner(t)
	m := newLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddSplitDefault()
	m.AddExclude("10.1.0.0/16")
	m.AddExtraRoute("172.20.0.0/16")
	m.AddDNSServerRoutes([]string{"9.9.9.9"}, true)
	m.Cleanup()

	for _, c := range f.Calls() {
		fields := strings.Fields(c)
		if len(fields) < 4 || fields[0] != "ip" || fields[1] != "route" || fields[2] != "add" {
			continue
		}
		dest := fields[3]
		if f.count("ip route del "+dest) != 1 {
			t.Errorf("added %q but Cleanup did not delete %s exactly once", c, dest)
		}
	}
}

func TestLinuxRoutes_ExcludeWithoutGatewayUsesUnreachableType(t *testing.T) {
	f := useFakeRunner(t)
	m := newLinuxRouteManager("tun0", "", "")
	m.AddExclude("10.9.0.0/16")
	if f.count("ip route add unreachable 10.9.0.0/16") != 1 {
		t.Fatalf("calls = %v; want `ip route add unreachable 10.9.0.0/16`", f.Calls())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

On Linux (or in a container: `docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -run TestLinuxRoutes ./...`):

Run: `go test -run TestLinuxRoutes ./...`
Expected: FAIL. `RecordOnlyOnSuccess` reports the four foreign routes as deleted; `ExcludeWithoutGateway` reports the wrong argument order. (On macOS the file is skipped by its build tag; use the container command.)

- [ ] **Step 3: Implement**

In the `linuxRouteManager` struct, replace:

```go
	addedSplits  bool     // whether 0.0.0.0/1 + 128.0.0.0/1 were installed
```

with:

```go
	addedSplits  []string // split-default destinations installed through the TUN
```

Add after `newLinuxRouteManager`:

```go
// addRoute reports whether this session now owns the route. Any failure, including
// "File exists", means the route is not ours and must stay out of Cleanup.
func (m *linuxRouteManager) addRoute(args ...string) bool {
	if err := run("ip", append([]string{"route", "add"}, args...)...); err != nil {
		logWarn("ip route add %s failed: %v\n", strings.Join(args, " "), err)
		return false
	}
	return true
}
```

Replace the body of the loop in `AddBypassEndpoint` from `ipStr := v4.String()` to the end of the loop with:

```go
		ipStr := v4.String()
		var ok bool
		if m.origGw != "" {
			ok = m.addRoute(ipStr, "via", m.origGw, "dev", m.origDev)
		} else {
			ok = m.addRoute(ipStr, "dev", m.origDev)
		}
		if ok {
			m.addedBypass = append(m.addedBypass, ipStr)
		}
```

Replace `AddSplitDefault` with:

```go
func (m *linuxRouteManager) AddSplitDefault() {
	for _, dst := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if m.addRoute(dst, "dev", m.tunName) {
			m.addedSplits = append(m.addedSplits, dst)
		}
	}
}
```

Replace `AddExclude` with:

```go
func (m *linuxRouteManager) AddExclude(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	var ok bool
	switch {
	case m.origDev != "" && m.origGw != "":
		ok = m.addRoute(dest, "via", m.origGw, "dev", m.origDev)
	case m.origDev != "":
		ok = m.addRoute(dest, "dev", m.origDev)
	default:
		ok = m.addRoute("unreachable", dest)
	}
	if ok {
		m.addedExclude = append(m.addedExclude, dest)
	}
}
```

Replace `AddExtraRoute` with:

```go
func (m *linuxRouteManager) AddExtraRoute(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	if m.addRoute(dest, "dev", m.tunName) {
		m.addedExtra = append(m.addedExtra, dest)
	}
}
```

Replace `AddDNSServerRoutes` with:

```go
func (m *linuxRouteManager) AddDNSServerRoutes(ips []string, bypass bool) {
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if !bypass {
			if m.addRoute(ip, "dev", m.tunName) {
				m.addedDNSTun = append(m.addedDNSTun, ip)
			}
			continue
		}
		if m.origDev == "" {
			continue
		}
		var ok bool
		if m.origGw != "" {
			ok = m.addRoute(ip, "via", m.origGw, "dev", m.origDev)
		} else {
			ok = m.addRoute(ip, "dev", m.origDev)
		}
		if ok {
			m.addedBypass = append(m.addedBypass, ip)
		}
	}
}
```

In `Cleanup`, replace:

```go
	if m.addedSplits {
		_ = run("ip", "route", "del", "0.0.0.0/1")
		_ = run("ip", "route", "del", "128.0.0.0/1")
	}
```

with:

```go
	for _, dst := range m.addedSplits {
		_ = run("ip", "route", "del", dst)
	}
```

- [ ] **Step 4: Run them to verify they pass**

Run (Linux or container): `go test -race -run TestLinuxRoutes ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

Also run the linux tests once in the container command from Step 2.

- [ ] **Step 6: Commit**

```bash
git add routes_linux.go routes_linux_test.go
git commit -m "fix: only clean up Linux routes this session added"
```

### Task B-3: macOS kill switch restores the default route on failure

**Files:**
- Modify: `routes_darwin.go:236-250` (`AddKillSwitchRoute`)
- Create: `routes_darwin_test.go`

**Interfaces:**
- Consumes: `useFakeRunner`, `failOn`, `count` (B-1).
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Create `routes_darwin_test.go`:

```go
//go:build darwin

package main

import "testing"

func TestDarwinKillSwitch_RestoresDefaultWhenBlackholeFails(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	m.AddKillSwitchRoute()

	if m.killSwitchAdded {
		t.Fatal("killSwitchAdded must stay false when the blackhole route failed")
	}
	if f.count("route -n add default 192.168.1.1") != 1 {
		t.Fatalf("calls = %v; want the original default restored", f.Calls())
	}
}

func TestDarwinKillSwitch_NoRestoreWhenBlackholeInstalled(t *testing.T) {
	f := useFakeRunner(t)
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	m.AddKillSwitchRoute()

	if !m.killSwitchAdded {
		t.Fatal("killSwitchAdded should be true")
	}
	if f.count("route -n add default 192.168.1.1") != 0 {
		t.Fatalf("calls = %v; default must not be re-added while the kill switch is active", f.Calls())
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run (macOS): `go test -run TestDarwinKillSwitch ./...`
Expected: FAIL: `calls = [route -n delete default route -n add -blackhole default]; want the original default restored`.

- [ ] **Step 3: Implement**

Replace `AddKillSwitchRoute` in `routes_darwin.go` with:

```go
func (m *darwinRouteManager) AddKillSwitchRoute() {
	m.killSwitch = true
	// Replace the existing default with a blackhole so traffic is blocked
	// when the VPN split routes are absent. The /1 split routes are more
	// specific and will supersede this while the VPN is running.
	deletedDefault := false
	if m.defGw != "" {
		deletedDefault = runSudo("route", "-n", "delete", "default") == nil
	}
	if _, err := runCapture("route", "-n", "add", "-blackhole", "default"); err == nil {
		m.killSwitchAdded = true
		logInfo("kill switch: blackhole default route installed\n")
		return
	}
	logWarn("kill switch: failed to install blackhole default route; restoring original default and continuing without kill switch\n")
	if !deletedDefault {
		return
	}
	if err := runSudo("route", "-n", "add", "default", m.defGw); err != nil {
		logError("kill switch: could not restore default route via %s: %v; run: sudo route add default %s\n", m.defGw, err, m.defGw)
	}
}
```

- [ ] **Step 4: Run it to verify it passes**

Run (macOS): `go test -race -run TestDarwinKillSwitch ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add routes_darwin.go routes_darwin_test.go
git commit -m "fix: restore macOS default route when kill switch setup fails"
```

### Task B-4: Abort VPN start when TUN configuration fails

**Files:**
- Modify: `vpn_linux.go:59-66` (TUN config in `cmdVpn`), add `configureLinuxTUN`
- Modify: `vpn_darwin.go:72-78` (TUN config in `cmdVpn`), imports (add `strconv`), add `configureDarwinTUN`
- Create: `tun_linux_test.go`, `tun_darwin_test.go`
- Modify: `docs/platform-notes.md`

**Interfaces:**
- Consumes: `useFakeRunner`, `failOn`, `Calls` (B-1).
- Produces:
  - `func configureLinuxTUN(name string, cfg VPNConfig) error` (linux)
  - `func configureDarwinTUN(name, ipCIDR string, mtu int, enableIPv6 bool) (peerIP string, err error)` (darwin)

- [ ] **Step 1: Write the failing tests**

Create `tun_linux_test.go`:

```go
//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestConfigureLinuxTUN_FailsFastOnAddressError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip addr add 10.255.0.2/24 dev urnet0", "RTNETLINK answers: Operation not permitted")

	err := configureLinuxTUN("urnet0", VPNConfig{IPCIDR: "10.255.0.2/24", MTU: 1420})

	if err == nil {
		t.Fatal("want error when the TUN address cannot be set")
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, " up") || strings.HasPrefix(c, "ip route") {
			t.Fatalf("ran %q after the address failed; setup must stop at the first error", c)
		}
	}
}

func TestConfigureLinuxTUN_IPv6AddressOptionalUnlessEnabled(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip addr add fd00::2/120 dev urnet0", "RTNETLINK answers: Permission denied")

	if err := configureLinuxTUN("urnet0", VPNConfig{IPCIDR: "10.255.0.2/24", MTU: 1420}); err != nil {
		t.Fatalf("IPv6 disabled: got %v, want nil (hosts with IPv6 off must still work)", err)
	}
	if err := configureLinuxTUN("urnet0", VPNConfig{IPCIDR: "10.255.0.2/24", MTU: 1420, EnableIPv6: true}); err == nil {
		t.Fatal("IPv6 enabled: want error when the IPv6 address cannot be set")
	}
}
```

Create `tun_darwin_test.go`:

```go
//go:build darwin

package main

import (
	"strings"
	"testing"
)

func TestConfigureDarwinTUN_FailsOnIfconfigError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ifconfig utun9 inet 10.255.0.2 10.255.0.1 mtu 1420 up", "ifconfig: ioctl (SIOCAIFADDR): Operation not permitted")

	if _, err := configureDarwinTUN("utun9", "10.255.0.2/24", 1420, false); err == nil {
		t.Fatal("want error when ifconfig fails")
	}
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, "route") {
			t.Fatalf("route command %q ran during TUN setup", c)
		}
	}
}

func TestConfigureDarwinTUN_ReturnsPeerAndToleratesIPv6WhenDisabled(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ifconfig utun9 inet6 fd00::2/120", "ifconfig: inet6: bad value")

	peer, err := configureDarwinTUN("utun9", "10.255.0.2/24", 1420, false)
	if err != nil || peer != "10.255.0.1" {
		t.Fatalf("got peer=%q err=%v; want 10.255.0.1, nil", peer, err)
	}
	if _, err := configureDarwinTUN("utun9", "10.255.0.2/24", 1420, true); err == nil {
		t.Fatal("IPv6 enabled: want error")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run on each OS (Linux via the container command from B-2): `go test -run 'TestConfigure(Linux|Darwin)TUN' ./...`
Expected: FAIL to compile with `undefined: configureLinuxTUN` / `undefined: configureDarwinTUN`.

- [ ] **Step 3: Implement**

`vpn_linux.go`: replace lines 59-66:

```go
	// Configure IP address and MTU.
	_ = run("ip", "addr", "add", cfg.IPCIDR, "dev", tunName)
	_ = run("ip", "link", "set", "dev", tunName, "mtu", strconv.Itoa(cfg.MTU))
	_ = run("ip", "link", "set", tunName, "up")

	// Add IPv6 address to support IPv6 traffic through the VPN.
	// Use a ULA (Unique Local Address) prefix with /120 subnet.
	_ = run("ip", "addr", "add", "fd00::2/120", "dev", tunName)
```

with:

```go
	if err := configureLinuxTUN(tunName, cfg); err != nil {
		return err
	}
```

and add this function after `cmdVpn`:

```go
// configureLinuxTUN must succeed before any route points at the TUN; a half-configured
// device would blackhole all routed traffic.
func configureLinuxTUN(name string, cfg VPNConfig) error {
	steps := [][]string{
		{"ip", "addr", "add", cfg.IPCIDR, "dev", name},
		{"ip", "link", "set", "dev", name, "mtu", strconv.Itoa(cfg.MTU)},
		{"ip", "link", "set", name, "up"},
	}
	for _, s := range steps {
		if err := run(s[0], s[1:]...); err != nil {
			return fmt.Errorf("configure TUN %s (%s): %w", name, strings.Join(s, " "), err)
		}
	}
	if err := run("ip", "addr", "add", "fd00::2/120", "dev", name); err != nil {
		if cfg.EnableIPv6 {
			return fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logDebug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return nil
}
```

`vpn_darwin.go`: replace lines 72-78:

```go
	// Derive TUN IP and peer; configure the interface.
	tunIP, peerIP := tunCIDRParts(cfg.IPCIDR)
	_ = runSudo("ifconfig", actualName, "inet", tunIP, peerIP, "mtu", fmt.Sprintf("%d", cfg.MTU), "up")

	// Add IPv6 address to support IPv6 traffic through the VPN.
	// Use a ULA (Unique Local Address) prefix with the same /120 subnet as IPv4.
	_ = runSudo("ifconfig", actualName, "inet6", "fd00::2/120")
```

with:

```go
	peerIP, err := configureDarwinTUN(actualName, cfg.IPCIDR, cfg.MTU, cfg.EnableIPv6)
	if err != nil {
		return err
	}
```

add `"strconv"` to the imports, and add after `tunCIDRParts`:

```go
// configureDarwinTUN must succeed before any route points at the utun; a half-configured
// device would blackhole all routed traffic.
func configureDarwinTUN(name, ipCIDR string, mtu int, enableIPv6 bool) (string, error) {
	tunIP, peerIP := tunCIDRParts(ipCIDR)
	if err := runSudo("ifconfig", name, "inet", tunIP, peerIP, "mtu", strconv.Itoa(mtu), "up"); err != nil {
		return "", fmt.Errorf("configure TUN %s: %w", name, err)
	}
	if err := runSudo("ifconfig", name, "inet6", "fd00::2/120"); err != nil {
		if enableIPv6 {
			return "", fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logDebug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return peerIP, nil
}
```

In `docs/platform-notes.md`, add under `## Current limitations`:

```markdown
- If the TUN address, MTU or link-up step fails, the client exits with an error before touching any route. The IPv6 ULA address is only required when `--enable_ipv6` is set.
- On macOS, if the kill switch blackhole route cannot be installed, the original default route is restored and the VPN continues without a kill switch (a WARN is logged).
```

- [ ] **Step 4: Run them to verify they pass**

Run on each OS: `go test -race -run 'TestConfigure(Linux|Darwin)TUN' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add vpn_linux.go vpn_darwin.go tun_linux_test.go tun_darwin_test.go docs/platform-notes.md
git commit -m "fix: abort VPN startup when TUN configuration fails"
```

### Task B-5: Serialize macOS DNS bypass removal with Cleanup and honor ctx

**Files:**
- Create: `dnsbootstrap.go`, `dnsbootstrap_test.go`
- Modify: `routes_darwin.go` (imports add `sync`; struct add `mu`; `AddDNSServerRoutes` bypass append; `RemoveDNSBypass`; `Cleanup` DNS bypass loop)
- Modify: `vpn_darwin.go:146-166` (goroutine), imports (drop `sync/atomic`)
- Modify: `routes_darwin_test.go` (add race test)

**Interfaces:**
- Consumes: `useFakeRunner`, `count` (B-1).
- Produces:
  - `type dnsBypassRemover interface { RemoveDNSBypass() }`
  - `func removeDNSBypassWhenWarm(ctx context.Context, rm dnsBypassRemover, pktsIn, pktsOut *uint64, maxWait, tick time.Duration)`

- [ ] **Step 1: Write the failing tests**

Create `dnsbootstrap_test.go` (no build tag, runs on both OSes):

```go
package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type countingRemover struct{ n atomic.Int32 }

func (c *countingRemover) RemoveDNSBypass() { c.n.Add(1) }

func TestRemoveDNSBypassWhenWarm_RemovesOnceTrafficFlows(t *testing.T) {
	var in, out uint64 = 1, 1
	r := &countingRemover{}
	removeDNSBypassWhenWarm(context.Background(), r, &in, &out, time.Minute, time.Millisecond)
	if r.n.Load() != 1 {
		t.Fatalf("removals = %d, want 1", r.n.Load())
	}
}

func TestRemoveDNSBypassWhenWarm_RemovesAtDeadline(t *testing.T) {
	var in, out uint64
	r := &countingRemover{}
	removeDNSBypassWhenWarm(context.Background(), r, &in, &out, 20*time.Millisecond, 5*time.Millisecond)
	if r.n.Load() != 1 {
		t.Fatalf("removals = %d, want 1", r.n.Load())
	}
}

func TestRemoveDNSBypassWhenWarm_StopsOnCancel(t *testing.T) {
	var in, out uint64
	r := &countingRemover{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	removeDNSBypassWhenWarm(ctx, r, &in, &out, time.Minute, time.Minute)
	if r.n.Load() != 0 {
		t.Fatal("must not touch routes after the session context is done; Cleanup owns them then")
	}
}
```

Append to `routes_darwin_test.go`:

```go
func TestDarwinDNSBypass_RemoveAndCleanupDoNotRace(t *testing.T) {
	f := useFakeRunner(t)
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")
	m.AddDNSServerRoutes([]string{"1.1.1.1", "8.8.8.8"}, true)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.RemoveDNSBypass() }()
	go func() { defer wg.Done(); m.Cleanup() }()
	wg.Wait()

	for _, ip := range []string{"1.1.1.1", "8.8.8.8"} {
		if n := f.count("route -n delete -host " + ip); n != 1 {
			t.Fatalf("%s deleted %d times, want exactly 1", ip, n)
		}
	}
}
```

and change its import line to:

```go
import (
	"sync"
	"testing"
)
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -race -run 'TestRemoveDNSBypassWhenWarm|TestDarwinDNSBypass' ./...`
Expected: FAIL to compile with `undefined: removeDNSBypassWhenWarm`. After adding only `dnsbootstrap.go`, the darwin test still fails on macOS with `WARNING: DATA RACE` and/or a double delete.

- [ ] **Step 3: Implement**

Create `dnsbootstrap.go`:

```go
package main

import (
	"context"
	"sync/atomic"
	"time"
)

type dnsBypassRemover interface{ RemoveDNSBypass() }

// removeDNSBypassWhenWarm drops the DNS bypass routes once the tunnel carries traffic
// in both directions, or at maxWait. It returns without touching routes if ctx ends first.
func removeDNSBypassWhenWarm(ctx context.Context, rm dnsBypassRemover, pktsIn, pktsOut *uint64, maxWait, tick time.Duration) {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
		case <-ticker.C:
			if atomic.LoadUint64(pktsIn) == 0 || atomic.LoadUint64(pktsOut) == 0 {
				continue
			}
		}
		rm.RemoveDNSBypass()
		logInfo("DNS bootstrap cache complete; DNS bypass removed\n")
		return
	}
}
```

`routes_darwin.go`: add `"sync"` to imports. In the struct, add after `addedDNSBypass`:

```go
	mu               sync.Mutex         // guards addedDNSBypass; RemoveDNSBypass runs on the bootstrap goroutine
```

(run `gofmt -w routes_darwin.go` afterwards; field alignment will follow the block).

In `AddDNSServerRoutes`, replace:

```go
				if err == nil {
					m.addedDNSBypass = append(m.addedDNSBypass, ip)
				}
```

with:

```go
				if err == nil {
					m.mu.Lock()
					m.addedDNSBypass = append(m.addedDNSBypass, ip)
					m.mu.Unlock()
				}
```

Replace `RemoveDNSBypass` with:

```go
func (m *darwinRouteManager) RemoveDNSBypass() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ip := range m.addedDNSBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
	m.addedDNSBypass = nil
}
```

In `Cleanup`, replace:

```go
	// DNS bypass routes (may already be nil if RemoveDNSBypass was called)
	for _, ip := range m.addedDNSBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
```

with:

```go
	// DNS bypass routes (may already be nil if RemoveDNSBypass was called)
	m.RemoveDNSBypass()
```

`vpn_darwin.go`: replace lines 146-166 (the `if cfg.DefaultRoute && cfg.DNSBootstrap == "cache" { go func() { ... }() }` block) with:

```go
	// DNS cache bootstrap: remove DNS bypass once the tunnel has traffic.
	if cfg.DefaultRoute && cfg.DNSBootstrap == "cache" {
		go removeDNSBypassWhenWarm(ctx, rm, &pktsIn, &pktsOut, 3*time.Second, 200*time.Millisecond)
	}
```

and remove `"sync/atomic"` from its imports (keep `"time"`).

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestRemoveDNSBypassWhenWarm|TestDarwinDNSBypass' ./...`
Expected: PASS (darwin test runs on macOS only).

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

If golangci-lint on Linux reports `removeDNSBypassWhenWarm` as unused (it is only called from a darwin file), add `//go:build darwin` to both `dnsbootstrap.go` and `dnsbootstrap_test.go` and re-run; the tests then run on macOS only.

- [ ] **Step 6: Commit**

```bash
git add dnsbootstrap.go dnsbootstrap_test.go routes_darwin.go routes_darwin_test.go vpn_darwin.go
git commit -m "fix: serialize macOS DNS bypass removal with route cleanup"
```

---
## Stream A: SOCKS (branch `sina/phase0_socks`)

Worktree: `../urnetwork-client-phase0_socks`. Owns `socks.go`, `socks_auth.go`, all `socks*_test.go`, `vpn_core_socks_test.go`, `cmd_socks.go`, `vpn_core.go`, both compose overrides, `docs/command-reference.md`, `docs/examples.md`, `docs/docker.md`. Shared hunks: the SOCKS-only `if` block in `vpn_linux.go`/`vpn_darwin.go`, `SOCKSAuth`/`Auth` additions in `config.go`, usage/Options lines in `main.go`.

### Task A-1: SocksOptions, shared SOCKS-only runner, non-zero exit when there is nothing to do

**Files:**
- Modify: `socks.go:18-66` (`StartSocks5` signature, extract `newSocksResolver`)
- Modify: `socks_test.go:29,40,52,63,239,283` (call sites)
- Modify: `vpn_core.go` (imports, `vpnRunCore` SOCKS block lines 112-115 and 256-268, new `socksOptionsFromVPN`, `runSocksOnly`)
- Modify: `vpn_linux.go:27-41`, `vpn_darwin.go:30-44` (SOCKS-only block)
- Modify: `cmd_socks.go:21`
- Create: `vpn_core_socks_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type SocksOptions struct { ListenAddr, BindIf string; Debug bool; AllowDomains, ExcludeDomains, DNSServers []string }` (A-3 adds `HandshakeTimeout time.Duration`, A-4 adds `Auth SocksAuth`)
  - `func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error)`
  - `func newSocksResolver(dnsServers []string) *net.Resolver`
  - `func socksOptionsFromVPN(cfg VPNConfig, bindIf string) SocksOptions`
  - `func runSocksOnly(ctx context.Context, cfg VPNConfig) error`

- [ ] **Step 1: Write the failing tests**

Create `vpn_core_socks_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunSocksOnly_NoListenIsError(t *testing.T) {
	err := runSocksOnly(context.Background(), VPNConfig{})
	if err == nil || !strings.Contains(err.Error(), "nothing to do") {
		t.Fatalf("want a 'nothing to do' error so the process exits non-zero, got %v", err)
	}
}

func TestRunSocksOnly_ServesUntilCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runSocksOnly(ctx, VPNConfig{SOCKSListen: "127.0.0.1:0"}) }()
	select {
	case err := <-done:
		t.Fatalf("returned before cancel: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runSocksOnly after cancel: %v", err)
	}
}

func TestSocksOptionsFromVPN(t *testing.T) {
	cfg := VPNConfig{
		SOCKSListen:    "127.0.0.1:1080",
		AllowDomains:   []string{"a.example"},
		ExcludeDomains: []string{"b.example"},
		DNSList:        "1.1.1.1, 9.9.9.9",
		Debug:          true,
	}
	got := socksOptionsFromVPN(cfg, "utun9")
	if got.ListenAddr != "127.0.0.1:1080" || got.BindIf != "utun9" || !got.Debug {
		t.Fatalf("unexpected options: %+v", got)
	}
	if len(got.DNSServers) != 2 || got.DNSServers[1] != "9.9.9.9" {
		t.Fatalf("dns servers not split: %v", got.DNSServers)
	}
	if got.AllowDomains[0] != "a.example" || got.ExcludeDomains[0] != "b.example" {
		t.Fatalf("domains not copied: %+v", got)
	}
}
```

Update the six `StartSocks5` calls in `socks_test.go`:

| Line | New call |
|------|----------|
| 29 | `StartSocks5(ctx, SocksOptions{ListenAddr: "127.0.0.1:0"})` |
| 40 | `StartSocks5(ctx, SocksOptions{ListenAddr: "127.0.0.1:0", DNSServers: []string{}})` |
| 52 | `StartSocks5(ctx, SocksOptions{ListenAddr: "127.0.0.1:0", DNSServers: []string{"9.9.9.9"}})` |
| 63 | `StartSocks5(ctx, SocksOptions{ListenAddr: "127.0.0.1:0", DNSServers: []string{"9.9.9.9:53"}})` |
| 239 | `StartSocks5(ctx, SocksOptions{ListenAddr: proxyAddr, DNSServers: []string{dnsAddr}})` |
| 283 | `StartSocks5(ctx, SocksOptions{ListenAddr: proxyAddr})` |

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestRunSocksOnly|TestSocksOptionsFromVPN|TestStartSocks5' ./...`
Expected: FAIL to compile with `undefined: runSocksOnly`, `undefined: SocksOptions`.

- [ ] **Step 3: Implement**

`socks.go`: replace lines 18-66 (doc comment through the end of `StartSocks5`) with:

```go
// SocksOptions configures StartSocks5.
type SocksOptions struct {
	ListenAddr     string
	BindIf         string // interface that VPN-routed traffic must leave through; empty means system routing
	Debug          bool
	AllowDomains   []string
	ExcludeDomains []string
	DNSServers     []string // first entry replaces the system resolver for hostname lookups
}

// StartSocks5 starts a SOCKS5 proxy and returns a stop function.
func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error) {
	resolver := newSocksResolver(opts.DNSServers)
	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			go handleSocksConn(ctx, conn, opts.BindIf, opts.Debug, opts.AllowDomains, opts.ExcludeDomains, resolver)
		}
	}()
	stop := func() error { _ = ln.Close(); <-done; return nil }
	return stop, nil
}

func newSocksResolver(dnsServers []string) *net.Resolver {
	if len(dnsServers) == 0 {
		return net.DefaultResolver
	}
	addr := dnsServers[0]
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", addr)
		},
	}
}
```

`vpn_core.go`: add `"errors"` to imports. Delete line 114 (`debugOn := cfg.Debug`). Replace lines 256-268:

```go
	// Optional SOCKS5 proxy bound to the VPN interface
	socksListen := cfg.SOCKSListen
	allowDomains := cfg.AllowDomains
	excludeDomains := cfg.ExcludeDomains
	var stopSocks func() error
	if socksListen != "" {
		if s, err := StartSocks5(ctx, socksListen, tunIfName, debugOn || isDebugEnabled(), allowDomains, excludeDomains, splitCSV(cfg.DNSList)); err != nil {
			logWarn("failed to start socks at %s: %v\n", socksListen, err)
		} else {
			stopSocks = s
			logInfo("SOCKS5 listening at %s (bound to %s)\n", socksListen, tunIfName)
		}
	}
```

with:

```go
	// Optional SOCKS5 proxy bound to the VPN interface
	var stopSocks func() error
	if cfg.SOCKSListen != "" {
		if s, err := StartSocks5(ctx, socksOptionsFromVPN(cfg, tunIfName)); err != nil {
			logWarn("failed to start socks at %s: %v\n", cfg.SOCKSListen, err)
		} else {
			stopSocks = s
			logInfo("SOCKS5 listening at %s (bound to %s)\n", cfg.SOCKSListen, tunIfName)
		}
	}
```

Add after `vpnRunCore`:

```go
func socksOptionsFromVPN(cfg VPNConfig, bindIf string) SocksOptions {
	return SocksOptions{
		ListenAddr:     cfg.SOCKSListen,
		BindIf:         bindIf,
		Debug:          cfg.Debug || isDebugEnabled(),
		AllowDomains:   cfg.AllowDomains,
		ExcludeDomains: cfg.ExcludeDomains,
		DNSServers:     splitCSV(cfg.DNSList),
	}
}

// runSocksOnly serves SOCKS with system routing when no TUN is configured.
func runSocksOnly(ctx context.Context, cfg VPNConfig) error {
	if cfg.SOCKSListen == "" {
		return errors.New("no TUN and no --socks given; nothing to do (set --tun=<name> and/or --socks=<addr>)")
	}
	stop, err := StartSocks5(ctx, socksOptionsFromVPN(cfg, ""))
	if err != nil {
		return fmt.Errorf("start socks failed: %w", err)
	}
	defer func() { _ = stop() }()
	logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
	<-ctx.Done()
	return nil
}
```

`vpn_linux.go`: replace the body of the SOCKS-only `if` (lines 28-41):

```go
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		if cfg.SOCKSListen == "" {
			logError("--tun=none specified but no --socks provided; nothing to do\n")
			return nil
		}
		stopSocks, err := StartSocks5(ctx, cfg.SOCKSListen, "", cfg.Debug, cfg.AllowDomains, cfg.ExcludeDomains, splitCSV(cfg.DNSList))
		if err != nil {
			return fmt.Errorf("start socks failed: %w", err)
		}
		defer func() { _ = stopSocks() }()
		logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
		<-ctx.Done()
		return nil
	}
```

with:

```go
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		return runSocksOnly(ctx, cfg)
	}
```

`vpn_darwin.go`: make the identical replacement in its SOCKS-only `if` (lines 31-44).

`cmd_socks.go`: replace line 21:

```go
	stopSocks, err := StartSocks5(ctx, cfg.ListenAddr, "", cfg.Debug, cfg.AllowDomains, cfg.ExcludeDomains, nil)
```

with:

```go
	stopSocks, err := StartSocks5(ctx, SocksOptions{
		ListenAddr:     cfg.ListenAddr,
		Debug:          cfg.Debug,
		AllowDomains:   cfg.AllowDomains,
		ExcludeDomains: cfg.ExcludeDomains,
	})
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -run 'TestRunSocksOnly|TestSocksOptionsFromVPN|TestStartSocks5|TestSocks5' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_test.go vpn_core.go vpn_core_socks_test.go vpn_linux.go vpn_darwin.go cmd_socks.go
git commit -m "fix: fail vpn without TUN or SOCKS and add SocksOptions"
```

### Task A-2: Accept loop survives transient errors

**Files:**
- Modify: `socks.go` (`StartSocks5` accept goroutine; new `acceptLoop`)
- Create: `socks_accept_test.go`

**Interfaces:**
- Consumes: `StartSocks5`, `SocksOptions` (A-1).
- Produces: `func acceptLoop(ctx context.Context, ln net.Listener, maxBackoff time.Duration, handle func(net.Conn))`

- [ ] **Step 1: Write the failing tests**

Create `socks_accept_test.go`:

```go
package main

import (
	"context"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

type scriptedListener struct {
	mu     sync.Mutex
	errs   []error
	conns  []net.Conn
	closed chan struct{}
	once   sync.Once
}

func newScriptedListener(errs []error, conns []net.Conn) *scriptedListener {
	return &scriptedListener{errs: errs, conns: conns, closed: make(chan struct{})}
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		l.mu.Unlock()
		return nil, err
	}
	if len(l.conns) > 0 {
		c := l.conns[0]
		l.conns = l.conns[1:]
		l.mu.Unlock()
		return c, nil
	}
	l.mu.Unlock()
	<-l.closed
	return nil, net.ErrClosed
}

func (l *scriptedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *scriptedListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func emfileErr() error {
	return &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", syscall.EMFILE)}
}

func TestAcceptLoop_RetriesAfterTransientErrors(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c2.Close() }()
	ln := newScriptedListener([]error{emfileErr(), emfileErr(), emfileErr()}, []net.Conn{c1})
	handled := make(chan net.Conn, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(context.Background(), ln, 10*time.Millisecond, func(c net.Conn) { handled <- c })
	}()

	select {
	case c := <-handled:
		_ = c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop gave up after transient accept errors")
	}
	_ = ln.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop did not exit after the listener closed")
	}
}

func TestAcceptLoop_ExitsOnContextCancelWhileBackingOff(t *testing.T) {
	errs := make([]error, 20)
	for i := range errs {
		errs[i] = emfileErr()
	}
	ln := newScriptedListener(errs, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(net.Conn) {})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop ignored context cancellation")
	}
	_ = ln.Close()
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run TestAcceptLoop ./...`
Expected: FAIL to compile with `undefined: acceptLoop`.

- [ ] **Step 3: Implement**

In `StartSocks5`, replace the accept goroutine:

```go
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			go handleSocksConn(ctx, conn, opts.BindIf, opts.Debug, opts.AllowDomains, opts.ExcludeDomains, resolver)
		}
	}()
```

with:

```go
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(conn net.Conn) {
			go handleSocksConn(ctx, conn, opts.BindIf, opts.Debug, opts.AllowDomains, opts.ExcludeDomains, resolver)
		})
	}()
```

Add after `StartSocks5`:

```go
// acceptLoop keeps serving through transient Accept errors such as EMFILE and only
// stops when the listener is closed or ctx ends.
func acceptLoop(ctx context.Context, ln net.Listener, maxBackoff time.Duration, handle func(net.Conn)) {
	const minBackoff = 5 * time.Millisecond
	var backoff time.Duration
	for {
		conn, err := ln.Accept()
		if err == nil {
			backoff = 0
			handle(conn)
			continue
		}
		if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
			return
		}
		switch {
		case backoff == 0:
			backoff = minBackoff
		case backoff < maxBackoff:
			backoff = min(backoff*2, maxBackoff)
		}
		logWarn("socks accept failed: %v; retrying in %s\n", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -run 'TestAcceptLoop|TestStartSocks5|TestSocks5' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_accept_test.go
git commit -m "fix: keep SOCKS accept loop alive on transient errors"
```

### Task A-3: Parse the request address for every command and encode BND.ADDR

This task restructures the handler into a `socksServer`. Negotiation still accepts only no-auth (A-4 changes that); UDP ASSOCIATE keeps its old body for now (A-7 rewrites it).

**Files:**
- Modify: `socks.go` (imports, constants, `SocksOptions.HandshakeTimeout`, `socksServer`, replace `handleSocksConn` and `writeSocksReply`, adapt `runUDPAssociate` header)
- Create: `socks_helpers_test.go`, `socks_proto_test.go`

**Interfaces:**
- Consumes: `SocksOptions`, `StartSocks5`, `newSocksResolver`, `acceptLoop` (A-1, A-2); `grabFreeAddr` (existing in `socks_test.go`).
- Produces:
  - constants `socksVersion5`, `socksMethodNoAuth`, `socksCmdConnect`, `socksCmdUDPAssociate`, `socksATYPIPv4`, `socksATYPDomain`, `socksATYPIPv6`, `socksRepSucceeded`, `socksRepGeneralFailure`, `socksRepNetUnreachable`, `socksRepHostUnreachable`, `socksRepConnRefused`, `socksRepCmdNotSupported`, `socksRepAddrTypeNotSupported`, `defaultSocksHandshakeTimeout`
  - `SocksOptions.HandshakeTimeout time.Duration` (zero means 10 s)
  - `type socksServer struct { opts SocksOptions; resolver *net.Resolver }` with methods `handleConn`, `negotiate`, `handleConnect`, `bindControl`, `resolve`, `routeViaVPN`, `runUDPAssociate`
  - `type socksAddr struct { atyp byte; host string; port int }`, `func (a socksAddr) String() string`
  - `var errSocksBadATYP`
  - `func readSocksAddr(r io.Reader) (socksAddr, error)`
  - `func writeSocksReply(w io.Writer, rep byte, bindAddr net.Addr) error`
  - `func appendSocksIP(b []byte, ip net.IP) []byte`
  - `func dialErrorReply(err error) byte`
  - `func relayTCP(dst, src net.Conn, wg *sync.WaitGroup)`
  - Test helpers: `startSocksForTest(t, SocksOptions) string`, `startTCPEcho(t, network, addr string) *net.TCPAddr`, `dialSocks(t, proxy string) net.Conn`, `socksGreet(t, c, methods ...byte) byte`, `encodeSocksAddr(host string, port int) []byte`, `socksRequest(t, c, cmd byte, host string, port int) (byte, socksAddr)`, `assertSocksEcho(t, proxy, host string, port int)`

- [ ] **Step 1: Write the failing tests**

Create `socks_helpers_test.go`:

```go
package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func startSocksForTest(t *testing.T, opts SocksOptions) string {
	t.Helper()
	if opts.ListenAddr == "" {
		opts.ListenAddr = grabFreeAddr(t)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := StartSocks5(ctx, opts)
	if err != nil {
		cancel()
		t.Fatalf("StartSocks5: %v", err)
	}
	t.Cleanup(func() {
		_ = stop()
		cancel()
	})
	return opts.ListenAddr
}

func startTCPEcho(t *testing.T, network, addr string) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("cannot listen on %s %s: %v", network, addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr)
}

func dialSocks(t *testing.T, proxy string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func socksGreet(t *testing.T, c net.Conn, methods ...byte) byte {
	t.Helper()
	if _, err := c.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("read method selection: %v", err)
	}
	if resp[0] != 5 {
		t.Fatalf("bad version in method selection: %v", resp)
	}
	return resp[1]
}

func encodeSocksAddr(host string, port int) []byte {
	var b []byte
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			b = append([]byte{1}, v4...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		b = append([]byte{3, byte(len(host))}, host...)
	}
	return append(b, byte(port>>8), byte(port))
}

func socksRequest(t *testing.T, c net.Conn, cmd byte, host string, port int) (byte, socksAddr) {
	t.Helper()
	req := append([]byte{5, cmd, 0}, encodeSocksAddr(host, port)...)
	if _, err := c.Write(req); err != nil {
		t.Fatalf("write request: %v", err)
	}
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(c, hdr); err != nil {
		t.Fatalf("read reply header: %v", err)
	}
	bnd, err := readSocksAddr(c)
	if err != nil {
		t.Fatalf("read reply address: %v", err)
	}
	return hdr[1], bnd
}

func assertSocksEcho(t *testing.T, proxy, host string, port int) {
	t.Helper()
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth); m != socksMethodNoAuth {
		t.Fatalf("method = %#x", m)
	}
	rep, bnd := socksRequest(t, c, socksCmdConnect, host, port)
	if rep != socksRepSucceeded {
		t.Fatalf("CONNECT %s:%d rep = %d", host, port, rep)
	}
	if bnd.port == 0 {
		t.Fatal("reply BND.PORT is 0; bind address was not encoded")
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("write through tunnel: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}
```

Create `socks_proto_test.go`:

```go
package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

func TestReadSocksAddr(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		want    socksAddr
		wantErr error
	}{
		{"ipv4", []byte{1, 10, 0, 0, 1, 0x01, 0xBB}, socksAddr{atyp: 1, host: "10.0.0.1", port: 443}, nil},
		{"domain", append(append([]byte{3, 11}, "example.com"...), 0, 80), socksAddr{atyp: 3, host: "example.com", port: 80}, nil},
		{"ipv6", append(append([]byte{4}, net.ParseIP("2001:db8::1").To16()...), 0x1F, 0x90), socksAddr{atyp: 4, host: "2001:db8::1", port: 8080}, nil},
		{"bad atyp", []byte{9, 0, 0}, socksAddr{atyp: 9}, errSocksBadATYP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readSocksAddr(bytes.NewReader(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWriteSocksReply_EncodesBindAddr(t *testing.T) {
	v6 := net.ParseIP("2001:db8::2")
	cases := []struct {
		name string
		addr net.Addr
		want []byte
	}{
		{"nil", nil, []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"tcp4", &net.TCPAddr{IP: net.IPv4(192, 0, 2, 7), Port: 1080}, []byte{5, 0, 0, 1, 192, 0, 2, 7, 0x04, 0x38}},
		{"udp6", &net.UDPAddr{IP: v6, Port: 53}, append(append([]byte{5, 0, 0, 4}, v6.To16()...), 0, 53)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeSocksReply(&buf, socksRepSucceeded, tc.addr); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf.Bytes(), tc.want) {
				t.Fatalf("got %v, want %v", buf.Bytes(), tc.want)
			}
		})
	}
}

func TestSocksConnect_AddressTypes(t *testing.T) {
	echo4 := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{})
	t.Run("ipv4", func(t *testing.T) { assertSocksEcho(t, proxy, "127.0.0.1", echo4.Port) })
	t.Run("domain", func(t *testing.T) { assertSocksEcho(t, proxy, "localhost", echo4.Port) })
	t.Run("ipv6", func(t *testing.T) {
		echo6 := startTCPEcho(t, "tcp6", "[::1]:0")
		assertSocksEcho(t, proxy, "::1", echo6.Port)
	})
}

func TestSocksRequest_UnsupportedCommand(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	const cmdBind = 0x02
	if rep, _ := socksRequest(t, c, cmdBind, "127.0.0.1", 80); rep != socksRepCmdNotSupported {
		t.Fatalf("BIND rep = %d, want %d", rep, socksRepCmdNotSupported)
	}
}

func TestSocksRequest_UnsupportedAddressType(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if _, err := c.Write([]byte{5, socksCmdConnect, 0, 9}); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil || resp[1] != socksRepAddrTypeNotSupported {
		t.Fatalf("reply = %v, %v; want rep %d", resp, err, socksRepAddrTypeNotSupported)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestReadSocksAddr|TestWriteSocksReply|TestSocksConnect|TestSocksRequest' ./...`
Expected: FAIL to compile with `undefined: readSocksAddr`, `undefined: socksAddr`, `undefined: socksMethodNoAuth`.

- [ ] **Step 3: Implement**

`socks.go` imports become:

```go
import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)
```

Add directly after the import block:

```go
const (
	socksVersion5 = 0x05

	socksMethodNoAuth = 0x00

	socksCmdConnect      = 0x01
	socksCmdUDPAssociate = 0x03

	socksATYPIPv4   = 0x01
	socksATYPDomain = 0x03
	socksATYPIPv6   = 0x04

	socksRepSucceeded            = 0x00
	socksRepGeneralFailure       = 0x01
	socksRepNetUnreachable       = 0x03
	socksRepHostUnreachable      = 0x04
	socksRepConnRefused          = 0x05
	socksRepCmdNotSupported      = 0x07
	socksRepAddrTypeNotSupported = 0x08

	defaultSocksHandshakeTimeout = 10 * time.Second
)

var errSocksBadATYP = errors.New("unsupported SOCKS address type")
```

Add to `SocksOptions`:

```go
	HandshakeTimeout time.Duration // zero means defaultSocksHandshakeTimeout
```

Replace `StartSocks5` with:

```go
// StartSocks5 starts a SOCKS5 proxy and returns a stop function.
func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error) {
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = defaultSocksHandshakeTimeout
	}
	srv := &socksServer{opts: opts, resolver: newSocksResolver(opts.DNSServers)}
	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(conn net.Conn) { go srv.handleConn(ctx, conn) })
	}()
	stop := func() error { _ = ln.Close(); <-done; return nil }
	return stop, nil
}

type socksServer struct {
	opts     SocksOptions
	resolver *net.Resolver
}
```

Delete the old `handleSocksConn` function and the old `writeSocksReply` function entirely, and add:

```go
func (s *socksServer) handleConn(ctx context.Context, c net.Conn) {
	defer func() { _ = c.Close() }()
	// Bounded handshake so idle clients cannot pin goroutines; each session clears it.
	_ = c.SetDeadline(time.Now().Add(s.opts.HandshakeTimeout))

	if err := s.negotiate(c); err != nil {
		if s.opts.Debug {
			fmt.Printf("[socks] handshake from %s failed: %v\n", c.RemoteAddr(), err)
		}
		return
	}
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	if hdr[0] != socksVersion5 {
		_ = writeSocksReply(c, socksRepGeneralFailure, nil)
		return
	}
	dst, err := readSocksAddr(c)
	if err != nil {
		if errors.Is(err, errSocksBadATYP) {
			_ = writeSocksReply(c, socksRepAddrTypeNotSupported, nil)
		}
		return
	}
	switch hdr[1] {
	case socksCmdConnect:
		s.handleConnect(ctx, c, dst)
	case socksCmdUDPAssociate:
		s.runUDPAssociate(ctx, c)
	default:
		_ = writeSocksReply(c, socksRepCmdNotSupported, nil)
	}
}

func (s *socksServer) negotiate(c net.Conn) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		return err
	}
	if head[0] != socksVersion5 {
		return fmt.Errorf("unsupported SOCKS version %d", head[0])
	}
	if _, err := io.ReadFull(c, make([]byte, int(head[1]))); err != nil {
		return err
	}
	_, err := c.Write([]byte{socksVersion5, socksMethodNoAuth})
	return err
}

type socksAddr struct {
	atyp byte
	host string // IP literal or domain name
	port int
}

func (a socksAddr) String() string { return net.JoinHostPort(a.host, strconv.Itoa(a.port)) }

func readSocksAddr(r io.Reader) (socksAddr, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return socksAddr{}, err
	}
	var host string
	switch atyp[0] {
	case socksATYPIPv4, socksATYPIPv6:
		size := net.IPv4len
		if atyp[0] == socksATYPIPv6 {
			size = net.IPv6len
		}
		b := make([]byte, size)
		if _, err := io.ReadFull(r, b); err != nil {
			return socksAddr{}, err
		}
		host = net.IP(b).String()
	case socksATYPDomain:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return socksAddr{}, err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(r, b); err != nil {
			return socksAddr{}, err
		}
		host = string(b)
	default:
		return socksAddr{atyp: atyp[0]}, errSocksBadATYP
	}
	var p [2]byte
	if _, err := io.ReadFull(r, p[:]); err != nil {
		return socksAddr{}, err
	}
	return socksAddr{atyp: atyp[0], host: host, port: int(binary.BigEndian.Uint16(p[:]))}, nil
}

func (s *socksServer) handleConnect(ctx context.Context, c net.Conn, dst socksAddr) {
	var domain string
	ip := net.ParseIP(dst.host)
	if dst.atyp == socksATYPDomain {
		domain = strings.ToLower(dst.host)
		if ip = s.resolve(ctx, dst.host); ip == nil {
			_ = writeSocksReply(c, socksRepHostUnreachable, nil)
			return
		}
	}
	target := net.JoinHostPort(ip.String(), strconv.Itoa(dst.port))
	useVPN := s.routeViaVPN(domain)
	if s.opts.Debug {
		fmt.Printf("[socks] CONNECT %s (ip=%s) bindIf=%s useVPN=%v\n", dst, ip, s.opts.BindIf, useVPN)
	}
	d := net.Dialer{Timeout: 30 * time.Second}
	if useVPN && s.opts.BindIf != "" {
		d.Control = s.bindControl
	}
	rc, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		rep := dialErrorReply(err)
		if s.opts.Debug {
			fmt.Printf("[socks] dial error to %s: %v (rep=%d)\n", target, err, rep)
		}
		_ = writeSocksReply(c, rep, nil)
		return
	}
	defer func() { _ = rc.Close() }()
	if err := writeSocksReply(c, socksRepSucceeded, rc.LocalAddr()); err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	var wg sync.WaitGroup
	wg.Add(2)
	go relayTCP(rc, c, &wg)
	go relayTCP(c, rc, &wg)
	wg.Wait()
}

func relayTCP(dst, src net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if tc, ok := dst.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}

func (s *socksServer) bindControl(_, _ string, rc syscall.RawConn) error {
	var bindErr error
	if err := rc.Control(func(fd uintptr) { bindErr = bindFDToInterface(int(fd), s.opts.BindIf) }); err != nil {
		return err
	}
	return bindErr
}

// resolve prefers IPv4 because the tunnel drops IPv6 unless --enable_ipv6 is set.
func (s *socksServer) resolve(ctx context.Context, host string) net.IP {
	addrs, _ := s.resolver.LookupIP(ctx, "ip", host)
	var pick net.IP
	for _, ip := range addrs {
		if ip.To4() != nil {
			return ip
		}
		if pick == nil {
			pick = ip
		}
	}
	return pick
}

func (s *socksServer) routeViaVPN(domain string) bool {
	if len(s.opts.AllowDomains) > 0 && (domain == "" || !domainMatches(domain, s.opts.AllowDomains)) {
		return false
	}
	return domain == "" || !domainMatches(domain, s.opts.ExcludeDomains)
}

func dialErrorReply(err error) byte {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return socksRepHostUnreachable
	}
	var se syscall.Errno
	if errors.As(err, &se) {
		switch se {
		case syscall.ECONNREFUSED:
			return socksRepConnRefused
		case syscall.ENETUNREACH:
			return socksRepNetUnreachable
		case syscall.EHOSTUNREACH, syscall.ETIMEDOUT:
			return socksRepHostUnreachable
		}
	}
	return socksRepGeneralFailure
}

func writeSocksReply(w io.Writer, rep byte, bindAddr net.Addr) error {
	ip, port := net.IPv4zero, 0
	switch a := bindAddr.(type) {
	case *net.TCPAddr:
		ip, port = a.IP, a.Port
	case *net.UDPAddr:
		ip, port = a.IP, a.Port
	}
	resp := appendSocksIP([]byte{socksVersion5, rep, 0x00}, ip)
	resp = binary.BigEndian.AppendUint16(resp, uint16(port))
	_, err := w.Write(resp)
	return err
}

func appendSocksIP(b []byte, ip net.IP) []byte {
	if v4 := ip.To4(); v4 != nil {
		return append(append(b, socksATYPIPv4), v4...)
	}
	if v6 := ip.To16(); v6 != nil {
		return append(append(b, socksATYPIPv6), v6...)
	}
	return append(b, socksATYPIPv4, 0, 0, 0, 0)
}
```

Adapt the existing `runUDPAssociate` (body unchanged until A-7). Replace its signature line:

```go
func runUDPAssociate(ctx context.Context, ctrl net.Conn, bindIf string, debug bool, allowDomains, excludeDomains []string, resolver *net.Resolver) {
```

with:

```go
func (s *socksServer) runUDPAssociate(ctx context.Context, ctrl net.Conn) {
	bindIf, debug, allowDomains, excludeDomains, resolver := s.opts.BindIf, s.opts.Debug, s.opts.AllowDomains, s.opts.ExcludeDomains, s.resolver
```

Keep `domainMatches` and `bindFDToInterface` as they are.

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestReadSocksAddr|TestWriteSocksReply|TestSocksConnect|TestSocksRequest|TestSocks5|TestStartSocks5' ./...`
Expected: PASS (the ipv6 subtest may SKIP on hosts without `::1`).

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_helpers_test.go socks_proto_test.go
git commit -m "fix: parse SOCKS request address for all commands and encode BND"
```

### Task A-4: RFC 1928 method selection and RFC 1929 username/password

**Files:**
- Create: `socks_auth.go`, `socks_auth_test.go`
- Modify: `socks.go` (constants, `SocksOptions.Auth`, `StartSocks5` validation, `negotiate`)

**Interfaces:**
- Consumes: A-3 helpers and `socksServer`.
- Produces:
  - constants `socksMethodUserPass = 0x02`, `socksMethodNoAcceptable = 0xFF`
  - `type SocksAuth struct { User, Pass string }`, `func (a SocksAuth) Enabled() bool`, `func (a SocksAuth) validate() error`, `func (a SocksAuth) matches(user, pass []byte) bool`
  - `func selectSocksMethod(offered []byte, authRequired bool) byte`
  - `func socksUserPassAuth(rw io.ReadWriter, want SocksAuth) error`, `var errSocksAuthFailed`
  - `SocksOptions.Auth SocksAuth`
  - test helper `socksUserPass(t, c, user, pass string) byte`

- [ ] **Step 1: Write the failing tests**

Create `socks_auth_test.go`:

```go
package main

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
)

func TestSelectSocksMethod(t *testing.T) {
	cases := []struct {
		name    string
		offered []byte
		auth    bool
		want    byte
	}{
		{"no-auth offered, auth off", []byte{0x00}, false, 0x00},
		{"only user/pass offered, auth off", []byte{0x02}, false, 0xFF},
		{"nothing offered", nil, false, 0xFF},
		{"gssapi only", []byte{0x01}, false, 0xFF},
		{"both offered, auth on", []byte{0x00, 0x02}, true, 0x02},
		{"only no-auth offered, auth on", []byte{0x00}, true, 0xFF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectSocksMethod(tc.offered, tc.auth); got != tc.want {
				t.Fatalf("got %#x, want %#x", got, tc.want)
			}
		})
	}
}

func TestSocksAuthValidate(t *testing.T) {
	long := strings.Repeat("u", 256)
	cases := []struct {
		name string
		a    SocksAuth
		ok   bool
	}{
		{"disabled", SocksAuth{}, true},
		{"complete", SocksAuth{User: "u", Pass: "p"}, true},
		{"user only", SocksAuth{User: "u"}, false},
		{"pass only", SocksAuth{Pass: "p"}, false},
		{"user too long", SocksAuth{User: long, Pass: "p"}, false},
		{"pass too long", SocksAuth{User: "u", Pass: long}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.a.validate(); (err == nil) != tc.ok {
				t.Fatalf("validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func socksUserPass(t *testing.T, c net.Conn, user, pass string) byte {
	t.Helper()
	msg := append([]byte{1, byte(len(user))}, user...)
	msg = append(append(msg, byte(len(pass))), pass...)
	if _, err := c.Write(msg); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("read auth status: %v", err)
	}
	return resp[1]
}

func TestSocksHandshake_NoAcceptableMethodClosesConnection(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, 0x02); m != 0xFF {
		t.Fatalf("method = %#x, want 0xFF", m)
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection still open after 0xFF")
	}
}

func TestSocksUserPass_SuccessThenConnect(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth, socksMethodUserPass); m != socksMethodUserPass {
		t.Fatalf("method = %#x, want user/pass", m)
	}
	if st := socksUserPass(t, c, "alice", "s3cret"); st != 0 {
		t.Fatalf("auth status = %d, want 0", st)
	}
	if rep, _ := socksRequest(t, c, socksCmdConnect, "127.0.0.1", echo.Port); rep != socksRepSucceeded {
		t.Fatalf("CONNECT after auth rep = %d", rep)
	}
}

func TestSocksUserPass_WrongPasswordRejected(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodUserPass)
	if st := socksUserPass(t, c, "alice", "wrong"); st == 0 {
		t.Fatal("wrong password accepted")
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection still open after failed auth")
	}
}

func TestSocksUserPass_NoAuthClientRejected(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth); m != 0xFF {
		t.Fatalf("method = %#x, want 0xFF when auth is required", m)
	}
}

func TestStartSocks5_RejectsHalfConfiguredAuth(t *testing.T) {
	_, err := StartSocks5(context.Background(), SocksOptions{ListenAddr: "127.0.0.1:0", Auth: SocksAuth{User: "alice"}})
	if err == nil {
		t.Fatal("want error for a username without a password")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestSelectSocksMethod|TestSocksAuth|TestSocksHandshake|TestSocksUserPass|TestStartSocks5_Rejects' ./...`
Expected: FAIL to compile with `undefined: selectSocksMethod`, `undefined: SocksAuth`.

- [ ] **Step 3: Implement**

Create `socks_auth.go`:

```go
package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
)

// SocksAuth holds RFC 1929 credentials. The zero value disables authentication.
type SocksAuth struct {
	User string
	Pass string
}

func (a SocksAuth) Enabled() bool { return a.User != "" || a.Pass != "" }

func (a SocksAuth) validate() error {
	if !a.Enabled() {
		return nil
	}
	if a.User == "" || a.Pass == "" {
		return errors.New("SOCKS auth needs both a username and a password")
	}
	if len(a.User) > 255 || len(a.Pass) > 255 {
		return errors.New("SOCKS username and password must be at most 255 bytes (RFC 1929)")
	}
	return nil
}

func (a SocksAuth) matches(user, pass []byte) bool {
	// Hashing first keeps the comparison constant-time even when lengths differ.
	wantU, gotU := sha256.Sum256([]byte(a.User)), sha256.Sum256(user)
	wantP, gotP := sha256.Sum256([]byte(a.Pass)), sha256.Sum256(pass)
	return subtle.ConstantTimeCompare(wantU[:], gotU[:])&subtle.ConstantTimeCompare(wantP[:], gotP[:]) == 1
}

func selectSocksMethod(offered []byte, authRequired bool) byte {
	want := byte(socksMethodNoAuth)
	if authRequired {
		want = socksMethodUserPass
	}
	for _, m := range offered {
		if m == want {
			return want
		}
	}
	return socksMethodNoAcceptable
}

const (
	socksUserPassVersion = 0x01
	socksUserPassOK      = 0x00
	socksUserPassFailed  = 0x01
)

var errSocksAuthFailed = errors.New("invalid SOCKS username or password")

func socksUserPassAuth(rw io.ReadWriter, want SocksAuth) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(rw, head); err != nil {
		return err
	}
	if head[0] != socksUserPassVersion {
		_, _ = rw.Write([]byte{socksUserPassVersion, socksUserPassFailed})
		return fmt.Errorf("unsupported auth sub-negotiation version %d", head[0])
	}
	user := make([]byte, int(head[1]))
	if _, err := io.ReadFull(rw, user); err != nil {
		return err
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(rw, plen); err != nil {
		return err
	}
	pass := make([]byte, int(plen[0]))
	if _, err := io.ReadFull(rw, pass); err != nil {
		return err
	}
	if !want.matches(user, pass) {
		_, _ = rw.Write([]byte{socksUserPassVersion, socksUserPassFailed})
		return errSocksAuthFailed
	}
	_, err := rw.Write([]byte{socksUserPassVersion, socksUserPassOK})
	return err
}
```

`socks.go` constant block, replace:

```go
	socksMethodNoAuth = 0x00
```

with:

```go
	socksMethodNoAuth       = 0x00
	socksMethodUserPass     = 0x02
	socksMethodNoAcceptable = 0xFF
```

Add to `SocksOptions`:

```go
	Auth             SocksAuth
```

At the top of `StartSocks5` add:

```go
	if err := opts.Auth.validate(); err != nil {
		return nil, err
	}
```

Replace `negotiate` with:

```go
func (s *socksServer) negotiate(c net.Conn) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		return err
	}
	if head[0] != socksVersion5 {
		return fmt.Errorf("unsupported SOCKS version %d", head[0])
	}
	offered := make([]byte, int(head[1]))
	if _, err := io.ReadFull(c, offered); err != nil {
		return err
	}
	method := selectSocksMethod(offered, s.opts.Auth.Enabled())
	if _, err := c.Write([]byte{socksVersion5, method}); err != nil {
		return err
	}
	switch method {
	case socksMethodNoAuth:
		return nil
	case socksMethodUserPass:
		err := socksUserPassAuth(c, s.opts.Auth)
		if errors.Is(err, errSocksAuthFailed) {
			logWarn("socks: rejected credentials from %s\n", c.RemoteAddr())
		}
		return err
	default:
		return errors.New("client offered no acceptable authentication method")
	}
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestSelectSocksMethod|TestSocksAuth|TestSocksHandshake|TestSocksUserPass|TestStartSocks5|TestSocks' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_auth.go socks_auth_test.go
git commit -m "feat: add SOCKS5 username/password auth and method negotiation"
```

### Task A-5: CLI flags and env fallback for SOCKS auth

Flag naming decision: all three commands use the same pair, `--socks_user`/`--socks_pass`, including the standalone `socks` command (whose listen flag is `--listen`). One flag pair and one env pair means one scrub rule in E-1 and one line in the docs. If the owner prefers `--listen_user`/`--listen_pass` on `socks`, only the usage line for `socks` and `parseSOCKSConfig` change.

**Files:**
- Modify: `socks_auth.go` (add `resolveSocksAuth`, env names)
- Modify: `socks_auth_test.go` (tests)
- Modify: `config.go` (`VPNConfig.SOCKSAuth`, `SOCKSConfig.Auth`, one line each in `parseVPNConfig` and `parseSOCKSConfig`)
- Modify: `vpn_core.go` (`socksOptionsFromVPN`), `cmd_socks.go`
- Modify: `main.go:27,28,32` (usage patterns), Options block after line 61

**Interfaces:**
- Consumes: `SocksAuth` (A-4), `socksOptionsFromVPN` (A-1).
- Produces:
  - `const envSocksUser = "URNETWORK_SOCKS_USER"`, `const envSocksPass = "URNETWORK_SOCKS_PASS"`
  - `func resolveSocksAuth(flagUser, flagPass string, getenv func(string) string) SocksAuth`
  - `VPNConfig.SOCKSAuth SocksAuth`, `SOCKSConfig.Auth SocksAuth`
  - docopt keys `--socks_user`, `--socks_pass` (used by E-1 and C docs)

- [ ] **Step 1: Write the failing tests**

Append to `socks_auth_test.go` (add `"github.com/docopt/docopt-go"` to its imports):

```go
func TestResolveSocksAuth(t *testing.T) {
	env := map[string]string{envSocksUser: "envuser", envSocksPass: "envpass"}
	withEnv := func(k string) string { return env[k] }
	noEnv := func(string) string { return "" }
	cases := []struct {
		name, flagUser, flagPass string
		getenv                   func(string) string
		want                     SocksAuth
	}{
		{"flags win", "cli", "clipass", withEnv, SocksAuth{User: "cli", Pass: "clipass"}},
		{"env fallback", "", "", withEnv, SocksAuth{User: "envuser", Pass: "envpass"}},
		{"mixed", "cli", "", withEnv, SocksAuth{User: "cli", Pass: "envpass"}},
		{"nothing set", "", "", noEnv, SocksAuth{}},
		{"password keeps spaces", "u", " p w ", noEnv, SocksAuth{User: "u", Pass: " p w "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveSocksAuth(tc.flagUser, tc.flagPass, tc.getenv); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseConfigs_SocksAuthFromFlagsAndEnv(t *testing.T) {
	t.Setenv(envSocksUser, "")
	t.Setenv(envSocksPass, "from-env")
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

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestResolveSocksAuth|TestParseConfigs_SocksAuth' ./...`
Expected: FAIL to compile with `undefined: resolveSocksAuth`, `cfg.SOCKSAuth undefined`.

- [ ] **Step 3: Implement**

Append to `socks_auth.go` (add `"strings"` to its imports):

```go
const (
	envSocksUser = "URNETWORK_SOCKS_USER"
	envSocksPass = "URNETWORK_SOCKS_PASS"
)

// resolveSocksAuth prefers flags and falls back to the environment so the password can
// stay off the command line, where every local user can read it.
func resolveSocksAuth(flagUser, flagPass string, getenv func(string) string) SocksAuth {
	a := SocksAuth{User: strings.TrimSpace(flagUser), Pass: flagPass}
	if a.User == "" {
		a.User = strings.TrimSpace(getenv(envSocksUser))
	}
	if a.Pass == "" {
		a.Pass = getenv(envSocksPass)
	}
	return a
}
```

`config.go`, in `VPNConfig` after `SOCKSListen         string` add:

```go
	SOCKSAuth           SocksAuth
```

In `SOCKSConfig` after `ExtenderSecret string` add:

```go
	Auth           SocksAuth
```

In `parseVPNConfig`, after `SOCKSListen:         socksListen,` add:

```go
		SOCKSAuth:           resolveSocksAuth(getStringOr(opts, "--socks_user", ""), getStringOr(opts, "--socks_pass", ""), os.Getenv),
```

In `parseSOCKSConfig`, after `ExtenderSecret: strings.TrimSpace(extSec),` add:

```go
		Auth:           resolveSocksAuth(getStringOr(opts, "--socks_user", ""), getStringOr(opts, "--socks_pass", ""), os.Getenv),
```

`vpn_core.go`, in `socksOptionsFromVPN` add the field:

```go
		Auth:           cfg.SOCKSAuth,
```

`cmd_socks.go`, add to the `SocksOptions` literal:

```go
		Auth:           cfg.Auth,
```

`main.go` usage. The quick-connect and vpn patterns contain the same substring; replace every occurrence of:

```
[--socks_listen=<addr>] [--allow_inbound_src=<list>]
```

with:

```
[--socks_listen=<addr>] [--socks_user=<user>] [--socks_pass=<pass>] [--allow_inbound_src=<list>]
```

(Edit tool: `replace_all: true`; expect exactly 2 replacements.) In the socks pattern, replace:

```
[--extender_secret=<secret>] [--domain=<list>]
```

with:

```
[--extender_secret=<secret>] [--socks_user=<user>] [--socks_pass=<pass>] [--domain=<list>]
```

In the Options block, after the line `    --socks_listen=<addr>        Alias for --socks` insert:

```
    --socks_user=<user>          SOCKS5 username (RFC 1929); env fallback URNETWORK_SOCKS_USER
    --socks_pass=<pass>          SOCKS5 password; env fallback URNETWORK_SOCKS_PASS (prefer env, argv is visible in ps)
```

Run `gofmt -w config.go vpn_core.go cmd_socks.go` afterwards.

- [ ] **Step 4: Run them to verify they pass**

```bash
go test -race -count=1 -run 'TestResolveSocksAuth|TestParseConfigs_SocksAuth' ./...
go build -o /tmp/urnet-client-a5 . && /tmp/urnet-client-a5 --help | grep -c 'socks_pass'
```

Expected: tests PASS; grep count `4` (three usage patterns plus the Options line).

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks_auth.go socks_auth_test.go config.go vpn_core.go cmd_socks.go main.go
git commit -m "feat: add --socks_user/--socks_pass flags with env fallback"
```

### Task A-6: Fail closed when the VPN interface cannot be bound

**Files:**
- Modify: `socks.go` (`bindFDToInterface`, `bindControl`, `handleConnect` dial error branch, old UDP body's `bindFDToInterface` call)
- Create: `socks_bind_test.go`

**Interfaces:**
- Consumes: A-3 helpers.
- Produces:
  - `var errBindInterface = errors.New("bind to VPN interface failed")`
  - `func bindFDToInterface(fd int, network, ifName string) error` (new `network` parameter; `tcp6`/`udp6` select the IPv6 option on macOS)

- [ ] **Step 1: Write the failing tests**

Create `socks_bind_test.go`:

```go
package main

import (
	"errors"
	"syscall"
	"testing"
)

func TestBindFDToInterface_MissingInterfaceIsError(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer func() { _ = syscall.Close(fd) }()
	if err := bindFDToInterface(fd, "tcp4", "urnet-nope0"); !errors.Is(err, errBindInterface) {
		t.Fatalf("err = %v, want errBindInterface", err)
	}
}

func TestBindFDToInterface_EmptyNameIsNoop(t *testing.T) {
	if err := bindFDToInterface(-1, "tcp4", ""); err != nil {
		t.Fatalf("empty interface name must be a no-op, got %v", err)
	}
}

func TestSocksConnect_BindFailureRefusesInsteadOfLeaking(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0"})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if rep, _ := socksRequest(t, c, socksCmdConnect, "127.0.0.1", echo.Port); rep != socksRepGeneralFailure {
		t.Fatalf("rep = %d, want general failure; the connection would have bypassed the VPN", rep)
	}
}

func TestSocksConnect_ExcludedDomainIgnoresBindIf(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0", ExcludeDomains: []string{"localhost"}})
	assertSocksEcho(t, proxy, "localhost", echo.Port)
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestBindFDToInterface|TestSocksConnect_Bind|TestSocksConnect_Excluded' ./...`
Expected: FAIL to compile (`undefined: errBindInterface`, too many arguments to `bindFDToInterface`). With only the signature changed, `TestSocksConnect_BindFailureRefusesInsteadOfLeaking` fails with `rep = 0`.

- [ ] **Step 3: Implement**

Replace `bindFDToInterface` in `socks.go` with:

```go
var errBindInterface = errors.New("bind to VPN interface failed")

const (
	darwinIPBoundIF     = 25  // IP_BOUND_IF
	darwinIPv6BoundIF   = 125 // IPV6_BOUND_IF
	linuxSOBindToDevice = 25  // SO_BINDTODEVICE, needs CAP_NET_RAW
)

// bindFDToInterface pins a socket to ifName. Any failure is returned so the caller refuses
// the request; silently continuing would send VPN traffic out the normal default route.
func bindFDToInterface(fd int, network, ifName string) error {
	if strings.TrimSpace(ifName) == "" {
		return nil
	}
	ifi, err := net.InterfaceByName(ifName)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	switch runtime.GOOS {
	case "darwin":
		level, opt := syscall.IPPROTO_IP, darwinIPBoundIF
		if strings.HasSuffix(network, "6") {
			level, opt = syscall.IPPROTO_IPV6, darwinIPv6BoundIF
		}
		err = syscall.SetsockoptInt(fd, level, opt, ifi.Index)
	case "linux":
		err = syscall.SetsockoptString(fd, syscall.SOL_SOCKET, linuxSOBindToDevice, ifName)
	default:
		err = fmt.Errorf("not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	return nil
}
```

Replace `bindControl` with:

```go
func (s *socksServer) bindControl(network, _ string, rc syscall.RawConn) error {
	var bindErr error
	if err := rc.Control(func(fd uintptr) { bindErr = bindFDToInterface(int(fd), network, s.opts.BindIf) }); err != nil {
		return err
	}
	return bindErr
}
```

In `handleConnect`, replace the dial error branch:

```go
	if err != nil {
		rep := dialErrorReply(err)
		if s.opts.Debug {
```

with:

```go
	if err != nil {
		rep := dialErrorReply(err)
		if errors.Is(err, errBindInterface) {
			logWarn("socks: %v; refusing CONNECT to %s instead of sending it outside the VPN\n", err, dst)
		}
		if s.opts.Debug {
```

In the old `runUDPAssociate` body, change the call inside the `lc` Control func from:

```go
				retErr = bindFDToInterface(int(fd), bindIf)
```

to:

```go
				retErr = bindFDToInterface(int(fd), network, bindIf)
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestBindFDToInterface|TestSocksConnect' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_bind_test.go
git commit -m "fix: refuse SOCKS requests when the VPN interface cannot be bound"
```

### Task A-7: UDP ASSOCIATE that lives as long as its control connection

**Files:**
- Modify: `socks.go` (imports add `bytes`; replace `runUDPAssociate`; add `relayFromClient`, `relayToClient`, `addrIP`)
- Create: `socks_udp_test.go`

**Interfaces:**
- Consumes: `socksServer`, `readSocksAddr`, `writeSocksReply`, `appendSocksIP`, `bindControl`, `resolve`, `routeViaVPN` (A-3, A-6), `SocksOptions.HandshakeTimeout` (A-3), test helpers (A-3).
- Produces:
  - `func (s *socksServer) runUDPAssociate(ctx context.Context, ctrl net.Conn)` (new body)
  - `func (s *socksServer) relayFromClient(ctx context.Context, pcClient, pcVPN, pcSys net.PacketConn, clientIP net.IP, client *atomic.Pointer[net.UDPAddr])`
  - `func relayToClient(pc, pcClient net.PacketConn, client *atomic.Pointer[net.UDPAddr])`
  - `func addrIP(a net.Addr) net.IP` (also used by A-8)

- [ ] **Step 1: Write the failing tests**

Create `socks_udp_test.go`:

```go
package main

import (
	"bytes"
	"net"
	"runtime"
	"testing"
	"time"
)

func startUDPEcho(t *testing.T) *net.UDPAddr {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr)
}

func socksUDPDatagram(host string, port int, payload []byte) []byte {
	return append(append([]byte{0, 0, 0}, encodeSocksAddr(host, port)...), payload...)
}

func udpAssociate(t *testing.T, proxy string) (net.Conn, *net.UDPAddr) {
	t.Helper()
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	rep, bnd := socksRequest(t, c, socksCmdUDPAssociate, "0.0.0.0", 0)
	if rep != socksRepSucceeded {
		t.Fatalf("UDP ASSOCIATE rep = %d", rep)
	}
	_ = c.SetDeadline(time.Time{})
	return c, &net.UDPAddr{IP: net.ParseIP(bnd.host), Port: bnd.port}
}

func newUDPClient(t *testing.T, addr string) net.PacketConn {
	t.Helper()
	pc, err := net.ListenPacket("udp4", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

func udpRoundTrip(t *testing.T, client net.PacketConn, relay *net.UDPAddr, echoPort int, msg string) error {
	t.Helper()
	if _, err := client.WriteTo(socksUDPDatagram("127.0.0.1", echoPort, []byte(msg)), relay); err != nil {
		return err
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2048)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		return err
	}
	from, err := readSocksAddr(bytes.NewReader(buf[3:n]))
	if err != nil || from.port != echoPort {
		t.Fatalf("reply header = %+v, %v", from, err)
	}
	if !bytes.HasSuffix(buf[:n], []byte(msg)) {
		t.Fatalf("reply payload = %q, want suffix %q", buf[:n], msg)
	}
	return nil
}

func TestSocksUDPAssociate_RoundTripOutlivesHandshakeTimeout(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{HandshakeTimeout: 150 * time.Millisecond})
	_, relay := udpAssociate(t, proxy)
	time.Sleep(400 * time.Millisecond)

	client := newUDPClient(t, "127.0.0.1:0")
	if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
		t.Fatalf("no reply through relay after the handshake timeout: %v", err)
	}
}

func TestSocksUDPAssociate_TornDownWhenControlCloses(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{})
	base := runtime.NumGoroutine()
	ctrl, relay := udpAssociate(t, proxy)
	client := newUDPClient(t, "127.0.0.1:0")
	if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
		t.Fatalf("association not working: %v", err)
	}

	_ = ctrl.Close()
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines leaked: before=%d after=%d", base, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := udpRoundTrip(t, client, relay, echo.Port, "late"); err == nil {
		t.Fatal("relay still forwarding after the control connection closed")
	}
}

func TestSocksUDPAssociate_DropsDatagramsFromOtherHosts(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{})
	_, relay := udpAssociate(t, proxy)
	foreign := newUDPClient(t, "127.0.0.2:0")
	if err := udpRoundTrip(t, foreign, relay, echo.Port, "spoof"); err == nil {
		t.Fatal("relay forwarded a datagram from a host other than the SOCKS client")
	}
}

func TestSocksUDPAssociate_BindFailureRefuses(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0"})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if rep, _ := socksRequest(t, c, socksCmdUDPAssociate, "0.0.0.0", 0); rep != socksRepGeneralFailure {
		t.Fatalf("rep = %d, want general failure instead of an unbound UDP path", rep)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -race -count=1 -run TestSocksUDPAssociate ./...`
Expected: FAIL. `RoundTripOutlivesHandshakeTimeout` fails with `i/o timeout` (association closed by the 150 ms deadline), `TornDown` fails on the goroutine count, `BindFailureRefuses` fails with `rep = 0`. `DropsDatagramsFromOtherHosts` fails on Linux and SKIPs on macOS (no `127.0.0.2` by default).

- [ ] **Step 3: Implement**

Add `"bytes"` to the `socks.go` imports. Replace the whole `runUDPAssociate` function with:

```go
func (s *socksServer) runUDPAssociate(ctx context.Context, ctrl net.Conn) {
	var (
		wg    sync.WaitGroup
		conns []net.PacketConn
	)
	defer func() {
		for _, pc := range conns {
			_ = pc.Close()
		}
		wg.Wait()
	}()
	refuse := func(format string, args ...any) {
		logWarn("socks udp: "+format, args...)
		_ = writeSocksReply(ctrl, socksRepGeneralFailure, nil)
	}

	// Listen where the client reached us, so LAN clients get a reachable relay address.
	relayAddr := net.JoinHostPort(addrIP(ctrl.LocalAddr()).String(), "0")
	pcClient, err := net.ListenPacket("udp", relayAddr)
	if err != nil {
		refuse("relay listen on %s: %v\n", relayAddr, err)
		return
	}
	conns = append(conns, pcClient)

	var pcVPN net.PacketConn
	if s.opts.BindIf != "" {
		lc := net.ListenConfig{Control: s.bindControl}
		if pcVPN, err = lc.ListenPacket(ctx, "udp4", "0.0.0.0:0"); err != nil {
			refuse("%v; refusing UDP ASSOCIATE instead of sending it outside the VPN\n", err)
			return
		}
		conns = append(conns, pcVPN)
	}
	pcSys, err := net.ListenPacket("udp", ":0")
	if err != nil {
		refuse("system socket: %v\n", err)
		return
	}
	conns = append(conns, pcSys)

	if err := writeSocksReply(ctrl, socksRepSucceeded, pcClient.LocalAddr()); err != nil {
		return
	}
	_ = ctrl.SetDeadline(time.Time{})
	if s.opts.Debug {
		fmt.Printf("[socks] UDP ASSOCIATE relay %s bindIf=%s\n", pcClient.LocalAddr(), s.opts.BindIf)
	}

	var client atomic.Pointer[net.UDPAddr]
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.relayFromClient(ctx, pcClient, pcVPN, pcSys, addrIP(ctrl.RemoteAddr()), &client)
	}()
	for _, pc := range conns[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			relayToClient(pc, pcClient, &client)
		}()
	}
	// RFC 1928: the association ends when the TCP control connection ends.
	_, _ = io.Copy(io.Discard, ctrl)
}

func (s *socksServer) relayFromClient(ctx context.Context, pcClient, pcVPN, pcSys net.PacketConn, clientIP net.IP, client *atomic.Pointer[net.UDPAddr]) {
	buf := make([]byte, 65535)
	for {
		n, from, err := pcClient.ReadFrom(buf)
		if err != nil {
			return
		}
		src, ok := from.(*net.UDPAddr)
		if !ok || !src.IP.Equal(clientIP) {
			continue
		}
		client.Store(src)
		if n < 4 || buf[2] != 0 { // fragmented datagrams are not supported
			continue
		}
		r := bytes.NewReader(buf[3:n])
		dst, err := readSocksAddr(r)
		if err != nil {
			continue
		}
		payload := buf[n-r.Len() : n]
		var domain string
		ip := net.ParseIP(dst.host)
		if dst.atyp == socksATYPDomain {
			domain = strings.ToLower(dst.host)
			if ip = s.resolve(ctx, dst.host); ip == nil {
				continue
			}
		}
		pc := pcSys
		if s.opts.BindIf != "" && s.routeViaVPN(domain) {
			pc = pcVPN
		}
		if s.opts.Debug {
			fmt.Printf("[socks-udp] -> %s:%d via %s\n", ip, dst.port, pc.LocalAddr())
		}
		_, _ = pc.WriteTo(payload, &net.UDPAddr{IP: ip, Port: dst.port})
	}
}

func relayToClient(pc, pcClient net.PacketConn, client *atomic.Pointer[net.UDPAddr]) {
	buf := make([]byte, 65535)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		src, ok := from.(*net.UDPAddr)
		dst := client.Load()
		if !ok || dst == nil {
			continue
		}
		pkt := appendSocksIP([]byte{0, 0, 0}, src.IP)
		pkt = binary.BigEndian.AppendUint16(pkt, uint16(src.Port))
		pkt = append(pkt, buf[:n]...)
		_, _ = pcClient.WriteTo(pkt, dst)
	}
}

func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	}
	return nil
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run TestSocksUDPAssociate ./...`
Expected: PASS (`DropsDatagramsFromOtherHosts` may SKIP on macOS).

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_udp_test.go
git commit -m "fix: keep SOCKS UDP associations alive and close them with control"
```

### Task A-8: Warn on unauthenticated non-loopback SOCKS, lock down compose, document

**Files:**
- Modify: `socks.go` (`StartSocks5`, new `socksExposedWithoutAuth`)
- Create: `socks_exposure_test.go`
- Modify: `docker-compose.linux.yml:17`, `docker-compose.macos.yml:7-8`
- Modify: `docs/command-reference.md` (SOCKS section), `docs/examples.md` (standalone socks example, new LAN section), `docs/docker.md` (overrides section)

**Interfaces:**
- Consumes: `addrIP` (A-7), `SocksAuth.Enabled` (A-4).
- Produces: `func socksExposedWithoutAuth(addr net.Addr, auth SocksAuth) bool`

- [ ] **Step 1: Write the failing test**

Create `socks_exposure_test.go`:

```go
package main

import (
	"net"
	"testing"
)

func TestSocksExposedWithoutAuth(t *testing.T) {
	auth := SocksAuth{User: "u", Pass: "p"}
	lan := &net.TCPAddr{IP: net.IPv4(192, 168, 88, 2), Port: 1080}
	cases := []struct {
		name string
		addr net.Addr
		auth SocksAuth
		want bool
	}{
		{"loopback v4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1080}, SocksAuth{}, false},
		{"loopback v6", &net.TCPAddr{IP: net.IPv6loopback, Port: 1080}, SocksAuth{}, false},
		{"all interfaces v4", &net.TCPAddr{IP: net.IPv4zero, Port: 1080}, SocksAuth{}, true},
		{"all interfaces v6", &net.TCPAddr{IP: net.IPv6unspecified, Port: 1080}, SocksAuth{}, true},
		{"lan address", lan, SocksAuth{}, true},
		{"lan address with auth", lan, auth, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := socksExposedWithoutAuth(tc.addr, tc.auth); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestSocksExposedWithoutAuth ./...`
Expected: FAIL to compile with `undefined: socksExposedWithoutAuth`.

- [ ] **Step 3: Implement**

Add to `socks.go`:

```go
func socksExposedWithoutAuth(addr net.Addr, auth SocksAuth) bool {
	if auth.Enabled() {
		return false
	}
	ip := addrIP(addr)
	return ip == nil || !ip.IsLoopback()
}
```

In `StartSocks5`, right after the successful `net.Listen`, add:

```go
	if socksExposedWithoutAuth(ln.Addr(), opts.Auth) {
		logWarn("SOCKS5 on %s accepts connections from other hosts WITHOUT authentication; anyone who can reach this port can use your VPN. Set --socks_user and URNETWORK_SOCKS_PASS, or bind 127.0.0.1\n", ln.Addr())
	}
```

`docker-compose.linux.yml`, replace:

```yaml
      - --socks=0.0.0.0:1080
```

with:

```yaml
      - --socks=127.0.0.1:1080
```

`docker-compose.macos.yml`, replace:

```yaml
    ports:
      - "1080:1080"
```

with:

```yaml
    ports:
      - "127.0.0.1:1080:1080"
```

(keep `--socks=0.0.0.0:1080` there: inside the bridge network the proxy must listen on the container interface; the host only publishes it on loopback.)

`docs/command-reference.md`, replace the whole `### SOCKS` section (from `### SOCKS` up to, not including, `### Inbound filtering`) with:

~~~markdown
### SOCKS

`vpn` and `quick-connect`:

- `--socks=<addr>` (alias: `--socks_listen`): start a SOCKS5 proxy. With `--tun`, proxied traffic is bound to the VPN interface. If that binding fails, the request is refused instead of leaving through your normal connection.
- `--socks_user=<user>` and `--socks_pass=<pass>`: require RFC 1929 username/password auth. Env fallbacks: `URNETWORK_SOCKS_USER`, `URNETWORK_SOCKS_PASS`. Prefer the env var for the password, because command-line arguments are visible to every local user.
- `--domain=<list>`: comma-separated domains that must route through the VPN (SOCKS only).
- `--exclude_domain=<list>`: comma-separated domains that bypass the VPN (SOCKS only).

If the proxy listens on anything other than a loopback address and no auth is configured, the client logs a WARN at startup. It still starts, so existing LAN setups keep working.

Supported SOCKS5 commands: CONNECT and UDP ASSOCIATE. BIND is answered with "command not supported".

Standalone proxy without TUN/VPN: `./urnet-client socks --listen=... --extender_ip=... --extender_port=... --extender_sni=...`

- `--listen=<addr>`
- `--extender_ip=<ip>`
- `--extender_port=<port>`
- `--extender_sni=<sni>`
- `--extender_secret=<secret>`
- `--socks_user=<user>`, `--socks_pass=<pass>` (same meaning and env fallbacks as above)

~~~

`docs/examples.md`, replace the `## Standalone SOCKS subcommand` section with:

~~~markdown
## SOCKS proxy for your LAN (for example a MikroTik container)

Other devices on the LAN can use the VPN through the proxy. Always set credentials when binding a non-loopback address:

```bash
export URNETWORK_SOCKS_USER=lan
export URNETWORK_SOCKS_PASS='a-long-random-secret'
sudo -E ./urnet-client vpn \
  --tun urnet0 \
  --socks=0.0.0.0:1080 \
  --location_query="country:Germany"
```

Clients then use `socks5://lan:<password>@<router-ip>:1080`. On a MikroTik container, pass the two variables as container envs instead of flags so the password does not show up in the process list.

Without credentials the proxy still starts but logs a WARN, and anyone who can reach the port can use your VPN.

## Standalone SOCKS subcommand

```bash
./urnet-client socks \
  --listen=127.0.0.1:1080 \
  --extender_ip=<IP> \
  --extender_port=443 \
  --extender_sni=<hostname>
```
~~~

`docs/docker.md`, replace the `OS-specific overrides:` bullet list with:

~~~markdown
OS-specific overrides:

- `docker-compose.linux.yml`: host networking; the proxy listens on `127.0.0.1:1080` of the host only.
- `docker-compose.macos.yml`: bridge networking; the proxy listens on `0.0.0.0:1080` inside the container and the port is published on `127.0.0.1:1080` of the host only. The startup WARN about a non-loopback bind refers to the container address and is expected here.

To offer the proxy to your LAN, change the bind (Linux: `--socks=0.0.0.0:1080`; macOS: `ports: ["1080:1080"]`) and set `URNETWORK_SOCKS_USER` / `URNETWORK_SOCKS_PASS` in `environment:`. See "SOCKS proxy for your LAN" in examples.md.
~~~

- [ ] **Step 4: Run it to verify it passes**

```bash
go test -race -count=1 -run TestSocksExposedWithoutAuth ./...
docker compose -f docker-compose.yml -f docker-compose.linux.yml config >/dev/null && docker compose -f docker-compose.yml -f docker-compose.macos.yml config >/dev/null && echo compose ok
```

Expected: PASS; `compose ok` (skip the compose check if Docker is not installed).

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add socks.go socks_exposure_test.go docker-compose.linux.yml docker-compose.macos.yml docs/command-reference.md docs/examples.md docs/docker.md
git commit -m "fix: warn on open SOCKS binds and publish compose on loopback"
```

---
## Stream E: hygiene (branch `sina/phase0_hygiene`)

Worktree: `../urnetwork-client-phase0_hygiene`. Owns `process.go`, `api_http.go`, `specs.go`, `urlcheck.go` and their tests, `README.md` (Security note). Shared hunks in `main.go`: insert after line 85, insert after line 101, line 106.

### Task E-1: Keep secrets off the background child's argv

**Files:**
- Modify: `process.go:3-37` (imports, `spawnBackground`, new helpers)
- Create: `process_test.go`
- Modify: `main.go` (insert after line 85; line 106)
- Modify: `README.md` (Security note)

**Interfaces:**
- Consumes: `getStringOr` (util.go). Docopt keys `--password`, `--jwt`, and `--socks_pass` (added by A-5; until A merges, that key is simply absent and ignored).
- Produces:
  - `type secretArg struct { Flag, Env, Value string }`
  - `var secretFlags []secretArg` (`--password`/`URNETWORK_PASSWORD`, `--jwt`/`URNETWORK_JWT`, `--socks_pass`/`URNETWORK_SOCKS_PASS`)
  - `func backgroundSecrets(opts docopt.Opts) []secretArg`
  - `func applySecretEnvFallbacks(opts docopt.Opts, getenv func(string) string)`
  - `func scrubSecretArgs(argv []string, secrets []secretArg) (args, env []string)`
  - `func spawnBackground(argv []string, secrets []secretArg) (int, error)`

Design note: the child must still see the secrets. Rather than changing how each command reads flags (which would touch files owned by A and C), the child fills missing secret flags from the environment right after docopt parsing. This also makes `URNETWORK_JWT` a general fallback for `--jwt`, which C-4 documents. `loadJWT` is not changed on purpose: quick-connect calls `loadJWT("")` to read the JWT it just minted, and an env fallback there would return the stale input token instead.

- [ ] **Step 1: Write the failing tests**

Create `process_test.go`:

```go
package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/docopt/docopt-go"
)

func testSecrets() []secretArg {
	return []secretArg{
		{Flag: "--password", Env: "URNETWORK_PASSWORD", Value: "hunter2"},
		{Flag: "--jwt", Env: "URNETWORK_JWT", Value: "eyJ.x.y"},
		{Flag: "--socks_pass", Env: "URNETWORK_SOCKS_PASS", Value: "sockspw"},
	}
}

func TestScrubSecretArgs(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{"equals form and background", []string{"urnet-client", "quick-connect", "--user_auth=me@x", "--password=hunter2", "--background"}, []string{"quick-connect", "--user_auth=me@x"}},
		{"separate value", []string{"urnet-client", "vpn", "--jwt", "eyJ.x.y", "--tun=urnet0"}, []string{"vpn", "--tun=urnet0"}},
		{"docopt prefix abbreviation", []string{"urnet-client", "quick-connect", "--pass=hunter2"}, []string{"quick-connect"}},
		{"--socks is not mistaken for --socks_pass", []string{"urnet-client", "vpn", "--socks=0.0.0.0:1080", "--socks_pass=sockspw", "--socks", "127.0.0.1:1081"}, []string{"vpn", "--socks=0.0.0.0:1080", "--socks", "127.0.0.1:1081"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := scrubSecretArgs(tc.argv, testSecrets())
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScrubSecretArgs_EnvCarriesValues(t *testing.T) {
	_, env := scrubSecretArgs([]string{"urnet-client"}, testSecrets())
	sort.Strings(env)
	want := []string{"URNETWORK_JWT=eyJ.x.y", "URNETWORK_PASSWORD=hunter2", "URNETWORK_SOCKS_PASS=sockspw"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("env = %q, want %q", env, want)
	}
}

func TestScrubSecretArgs_EmptySecretsChangeNothing(t *testing.T) {
	argv := []string{"urnet-client", "vpn", "--tun=urnet0", "--background"}
	args, env := scrubSecretArgs(argv, []secretArg{{Flag: "--jwt", Env: "URNETWORK_JWT"}})
	if !reflect.DeepEqual(args, []string{"vpn", "--tun=urnet0"}) || len(env) != 0 {
		t.Fatalf("args=%q env=%q", args, env)
	}
}

func TestApplySecretEnvFallbacks(t *testing.T) {
	opts := docopt.Opts{"--password": nil, "--jwt": "cli-jwt", "--socks_pass": nil}
	env := map[string]string{"URNETWORK_PASSWORD": "envpw", "URNETWORK_JWT": "env-jwt"}
	applySecretEnvFallbacks(opts, func(k string) string { return env[k] })
	if opts["--password"] != "envpw" {
		t.Fatalf("--password = %v, want envpw from env", opts["--password"])
	}
	if opts["--jwt"] != "cli-jwt" {
		t.Fatal("env must never override an explicit flag")
	}
	if opts["--socks_pass"] != nil {
		t.Fatalf("--socks_pass = %v, want untouched", opts["--socks_pass"])
	}
}

func TestBackgroundSecrets_ReadsParsedOptions(t *testing.T) {
	opts := docopt.Opts{"--password": "pw", "--jwt": nil}
	got := backgroundSecrets(opts)
	if len(got) != len(secretFlags) || got[0].Value != "pw" || got[1].Value != "" {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestScrubSecretArgs|TestApplySecretEnvFallbacks|TestBackgroundSecrets' ./...`
Expected: FAIL to compile with `undefined: secretArg`, `undefined: scrubSecretArgs`.

- [ ] **Step 3: Implement**

`process.go` imports become:

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/docopt/docopt-go"
)
```

Replace `spawnBackground` with:

```go
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
// through the environment. Explicit flags always win.
func applySecretEnvFallbacks(opts docopt.Opts, getenv func(string) string) {
	for _, s := range secretFlags {
		if getStringOr(opts, s.Flag, "") != "" {
			continue
		}
		if v := getenv(s.Env); v != "" {
			opts[s.Flag] = v
		}
	}
}

// scrubSecretArgs drops the program name, --background and every secret flag from argv,
// and returns env entries that carry the secret values instead. Matching requires the
// value too, because docopt accepts unambiguous prefixes (--pass) and --socks is itself
// a prefix of --socks_pass.
func scrubSecretArgs(argv []string, secrets []secretArg) (args, env []string) {
	for _, s := range secrets {
		if s.Value != "" {
			env = append(env, s.Env+"="+s.Value)
		}
	}
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		if a == "--background" || strings.HasPrefix(a, "--background=") {
			continue
		}
		name, val, hasEq := strings.Cut(a, "=")
		dropped := false
		for _, s := range secrets {
			if s.Value == "" || len(name) <= 2 || !strings.HasPrefix(name, "--") || !strings.HasPrefix(s.Flag, name) {
				continue
			}
			if hasEq && val == s.Value {
				dropped = true
				break
			}
			if !hasEq && i+1 < len(argv) && argv[i+1] == s.Value {
				i++
				dropped = true
				break
			}
		}
		if !dropped {
			args = append(args, a)
		}
	}
	return args, env
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
```

`main.go`: after the docopt error block (after line 85, the closing `}` of `if err != nil { ... os.Exit(2) }`), insert:

```go
	applySecretEnvFallbacks(opts, os.Getenv)
```

and replace line 106:

```go
			pid, err := spawnBackground(os.Args)
```

with:

```go
			pid, err := spawnBackground(os.Args, backgroundSecrets(opts))
```

`README.md`, replace the `## Security note` paragraph with:

```markdown
## Security note

Prefer environment variables over secret flags: `URNETWORK_PASSWORD` instead of `--password`, `URNETWORK_JWT` instead of `--jwt`, `URNETWORK_SOCKS_PASS` instead of `--socks_pass`. Command-line arguments appear in shell history and in `ps` for every local user. With `--background`, these three flags are removed from the child's command line and handed over through its environment.
```

- [ ] **Step 4: Run them to verify they pass**

```bash
go test -race -count=1 -run 'TestScrubSecretArgs|TestApplySecretEnvFallbacks|TestBackgroundSecrets' ./...
```

Expected: PASS.

Manual smoke (macOS or Linux, no root needed; SOCKS-only mode):

```bash
go build -o /tmp/urnet-client-e1 .
/tmp/urnet-client-e1 vpn --socks=127.0.0.1:18080 --jwt=fake.jwt.value --background
ps -o pid,args -ax | grep '[u]rnet-client-e1'
```

Expected: the child's args do not contain `fake.jwt.value`. Stop it with `kill <pid>`.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add process.go process_test.go main.go README.md
git commit -m "fix: pass secret flags to the background child via environment"
```

### Task E-2: Bounded HTTP client and startup lookup timeout

**Files:**
- Modify: `api_http.go` (imports, `defaultHTTPClient`, shared `doAPIRequest`, both helpers)
- Modify: `api_http_test.go` (new tests)
- Modify: `specs.go:3-8` (imports), `specs.go:27-40` (lookup timeout), new `providerLookupTimeout`
- Modify: `specs_test.go` (new test; add `"time"` import)

**Interfaces:**
- Consumes: nothing new. Keeps the `defaultHTTPClient httpDoer` test seam.
- Produces:
  - `const apiHTTPTimeout = 30 * time.Second`, `const maxAPIResponseBytes = 4 << 20`, `const maxAPIErrorBodyBytes = 1 << 10`
  - `func doAPIRequest(req *http.Request, out any, what string) error`
  - `var providerLookupTimeout = 15 * time.Second`

- [ ] **Step 1: Write the failing tests**

Append to `api_http_test.go` (imports become `bytes`, `context`, `encoding/json`, `io`, `net/http`, `net/http/httptest`, `strings`, `testing`, `time`):

```go
func useTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	t.Cleanup(func() { defaultHTTPClient = old })
	return srv
}

func TestDefaultHTTPClientHasTimeout(t *testing.T) {
	c, ok := defaultHTTPClient.(*http.Client)
	if !ok {
		t.Fatalf("defaultHTTPClient is %T, want *http.Client", defaultHTTPClient)
	}
	if c == http.DefaultClient || c.Timeout != 30*time.Second {
		t.Fatalf("want a dedicated client with a 30s timeout, got timeout %s", c.Timeout)
	}
}

func TestHttpFindLocations_RejectsOversizedBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"groups":[],"pad":"`)
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxAPIResponseBytes))
		_, _ = io.WriteString(w, `"}`)
	})
	_, err := httpFindLocations(context.Background(), srv.URL, "", "x")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want a size limit error", err)
	}
}

func TestHttpProviderLocations_TruncatesErrorBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(bytes.Repeat([]byte("e"), 1<<20))
	})
	_, err := httpProviderLocations(context.Background(), srv.URL, "")
	if err == nil || len(err.Error()) > 2048 {
		t.Fatalf("error must exist and stay short, got %d bytes", len(err.Error()))
	}
}

func TestHttpFindLocations_DecodesSmallBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"locations": []map[string]string{{"name": "Berlin"}}})
	})
	res, err := httpFindLocations(context.Background(), srv.URL, "", "x")
	if err != nil || len(res.Locations) != 1 || res.Locations[0].Name != "Berlin" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
```

Append to `specs_test.go` (add `"time"` to its imports):

```go
func TestBuildProviderSpecs_LookupTimeoutFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	oldClient := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	t.Cleanup(func() { defaultHTTPClient = oldClient })
	oldTimeout := providerLookupTimeout
	providerLookupTimeout = 100 * time.Millisecond
	t.Cleanup(func() { providerLookupTimeout = oldTimeout })

	start := time.Now()
	_, specs := buildProviderSpecs(context.Background(), srv.URL, "", LocationConfig{LocationQuery: "country:Germany"})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("lookup took %s; a slow API must not block VPN startup", elapsed)
	}
	if len(specs) != 1 || !specs[0].BestAvailable {
		t.Fatalf("want BestAvailable fallback, got %v", specs)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestDefaultHTTPClient|TestHttp|TestBuildProviderSpecs_LookupTimeout' ./...`
Expected: FAIL to compile with `undefined: maxAPIResponseBytes`, `undefined: providerLookupTimeout`.

- [ ] **Step 3: Implement**

`api_http.go` imports become:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/urnetwork/connect"
)
```

Replace:

```go
// defaultHTTPClient is used for all API HTTP calls.
// Replace in tests to avoid real network requests.
var defaultHTTPClient httpDoer = http.DefaultClient
```

with:

```go
const (
	apiHTTPTimeout       = 30 * time.Second
	maxAPIResponseBytes  = 4 << 20
	maxAPIErrorBodyBytes = 1 << 10
)

// defaultHTTPClient is used for all API HTTP calls.
// Replace in tests to avoid real network requests.
var defaultHTTPClient httpDoer = &http.Client{Timeout: apiHTTPTimeout}

func doAPIRequest(req *http.Request, out any, what string) error {
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIErrorBodyBytes))
		return fmt.Errorf("%s http %d: %s", what, resp.StatusCode, string(data))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxAPIResponseBytes {
		return fmt.Errorf("%s: response exceeds %d bytes", what, maxAPIResponseBytes)
	}
	return json.Unmarshal(data, out)
}
```

In `httpFindLocations`, replace everything from `resp, err := defaultHTTPClient.Do(req)` to the end of the function with:

```go
	var out findLocationsHTTPResult
	if err := doAPIRequest(req, &out, "find-locations"); err != nil {
		return nil, err
	}
	return &out, nil
}
```

In `httpProviderLocations`, make the same replacement with `"provider-locations"` as `what`.

`specs.go`: imports add `"time"`. Add above `buildProviderSpecs`:

```go
// providerLookupTimeout bounds startup location lookups so a slow API cannot stall VPN start.
var providerLookupTimeout = 15 * time.Second
```

Replace:

```go
		if q := loc.LocationQuery; q != "" {
			if httpRes, err := httpFindLocations(ctx, apiURL, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
```

with:

```go
		if q := loc.LocationQuery; q != "" {
			lookupCtx, cancel := context.WithTimeout(ctx, providerLookupTimeout)
			defer cancel()
			if httpRes, err := httpFindLocations(lookupCtx, apiURL, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
```

and in the same block replace:

```go
				if fb := findSpecsByQueryFallback(ctx, apiURL, jwt, q); len(fb) > 0 {
```

with:

```go
				if fb := findSpecsByQueryFallback(lookupCtx, apiURL, jwt, q); len(fb) > 0 {
```

(The strategy on the first line of the function keeps the long-lived `ctx`.)

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestDefaultHTTPClient|TestHttp|TestBuildProviderSpecs' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add api_http.go api_http_test.go specs.go specs_test.go
git commit -m "fix: bound API HTTP time and body size during startup lookups"
```

### Task E-3: Reject cleartext endpoints except on localhost

**Files:**
- Create: `urlcheck.go`, `urlcheck_test.go`
- Modify: `main.go` (insert after line 101)

**Interfaces:**
- Consumes: `getStringOr`.
- Produces:
  - `func validateEndpointURL(name, raw, secureScheme, devScheme string) error`
  - `func validateEndpointFlags(opts docopt.Opts) error`
  - `func isLoopbackHost(host string) bool`

- [ ] **Step 1: Write the failing tests**

Create `urlcheck_test.go`:

```go
package main

import (
	"testing"

	"github.com/docopt/docopt-go"
)

func TestValidateEndpointURL(t *testing.T) {
	cases := []struct {
		name, raw, secure, dev string
		ok                     bool
	}{
		{"https api", "https://api.bringyour.com", "https", "http", true},
		{"http remote api", "http://api.bringyour.com", "https", "http", false},
		{"http localhost", "http://localhost:8080", "https", "http", true},
		{"http 127.0.0.1", "http://127.0.0.1:8080", "https", "http", true},
		{"http ::1", "http://[::1]:8080", "https", "http", true},
		{"wss connect", "wss://connect.bringyour.com", "wss", "ws", true},
		{"ws remote connect", "ws://connect.bringyour.com", "wss", "ws", false},
		{"ws localhost", "ws://localhost:9000", "wss", "ws", true},
		{"wrong scheme", "ftp://api.bringyour.com", "https", "http", false},
		{"no scheme", "api.bringyour.com", "https", "http", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEndpointURL("--x", tc.raw, tc.secure, tc.dev)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestValidateEndpointFlags(t *testing.T) {
	if err := validateEndpointFlags(docopt.Opts{"--api_url": DefaultAPIURL, "--connect_url": DefaultConnectURL}); err != nil {
		t.Fatalf("defaults must pass: %v", err)
	}
	if err := validateEndpointFlags(docopt.Opts{"--api_url": "http://api.example.com"}); err == nil {
		t.Fatal("cleartext remote api_url must fail")
	}
	if err := validateEndpointFlags(docopt.Opts{}); err != nil {
		t.Fatalf("commands without URL flags must pass: %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestValidateEndpoint' ./...`
Expected: FAIL to compile with `undefined: validateEndpointURL`.

- [ ] **Step 3: Implement**

Create `urlcheck.go`:

```go
package main

import (
	"fmt"
	"net"
	neturl "net/url"
	"strings"

	"github.com/docopt/docopt-go"
)

// validateEndpointURL allows the insecure devScheme only for loopback hosts, so a typo
// cannot send JWTs in cleartext across the network.
func validateEndpointURL(name, raw, secureScheme, devScheme string) error {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s: invalid URL %q (want %s://host)", name, raw, secureScheme)
	}
	switch strings.ToLower(u.Scheme) {
	case secureScheme:
		return nil
	case devScheme:
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("%s: %s:// is only allowed for localhost; use %s://", name, devScheme, secureScheme)
	default:
		return fmt.Errorf("%s: unsupported scheme %q (want %s://)", name, u.Scheme, secureScheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateEndpointFlags(opts docopt.Opts) error {
	if v := getStringOr(opts, "--api_url", ""); v != "" {
		if err := validateEndpointURL("--api_url", v, "https", "http"); err != nil {
			return err
		}
	}
	if v := getStringOr(opts, "--connect_url", ""); v != "" {
		if err := validateEndpointURL("--connect_url", v, "wss", "ws"); err != nil {
			return err
		}
	}
	return nil
}
```

`main.go`: after line 101 (`setLogLevel(lvl, dbg)`), insert:

```go

	if err := validateEndpointFlags(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
```

- [ ] **Step 4: Run them to verify they pass**

```bash
go test -race -count=1 -run 'TestValidateEndpoint' ./...
go build -o /tmp/urnet-client-e3 . && /tmp/urnet-client-e3 locations --api_url=http://api.example.com; echo "exit=$?"
```

Expected: PASS; the binary prints `error: --api_url: http:// is only allowed for localhost; use https://` and `exit=2`.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add urlcheck.go urlcheck_test.go main.go
git commit -m "fix: reject cleartext API and connect URLs except on localhost"
```

---
## Stream C: config (branch `sina/phase0_config`)

Worktree: `../urnetwork-client-phase0_config`. Owns `cmd_quickconnect.go`, `cmd_vpn.go`, their tests, `config_test.go`, `docs/configuration.md`. Shared hunks: in `config.go` the `ConfigFile` struct, `loadConfigFile`, the section comment at line 123 and new functions appended at the end of the file; in `main.go` the `vpn` case (lines 139-150).

### Task C-1: One config resolver with file log level and the socks_listen alias

**Files:**
- Modify: `config.go:123` (section comment), `config.go:146-169` (`ConfigFile`), `config.go:173-186` (`loadConfigFile`), append `resolveLogLevel`, `resolveVPNConfig`
- Create: `config_test.go`

**Interfaces:**
- Consumes: `parseVPNConfig`, `applyConfigFile`, `setLogLevel`, `getStringOr`, `mustBool` (existing).
- Produces:
  - `ConfigFile.SOCKSListenAlias string` (`yaml:"socks_listen"`)
  - `func resolveLogLevel(flagLevel string, flagDebug bool, fileLevel string, fileDebug bool) (string, bool)`
  - `func resolveVPNConfig(opts docopt.Opts) (VPNConfig, error)` (JWT left empty; applies the effective log level)
  - Test helpers: `func vpnTestOpts(overrides map[string]interface{}) docopt.Opts`, `func writeTempConfig(t *testing.T, body string) string`

- [ ] **Step 1: Write the failing tests**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docopt/docopt-go"
)

// vpnTestOpts mimics what docopt returns for vpn/quick-connect, including its defaults.
func vpnTestOpts(overrides map[string]interface{}) docopt.Opts {
	o := docopt.Opts{
		"--api_url":             DefaultAPIURL,
		"--connect_url":         DefaultConnectURL,
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

func TestResolveVPNConfig(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	cases := []struct {
		name  string
		file  string
		flags map[string]interface{}
		check func(t *testing.T, cfg VPNConfig)
	}{
		{"defaults without file", "", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.MTU != 1420 || cfg.TunName != "" || cfg.APIURL != DefaultAPIURL || cfg.JWT != "" {
				t.Fatalf("unexpected defaults: %+v", cfg)
			}
		}},
		{"file fills unset values", "tun: utun7\nmtu: 1380\nsocks: 127.0.0.1:1081\napi_url: https://api.example.test\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.TunName != "utun7" || cfg.MTU != 1380 || cfg.SOCKSListen != "127.0.0.1:1081" || cfg.APIURL != "https://api.example.test" {
				t.Fatalf("file values not applied: %+v", cfg)
			}
		}},
		{"CLI wins over file", "tun: utun7\n", map[string]interface{}{"--tun": "utun9"}, func(t *testing.T, cfg VPNConfig) {
			if cfg.TunName != "utun9" {
				t.Fatalf("tun = %q, want utun9", cfg.TunName)
			}
		}},
		{"socks_listen alias", "socks_listen: 127.0.0.1:1082\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.SOCKSListen != "127.0.0.1:1082" {
				t.Fatalf("socks_listen alias ignored: %q", cfg.SOCKSListen)
			}
		}},
		{"socks beats socks_listen", "socks: 127.0.0.1:1083\nsocks_listen: 127.0.0.1:1084\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.SOCKSListen != "127.0.0.1:1083" {
				t.Fatalf("socks = %q, want the canonical key to win", cfg.SOCKSListen)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := map[string]interface{}{}
			for k, v := range tc.flags {
				flags[k] = v
			}
			if tc.file != "" {
				flags["--config"] = writeTempConfig(t, tc.file)
			}
			cfg, err := resolveVPNConfig(vpnTestOpts(flags))
			if err != nil {
				t.Fatalf("resolveVPNConfig: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}

func TestResolveVPNConfig_MissingFileIsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": missing})); err == nil {
		t.Fatal("want error for a missing config file")
	}
}

func TestResolveLogLevel(t *testing.T) {
	cases := []struct {
		name                string
		flagLevel           string
		flagDebug           bool
		fileLevel           string
		fileDebug           bool
		wantLevel           string
		wantDebug           bool
	}{
		{"nothing set", "", false, "", false, "", false},
		{"flag level wins over file", "warn", false, "debug", true, "warn", false},
		{"--debug flag wins over file level", "", true, "warn", false, "debug", true},
		{"file level used", "", false, "warn", false, "warn", false},
		{"file debug used", "", false, "", true, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lvl, dbg := resolveLogLevel(tc.flagLevel, tc.flagDebug, tc.fileLevel, tc.fileDebug)
			if lvl != tc.wantLevel || dbg != tc.wantDebug {
				t.Fatalf("got (%q, %v), want (%q, %v)", lvl, dbg, tc.wantLevel, tc.wantDebug)
			}
		})
	}
}

func TestResolveVPNConfig_AppliesFileLogLevel(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	path := writeTempConfig(t, "log_level: warn\n")

	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path})); err != nil {
		t.Fatal(err)
	}
	if isInfoEnabled() {
		t.Fatal("log_level: warn from the file was not applied")
	}

	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path, "--log_level": "debug"})); err != nil {
		t.Fatal(err)
	}
	if !isDebugEnabled() {
		t.Fatal("--log_level must beat the file")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestResolveVPNConfig|TestResolveLogLevel' ./...`
Expected: FAIL to compile with `undefined: resolveVPNConfig`, `undefined: resolveLogLevel`.

- [ ] **Step 3: Implement**

`config.go:123`, replace:

```go
// Config file (--config / URNETWORK_CONFIG)
```

with:

```go
// Config file (--config)
```

In `ConfigFile`, after the `SOCKSListen` line add:

```go
	SOCKSListenAlias  string   `yaml:"socks_listen"` // older docs used this key; "socks" wins when both are set
```

In `loadConfigFile`, replace:

```go
	var cf ConfigFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return ConfigFile{}, fmt.Errorf("config file parse: %w", err)
	}
	return cf, nil
```

with:

```go
	var cf ConfigFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return ConfigFile{}, fmt.Errorf("config file parse: %w", err)
	}
	if cf.SOCKSListen == "" {
		cf.SOCKSListen = cf.SOCKSListenAlias
	}
	return cf, nil
```

Append to the end of `config.go`:

```go
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

// resolveVPNConfig is the single entry point for vpn and quick-connect: CLI flags, then
// the --config file, then defaults. It applies the effective log level and leaves JWT empty.
func resolveVPNConfig(opts docopt.Opts) (VPNConfig, error) {
	cfg := parseVPNConfig(opts, "")
	cf, err := loadConfigFile(getStringOr(opts, "--config", ""))
	if err != nil {
		return VPNConfig{}, err
	}
	cfg = applyConfigFile(cfg, cf)
	setLogLevel(resolveLogLevel(getStringOr(opts, "--log_level", ""), mustBool(opts, "--debug"), cf.LogLevel, cf.Debug))
	return cfg, nil
}
```

Run `gofmt -w config.go config_test.go`.

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestResolveVPNConfig|TestResolveLogLevel|TestSetLogLevel' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add config.go config_test.go
git commit -m "fix: apply config file log_level and accept socks_listen key"
```

### Task C-2: vpn dispatch uses the resolver and fails without a JWT in TUN mode

**Files:**
- Create: `cmd_vpn.go`, `cmd_vpn_test.go`
- Modify: `main.go:139-150`

**Interfaces:**
- Consumes: `resolveVPNConfig`, `vpnTestOpts` (C-1); `loadJWT`, `isTUNDisabled`, `cmdVpn` (existing).
- Produces:
  - `func cmdVpnFromOpts(ctx context.Context, opts docopt.Opts) error`
  - `func vpnUsesTUN(cfg VPNConfig) bool`

- [ ] **Step 1: Write the failing tests**

Create `cmd_vpn_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
)

func TestVpnUsesTUN(t *testing.T) {
	cases := map[string]bool{"": false, "none": false, "off": false, " NONE ": false, "utun9": true, "urnet0": true, "--default_route": true}
	for tun, want := range cases {
		if got := vpnUsesTUN(VPNConfig{TunName: tun}); got != want {
			t.Errorf("vpnUsesTUN(%q) = %v, want %v", tun, got, want)
		}
	}
}

func TestCmdVpnFromOpts_MissingJWTFailsInTUNMode(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	err := cmdVpnFromOpts(context.Background(), vpnTestOpts(map[string]interface{}{"--tun": "urnet-test0"}))
	if err == nil || !strings.Contains(err.Error(), "JWT") {
		t.Fatalf("err = %v, want a JWT error before any TUN work", err)
	}
}

func TestCmdVpnFromOpts_SocksOnlyDoesNotNeedJWT(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := cmdVpnFromOpts(ctx, vpnTestOpts(map[string]interface{}{"--tun": "none", "--socks": "127.0.0.1:0"}))
	if err != nil {
		t.Fatalf("SOCKS-only mode must not need a JWT: %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestVpnUsesTUN|TestCmdVpnFromOpts' ./...`
Expected: FAIL to compile with `undefined: vpnUsesTUN`, `undefined: cmdVpnFromOpts`.

- [ ] **Step 3: Implement**

Create `cmd_vpn.go`:

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"
)

func cmdVpnFromOpts(ctx context.Context, opts docopt.Opts) error {
	cfg, err := resolveVPNConfig(opts)
	if err != nil {
		return err
	}
	jwt, err := loadJWT(getStringOr(opts, "--jwt", ""))
	if err != nil && vpnUsesTUN(cfg) {
		return fmt.Errorf("vpn needs a JWT (run 'login' or pass --jwt): %w", err)
	}
	cfg.JWT = jwt
	return cmdVpn(ctx, cfg)
}

// vpnUsesTUN mirrors the SOCKS-only detection in cmdVpn; SOCKS-only mode never calls the API.
func vpnUsesTUN(cfg VPNConfig) bool {
	tun := strings.TrimSpace(cfg.TunName)
	return tun != "" && !isTUNDisabled(tun)
}
```

`main.go`, replace:

```go
	case mustBool(opts, "vpn"):
		jwt, _ := loadJWT(getStringOr(opts, "--jwt", ""))
		cfg := parseVPNConfig(opts, jwt)
		if cfgPath := strings.TrimSpace(getStringOr(opts, "--config", "")); cfgPath != "" {
			cf, err := loadConfigFile(cfgPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			cfg = applyConfigFile(cfg, cf)
		}
		runErr = cmdVpn(ctx, cfg)
```

with:

```go
	case mustBool(opts, "vpn"):
		runErr = cmdVpnFromOpts(ctx, opts)
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestVpnUsesTUN|TestCmdVpnFromOpts' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add cmd_vpn.go cmd_vpn_test.go main.go
git commit -m "fix: fail vpn with a clear error when no JWT is available"
```

### Task C-3: quick-connect uses the config file and a bounded login retry

**Files:**
- Modify: `cmd_quickconnect.go:15-16` (resolve config first), `86-114` (retry loop), `187-196` (use resolved config), new `loginRetryBackoff`
- Create: `cmd_quickconnect_test.go`

**Interfaces:**
- Consumes: `resolveVPNConfig`, `vpnTestOpts`, `writeTempConfig` (C-1).
- Produces:
  - `const loginRetryMin = 10 * time.Second`, `const loginRetryMax = 5 * time.Minute`
  - `func loginRetryBackoff(attempt int) time.Duration`

- [ ] **Step 1: Write the failing tests**

Create `cmd_quickconnect_test.go`:

```go
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLoginRetryBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 10 * time.Second},
		{1, 20 * time.Second},
		{2, 40 * time.Second},
		{4, 160 * time.Second},
		{5, 5 * time.Minute},
		{50, 5 * time.Minute},
	}
	for _, tc := range cases {
		if got := loginRetryBackoff(tc.attempt); got != tc.want {
			t.Errorf("loginRetryBackoff(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestCmdQuickConnect_LoadsConfigFile(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	t.Setenv("URNETWORK_USERNAME", "")
	t.Setenv("URNETWORK_PASSWORD", "")
	bad := writeTempConfig(t, "mtu: [not-an-int\n")

	err := cmdQuickConnect(context.Background(), vpnTestOpts(map[string]interface{}{"--config": bad}))

	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("err = %v; quick-connect must load --config before doing anything else", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test -run 'TestLoginRetryBackoff|TestCmdQuickConnect' ./...`
Expected: FAIL to compile with `undefined: loginRetryBackoff`. After adding only that function, `TestCmdQuickConnect_LoadsConfigFile` fails with `err = no JWT available; ...` because the config is ignored.

- [ ] **Step 3: Implement**

Replace the first line of the function body:

```go
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
```

with:

```go
	vpnCfg, err := resolveVPNConfig(opts)
	if err != nil {
		return err
	}
	apiURL := vpnCfg.APIURL
```

Replace:

```go
				retryEvery := renewInterval
				if retryEvery <= 0 {
					retryEvery = time.Minute
				}
				for {
```

with:

```go
				for attempt := 0; ; attempt++ {
```

and inside that loop replace:

```go
					logWarn("jwt still not usable; retrying in %s\n", retryEvery.String())
					select {
					case <-time.After(retryEvery):
```

with:

```go
					wait := loginRetryBackoff(attempt)
					logWarn("jwt still not usable; retrying in %s\n", wait)
					select {
					case <-time.After(wait):
```

Replace the end of the function:

```go
	vpnCfg := parseVPNConfig(opts, finalJWT)
	runErr := cmdVpn(ctx, vpnCfg)
```

with:

```go
	vpnCfg.JWT = finalJWT
	runErr := cmdVpn(ctx, vpnCfg)
```

Add after `cmdQuickConnect`:

```go
const (
	loginRetryMin = 10 * time.Second
	loginRetryMax = 5 * time.Minute
)

// loginRetryBackoff is independent of --jwt_renew_interval, which is usually hours and
// used to stall startup for that long after a single failed login.
func loginRetryBackoff(attempt int) time.Duration {
	d := loginRetryMin
	for i := 0; i < attempt && d < loginRetryMax; i++ {
		d *= 2
	}
	return min(d, loginRetryMax)
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test -race -count=1 -run 'TestLoginRetryBackoff|TestCmdQuickConnect' ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add cmd_quickconnect.go cmd_quickconnect_test.go
git commit -m "fix: honor --config in quick-connect and bound login retry wait"
```

### Task C-4: Configuration docs match the code

**Files:**
- Modify: `docs/configuration.md` (full replacement)

**Interfaces:**
- Consumes: key names and behavior from C-1..C-3; env vars from A-5 (`URNETWORK_SOCKS_USER`, `URNETWORK_SOCKS_PASS`) and E-1 (`URNETWORK_JWT`).
- Produces: nothing code-level.

- [ ] **Step 1: Write the failing check**

```bash
grep -c '^socks_listen:' docs/configuration.md; grep -c 'URNETWORK_SOCKS_PASS' docs/configuration.md; grep -c 'Known limitations' docs/configuration.md
```

Expected (failing state): `1`, `0`, `0`.

- [ ] **Step 2: Implement**

Replace `docs/configuration.md` with:

~~~markdown
# Configuration

## Precedence

For `vpn` and `quick-connect`:

1. CLI flags
2. Config file (`--config=<path>`)
3. Built-in defaults

Environment variables are fallbacks for specific flags only (listed below). They apply when the matching flag is not given.

## YAML config file

`vpn` and `quick-connect` accept `--config=<path>`. For `quick-connect`, the file's `api_url` is also used for login.

```yaml
api_url: https://api.bringyour.com
connect_url: wss://connect.bringyour.com
tun: utun10
ip_cidr: 10.255.0.2/24
mtu: 1420
default_route: true
dns:
  - 1.1.1.1
  - 1.0.0.1
dns_service: Wi-Fi
dns_bootstrap: bypass
socks: 127.0.0.1:1080
location_query: "country:Germany"
log_level: info
stats_interval: 5
debug: false
```

`socks` is the canonical key. `socks_listen` is still accepted for older files; if both are set, `socks` wins.

SOCKS credentials are not read from the file. Use `--socks_user` and `URNETWORK_SOCKS_PASS`.

### Log level

The effective level is the first one set among: `--log_level`, `--debug`, `log_level` in the file, `debug: true` in the file. The default is `info`.

### Known limitations

These are planned to go away with a config rework:

- A CLI value equal to the built-in default counts as "not set". For example `--mtu=1420` together with `mtu: 1380` in the file gives 1380. The same applies to `--ip_cidr=10.255.0.2/24`, `--stats_interval=5`, `--dns_bootstrap=bypass`, and the default `--api_url` / `--connect_url`.
- Booleans in the file (`default_route`, `allow_inbound_local`, `debug`) can only turn a feature on. `false` in the file has no effect.
- `stats_interval: 0` in the file does not disable stats. Use `--stats_interval=0`.

## Environment variables

- `URNETWORK_HOME`: directory containing `jwt` (default `~/.urnetwork`).
- `URNETWORK_USERNAME`: fallback for `--user_auth` in `quick-connect`.
- `URNETWORK_PASSWORD`: fallback for `--password`.
- `URNETWORK_JWT`: fallback for `--jwt`.
- `URNETWORK_SOCKS_USER`: fallback for `--socks_user`.
- `URNETWORK_SOCKS_PASS`: fallback for `--socks_pass`.

## Security note

Command-line arguments are visible in shell history and to every local user through `ps`. Prefer the environment variables above for passwords and tokens. With `--background`, `--password`, `--jwt` and `--socks_pass` are moved from the child's command line into its environment.

Only `https://` API URLs and `wss://` connect URLs are accepted, except `http://` / `ws://` to `localhost` for development.
~~~

- [ ] **Step 3: Verify**

```bash
grep -c '^socks_listen:' docs/configuration.md; grep -c 'URNETWORK_SOCKS_PASS' docs/configuration.md; grep -c 'Known limitations' docs/configuration.md
grep -nP '\x{2014}|[\x{201C}\x{201D}\x{2018}\x{2019}]' docs/configuration.md || echo "no em dashes or smart quotes"
```

Expected: `0`, `2`, `1`, then `no em dashes or smart quotes`.

- [ ] **Step 4: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 5: Commit**

```bash
git add docs/configuration.md
git commit -m "docs: align configuration guide with config keys and env vars"
```

---

## Integration (on `sina/phase0_critical_fixes` after all five merges)

### Task Z-1: Validate URLs that come from the config file

E-3 validates `--api_url`/`--connect_url` flags in `main.go`. A URL from the config file only exists after `resolveVPNConfig` (C-1), which lives in a different stream, so the wiring happens here.

**Files:**
- Modify: `config.go` (`resolveVPNConfig`)
- Modify: `config_test.go` (new test)

**Interfaces:**
- Consumes: `validateEndpointURL` (E-3), `resolveVPNConfig`, `vpnTestOpts`, `writeTempConfig` (C-1).
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `config_test.go`:

```go
func TestResolveVPNConfig_RejectsCleartextURLFromFile(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	path := writeTempConfig(t, "api_url: http://api.example.test\n")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path})); err == nil {
		t.Fatal("cleartext api_url from the config file must be rejected")
	}
	ok := writeTempConfig(t, "connect_url: ws://localhost:9000\n")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": ok})); err != nil {
		t.Fatalf("ws://localhost must be allowed: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -run TestResolveVPNConfig_RejectsCleartextURLFromFile ./...`
Expected: FAIL: `cleartext api_url from the config file must be rejected`.

- [ ] **Step 3: Implement**

In `resolveVPNConfig`, after `cfg = applyConfigFile(cfg, cf)` add:

```go
	if err := validateEndpointURL("api_url", cfg.APIURL, "https", "http"); err != nil {
		return VPNConfig{}, err
	}
	if err := validateEndpointURL("connect_url", cfg.ConnectURL, "wss", "ws"); err != nil {
		return VPNConfig{}, err
	}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test -race -count=1 -run TestResolveVPNConfig ./...`
Expected: PASS.

- [ ] **Step 5: Full verification**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
```

- [ ] **Step 6: Commit**

```bash
git add config.go config_test.go
git commit -m "fix: reject cleartext endpoint URLs from the config file"
```

### Task Z-2: Final verification and smoke checks

**Files:** none changed.

- [ ] **Step 1: Full verification plus Linux-tagged tests**

```bash
go build ./... && GOOS=linux go build ./... && GOOS=darwin go vet ./... && GOOS=linux go vet ./... && go test -race -count=1 ./... && golangci-lint run
make version-check
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -race -count=1 ./...
```

Expected: all pass. The container run executes `*_linux_test.go`; the host run on macOS executes `*_darwin_test.go`.

- [ ] **Step 2: Smoke checks with the real binary (no root needed)**

```bash
go build -o /tmp/urnet-client-z .
/tmp/urnet-client-z --help | grep -c 'socks_pass'
/tmp/urnet-client-z vpn --tun=none; echo "exit=$?"
/tmp/urnet-client-z locations --api_url=http://api.example.com; echo "exit=$?"
```

Expected: `4`; `error: no TUN and no --socks given; nothing to do ...` with `exit=1`; the cleartext URL error with `exit=2`.

SOCKS auth smoke (two terminals):

```bash
URNETWORK_SOCKS_USER=u URNETWORK_SOCKS_PASS=p /tmp/urnet-client-z vpn --tun=none --socks=127.0.0.1:18081
curl -sS -o /dev/null -w '%{http_code}\n' --socks5 u:p@127.0.0.1:18081 https://example.com
curl -sS -o /dev/null --socks5 127.0.0.1:18081 https://example.com; echo "exit=$?"
/tmp/urnet-client-z vpn --tun=none --socks=0.0.0.0:18082   # expect the WARN line at startup
```

Expected: `200` with credentials; curl fails without credentials; WARN printed for the `0.0.0.0` bind.

- [ ] **Step 3: Manual root checks (owner, optional before merge to main)**

- Linux: `sudo ./urnet-client vpn --tun urnet0 --default_route --exclude_route=10.0.0.0/8` with `10.0.0.0/8` already present in the routing table; after Ctrl-C, `ip route show 10.0.0.0/8` still shows the user's route.
- macOS: `sudo ./urnet-client vpn --tun utun10 --default_route --kill_switch`; check whether the blackhole route installs. If the log shows "failed to install blackhole default route", confirm `netstat -rn | grep default` shows the original gateway again (B-3) and record the result for the Phase 1 kill switch work.

- [ ] **Step 4: Stop here**

Do not push. Report results to the owner and ask for approval before `git push`.

---

## PM checklist

| Finding | Task | Branch |
|---------|------|--------|
| A1 method selection, 0xFF | A-4 | sina/phase0_socks |
| A2 RFC 1929 auth, flags, env | A-4 (server), A-5 (flags/env), A-8 (docs) | sina/phase0_socks |
| A3 WARN on open bind, compose loopback, LAN docs | A-8 | sina/phase0_socks |
| A4 bind errors fail closed (TCP and UDP) | A-6, A-7 | sina/phase0_socks |
| A5 UDP ASSOCIATE parsing, deadline, lifetime, reachable relay | A-3 (address parsing), A-7 | sina/phase0_socks |
| A6 UDP socket and goroutine leak | A-7 | sina/phase0_socks |
| A7 accept loop backoff | A-2 | sina/phase0_socks |
| A8 BND.ADDR encoding | A-3 | sina/phase0_socks |
| B0 command runner seam | B-1 | sina/phase0_routes |
| B1 Linux record-on-success (plus `unreachable` syntax) | B-2 | sina/phase0_routes |
| B2 macOS kill switch restore | B-3 | sina/phase0_routes |
| B3 TUN setup errors abort before routes | B-4 | sina/phase0_routes |
| B4 DNS bypass race and ctx | B-5 | sina/phase0_routes |
| B5 "nothing to do" exits non-zero | A-1 | sina/phase0_socks |
| B5 vpn discards loadJWT error | C-2 | sina/phase0_config |
| C1 quick-connect honors --config | C-1, C-3 | sina/phase0_config |
| C2 file log_level applied, flag wins | C-1 | sina/phase0_config |
| C3 socks vs socks_listen | C-1 (alias), C-4 (docs) | sina/phase0_config |
| C4 default-equals-unset, bool-only-enable | C-4 (documented only) | sina/phase0_config |
| C5 bounded login retry | C-3 | sina/phase0_config |
| D1 injectable Version, Docker, CI build-args, check | D-1, D-3 | sina/phase0_ci |
| D2 cancel-in-progress only for PRs | D-3 | sina/phase0_ci |
| D3 buildx context, stale vars | D-2 | sina/phase0_ci |
| D4 Renovate policy | D-4 | sina/phase0_ci |
| D5 darwin vet in CI | D-2 (make lint, run by CI Lint step) | sina/phase0_ci |
| D6 go-version-file | D-3 | sina/phase0_ci |
| E1 secrets off child argv | E-1 | sina/phase0_hygiene |
| E2 HTTP timeout, body cap, lookup timeout | E-2 | sina/phase0_hygiene |
| E3 https/wss only except localhost | E-3 (flags), Z-1 (config file) | sina/phase0_hygiene, integration |

Review gates per stream (from the dev workflow): fresh opus reviewer with clean context after each stream; security pass required for A (auth, exposure), E (secrets, HTTP) and B (routing, kill switch).

## Deferred to Phase 1+

### Before Phase 1 (prerequisite)

- Owner task (user, needs a Claude Code restart): Install Go LSP for Claude Code. Add `~/go/bin` to `PATH` (gopls is already installed there), enable a gopls LSP plugin via `/plugin`, restart Claude Code, and verify with an LSP `documentSymbol` call on `socks.go`. Reason: the Phase 1 package split moves many symbols across packages; `findReferences` and call hierarchy reduce missed call sites.

### Deferred items

- DNS and IPv6 leak policy: with `--default_route`, IPv6 still follows the system default route and DNS can go to the original resolvers. Needs an explicit policy (block v6, force DNS through the tunnel).
- Firewall-based kill switch (pf on macOS, nftables on Linux) instead of a blackhole route, including a clear "unblock" command. Also verify whether `route -n add -blackhole default` without a gateway argument works on macOS at all.
- Tri-state config (distinguish "flag not given" from "flag equals default"), booleans that can be turned off from the file, `stats_interval: 0`.
- SOCKS credentials in the config file (with file permission checks).
- Package split (socks, routes, config, api into internal packages); move `commandRunner` into its own package.
- Symlink-safe writes for `~/.urnetwork/jwt` and `--log_file` (open with `O_NOFOLLOW`, check ownership).
- Absolute paths for `ip`, `route`, `ifconfig`, `networksetup`, `scutil` so `PATH` cannot redirect privileged commands.
- macOS test job in CI (`macos-latest` running `go test -race ./...`) so `*_darwin_test.go` runs outside developer machines.
- Release version accuracy: CLI release assets are built before semantic-release computes the version, and released images are retagged from a build that reports the branch version (`main`). Build or stamp artifacts after the version is known.
- `vpnRunCore` only logs when the requested SOCKS proxy fails to start; decide whether that should be fatal.
- SOCKS sessions are not cancelled on shutdown (only the listener closes); tie sessions to ctx.
- Rate limiting or backoff for repeated failed SOCKS auth attempts.
- UDP ASSOCIATE fragments (FRAG != 0) are dropped.
- Standalone `socks` command still does not connect to the extender (existing known gap).
- `make lint` runs `go mod tidy`, which can modify the tree in CI without failing.
- Route bypass for API endpoints resolves hostnames with the system resolver at startup only; IP changes during a session are not handled.
- Linux `SetDNS` is a no-op; document or implement resolv.conf/systemd-resolved handling.
