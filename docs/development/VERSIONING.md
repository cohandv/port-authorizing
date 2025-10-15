# Automatic Versioning and Release Process

Port Authorizing uses **fully automated versioning** based on [Conventional Commits](https://www.conventionalcommits.org/). Every push to `main` triggers automatic analysis, versioning, and release.

## 🤖 How It Works

1. **Push to main** → GitHub Actions analyzes commits
2. **Determines version bump** based on commit types
3. **Automatically creates tag** (e.g., `v1.2.3`)
4. **Builds binaries** for all platforms
5. **Creates GitHub Release** with release notes
6. **Publishes Docker images** to Docker Hub
7. **Updates CHANGELOG.md** automatically

**No manual tagging required!** 🎉

## 📝 Commit Message Format

Use [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Commit Types

| Type | Version Bump | Example | Description |
|------|--------------|---------|-------------|
| `feat` | **MINOR** (0.X.0) | `feat: add LDAP authentication` | New features |
| `fix` | **PATCH** (0.0.X) | `fix: resolve connection timeout` | Bug fixes |
| `perf` | **PATCH** (0.0.X) | `perf: optimize query parsing` | Performance improvements |
| `docs` | **PATCH** (0.0.X) | `docs: update installation guide` | Documentation only |
| `refactor` | **PATCH** (0.0.X) | `refactor: simplify proxy logic` | Code refactoring |
| `test` | **PATCH** (0.0.X) | `test: add whitelist tests` | Adding tests |
| `build` | **PATCH** (0.0.X) | `build: update dependencies` | Build system changes |
| `ci` | **PATCH** (0.0.X) | `ci: improve release workflow` | CI/CD changes |
| `chore` | **No release** | `chore: update .gitignore` | Maintenance tasks |
| `BREAKING CHANGE` | **MAJOR** (X.0.0) | See below | Breaking changes |

### Examples

#### Feature (Minor Version Bump)
```bash
git commit -m "feat: add SAML2 authentication provider"
# Results in: 1.0.0 → 1.1.0
```

#### Bug Fix (Patch Version Bump)
```bash
git commit -m "fix: resolve PostgreSQL connection hang on blocked queries"
# Results in: 1.1.0 → 1.1.1
```

#### Breaking Change (Major Version Bump)
```bash
git commit -m "feat!: change configuration file format

BREAKING CHANGE: Configuration file now uses YAML v2 format.
Migration required. See docs/migration.md for details."
# Results in: 1.1.1 → 2.0.0
```

Or:
```bash
git commit -m "refactor: redesign API authentication

BREAKING CHANGE: API endpoints now require v2 authentication headers"
# Results in: 1.1.1 → 2.0.0
```

#### With Scope
```bash
git commit -m "feat(auth): add OIDC token refresh"
git commit -m "fix(proxy): handle connection timeouts"
git commit -m "docs(readme): add Docker examples"
```

#### Chore (No Release)
```bash
git commit -m "chore: update .gitignore"
# No version bump, no release
```

## 🚀 Release Process

### Automatic Release (Standard Workflow)

1. **Make your changes** following conventional commits:
   ```bash
   git checkout -b feature/my-feature
   # Make changes
   git add .
   git commit -m "feat: add new awesome feature"
   git push origin feature/my-feature
   ```

2. **Create Pull Request** to `main`
   - Review and merge

3. **Automatic release happens**:
   ```
   Push to main
      ↓
   Semantic Release analyzes commits
      ↓
   Determines version (e.g., 1.2.3)
      ↓
   Creates tag v1.2.3
      ↓
   Updates CHANGELOG.md
      ↓
   Builds binaries (Linux, macOS, Windows)
      ↓
   Creates GitHub Release
      ↓
   Publishes Docker images
      ↓
   Done! 🎉
   ```

### Manual Trigger

You can manually trigger a release:

```bash
# Go to GitHub Actions → Release workflow → Run workflow
```

Or using GitHub CLI:
```bash
gh workflow run release.yml
```

## 📦 What Gets Released

Each automatic release includes:

### Binaries (6 platforms)
- `port-authorizing-linux-amd64`
- `port-authorizing-linux-arm64`
- `port-authorizing-darwin-amd64` (macOS Intel)
- `port-authorizing-darwin-arm64` (macOS Apple Silicon)
- `port-authorizing-windows-amd64.exe`
- SHA256 checksums for all binaries

### Docker Images
- `cohandv/port-authorizing:v1.2.3` (specific version)
- `cohandv/port-authorizing:1.2` (minor version)
- `cohandv/port-authorizing:1` (major version)
- `cohandv/port-authorizing:latest` (latest stable)

### Documentation
- Updated `CHANGELOG.md`
- Generated release notes on GitHub
- Installation instructions

## 🔍 Checking Releases

### View Latest Version
```bash
# CLI
port-authorizing --version

# Docker
docker run --rm cohandv/port-authorizing:latest --version

# GitHub API
curl -s https://api.github.com/repos/cohandv/port-authorizing/releases/latest | jq -r .tag_name
```

### View All Releases
```bash
# GitHub
open https://github.com/cohandv/port-authorizing/releases

# API
curl -s https://api.github.com/repos/cohandv/port-authorizing/releases
```

## 🎯 Version Examples

| Starting Version | Commit Message | New Version | Explanation |
|-----------------|----------------|-------------|-------------|
| `1.0.0` | `feat: add LDAP support` | `1.1.0` | New feature → minor bump |
| `1.1.0` | `fix: resolve timeout` | `1.1.1` | Bug fix → patch bump |
| `1.1.1` | `feat!: new config format` | `2.0.0` | Breaking change → major bump |
| `2.0.0` | `chore: update deps` | `2.0.0` | No release for chore |
| `2.0.0` | `docs: fix typo` | `2.0.1` | Docs → patch bump |

## 🚨 Important Notes

### Multiple Commits
If you push multiple commits, the **highest version bump wins**:
```bash
git commit -m "fix: minor bug"        # Would be 1.0.1
git commit -m "feat: new feature"     # Would be 1.1.0
git push
# Result: Version 1.1.0 (feat wins over fix)
```

### No Releasable Commits
If you only push `chore` commits:
```bash
git commit -m "chore: update README"
git push
# Result: No release created
```

### Force a Patch Release
If you want to force a release for any commit:
```bash
git commit -m "docs: update documentation"
# This creates a patch release even though it's just docs
```

## 🔧 Skipping CI

To skip the CI/release process:
```bash
git commit -m "docs: update readme [skip ci]"
```

## 📊 Release Workflow Details

### Workflow Triggers
- Push to `main` branch
- Manual trigger via GitHub Actions

### Jobs
1. **release** - Analyzes commits and creates tag
2. **build-binaries** - Builds for all platforms
3. **upload-release-assets** - Uploads binaries to GitHub
4. **trigger-docker-build** - Triggers Docker image build

### Required GitHub Secrets
- `GITHUB_TOKEN` - Automatically provided
- `DOCKERHUB_TOKEN` - For Docker Hub publishing

## 🐛 Troubleshooting

### Release Not Created
**Cause**: No commits warrant a release (only `chore` commits)
**Solution**: Use proper commit types (`feat`, `fix`, etc.)

### Version Skipped
**Cause**: `[skip ci]` in commit message
**Solution**: Remove `[skip ci]` from commit message

### Docker Image Not Updated
**Cause**: Docker workflow might be separate
**Solution**: Check `.github/workflows/docker-publish.yml` is triggered

## 🎓 Learn More

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Semantic Release](https://github.com/semantic-release/semantic-release)
- [Keep a Changelog](https://keepachangelog.com/)

## 💡 Tips

1. **Write clear commit messages** - They become your release notes
2. **Use scopes** - Helps identify what changed (e.g., `feat(auth)`, `fix(proxy)`)
3. **One logical change per commit** - Makes versioning more predictable
4. **Test before merging to main** - Every merge can trigger a release
5. **Review CHANGELOG.md** - It's auto-generated from your commits

## 🔄 Migration from Manual Versioning

If you previously used manual tags:

1. **Existing tags are preserved** - Semantic release starts from the last tag
2. **Future releases are automatic** - Just follow conventional commits
3. **Old CHANGELOG.md** - Will be appended to by semantic-release

---

**No more manual versioning! Just write good commit messages and let automation handle the rest.** 🚀
