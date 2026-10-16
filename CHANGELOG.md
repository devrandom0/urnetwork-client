## [4.0.3](https://github.com/devrandom0/urnetwork-client/compare/v4.0.2...v4.0.3) (2026-10-16)

## [4.0.2](https://github.com/devrandom0/urnetwork-client/compare/v4.0.1...v4.0.2) (2026-10-12)

### Bug Fixes

* **ci:** only abort dockerhub cleanup on short tag list and log delete errors ([3c34de6](https://github.com/devrandom0/urnetwork-client/commit/3c34de6c944bad8af970e8844c75bad9aec115ad))

## [4.0.1](https://github.com/devrandom0/urnetwork-client/compare/v4.0.0...v4.0.1) (2026-10-12)

## [4.0.0](https://github.com/devrandom0/urnetwork-client/compare/v3.0.2...v4.0.0) (2026-10-12)

### ⚠ BREAKING CHANGES

* --dns_bootstrap defaults to none instead of bypass.
With --default_route the system DNS is replaced by the client's stub
for the session. IPv6 is blocked when --enable_ipv6 is off instead of
leaking outside the tunnel. --dns_service is ignored.

### Features

* add DNS stub forwarder with tunnel-bound upstreams ([4b451ad](https://github.com/devrandom0/urnetwork-client/commit/4b451addce8e689a6fafe8be77192ea05b33862d))
* add enable_ipv6 key and make none the DNS bootstrap default ([5cde9a5](https://github.com/devrandom0/urnetwork-client/commit/5cde9a508186eb5b028ce3e1a95bd09ffdf5f482))
* add recover command to undo a dead run's DNS and routes ([133060e](https://github.com/devrandom0/urnetwork-client/commit/133060e00878e0d66352401a473f24f170ca3644))
* add safefile.Overwrite for bind-mounted files ([7212786](https://github.com/devrandom0/urnetwork-client/commit/7212786b284155c96e3c5779156eafa4eb536ac1))
* add the leak policy orchestrator with fail-closed rollback ([c659284](https://github.com/devrandom0/urnetwork-client/commit/c659284ec3f5b8752cf863023d443e06a1349b1a))
* apply the IPv6 and DNS leak policy with --default_route ([096d484](https://github.com/devrandom0/urnetwork-client/commit/096d4846ed6234aed20136d202984e57c8f59237))
* block IPv6 and send DNS through the tunnel by default ([75ec6cf](https://github.com/devrandom0/urnetwork-client/commit/75ec6cf8b80645a0f6963a1d3471e06dccf0a7cd))
* block or tunnel IPv6 routes on Linux ([9c06a36](https://github.com/devrandom0/urnetwork-client/commit/9c06a36c899e3bf650db81d1cb4731ddf82cbbd3))
* block or tunnel IPv6 routes on macOS ([9c0ec07](https://github.com/devrandom0/urnetwork-client/commit/9c0ec07b6f7080cc3f10cfc44324c09105f8d5e9))
* filter AAAA and pin control-plane names in the DNS stub ([7484ea4](https://github.com/devrandom0/urnetwork-client/commit/7484ea4a6d5039f7ef9de0f06d46d3d86b6ad3b3))
* let SOCKS resolve through the DNS stub, A records only ([0bc4e82](https://github.com/devrandom0/urnetwork-client/commit/0bc4e82a4c99c90d21f372d712d5adbc1bab43b7))
* make GOOS=windows go build ./... succeed ([79dcac2](https://github.com/devrandom0/urnetwork-client/commit/79dcac24f1d32700b3cceda1bc95a91561b75e34))
* point resolv.conf at the DNS stub with a crash-safe backup ([e6a464a](https://github.com/devrandom0/urnetwork-client/commit/e6a464afa52304f029dc4f04417da6d4a0595024))
* publish the DNS stub to macOS through scutil ([4a5d07e](https://github.com/devrandom0/urnetwork-client/commit/4a5d07eaaefa3615c37b9cb19919231e06f4bf70))
* restore DNS from a watchdog when the VPN dies ([3a92569](https://github.com/devrandom0/urnetwork-client/commit/3a92569fa1a00d010dc4e87a63bc43ca14b7bb3b))
* return routed control-plane addresses from AddBypassEndpoint ([9bd4ab2](https://github.com/devrandom0/urnetwork-client/commit/9bd4ab2e5ebe3e4ec5ea5cb59c59058e51133809))
* route Linux DNS to the stub via systemd-resolved or resolv.conf ([c47eb1d](https://github.com/devrandom0/urnetwork-client/commit/c47eb1daef2833d12fa4b4aae325c05d0cd8b185))

### Bug Fixes

* always try IPv6 reject routes and warn on foreign DNS paths ([0b82508](https://github.com/devrandom0/urnetwork-client/commit/0b825080294768910363903775b4cb6c4821bd5d))
* assign the TUN IPv6 address only with --enable_ipv6 ([d233311](https://github.com/devrandom0/urnetwork-client/commit/d23331178ec5c5185302ca8a56f64804218cbf82))
* bound stub latency, harden its listeners, strip IPv6 hints ([f0fc3d0](https://github.com/devrandom0/urnetwork-client/commit/f0fc3d028ac23a4dd453ceefebac62f588a44c7e))
* bound the wait for a provider window and log stub queries ([0957202](https://github.com/devrandom0/urnetwork-client/commit/095720281e66c60d32122fb61aaac815cc0f4ad1))
* close UDP relay sockets before ctrl in closeWhenIdle ([bd1c8f3](https://github.com/devrandom0/urnetwork-client/commit/bd1c8f3b4e4466805b50e3cc81610a25adb7153f))
* drop outbound IPv6 packets when IPv6 is off ([13dd3bc](https://github.com/devrandom0/urnetwork-client/commit/13dd3bc08e1ca5d1417d53f7c3ccac1e0897d92e))
* gate safefile test hook behind safefiletest build tag ([fd73a64](https://github.com/devrandom0/urnetwork-client/commit/fd73a64d20ffd2bf761a92a33697e902a1e8132d))
* keep docker-build off tag refs entirely ([6d7898f](https://github.com/devrandom0/urnetwork-client/commit/6d7898f405ee96282284b29cb5438126d18fe28c))
* keep resolv.conf backup when the DNS-stub write fails ([4033c12](https://github.com/devrandom0/urnetwork-client/commit/4033c128d83e917e1aa58e426c0d4d34ff7c9738))
* log the IPv6 kill switch hint even if the IPv4 blackhole fails ([07fddc6](https://github.com/devrandom0/urnetwork-client/commit/07fddc601a823c65c35a4401e16599db6095999a))
* make InterfaceControl reject an empty interface name ([3d52d73](https://github.com/devrandom0/urnetwork-client/commit/3d52d733cbd2a84ea0116afedf5b3566184c01b9))
* make lint reproducible on macOS ([8f7a280](https://github.com/devrandom0/urnetwork-client/commit/8f7a280e09329e5d773fd4444ba558f26e20647b))
* make parentDirErr/leafDirErr wording operation-neutral ([a3e8f58](https://github.com/devrandom0/urnetwork-client/commit/a3e8f58a9965108fdf1894579485cca7336eeaf7))
* name timed-out tools and warn when recover cannot list routes ([8d91f27](https://github.com/devrandom0/urnetwork-client/commit/8d91f276ec35c7627e0cd2b1bd3661dfcb772d1a))
* open the cleaned path ReadFile's symlink check validated ([b220ce3](https://github.com/devrandom0/urnetwork-client/commit/b220ce3db165333d96705387ecab409e5381ee1c))
* parse DefaultGateway from stdout only ([528e2ac](https://github.com/devrandom0/urnetwork-client/commit/528e2ac5d3d809dfc3dd8ae5088053f41b637f8e))
* pin ReadFile's directory as root to close a TOCTOU ([0e4a89c](https://github.com/devrandom0/urnetwork-client/commit/0e4a89c8c4e2cb36271cce83e821a4a1db797364))
* rate-limit slow-send warnings and state the real drop reason ([36338c0](https://github.com/devrandom0/urnetwork-client/commit/36338c0c5aacdb2842ea9a5db2567d41a258f113))
* record the DNS owner pid and never recover a live instance ([98bae8a](https://github.com/devrandom0/urnetwork-client/commit/98bae8a537b20ef48f7b61ebedd6de226293797e))
* refuse --background cleanly on windows ([29a52e8](https://github.com/devrandom0/urnetwork-client/commit/29a52e87464b3ca96bb921d66a874d936368a0ed))
* refuse symlinked parent dirs on root ReadFile ([cf62b06](https://github.com/devrandom0/urnetwork-client/commit/cf62b06c4de1c7a5bea77eac7a50877abc2b80ef))
* remove the dead tags: v* CI trigger ([f1e4dd0](https://github.com/devrandom0/urnetwork-client/commit/f1e4dd0a27dcfaf00a16815b66c59d92f3fdc933))
* restore DNS first and bound every cleanup step ([1bdad5b](https://github.com/devrandom0/urnetwork-client/commit/1bdad5b8eb5e97b2ac403fcc77a0a64b6f7cac6b))
* retry watchdog DNS recovery and warn against kill -9 ([a8f6d06](https://github.com/devrandom0/urnetwork-client/commit/a8f6d060b21811c1c222bb94012daabdf23065f0))
* serve the macOS DNS stub on loopback, not the TUN ([de0dbdd](https://github.com/devrandom0/urnetwork-client/commit/de0dbddadb4949f35ec1bc3986f73c995be72232))
* surface stderr in CaptureStdout failures ([29804a7](https://github.com/devrandom0/urnetwork-client/commit/29804a762853496f626977e45b185b6e91406e88))
* survive a dead log reader during DNS cleanup and recovery ([94c142b](https://github.com/devrandom0/urnetwork-client/commit/94c142b353ed7af8f2821e2e1c1016d704b388be))
* validate DNS config early and pin only routed endpoints ([acd2663](https://github.com/devrandom0/urnetwork-client/commit/acd26635385317aa6a6e14c9cc6503f3bb753140))

## [3.0.2](https://github.com/devrandom0/urnetwork-client/compare/v3.0.1...v3.0.2) (2026-10-08)

## [3.0.1](https://github.com/devrandom0/urnetwork-client/compare/v3.0.0...v3.0.1) (2026-10-08)

## [3.0.0](https://github.com/devrandom0/urnetwork-client/compare/v2.0.0...v3.0.0) (2026-09-28)

### ⚠ BREAKING CHANGES

* file writes now refuse group- or world-writable directories, untrusted symlinks in the target directory path, and oversized JWT/config files; privileged system tools must be owned by root and not group/world-writable, or the command fails instead of falling back to PATH. See docs/configuration.md and docs/platform-notes.md.

### Features

* split into internal packages and harden root file and tool use ([ed28b6f](https://github.com/devrandom0/urnetwork-client/commit/ed28b6fbf7e57b99fc7ec07943b44120aa968b9a))

### Bug Fixes

* accept only root-owned, non-writable privileged tools ([c7704c0](https://github.com/devrandom0/urnetwork-client/commit/c7704c0e18931e0285177f92c7c9836da80ab194))
* add safefile helpers that refuse symlinked targets ([fd706d7](https://github.com/devrandom0/urnetwork-client/commit/fd706d7573c4c75fbccb835604b314a66fd1ee49))
* pin safefile dirs with os.Root and check parent dirs ([782de2b](https://github.com/devrandom0/urnetwork-client/commit/782de2b161a231b8b9c1f0e0c9c6661ad8f7eafa))
* relax safefile rules that only matter as root ([9a1e908](https://github.com/devrandom0/urnetwork-client/commit/9a1e908eb303af4ea0a89e434d55176e5876e940))
* run privileged tools from fixed system paths ([c2ea5a1](https://github.com/devrandom0/urnetwork-client/commit/c2ea5a1145b2babbf807f4a7ac55bdc666a3dfad))
* write jwt and log file without following symlinks ([b5a660b](https://github.com/devrandom0/urnetwork-client/commit/b5a660bae53ab81aec04b50487f52f5111a5ed7a))

## [2.0.0](https://github.com/devrandom0/urnetwork-client/compare/v1.13.26...v2.0.0) (2026-09-28)

### ⚠ BREAKING CHANGES

* SOCKS requests are refused when the proxy cannot bind
to the VPN interface, instead of silently leaving through the physical
interface. With --tun, SOCKS hostname lookups also go through the tunnel,
so a LAN or Docker resolver needs --dns=<public resolver>.
--api_url and --connect_url (flags or config file) must use https/wss;
http/ws is only accepted for localhost. Kill switch and split default
route failures now abort VPN startup instead of warning.

### Features

* add --socks_user/--socks_pass flags with env fallback ([2f02d5f](https://github.com/devrandom0/urnetwork-client/commit/2f02d5ff5c806ae587b01eeff63f32f6a424873d))
* add SOCKS5 username/password auth and method negotiation ([204e1d8](https://github.com/devrandom0/urnetwork-client/commit/204e1d84159f60b986a10a1412e04d0f8ee49ad7))
* fail closed on SOCKS bind and reject cleartext endpoints ([5e165fd](https://github.com/devrandom0/urnetwork-client/commit/5e165fd804fd1ffd8dce53f4698e57f5d87084b5))

### Bug Fixes

* abort startup when kill switch or split routes fail ([c2a0ca2](https://github.com/devrandom0/urnetwork-client/commit/c2a0ca2ffc72f82a47489d4e0b1154de4f52d54f))
* abort VPN startup when TUN configuration fails ([d1f40f2](https://github.com/devrandom0/urnetwork-client/commit/d1f40f2536939524c8dc6f84f02e0ae5c37afe07))
* allow setting Version with -ldflags and in Docker builds ([59da544](https://github.com/devrandom0/urnetwork-client/commit/59da5445ad15477ad615f3fcb111beca5e65694b))
* apply config file log_level and accept socks_listen key ([75185a7](https://github.com/devrandom0/urnetwork-client/commit/75185a75f6fc2cee574ab1ceed3976226e2a1e69))
* bound API HTTP time and body size during startup lookups ([9725f09](https://github.com/devrandom0/urnetwork-client/commit/9725f09a50a985f794668a28ea02c3912448d5aa))
* cap SOCKS connections, idle UDP and failed auth ([3531306](https://github.com/devrandom0/urnetwork-client/commit/35313062c1ebed87ae89f3188a44c2894e6c3c88))
* fail VPN startup on half-configured SOCKS auth ([8b0f29f](https://github.com/devrandom0/urnetwork-client/commit/8b0f29f76c41b94876c77d194ec2f7147f51ea98))
* fail vpn with a clear error when no JWT is available ([73a0edb](https://github.com/devrandom0/urnetwork-client/commit/73a0edb0ad764005a464cc567886bc20e10a8d08))
* fail vpn without TUN or SOCKS and add SocksOptions ([3ef9536](https://github.com/devrandom0/urnetwork-client/commit/3ef95366a298778df5c74e8921c7dcc76d6d9bd6))
* harden --background handoff of args, config and secrets ([3ebb454](https://github.com/devrandom0/urnetwork-client/commit/3ebb4540ef6c8491a04bd37ec53ab4420fdb6082))
* honor --config in quick-connect and bound login retry wait ([4da6951](https://github.com/devrandom0/urnetwork-client/commit/4da69514a5960fe6b4c2132b91de18c378c997d0))
* honor explicit --jwt in quick-connect and start VPN with it ([a42488c](https://github.com/devrandom0/urnetwork-client/commit/a42488cab7f092c2e3970f628b4a11f9324dcd00))
* keep SOCKS accept loop alive on transient errors ([bbd9417](https://github.com/devrandom0/urnetwork-client/commit/bbd9417e009a8bf10c1d63d385a50c4656a26e2b))
* keep SOCKS UDP associations alive and close them with control ([958694e](https://github.com/devrandom0/urnetwork-client/commit/958694e285c8b0245c8abc2e5ea740b1c849edd0))
* only clean up Linux routes this session added ([5a47ad7](https://github.com/devrandom0/urnetwork-client/commit/5a47ad78b80fec18a2938b87ea060c2170549fa5))
* parse SOCKS request address for all commands and encode BND ([929700d](https://github.com/devrandom0/urnetwork-client/commit/929700d5b46a07da715460bb46aa406c3fd2ddda))
* pass secret flags to the background child via environment ([f2c83ff](https://github.com/devrandom0/urnetwork-client/commit/f2c83ff865bc35ae30904130890001d8a20d7d51))
* pin SOCKS UDP client and accept replies only from peers ([45f089e](https://github.com/devrandom0/urnetwork-client/commit/45f089efca6e22ea477cbccf5e4d18cf1d0260b0))
* preserve docopt flag pairing when scrubbing secrets ([0702710](https://github.com/devrandom0/urnetwork-client/commit/07027109b1306c607c2fd11fc2363e417b864418))
* refuse API redirects to cleartext non-loopback URLs ([c9460b3](https://github.com/devrandom0/urnetwork-client/commit/c9460b35eaccad9d555365ca028b6d79c3a80767))
* refuse SOCKS requests when the VPN interface cannot be bound ([228ea81](https://github.com/devrandom0/urnetwork-client/commit/228ea81c545277a76d4d7e068f9b11a419bd64d7))
* reject cleartext API and connect URLs except on localhost ([5f8a95d](https://github.com/devrandom0/urnetwork-client/commit/5f8a95d77ac0cf57f06cbdc2359307d5769e6e15))
* reject cleartext endpoint URLs from the config file ([b85a364](https://github.com/devrandom0/urnetwork-client/commit/b85a3642bfb77327952f276baf9b23cab7c1aee8))
* resolve SOCKS hostnames through the tunnel, fail closed ([984b6a2](https://github.com/devrandom0/urnetwork-client/commit/984b6a2f6d54e2e17f48ef46f236f276064fc350))
* restore logError used by macOS kill switch restore ([17e0ad0](https://github.com/devrandom0/urnetwork-client/commit/17e0ad0ee64490e10aeb2ae4cabf3d37abfa4392))
* restore macOS default route when kill switch setup fails ([228875f](https://github.com/devrandom0/urnetwork-client/commit/228875ff4725e964dda41d0fc4dbb63afbb53a4f))
* serialize macOS DNS bypass removal with route cleanup ([0ac7372](https://github.com/devrandom0/urnetwork-client/commit/0ac7372ff992c78367ebc02d14aa8482ca8c8e8b))
* warn on open SOCKS binds and publish compose on loopback ([4e1bdac](https://github.com/devrandom0/urnetwork-client/commit/4e1bdac99b53e3b139fd566e2ff0ff7ee9845fb5))

## [1.13.26](https://github.com/devrandom0/urnetwork-client/compare/v1.13.25...v1.13.26) (2026-09-28)

## [1.13.25](https://github.com/devrandom0/urnetwork-client/compare/v1.13.24...v1.13.25) (2026-09-28)

## [1.13.24](https://github.com/devrandom0/urnetwork-client/compare/v1.13.23...v1.13.24) (2026-09-26)

## [1.13.23](https://github.com/devrandom0/urnetwork-client/compare/v1.13.22...v1.13.23) (2026-09-21)

## [1.13.22](https://github.com/devrandom0/urnetwork-client/compare/v1.13.21...v1.13.22) (2026-09-21)

## [1.13.21](https://github.com/devrandom0/urnetwork-client/compare/v1.13.20...v1.13.21) (2026-09-19)

## [1.13.20](https://github.com/devrandom0/urnetwork-client/compare/v1.13.19...v1.13.20) (2026-09-16)

## [1.13.19](https://github.com/devrandom0/urnetwork-client/compare/v1.13.18...v1.13.19) (2026-09-13)

## [1.13.18](https://github.com/devrandom0/urnetwork-client/compare/v1.13.17...v1.13.18) (2026-09-11)

## [1.13.17](https://github.com/devrandom0/urnetwork-client/compare/v1.13.16...v1.13.17) (2026-09-11)

## [1.13.16](https://github.com/devrandom0/urnetwork-client/compare/v1.13.15...v1.13.16) (2026-09-11)

## [1.13.15](https://github.com/devrandom0/urnetwork-client/compare/v1.13.14...v1.13.15) (2026-09-11)

### Bug Fixes

* **ci:** pin conventional-changelog-conventionalcommits to v8 ([8ab380b](https://github.com/devrandom0/urnetwork-client/commit/8ab380bd0921c45e455e6cbb44114fac6dbd8a40))

## [1.13.14](https://github.com/devrandom0/urnetwork-client/compare/v1.13.13...v1.13.14) (2026-08-14)

## [1.13.13](https://github.com/devrandom0/urnetwork-client/compare/v1.13.12...v1.13.13) (2026-08-10)

## [1.13.12](https://github.com/devrandom0/urnetwork-client/compare/v1.13.11...v1.13.12) (2026-08-05)

## [1.13.11](https://github.com/devrandom0/urnetwork-client/compare/v1.13.10...v1.13.11) (2026-08-05)

## [1.13.10](https://github.com/devrandom0/urnetwork-client/compare/v1.13.9...v1.13.10) (2026-08-02)

## [1.13.9](https://github.com/devrandom0/urnetwork-client/compare/v1.13.8...v1.13.9) (2026-07-17)

## [1.13.8](https://github.com/devrandom0/urnetwork-client/compare/v1.13.7...v1.13.8) (2026-07-08)

## [1.13.7](https://github.com/devrandom0/urnetwork-client/compare/v1.13.6...v1.13.7) (2026-06-20)

## [1.13.6](https://github.com/devrandom0/urnetwork-client/compare/v1.13.5...v1.13.6) (2026-06-20)

### Bug Fixes

* **ci:** recover from orphaned release tag on diverged commit ([#77](https://github.com/devrandom0/urnetwork-client/issues/77)) ([ce54122](https://github.com/devrandom0/urnetwork-client/commit/ce5412283ef1e11d1a3b3eea8533a28ffda69d60))

## [1.13.5](https://github.com/devrandom0/urnetwork-client/compare/v1.13.4...v1.13.5) (2026-06-20)

## [1.13.5](https://github.com/devrandom0/urnetwork-client/compare/v1.13.4...v1.13.5) (2026-06-08)

## [1.13.5](https://github.com/devrandom0/urnetwork-client/compare/v1.13.4...v1.13.5) (2026-05-21)

## [1.13.5](https://github.com/devrandom0/urnetwork-client/compare/v1.13.4...v1.13.5) (2026-05-20)

## [1.13.5](https://github.com/devrandom0/urnetwork-client/compare/v1.13.4...v1.13.5) (2026-05-05)

## [1.13.3](https://github.com/devrandom0/urnetwork-client/compare/v1.13.2...v1.13.3) (2026-05-03)

### Bug Fixes

* update dns binding issue when dnServers is non-empty ([a52c954](https://github.com/devrandom0/urnetwork-client/commit/a52c9549663b7cbd8c1c8a9ef5235f9166cd7cb9))

## [1.13.2](https://github.com/devrandom0/urnetwork-client/compare/v1.13.1...v1.13.2) (2026-04-22)

## [1.13.1](https://github.com/devrandom0/urnetwork-client/compare/v1.13.0...v1.13.1) (2026-04-18)

## [1.13.0](https://github.com/devrandom0/urnetwork-client/compare/v1.12.0...v1.13.0) (2026-04-11)

### Features

* add --enable_ipv6 flag to control IPv6 traffic routing ([193d3ec](https://github.com/devrandom0/urnetwork-client/commit/193d3eccf957eb1888ef53eba69cd450f00cda7d))
* add --kill_switch flag to prevent IP leaks on VPN disconnect ([d93cfb5](https://github.com/devrandom0/urnetwork-client/commit/d93cfb53970a6210929a01554291420430d35ce7))

# Changelog

All notable changes to this project will be documented in this file by semantic-release.

## [1.13.0](https://github.com/devrandom0/urnetwork-client/compare/v1.12.0...v1.13.0) (2026-04-11)

### Features

* add --enable_ipv6 flag to control IPv6 traffic routing ([193d3ec](https://github.com/devrandom0/urnetwork-client/commit/193d3eccf957eb1888ef53eba69cd450f00cda7d))
* add --kill_switch flag to prevent IP leaks on VPN disconnect ([d93cfb5](https://github.com/devrandom0/urnetwork-client/commit/d93cfb53970a6210929a01554291420430d35ce7))

## [1.12.0](https://github.com/devrandom0/urnetwork-client/compare/v1.11.0...v1.12.0) (2026-04-10)

### Features

* add IPv6-to-IPv4 fallback for SOCKS proxy when IPv6 provider unavailable ([4f7a015](https://github.com/devrandom0/urnetwork-client/commit/4f7a015edd8c7f88eb87a71b899655d53f9d2e03))

## [1.11.0](https://github.com/devrandom0/urnetwork-client/compare/v1.10.0...v1.11.0) (2026-04-10)

### Features

* add IPv6 address configuration to TUN interfaces on macOS and Linux ([919ff84](https://github.com/devrandom0/urnetwork-client/commit/919ff84a457063aa4cfc7c9bc7ddfcb4123d3096))
* enable IPv6 interface binding for SOCKS on macOS and Linux ([b78adfe](https://github.com/devrandom0/urnetwork-client/commit/b78adfe465c91b5bbe8a4868284d40b0a3d01453))

## [1.10.0](https://github.com/devrandom0/urnetwork-client/compare/v1.9.0...v1.10.0) (2026-04-10)

### Features

* implement Phase 5 – IPv6 filter, structured logging, config file, graceful shutdown ([e0c3d5e](https://github.com/devrandom0/urnetwork-client/commit/e0c3d5eec5f974c788ea50ad2ecdde939c5cc5cd))

### Bug Fixes

* update for lint ([591b01e](https://github.com/devrandom0/urnetwork-client/commit/591b01e2f20cc4ea46e098a6c2d7722c0ca61003))

## [Unreleased]

### CI/CD

* CI via Makefile: lint, unit (race), integration with JWT gating, Docker build test (no push) ([d831af1](https://github.com/devrandom0/urnetwork-client/commit/d831af1), [cf99723](https://github.com/devrandom0/urnetwork-client/commit/cf99723), [229c921](https://github.com/devrandom0/urnetwork-client/commit/229c921))
* Add get-jwt job with secrets; mask credentials; robust gating ([cf61582](https://github.com/devrandom0/urnetwork-client/commit/cf61582), [9f3b624](https://github.com/devrandom0/urnetwork-client/commit/9f3b624))
* Release assets: build linux/darwin amd64+arm64 binaries and attach to GitHub Releases ([0455b25](https://github.com/devrandom0/urnetwork-client/commit/0455b25))
* Build once, reuse CLI via artifact in jobs; remove redundant builds ([pending])

### Build & Docker

* Narrow Docker context and COPY; ignore tests/docs/ci in .dockerignore ([d09a648](https://github.com/devrandom0/urnetwork-client/commit/d09a648))
* Update runtime base to alpine:3.22 ([ec80284](https://github.com/devrandom0/urnetwork-client/commit/ec80284))
* docker-compose: default to GHCR image; optional local build; fix YAML ([pending])

### Dependencies (Renovate)

* Add Renovate configuration (gomod, Dockerfile, GitHub Actions) ([72c4276](https://github.com/devrandom0/urnetwork-client/commit/72c4276))
* deps: actions/checkout to v5 ([e16bdd9](https://github.com/devrandom0/urnetwork-client/commit/e16bdd9), [04e7bec](https://github.com/devrandom0/urnetwork-client/commit/04e7bec), [#9](https://github.com/devrandom0/urnetwork-client/pull/9))
* deps: node to v22 ([b8eb626](https://github.com/devrandom0/urnetwork-client/commit/b8eb626), [#3](https://github.com/devrandom0/urnetwork-client/pull/3))
* deps: alpine to 3.22 ([ec80284](https://github.com/devrandom0/urnetwork-client/commit/ec80284), [#1](https://github.com/devrandom0/urnetwork-client/pull/1))
* deps: github.com/urnetwork/connect digest bump ([61273b6](https://github.com/devrandom0/urnetwork-client/commit/61273b6), [#4](https://github.com/devrandom0/urnetwork-client/pull/4))

### Docs/Meta

* README badge and formatting ([eb0aa5e](https://github.com/devrandom0/urnetwork-client/commit/eb0aa5e))
* Remove CircleCI config ([59ec5a8](https://github.com/devrandom0/urnetwork-client/commit/59ec5a8), [6547725](https://github.com/devrandom0/urnetwork-client/commit/6547725))## [1.13.0](https://github.com/devrandom0/urnetwork-client/compare/v1.12.0...v1.13.0) (2026-04-11)

### Features

* add --enable_ipv6 flag to control IPv6 traffic routing ([193d3ec](https://github.com/devrandom0/urnetwork-client/commit/193d3eccf957eb1888ef53eba69cd450f00cda7d))
* add --kill_switch flag to prevent IP leaks on VPN disconnect ([d93cfb5](https://github.com/devrandom0/urnetwork-client/commit/d93cfb53970a6210929a01554291420430d35ce7))

## [1.13.0](https://github.com/devrandom0/urnetwork-client/compare/v1.12.0...v1.13.0) (2026-04-11)

### Features

* add --enable_ipv6 flag to control IPv6 traffic routing ([193d3ec](https://github.com/devrandom0/urnetwork-client/commit/193d3eccf957eb1888ef53eba69cd450f00cda7d))
* add --kill_switch flag to prevent IP leaks on VPN disconnect ([d93cfb5](https://github.com/devrandom0/urnetwork-client/commit/d93cfb53970a6210929a01554291420430d35ce7))

## [1.13.0](https://github.com/devrandom0/urnetwork-client/compare/v1.12.0...v1.13.0) (2026-04-11)

### Features

* add --enable_ipv6 flag to control IPv6 traffic routing ([193d3ec](https://github.com/devrandom0/urnetwork-client/commit/193d3eccf957eb1888ef53eba69cd450f00cda7d))
* add --kill_switch flag to prevent IP leaks on VPN disconnect ([d93cfb5](https://github.com/devrandom0/urnetwork-client/commit/d93cfb53970a6210929a01554291420430d35ce7))

## [1.12.0](https://github.com/devrandom0/urnetwork-client/compare/v1.11.0...v1.12.0) (2026-04-10)

### Features

* add IPv6-to-IPv4 fallback for SOCKS proxy when IPv6 provider unavailable ([4f7a015](https://github.com/devrandom0/urnetwork-client/commit/4f7a015edd8c7f88eb87a71b899655d53f9d2e03))

## [1.11.0](https://github.com/devrandom0/urnetwork-client/compare/v1.10.0...v1.11.0) (2026-04-10)

### Features

* add IPv6 address configuration to TUN interfaces on macOS and Linux ([919ff84](https://github.com/devrandom0/urnetwork-client/commit/919ff84a457063aa4cfc7c9bc7ddfcb4123d3096))
* enable IPv6 interface binding for SOCKS on macOS and Linux ([b78adfe](https://github.com/devrandom0/urnetwork-client/commit/b78adfe465c91b5bbe8a4868284d40b0a3d01453))

## [1.10.0](https://github.com/devrandom0/urnetwork-client/compare/v1.9.0...v1.10.0) (2026-04-10)

### Features

* implement Phase 5 – IPv6 filter, structured logging, config file, graceful shutdown ([e0c3d5e](https://github.com/devrandom0/urnetwork-client/commit/e0c3d5eec5f974c788ea50ad2ecdde939c5cc5cd))

### Bug Fixes

* update for lint ([591b01e](https://github.com/devrandom0/urnetwork-client/commit/591b01e2f20cc4ea46e098a6c2d7722c0ca61003))

## [1.9.0](https://github.com/devrandom0/urnetwork-client/compare/v1.8.3...v1.9.0) (2026-04-10)

### Features

* implement Phase 3.5 and 3.8 - dependency interfaces and specs tests ([91dd1bb](https://github.com/devrandom0/urnetwork-client/commit/91dd1bb501445f73329a06bf02b56092dcb50cae))

## [1.8.3](https://github.com/devrandom0/urnetwork-client/compare/v1.8.2...v1.8.3) (2026-04-10)

### Bug Fixes

* lint ([a9e4a79](https://github.com/devrandom0/urnetwork-client/commit/a9e4a79217e551d8c87f83df1a11411406560fe0))
* lint ([05cea66](https://github.com/devrandom0/urnetwork-client/commit/05cea66e932d9b45f17eb600e194a016b3e485ba))
* update for lint ([f478fab](https://github.com/devrandom0/urnetwork-client/commit/f478fab3b02763362d2c52f9818461186abb4161))
* update for lint ([9fe3911](https://github.com/devrandom0/urnetwork-client/commit/9fe39110a5e8dbaf7921fcf8041cf24c40de930d))
* update workflow ([b2dd668](https://github.com/devrandom0/urnetwork-client/commit/b2dd668ddcf3b0d22f4e707f9bf79f60ffa2725a))
* update workflow ([feee906](https://github.com/devrandom0/urnetwork-client/commit/feee9063a503a894428faab66942bf1f05573a05))

## [1.8.2](https://github.com/devrandom0/urnetwork-client/compare/v1.8.1...v1.8.2) (2026-04-10)

## [1.8.1](https://github.com/devrandom0/urnetwork-client/compare/v1.8.0...v1.8.1) (2026-04-10)

## [1.8.0](https://github.com/devrandom0/urnetwork-client/compare/v1.7.12...v1.8.0) (2026-04-10)

### Features

* upgrade to go 1.25 ([5234b66](https://github.com/devrandom0/urnetwork-client/commit/5234b666dc2d77be2fb57266d2e71d343916e471))

## [1.7.12](https://github.com/devrandom0/urnetwork-client/compare/v1.7.11...v1.7.12) (2026-04-10)

## [1.7.11](https://github.com/devrandom0/urnetwork-client/compare/v1.7.10...v1.7.11) (2026-02-25)

## [1.7.10](https://github.com/devrandom0/urnetwork-client/compare/v1.7.9...v1.7.10) (2026-02-10)

## [1.7.9](https://github.com/devrandom0/urnetwork-client/compare/v1.7.8...v1.7.9) (2026-02-08)

## [1.7.8](https://github.com/devrandom0/urnetwork-client/compare/v1.7.7...v1.7.8) (2026-02-01)

## [1.7.7](https://github.com/devrandom0/urnetwork-client/compare/v1.7.6...v1.7.7) (2026-01-26)

## [1.7.6](https://github.com/devrandom0/urnetwork-client/compare/v1.7.5...v1.7.6) (2026-01-23)

## [1.7.5](https://github.com/devrandom0/urnetwork-client/compare/v1.7.4...v1.7.5) (2026-01-20)

## [1.7.4](https://github.com/devrandom0/urnetwork-client/compare/v1.7.3...v1.7.4) (2026-01-20)

### Bug Fixes

* clean up duplicate CHANGELOG entries ([5a889bb](https://github.com/devrandom0/urnetwork-client/commit/5a889bb4ec47ad8b55623c91c4eb565192ac66c9))
* clean up duplicate CHANGELOG entries ([0d61f6c](https://github.com/devrandom0/urnetwork-client/commit/0d61f6cf6c34117c9105baad13dfb264899d0049))
* clean up duplicate CHANGELOG entries for v1.7.4 ([0929957](https://github.com/devrandom0/urnetwork-client/commit/0929957dba6630dc3f9d6076a7bb1f5f8844efa9))
* update go.mod ([8c57e52](https://github.com/devrandom0/urnetwork-client/commit/8c57e52c25c0275d171aa48c4407a7e9fb3a1ba3))

## [1.7.3](https://github.com/devrandom0/urnetwork-client/compare/v1.7.2...v1.7.3) (2026-01-07)

## [1.7.2](https://github.com/devrandom0/urnetwork-client/compare/v1.7.1...v1.7.2) (2026-01-07)

## [1.7.1](https://github.com/devrandom0/urnetwork-client/compare/v1.7.0...v1.7.1) (2025-11-17)

## [1.7.0](https://github.com/devrandom0/urnetwork-client/compare/v1.6.12...v1.7.0) (2025-11-17)

### Features

* add aarch64 ([5c9ffe4](https://github.com/devrandom0/urnetwork-client/commit/5c9ffe4462176163c8b2cc40ae215203fa6aece1))

## [1.6.12](https://github.com/devrandom0/urnetwork-client/compare/v1.6.11...v1.6.12) (2025-11-06)

## [1.6.11](https://github.com/devrandom0/urnetwork-client/compare/v1.6.10...v1.6.11) (2025-11-06)

## [1.6.10](https://github.com/devrandom0/urnetwork-client/compare/v1.6.9...v1.6.10) (2025-10-28)

### Bug Fixes

* exit with non-zero code on login/verify errors ([d041b6e](https://github.com/devrandom0/urnetwork-client/commit/d041b6e173facf59cd46e7a539fb483fb3b6f211))

## [1.6.9](https://github.com/devrandom0/urnetwork-client/compare/v1.6.8...v1.6.9) (2025-10-27)

## [1.6.8](https://github.com/devrandom0/urnetwork-client/compare/v1.6.7...v1.6.8) (2025-10-27)

## [1.6.7](https://github.com/devrandom0/urnetwork-client/compare/v1.6.6...v1.6.7) (2025-10-27)

## [1.6.6](https://github.com/devrandom0/urnetwork-client/compare/v1.6.5...v1.6.6) (2025-09-28)

## [1.6.5](https://github.com/devrandom0/urnetwork-client/compare/v1.6.4...v1.6.5) (2025-09-11)

## [1.6.4](https://github.com/devrandom0/urnetwork-client/compare/v1.6.3...v1.6.4) (2025-09-04)

## [1.6.3](https://github.com/devrandom0/urnetwork-client/compare/v1.6.2...v1.6.3) (2025-09-04)

## [1.6.2](https://github.com/devrandom0/urnetwork-client/compare/v1.6.1...v1.6.2) (2025-09-01)

## [1.6.1](https://github.com/devrandom0/urnetwork-client/compare/v1.6.0...v1.6.1) (2025-09-01)

## [1.5.0](https://github.com/devrandom0/urnetwork-client/compare/v1.4.1...v1.5.0) (2025-08-31)

### Features

* **cli:** print startup configuration summary on Linux and macOS (no secrets) ([1ac6994](https://github.com/devrandom0/urnetwork-client/commit/1ac6994701f3adb8b80b6753b7c5d302d1800afe))

## [1.4.1](https://github.com/devrandom0/urnetwork-client/compare/v1.4.0...v1.4.1) (2025-08-31)

## [1.4.0](https://github.com/devrandom0/urnetwork-client/compare/v1.3.2...v1.4.0) (2025-08-31)

### Features

* **vpn:** add --no_fw_rules to enforce local_only/allow/deny in userspace; implement IPv4 packet filtering in TUN loop and provider receive; gate OS-level iptables/route/DNS changes; docs ([512fb4a](https://github.com/devrandom0/urnetwork-client/commit/512fb4a218e630ab34e969026c9c6bcfde5164ab))

## [1.3.2](https://github.com/devrandom0/urnetwork-client/compare/v1.3.1...v1.3.2) (2025-08-31)

# Changelog

All notable changes to this project will be documented in this file by semantic-release.

## [Unreleased]

### CI/CD

* CI via Makefile: lint, unit (race), integration with JWT gating, Docker build test (no push) ([d831af1](https://github.com/devrandom0/urnetwork-client/commit/d831af1), [cf99723](https://github.com/devrandom0/urnetwork-client/commit/cf99723), [229c921](https://github.com/devrandom0/urnetwork-client/commit/229c921))
* Add get-jwt job with secrets; mask credentials; robust gating ([cf61582](https://github.com/devrandom0/urnetwork-client/commit/cf61582), [9f3b624](https://github.com/devrandom0/urnetwork-client/commit/9f3b624))
* Release assets: build linux/darwin amd64+arm64 binaries and attach to GitHub Releases ([0455b25](https://github.com/devrandom0/urnetwork-client/commit/0455b25))
* Build once, reuse CLI via artifact in jobs; remove redundant builds ([pending])

### Build & Docker

* Narrow Docker context and COPY; ignore tests/docs/ci in .dockerignore ([d09a648](https://github.com/devrandom0/urnetwork-client/commit/d09a648))
* Update runtime base to alpine:3.22 ([ec80284](https://github.com/devrandom0/urnetwork-client/commit/ec80284))
* docker-compose: default to GHCR image; optional local build; fix YAML ([pending])

### Dependencies (Renovate)

* Add Renovate configuration (gomod, Dockerfile, GitHub Actions) ([72c4276](https://github.com/devrandom0/urnetwork-client/commit/72c4276))
* deps: actions/checkout to v5 ([e16bdd9](https://github.com/devrandom0/urnetwork-client/commit/e16bdd9), [04e7bec](https://github.com/devrandom0/urnetwork-client/commit/04e7bec), [#9](https://github.com/devrandom0/urnetwork-client/pull/9))
* deps: node to v22 ([b8eb626](https://github.com/devrandom0/urnetwork-client/commit/b8eb626), [#3](https://github.com/devrandom0/urnetwork-client/pull/3))
* deps: alpine to 3.22 ([ec80284](https://github.com/devrandom0/urnetwork-client/commit/ec80284), [#1](https://github.com/devrandom0/urnetwork-client/pull/1))
* deps: github.com/urnetwork/connect digest bump ([61273b6](https://github.com/devrandom0/urnetwork-client/commit/61273b6), [#4](https://github.com/devrandom0/urnetwork-client/pull/4))

### Docs/Meta

* README badge and formatting ([eb0aa5e](https://github.com/devrandom0/urnetwork-client/commit/eb0aa5e))
* Remove CircleCI config ([59ec5a8](https://github.com/devrandom0/urnetwork-client/commit/59ec5a8), [6547725](https://github.com/devrandom0/urnetwork-client/commit/6547725))

## 1.1.0 (2025-08-31)

### Features (Linux routing)

* Linux: switch to macOS-like split-default routing (0.0.0.0/1 and 128.0.0.0/1 via TUN), add control-plane bypass and excludes; precise cleanup ([1ba9c03](https://github.com/devrandom0/urnetwork-client/commit/1ba9c03))

### Bug Fixes / Improvements

* Linux routing: ensure single TUN-preferred default; remove duplicates; re-add original default with metrics; robust multi-default parsing ([0b9d915](https://github.com/devrandom0/urnetwork-client/commit/0b9d915), [fb3d535](https://github.com/devrandom0/urnetwork-client/commit/fb3d535))

### Docs (README)

* README: fix Markdown fences; finalize Linux split-default notes ([f8afe98](https://github.com/devrandom0/urnetwork-client/commit/f8afe98))

### CI/CD (GitHub Actions)

* Migrate to GitHub Actions: lint, test, multi-arch Docker push to GHCR ([93b8756](https://github.com/devrandom0/urnetwork-client/commit/93b8756))
* Buildx caching: enable GHA + registry cache, inline cache ([60d936e](https://github.com/devrandom0/urnetwork-client/commit/60d936e))
* Image tagging: add semver tags; cancel in-progress runs; semantic-release for auto versioning ([af44cd4](https://github.com/devrandom0/urnetwork-client/commit/af44cd4), [e8d1476](https://github.com/devrandom0/urnetwork-client/commit/e8d1476))
* semantic-release: fix missing plugin; use outputs for image tagging ([6e048e1](https://github.com/devrandom0/urnetwork-client/commit/6e048e1), [49da177](https://github.com/devrandom0/urnetwork-client/commit/49da177))
* GHCR: PAT fallback auth; add manual force tag and publish workflow ([229602a](https://github.com/devrandom0/urnetwork-client/commit/229602a), [7ca8259](https://github.com/devrandom0/urnetwork-client/commit/7ca8259))



## 1.0.0 (2025-08-30)

### Features

* cicd ([e18f1c8](https://github.com/devrandom0/urnetwork-client/commit/e18f1c81d0ea893252f9f6b90c6ae3376e9ec2ed))

### Bug Fixes

* auto assign tunnel name ([503f244](https://github.com/devrandom0/urnetwork-client/commit/503f2444a60f1c836a7b584787ab6f18413cb746))
* because of mikrotik, changed Dockerfile user to root ([739ef84](https://github.com/devrandom0/urnetwork-client/commit/739ef84371e133ce839577b4200821467d19845d))
* Docker build and missing cmdSocks function ([063beb7](https://github.com/devrandom0/urnetwork-client/commit/063beb7b56872c4ef40a27b7b1ba3531bff601e6))
* update go mod ([df392c6](https://github.com/devrandom0/urnetwork-client/commit/df392c6a805a740be62bda8a284cdae34d587dbe))
