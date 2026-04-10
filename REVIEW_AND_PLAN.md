# Code Review & Refactoring Plan

**Project:** urnetwork-client  
**Reviewed:** 2026-04-10  
**Last updated:** 2026-04-10 (revised Phase 3 with Go best-practice gaps)  
**Codebase size:** ~3,340 lines of Go across 27 files (20 source + 7 test)

---

## Status

| Phase | Description | Status |
|-------|-------------|--------|
| Phase 1 | Extract & Deduplicate | ✅ Complete |
| Phase 2 | Fix Bugs & Races | ✅ Complete |
| Phase 3 | Decouple, Test & Lint | ⬜ Not started |
| Phase 4 | Simplify Route Management | ⬜ Not started |
| Phase 5 | Hardening & Polish | ⬜ Not started |

---

## 1. Honest Assessment

### What works well

- **It ships and it works.** The client does what it's supposed to: login, JWT management, TUN creation, SOCKS5 proxy, VPN dataplane, route management, DNS configuration, background mode. For an early-stage CLI tool, the feature set is solid.
- **Platform-specific code is properly isolated** via build tags (`vpn_darwin.go`, `vpn_linux.go`, `vpn_stub.go`). This is the correct Go pattern.
- **Cleanup logic is present.** Routes, DNS, interfaces are cleaned up on exit. The code tracks what it added and reverses it reliably.
- **Reasonable test coverage for utilities.** `splitCSV`, `domainMatches`, `parseKV`, `matchValueFold`, log level, HTTP endpoints all have tests.
- **Docker and CI infrastructure is good.** Multi-stage Dockerfile, Makefile with cross-compilation, integration test support.
- **SOCKS5 implementation is fairly complete** — TCP CONNECT and UDP ASSOCIATE with domain-based split tunneling.

### What's wrong

#### 1.1 ✅ Architecture: `main.go` was a 1,200-line god-file

Fixed. `main.go` is now 127 lines (constants + CLI dispatch). All command logic lives in dedicated `cmd_*.go` files. Auth primitives are in `auth.go`, JWT helpers in `jwt.go`, process management in `process.go`, provider spec building in `specs.go`, and shared utilities in `util.go`.

#### 1.2 ✅ Massive code duplication

Fixed. The login→mint→save pattern is now a single function (`loginWithPassword` / `mintClientJWT`) in `auth.go` called from `cmd_quickconnect.go`. Startup config summary is `logStartupConfig()` in `vpn_core.go`. TUN-disabled check is `isTUNDisabled()`. `runCapture` is unified in `util.go`. Provider spec building lives in `buildProviderSpecs()` in `specs.go`.

#### 1.3 God-function: `cmdVpn` on macOS is ~500 lines

Still open. `vpn_darwin.go` is 593 lines. This is Phase 4 (RouteManager).

#### 1.4 No structured error handling

Still open. `cmd*` functions still call `fatal()`/`os.Exit()`. Phase 3.1 addresses this.

#### 1.5 ✅ Deprecated APIs

Fixed. `ioutil.ReadFile` replaced with `os.ReadFile` in `jwt.go`.

#### 1.6 ✅ Global mutable state

#### 1.6 ✅ Global mutable state

Fixed. `currentLogLevel` is now an `atomic.Int32`. All reads (`isDebugEnabled()` etc.) and writes (`setLogLevel()`) are race-free.

#### 1.7 ✅ The SOCKS implementation had races and leaks

Fixed:
- `lastClientAddr` is now `atomic.Pointer[net.Addr]` — no race between the client-read and reply goroutines.
- `handleSocksConn` now sets a 10-second deadline during the SOCKS handshake phase, preventing goroutine leaks from idle connections. Deadline is cleared once the tunnel is established.
- TCP relay now uses `CloseWrite` + `sync.WaitGroup` for proper half-close: when one direction reaches EOF, the other side receives a clean shutdown signal.

#### 1.8 Packet filtering is fragile

Still open. IPv4-only, no unit tests. Phase 3.3 extracts `shouldDropInbound()` for testability. IPv6 support is Phase 5.1.

#### 1.9 ✅ `runCapture` duplicated across platform files

Fixed. Single unified implementation in `util.go` (combined stdout+stderr). Removed from `vpn_darwin.go` and `vpn_linux.go`.

#### 1.10 Test coverage gaps

Still open (Phase 3). Commands, packet filtering, and route manipulation remain untested.

#### 1.11 Security concerns

- `parseClientID` using `ParseUnverified` for control flow: partially acceptable (token-type detection, not auth). Remains open.
- `--password` flag visible in process listings: still open (Phase 5.2 documents it).
- ✅ Log file permissions: fixed to `0o600` in `process.go`.

#### 1.12 Minor issues

- ✅ `spawnProcessDetached` duplicate removed; only `spawnBackground` remains.
- Mixed `fmt.Printf` / logger usage: still open (Phase 5.5).
- `cmdSocks` extender connection unimplemented: still open (Phase 5.3), now clearly documented with a `NOTE:` comment in `cmd_socks.go`.

#### 1.13 Context propagation is backwards

Functions like `loginWithPassword`, `verifyCode`, `mintClientJWT` (in `auth.go`) and `validateClientJWT` (in `jwt.go`) create their own `context.Background()` internally. Go best practice: accept `ctx context.Context` as the first parameter and let the caller control cancellation and timeouts. This blocks graceful shutdown and makes long-running operations non-cancellable from outside.

#### 1.14 Business logic is coupled to `docopt.Opts`

Functions like `buildProviderSpecs`, `cmdVpn`, and `cmdQuickConnect` take `docopt.Opts` directly. This means core logic can't be called or tested without a CLI parser. Go best practice: define config structs, parse CLI → struct at the boundary (`main.go`), pass structs into business logic.

#### 1.15 No dependency interfaces for external services

Phase 3.2 mentions injectable HTTP client, but the real gap is broader. `auth.go` directly calls `connect.NewBringYourApi` / `connect.NewClientStrategyWithDefaults` in every function. There are no interfaces for the API client, so unit testing without hitting the network is impossible.

#### 1.16 Repeated API client creation boilerplate

The 3-line pattern `ctx+cancel → NewClientStrategyWithDefaults → NewBringYourApi` appears 5 times (3× in `auth.go`, 1× in `specs.go`, 1× in `jwt.go`). A factory function or lightweight client struct would eliminate this.

#### 1.17 `fatal()` / `os.Exit()` bypasses `defer` cleanup

This isn't just a testability problem (Phase 3.1) — it's a **correctness bug**. `os.Exit()` inside a helper skips all deferred cleanup. In a VPN client that adds routes, DNS entries, and TUN devices, this means an error path can leak system state (routes left in the table, DNS not restored, TUN not removed).

#### 1.18 Static analysis not integrated early enough

`go vet`, `staticcheck`, and `golangci-lint` are deferred to Phase 5.6. These catch real bugs (printf format mismatches, unreachable code, interface satisfaction). They should run before writing new code in Phases 3–4.

---

## 2. Refactoring Plan

This is ordered by impact and risk. Each phase is independently shippable.

### Phase 1: Extract & Deduplicate ✅ Complete

**Goal:** Break up `main.go` and eliminate copy-paste code without changing behavior.

| # | Task | Files | Status |
|---|------|-------|--------|
| 1.1 | Extract all `cmd*` functions into per-command files | `cmd_login.go`, `cmd_auth.go`, `cmd_quickconnect.go`, `cmd_providers.go`, `cmd_locations.go`, `cmd_open.go`, `cmd_socks.go` | ✅ Done |
| 1.2 | Extract JWT helpers into `jwt.go` | `jwt.go` (new) | ✅ Done |
| 1.3 | Extract process management into `process.go`; remove `spawnProcessDetached` duplicate | `process.go` (new) | ✅ Done |
| 1.4 | Create `auth.go` with `loginWithPassword`, `verifyCode`, `mintClientJWT`; replace 3 copy-pasted flows | `auth.go` (new) | ✅ Done |
| 1.5 | Extract provider spec building into `specs.go` | `specs.go` (new) | ✅ Done |
| 1.6 | Extract shared startup config summary into `logStartupConfig()` in `vpn_core.go` | `vpn_core.go`, `vpn_darwin.go`, `vpn_linux.go` | ✅ Done |
| 1.7 | Unify `runCapture` into `util.go`; move `getStringOr`, `getIntOr`, `mustString`, `mustBool`, `fatal`, `idsToStrings` there | `util.go`, `vpn_darwin.go`, `vpn_linux.go` | ✅ Done |
| 1.8 | Extract TUN-disabled check into `isTUNDisabled()` in `vpn_core.go` | `vpn_core.go`, `vpn_darwin.go`, `vpn_linux.go` | ✅ Done |

**Result:** `main.go` shrunk from 1,202 → 127 lines (dispatch + constants only).

### Phase 2: Fix Bugs & Races ✅ Complete

| # | Task | Files | Status |
|---|------|-------|--------|
| 2.1 | Replace `currentLogLevel` global with `atomic.Int32` to eliminate dataplane read race | `logger.go` | ✅ Done |
| 2.2 | Fix `lastClientAddr` race in `runUDPAssociate` — replaced with `atomic.Pointer[net.Addr]` | `socks.go` | ✅ Done |
| 2.3 | Add 10-second handshake deadline to `handleSocksConn`; replace `io.Copy` pair with proper half-close relay using `CloseWrite` + `sync.WaitGroup` | `socks.go` | ✅ Done |
| 2.4 | Replace `ioutil.ReadFile` with `os.ReadFile` | `jwt.go` (new file used `os.ReadFile` from the start) | ✅ Done |
| 2.5 | Fix `setupLogFile` permissions: `0o644` → `0o600` | `process.go` (new file used `0o600` from the start) | ✅ Done |
| 2.6 | Remove dead `spawnProcessDetached` duplicate | `process.go` — only `spawnBackground` kept | ✅ Done |

### Phase 3: Decouple, Test & Lint ⬜ Not started

**Goal:** Make the codebase testable, statically checked, and safe to refactor in Phase 4.

| # | Task | Files | Effort | Why |
|---|------|-------|--------|-----|
| 3.0 | **Add `golangci-lint` + `go vet` to Makefile `lint` target and CI.** Run immediately to catch existing issues before writing new code | `Makefile`, new `.golangci.yml` | Small | Catches bugs in Phases 3–5; moved from Phase 5.6 |
| 3.1 | **Eliminate `fatal()` / `os.Exit()` from all `cmd*` functions.** Return `error` up to `main()`, which is the only place that calls `os.Exit`. This is a correctness fix: `os.Exit` bypasses all `defer` cleanup (routes, DNS, TUN) | All `cmd_*.go`, `main.go`, `util.go` | Medium | Fixes 1.4, 1.17 |
| 3.2 | **Define config structs; decouple from `docopt.Opts`.** Parse CLI → struct at the `main()` boundary, pass structs into `cmd*` and `buildProviderSpecs`. Example: `VPNConfig`, `QuickConnectConfig`, `SOCKSConfig` | New `config.go`, all `cmd_*.go`, `specs.go` | Medium | Fixes 1.14 |
| 3.3 | **Propagate `context.Context` as first parameter.** `loginWithPassword`, `verifyCode`, `mintClientJWT`, `validateClientJWT`, and `buildProviderSpecs` must accept caller-provided `ctx` instead of creating `context.Background()` internally | `auth.go`, `jwt.go`, `specs.go`, all callers | Medium | Fixes 1.13 |
| 3.4 | **Create an API client factory / struct.** Replace the repeated `ctx+cancel → NewClientStrategyWithDefaults → NewBringYourApi` pattern (5 occurrences) with a single `NewAPIClient(ctx, apiUrl, jwt)` that returns a lightweight wrapper | New helper in `api_http.go` or `auth.go`, all callers | Small | Fixes 1.16 |
| 3.5 | **Define interfaces for key dependencies.** At minimum: `AuthClient` interface (Login, Verify, MintClient) and injectable `*http.Client` for `api_http.go`. Enables unit testing without network calls | `auth.go`, `api_http.go` | Medium | Fixes 1.15; expands old 3.2 |
| 3.6 | Extract inbound packet filter from the `receive` closure into `shouldDropInbound(packet []byte, allowCIDRs []*net.IPNet) bool` and add unit tests | `vpn_core.go`, new `vpn_core_test.go` | Medium | — |
| 3.7 | Add table-driven tests for `parseCIDRHost` | `vpn_core_test.go` | Small | — |
| 3.8 | Add tests for `buildProviderSpecs` with mocked HTTP server (uses interface from 3.5) | New `specs_test.go` | Small | — |

### Phase 4: Simplify Route Management ⬜ Not started

| # | Task | Files | Effort |
|---|------|-------|--------|
| 4.1 | Create a `RouteManager` interface with `AddRoute`, `AddBypass`, `Cleanup` methods; implement `darwinRouteManager` and `linuxRouteManager` | new `routes.go`, `routes_darwin.go`, `routes_linux.go` | Large |
| 4.2 | Refactor `vpn_darwin.go`'s `cmdVpn` to use `RouteManager` — ~200 lines of route add/change/fallback/cleanup becomes ~30 lines | `vpn_darwin.go` | Large |
| 4.3 | Refactor `vpn_linux.go` similarly | `vpn_linux.go` | Medium |
| 4.4 | `RouteManager.Cleanup()` replaces all teardown code at end of `cmdVpn` on both platforms | Both platform files | Medium |

### Phase 5: Hardening & Polish ⬜ Not started

| # | Task | Files | Effort |
|---|------|-------|--------|
| 5.1 | Add IPv6 support to the packet filter (currently IPv4-only) | `vpn_core.go` | Medium |
| 5.2 | Document the `--password` flag security caveat in README; recommend env vars | `README.md` | Small |
| 5.3 | Implement extender connection in `cmdSocks` or document it as unimplemented | `cmd_socks.go` | Medium |
| 5.4 | Add a `--config` flag to load options from a YAML/TOML file | `config.go` (extend the structs from 3.2 with file loading) | Medium |
| 5.5 | Add structured logging (timestamps, log level prefix) instead of bare `fmt.Printf` | `logger.go` | Medium |
| 5.6 | Add graceful shutdown with timeout (wait for goroutines, not just context cancellation) | `vpn_core.go` | Medium |
| 5.7 | Consider `cmd/urnet-client/` + `internal/` package layout if codebase exceeds ~5k lines | All files | Large |

---

## 3. Recommended Priority

**Phase 1 + 2 are done.** The codebase is now structured, race-free, and passes `go test -race ./...`.

**Phase 3 is next** and is now expanded to address Go best-practice gaps:

1. **Start with 3.0 (lint).** Run `golangci-lint` first to catch low-hanging bugs before writing new code. This takes 30 minutes and pays for itself immediately.
2. **Then 3.1 (eliminate `os.Exit`).** This is the highest-priority correctness fix — `os.Exit` inside helpers bypasses `defer` cleanup, which can leak routes and DNS configuration on error paths.
3. **Then 3.2 + 3.3 (config structs + context propagation).** These are the biggest structural improvements: they decouple business logic from the CLI framework and enable proper cancellation. Do them together since they touch the same function signatures.
4. **Then 3.4–3.5 (API factory + interfaces).** Quick wins that eliminate boilerplate and enable mocking.
5. **Finally 3.6–3.8 (unit tests).** With testable function signatures and injectable dependencies, writing tests becomes straightforward.

**Phase 4** is the big win for long-term maintainability of VPN route logic but carries the highest risk of introducing routing regressions. Do it after Phase 3 provides a test safety net.

**Phase 5** is polish — pick items opportunistically. Note: 5.4 (`--config` flag) now builds on the config structs from 3.2, and 5.7 (graceful shutdown) benefits from the context propagation in 3.3.

---

## 4. What I would NOT do

- **Don't rewrite from scratch.** The code works, has users, and the architecture is fundamentally sound (platform-specific files, shared core, CLI dispatch). It just needs decomposition.
- **Don't add a framework** (cobra, viper, etc.) unless you're also adding subcommand nesting or config file support. docopt is fine for the current interface. The config structs in 3.2 give you the decoupling benefits without a framework swap.
- **Don't refactor the `connect` library integration.** The callback-based API is dictated by the upstream library. Wrapping it further adds indirection without benefit.
- **Don't split into packages prematurely.** A single `main` package is fine at ~3.3k lines. But plan the migration: once the codebase exceeds ~5k lines or you add a second binary, move to `cmd/urnet-client/` + `internal/{vpn,socks,auth}/`. Phase 5.7 tracks this.
- **Don't over-abstract before testing.** Define the minimum interfaces needed for mocking (Phase 3.5), not a full abstraction layer. Let tests tell you where the boundaries should be.
