## [1.7.4](https://github.com/cohandv/port-authorizing/compare/v1.7.3...v1.7.4) (2025-10-15)


### Bug Fixes

* ignore main tag ([a8945fe](https://github.com/cohandv/port-authorizing/commit/a8945fe5fd3266168ec91d947d391c5735a62331))

## [1.7.3](https://github.com/cohandv/port-authorizing/compare/v1.7.2...v1.7.3) (2025-10-15)


### Bug Fixes

* callback for oidc was hardcoded, now is get from server ([001d2b0](https://github.com/cohandv/port-authorizing/commit/001d2b09f45aa9a7027d5e8d783c58b451f357bc))

## [1.7.2](https://github.com/cohandv/port-authorizing/compare/v1.7.1...v1.7.2) (2025-10-15)


### Bug Fixes

* updated refs to github repo ([362dd72](https://github.com/cohandv/port-authorizing/commit/362dd72e635b0c8146bd0926e9fe30f1c32bc160))

## [1.7.1](https://github.com/cohandv/port-authorizing/compare/v1.7.0...v1.7.1) (2025-10-15)


### Bug Fixes

* missing using context for list and connect commands ([c817b50](https://github.com/cohandv/port-authorizing/commit/c817b5091ed795632ccd8f717bbb1873c6449e84))

## [1.7.0](https://github.com/cohandv/port-authorizing/compare/v1.6.0...v1.7.0) (2025-10-15)


### Features

* implementation of websockets ([#2](https://github.com/cohandv/port-authorizing/issues/2)) ([6823bba](https://github.com/cohandv/port-authorizing/commit/6823bba3727c6203eb04ea7f1951aa2db8aca99d))

## [1.6.0](https://github.com/cohandv/port-authorizing/compare/v1.5.0...v1.6.0) (2025-10-08)


### Features

* added config contexts ([553dcee](https://github.com/cohandv/port-authorizing/commit/553dcee79b09fb1333420397d024624730071a71))

## [1.5.0](https://github.com/cohandv/port-authorizing/compare/v1.4.0...v1.5.0) (2025-10-07)


### Features

* Added approval process ([c616e1d](https://github.com/cohandv/port-authorizing/commit/c616e1d68e57fee86daff0c1dd7096506156f2ad))

## [1.4.0](https://github.com/cohandv/port-authorizing/compare/v1.3.0...v1.4.0) (2025-10-05)


### Features

* implemented whitelist of http endpoints ([4fe5c9e](https://github.com/cohandv/port-authorizing/commit/4fe5c9e54fce09939b7ce5e9dda0dde26e41cc8d))

## [1.3.0](https://github.com/cohandv/port-authorizing/compare/v1.2.1...v1.3.0) (2025-10-05)


### Features

* add comprehensive unit tests for core packages ([94c6fae](https://github.com/cohandv/port-authorizing/commit/94c6faeeedabcef8abe0ce085c99f7630e05ef47))


### Bug Fixes

* address CVE vulnerabilities in Docker image ([4a6d803](https://github.com/cohandv/port-authorizing/commit/4a6d803f85009ae9bb1eb8c747813e9a06d36b24))


### Documentation

* add CVE fixes documentation ([9a61810](https://github.com/cohandv/port-authorizing/commit/9a618109fc3770b25c10c17eebbbcae454a66838))


### CI/CD

* add automated testing to GitHub Actions workflows ([e4f27f6](https://github.com/cohandv/port-authorizing/commit/e4f27f6511070bb7cc507c8da9db78f49560b2f0))

## [1.2.1](https://github.com/cohandv/port-authorizing/compare/v1.2.0...v1.2.1) (2025-10-05)


### Documentation

* add prominent GitHub repository links to Docker Hub README ([36e11b7](https://github.com/cohandv/port-authorizing/commit/36e11b7fe8085994d4f2ff93db0a4e09120f10e1))

## [1.2.0](https://github.com/cohandv/port-authorizing/compare/v1.1.0...v1.2.0) (2025-10-05)


### Features

* build and push Docker images in release workflow ([d378ccb](https://github.com/cohandv/port-authorizing/commit/d378ccb845788fe16b8db1e099048194eaae02be))
* build and push Docker images in release workflow ([a25254b](https://github.com/cohandv/port-authorizing/commit/a25254b6cff2d88b4fe7d73037d951f1c032a9ee))

## [1.1.0](https://github.com/cohandv/port-authorizing/compare/v1.0.1...v1.1.0) (2025-10-05)


### Features

* adding a new change ([b018e6e](https://github.com/cohandv/port-authorizing/commit/b018e6e9356bde3ae17a62cbc346483e9a1fb7cb))

## [1.0.1](https://github.com/cohandv/port-authorizing/compare/v1.0.0...v1.0.1) (2025-10-05)


### Bug Fixes

* capture semantic-release outputs for dependent jobs ([2aefa7d](https://github.com/cohandv/port-authorizing/commit/2aefa7d8a062aaf8398c56c5e9778338a09a5f41))

## 1.0.0 (2025-10-05)


### Features

* trigger first automatic release ([6245179](https://github.com/cohandv/port-authorizing/commit/6245179ee50f1336ec7bbaaca7a2e3d001a3b7da))

# Changelog

All notable changes to Port Authorizing are **automatically documented** in this file.

This file is generated automatically by [semantic-release](https://github.com/semantic-release/semantic-release) based on [Conventional Commits](https://www.conventionalcommits.org/).

The project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note**: This changelog is automatically updated on each release. Do not edit manually.

## [Unreleased]

### Added
- GitHub Actions workflow for automated binary releases
- Multi-platform binary builds (Linux, macOS, Windows for amd64/arm64)
- Protocol maturity matrix in documentation
- Docker image can now be used as CLI client
- Enhanced version command with build information
- Comprehensive release process with checksums

### Changed
- Unified CLI and API into single `port-authorizing` binary
- Updated documentation to reflect support for any service type (not just databases)
- Improved Docker Hub README with client usage examples
- Enhanced Dockerfile with client mode instructions

### Fixed
- Fixed CLI flag parsing after unification (apiURL and configPath)
- Fixed connection proxy issues with proper variable scoping
- PostgreSQL username validation to prevent impersonation
- Query whitelist enforcement (was being bypassed)
- Case-insensitive regex matching for whitelists
- Client hanging on blocked queries (now sends proper PostgreSQL errors)

### Security
- **CRITICAL**: Fixed PostgreSQL authentication bypass where any username/password was accepted after JWT authentication
- **CRITICAL**: Fixed whitelist bypass allowing developers to execute DELETE queries despite SELECT-only whitelist
- Added username validation to ensure psql client username matches authenticated API user
- Added ReadyForQuery message after error responses to prevent client hangs

## [1.0.0] - Initial Release (if applicable)

### Added
- Multi-provider authentication (Local, OIDC, LDAP, SAML2)
- Role-based access control with tag-based policies
- PostgreSQL transparent proxy with query whitelisting
- HTTP/HTTPS transparent proxy
- TCP basic proxy
- Time-limited connections with automatic expiry
- Comprehensive audit logging
- JWT token-based authentication
- Docker support with multi-architecture images
- CLI for connection management
- API server for proxy coordination

### Protocol Support
- ✅ PostgreSQL: Mature (full protocol awareness)
- ✅ HTTP/HTTPS: Mature (transparent proxying)
- 🚧 TCP: Beta (basic proxying)

---

## Version Format

- **Major.Minor.Patch** (e.g., 2.0.1)
- **Major**: Breaking changes or significant architectural changes
- **Minor**: New features, non-breaking changes
- **Patch**: Bug fixes, security patches, documentation updates

## Release Tags

Each release is tagged as `vX.Y.Z` (e.g., `v2.0.0`) and includes:
- Pre-built binaries for Linux, macOS, and Windows
- Docker images on Docker Hub (`cohandv/port-authorizing:vX.Y.Z`)
- SHA256 checksums for all binaries
- Detailed release notes
