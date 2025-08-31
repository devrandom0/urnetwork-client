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

# Changelog

All notable changes to this project will be documented in this file by semantic-release.
