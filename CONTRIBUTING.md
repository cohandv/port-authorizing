# Contributing to Port Authorizing

Thank you for your interest in contributing! This document provides guidelines and instructions for contributing.

## 🚀 Quick Start

1. **Fork the repository**
2. **Clone your fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/port-authorizing.git
   cd port-authorizing
   ```
3. **Create a branch**:
   ```bash
   git checkout -b feat/my-awesome-feature
   ```
4. **Make your changes**
5. **Test your changes**:
   ```bash
   make test
   make build
   ./bin/port-authorizing --version
   ```
6. **Commit with conventional commit format**:
   ```bash
   git commit -m "feat: add awesome feature"
   ```
7. **Push and create Pull Request**

## 📝 Commit Message Convention

We use [Conventional Commits](https://www.conventionalcommits.org/) for automatic versioning and changelog generation.

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Description | Version Bump | Example |
|------|-------------|--------------|---------|
| `feat` | New feature | **MINOR** | `feat: add LDAP authentication` |
| `fix` | Bug fix | **PATCH** | `fix: resolve connection timeout` |
| `docs` | Documentation | **PATCH** | `docs: update installation guide` |
| `refactor` | Code refactoring | **PATCH** | `refactor: simplify proxy logic` |
| `test` | Adding tests | **PATCH** | `test: add whitelist tests` |
| `perf` | Performance improvement | **PATCH** | `perf: optimize query parsing` |
| `build` | Build system | **PATCH** | `build: update dependencies` |
| `ci` | CI/CD changes | **PATCH** | `ci: improve workflow` |
| `chore` | Maintenance | **No release** | `chore: update .gitignore` |

### Breaking Changes

For breaking changes, add `!` after type or `BREAKING CHANGE:` in footer:

```bash
feat!: change API authentication format

BREAKING CHANGE: API now requires v2 auth headers.
See migration guide for details.
```

### Examples

✅ **Good commit messages:**
```bash
feat: add SAML2 authentication provider
fix: resolve PostgreSQL connection hang
docs: add Docker deployment guide
refactor(proxy): simplify TCP connection handling
test(auth): add OIDC integration tests
```

❌ **Bad commit messages:**
```bash
update stuff
fixed bug
WIP
asdf
```

## 🏗️ Development Setup

### Prerequisites

- Go 1.24+
- Docker & Docker Compose (for testing)
- Make

### Local Development

```bash
# Install dependencies
go mod download

# Build
make build

# Run server
./bin/port-authorizing server --config config.yaml

# Run client
./bin/port-authorizing login -u admin -p admin123
./bin/port-authorizing list
```

### Running Tests

```bash
# Start test environment
docker-compose up -d

# Run tests
make test

# Test specific package
go test ./internal/proxy/...

# With coverage
go test -cover ./...
```

### Docker Testing

```bash
# Build Docker image
make docker-build

# Run in Docker
docker run -p 8080:8080 -v $(pwd)/config.yaml:/app/config.yaml cohandv/port-authorizing:latest

# Test client mode
docker run --rm cohandv/port-authorizing:latest --version
```

## 📋 Code Standards

### Go Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Use `golint` for linting
- Keep functions small and focused
- Add comments for exported functions

```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run
```

### Code Organization

```
port-authorizing/
├── cmd/                  # Entry points
│   └── port-authorizing/ # Unified binary
├── internal/             # Private application code
│   ├── api/             # API server
│   ├── auth/            # Authentication providers
│   ├── cli/             # CLI commands
│   ├── config/          # Configuration
│   ├── proxy/           # Proxy implementations
│   └── security/        # Security features
├── docs/                # Documentation
└── .github/             # GitHub Actions workflows
```

### Testing Requirements

- Add tests for new features
- Maintain test coverage above 70%
- Include integration tests for protocols
- Test error conditions

## 🔄 Pull Request Process

### Before Submitting

1. ✅ Code builds successfully
2. ✅ Tests pass
3. ✅ Code is formatted (`gofmt`)
4. ✅ No linter warnings
5. ✅ Documentation updated (if needed)
6. ✅ Commit messages follow convention

### PR Title

Use conventional commit format:
```
feat: add SAML2 authentication
fix: resolve connection timeout issue
docs: update installation guide
```

### PR Description Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing
Describe how you tested your changes

## Checklist
- [ ] My code follows the code style of this project
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] I have updated the documentation accordingly
- [ ] My commit messages follow the conventional commit format
```

### Review Process

1. Automated checks run (build, test, lint)
2. Maintainer reviews code
3. Address feedback
4. Approval & merge
5. **Automatic release** (if commit triggers version bump)

## 🎯 Areas for Contribution

### High Priority

- [ ] Additional authentication providers (GitHub, GitLab, etc.)
- [ ] More protocol support (Redis, MongoDB, etc.)
- [ ] Enhanced query whitelisting (SQL parsing)
- [ ] Web UI for connection management
- [ ] Metrics and monitoring integration

### Good First Issues

Look for issues tagged with `good first issue`:
- Documentation improvements
- Test additions
- Minor bug fixes
- Example configurations

### Testing & Documentation

- Add integration tests
- Improve documentation
- Add usage examples
- Create video tutorials

## 🐛 Reporting Bugs

### Before Reporting

1. Check existing issues
2. Verify with latest version
3. Test with minimal configuration

### Bug Report Template

```markdown
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce:
1. Configure '...'
2. Run '...'
3. Connect to '...'
4. See error

**Expected behavior**
What you expected to happen.

**Environment:**
- OS: [e.g., Ubuntu 22.04]
- Version: [e.g., v1.2.3]
- Go Version: [e.g., 1.24]
- Docker: [Yes/No]

**Configuration:**
```yaml
# Relevant config.yaml sections (sanitize secrets!)
```

**Logs:**
```
# Relevant error logs
```
```

## 💡 Feature Requests

We welcome feature requests! Please:

1. Check if already requested
2. Describe the use case
3. Explain expected behavior
4. Provide examples if possible

## 📚 Documentation

### Documentation Structure

```
docs/
├── architecture/      # System architecture
├── authentication/    # Auth provider guides
├── deployment/        # Deployment guides
├── development/       # Development guides
└── protocols/         # Protocol-specific docs
```

### Writing Documentation

- Use clear, concise language
- Include code examples
- Add diagrams where helpful
- Test all examples

## 🤝 Code Review

### As a Reviewer

- Be respectful and constructive
- Focus on code, not the person
- Explain the "why" behind suggestions
- Approve when ready (or request changes)

### As an Author

- Respond to all comments
- Don't take feedback personally
- Ask questions if unclear
- Make requested changes or discuss alternatives

## 🏆 Recognition

Contributors are recognized in:
- GitHub contributors page
- Release notes (for significant contributions)
- CONTRIBUTORS.md file (coming soon)

## 📞 Getting Help

- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - General questions
- **Documentation** - Check docs/ folder first

## 📄 License

By contributing, you agree that your contributions will be licensed under the same license as the project (MIT License).

---

**Thank you for contributing!** Every contribution, no matter how small, makes a difference. 🙏

