# GitHub Actions - Docker Hub Publishing

## Overview

The project includes a GitHub Actions workflow that automatically builds and publishes Docker images to Docker Hub when code is pushed to the `main` branch or when version tags are created.

## Workflow: `docker-publish.yml`

Location: `.github/workflows/docker-publish.yml`

### Triggers

The workflow runs on:
- **Push to main branch**: Builds and pushes `latest` tag
- **Version tags** (v*): Builds and pushes versioned tags
- **Pull requests**: Builds but doesn't push (validation only)

### What It Does

1. **Checks out code** with full git history
2. **Sets up Go** (version 1.21)
3. **Determines version** from git tags or uses "dev"
4. **Sets up Docker Buildx** for multi-architecture builds
5. **Logs into Docker Hub** (using secrets)
6. **Builds Docker image** for multiple architectures
7. **Pushes to Docker Hub** (if not a PR)
8. **Updates Docker Hub description** from README

### Multi-Architecture Support

Images are built for:
- `linux/amd64` (x86_64)
- `linux/arm64` (ARM64 / Apple Silicon)

### Image Tags

The workflow creates multiple tags:

| Git Action | Docker Tags |
|------------|-------------|
| Push to `main` | `latest` |
| Tag `v1.2.3` | `v1.2.3`, `1.2`, `1`, `latest` |
| PR #123 | `pr-123` (not pushed) |

## Setup Instructions

### 1. Create Docker Hub Account

If you don't have one:
1. Go to https://hub.docker.com/
2. Sign up for a free account
3. Create a repository: `cohandv/port-authorizing`

### 2. Generate Docker Hub Access Token

1. Log into Docker Hub
2. Go to **Account Settings** → **Security**
3. Click **New Access Token**
4. Name: `github-actions`
5. Permissions: **Read, Write, Delete**
6. Copy the token (you won't see it again!)

### 3. Add Secret to GitHub Repository

1. Go to your GitHub repository
2. Click **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**
4. Name: `DOCKERHUB_TOKEN`
5. Value: Paste the Docker Hub access token
6. Click **Add secret**

### 4. Verify Workflow

1. Make a commit and push to `main`:
   ```bash
   git add .
   git commit -m "test: trigger docker build"
   git push origin main
   ```

2. Go to **Actions** tab on GitHub
3. Watch the workflow run
4. Check Docker Hub for the new image

## Creating a Release

To create a versioned release:

```bash
# Tag the release
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

This will:
1. Trigger the workflow
2. Build the image with version info
3. Push multiple tags:
   - `cohandv/port-authorizing:v1.0.0`
   - `cohandv/port-authorizing:1.0`
   - `cohandv/port-authorizing:1`
   - `cohandv/port-authorizing:latest`

## Using the Published Image

### Pull Latest

```bash
docker pull cohandv/port-authorizing:latest
```

### Pull Specific Version

```bash
docker pull cohandv/port-authorizing:v1.0.0
```

### Run

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  --name port-authorizing \
  cohandv/port-authorizing:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  port-authorizing:
    image: cohandv/port-authorizing:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
    restart: unless-stopped
```

## Workflow Configuration

### Environment Variables

```yaml
env:
  DOCKER_HUB_USERNAME: cohandv
  IMAGE_NAME: cohandv/port-authorizing
```

To use your own Docker Hub account, update these values.

### Build Arguments

The workflow passes these build args to Docker:

```yaml
build-args: |
  VERSION=${{ steps.get_version.outputs.VERSION }}
  BUILD_TIME=${{ github.event.head_commit.timestamp }}
  GIT_COMMIT=${{ github.sha }}
```

These are embedded in the binary and visible via `port-authorizing version`.

### Caching

The workflow uses GitHub Actions cache to speed up builds:

```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

This caches Docker layers between runs, making builds faster.

## Troubleshooting

### Build Fails

1. Check the **Actions** tab for logs
2. Common issues:
   - Go compilation errors
   - Missing dependencies
   - Docker build failures

### Push Fails

1. Verify `DOCKERHUB_TOKEN` secret is set
2. Check token permissions (needs Write access)
3. Verify repository name matches: `cohandv/port-authorizing`

### Image Not Appearing

1. Check if push succeeded in Actions logs
2. Verify you're logged into correct Docker Hub account
3. Repository might be private (check settings)

### Multi-arch Build Issues

If arm64 build fails:
1. Buildx might not be set up correctly
2. Some dependencies might not support arm64
3. Check Actions logs for specific errors

## Advanced Configuration

### Skip CI

To skip the workflow:

```bash
git commit -m "docs: update README [skip ci]"
```

### Manual Trigger

Add to workflow:

```yaml
on:
  workflow_dispatch:  # Allows manual trigger from GitHub UI
```

### Different Registry

To use a different Docker registry (e.g., GitHub Container Registry):

```yaml
- name: Log in to GitHub Container Registry
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

Update image name:
```yaml
IMAGE_NAME: ghcr.io/${{ github.repository_owner }}/port-authorizing
```

## Monitoring

### GitHub Actions Dashboard

- **Actions** tab shows all workflow runs
- Click a run to see detailed logs
- Failed runs are highlighted in red

### Docker Hub

- View images at: https://hub.docker.com/r/cohandv/port-authorizing
- Check pull statistics
- View tags and sizes

### Notifications

Configure GitHub to notify you of workflow failures:
1. **Settings** → **Notifications**
2. Enable **Actions** notifications

## Cost

- **GitHub Actions**: Free for public repositories
- **Docker Hub**:
  - Free tier: Unlimited public repositories
  - Pull rate limit: 200/6hrs (anonymous), unlimited (authenticated)

## Security Best Practices

1. ✅ Use access tokens, not passwords
2. ✅ Limit token permissions (Read + Write only)
3. ✅ Rotate tokens periodically
4. ✅ Never commit tokens to git
5. ✅ Use GitHub Secrets for sensitive data
6. ✅ Enable 2FA on Docker Hub
7. ✅ Review workflow permissions

## Related Documentation

- [Building Guide](building.md)
- [Docker Testing](docker-testing.md)
- [Deployment Guide](../guides/getting-started.md)

