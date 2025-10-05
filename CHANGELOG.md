# Changelog

All notable changes to this project will be documented in this file.

## [2.0.0] - 2025-10-04

### 🚀 Major Changes

#### Unified Binary
- **BREAKING**: Merged `port-authorizing-api` and `port-authorizing-cli` into single `port-authorizing` binary
- New command structure:
  - `port-authorizing server` - Start API server
  - `port-authorizing login` - Client login
  - `port-authorizing list` - List connections
  - `port-authorizing connect` - Connect to service
  - `port-authorizing version` - Show version info
- See [MIGRATION.md](MIGRATION.md) for upgrade instructions

#### Documentation Reorganization
- Moved all documentation to `docs/` folder organized by topic
- Removed changelog/summary docs (`*_SUMMARY.md`, `*_UPDATE.md`)
- Created comprehensive [docs index](docs/README.md)
- Simplified main README to be more concise

#### CI/CD
- Added GitHub Actions workflow for automatic Docker Hub publishing
- Multi-architecture Docker images (`linux/amd64`, `linux/arm64`)
- Automatic versioning from git tags
- Published to: `cohandv/port-authorizing`

### ✨ Features

#### OIDC Authentication
- Browser-based OIDC authentication with Authorization Code Flow
- Support for Keycloak, generic OIDC providers
- Automatic role mapping from identity provider
- No password required for OIDC users in PostgreSQL proxy

#### Security Enhancements
- Username enforcement: users can only connect as themselves
- Password-less PostgreSQL for OIDC/SAML users
- Case-insensitive query whitelist matching
- Proper error responses prevent client hanging on blocked queries

#### Multi-Provider Authentication
- Local users (username/password)
- OIDC (Keycloak, Auth0, etc.)
- LDAP (Active Directory, OpenLDAP)
- SAML2 support

#### Role-Based Access Control
- Tag-based connection filtering
- Different whitelists per role per environment
- Flexible policy matching (any/all tags)

### 📝 Documentation

New documentation structure:
```
docs/
├── guides/           - User guides
├── architecture/     - System design docs
├── deployment/       - Build & deploy guides
└── security/         - Security documentation
```

Key docs:
- [Getting Started](docs/guides/getting-started.md)
- [Authentication Guide](docs/guides/authentication.md)
- [OIDC Setup](docs/guides/oidc-setup.md)
- [Configuration](docs/guides/configuration.md)
- [GitHub Actions Setup](docs/deployment/github-actions.md)

### 🐛 Bug Fixes

- Fixed PostgreSQL protocol ReadyForQuery message to prevent client hangs
- Fixed OIDC scope validation (removed invalid "roles" scope)
- Fixed Keycloak redirect URI configuration
- Fixed username extraction from OIDC ID tokens

### 🔧 Infrastructure

- Updated Dockerfile for unified binary
- Updated Makefile with new build targets
- Added `.github/workflows/docker-publish.yml`
- Improved Docker Compose setup with Keycloak auto-import

### 📦 Dependencies

- Added `github.com/spf13/cobra` for CLI framework
- Updated all Go dependencies to latest stable versions

### 🗑️ Removed

- Separate `port-authorizing-api` binary (merged into unified binary)
- Separate `port-authorizing-cli` binary (merged into unified binary)
- Summary/update documentation files
- Redundant configuration examples

### 📚 Migration

See [MIGRATION.md](MIGRATION.md) for detailed upgrade instructions.

**Quick migration:**
```bash
# Old
./port-authorizing-api --config config.yaml
./port-authorizing-cli login

# New
./port-authorizing server --config config.yaml
./port-authorizing login
```

---

## [1.0.0] - Previous Version

### Features
- Basic proxy functionality
- Local user authentication
- PostgreSQL, HTTP, TCP proxying
- Query logging
- Time-limited connections

---

## Format

This changelog follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

### Types of changes
- **Added** for new features
- **Changed** for changes in existing functionality
- **Deprecated** for soon-to-be removed features
- **Removed** for now removed features
- **Fixed** for any bug fixes
- **Security** for vulnerability fixes

