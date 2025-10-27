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
