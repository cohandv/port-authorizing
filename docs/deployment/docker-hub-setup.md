# Docker Hub Automatic Publishing Setup

## Overview

The GitHub Actions workflow automatically:
1. Builds Docker images for `linux/amd64` and `linux/arm64`
2. Pushes to `cohandv/port-authorizing` on Docker Hub
3. Updates the Docker Hub README from `DOCKER_HUB_README.md`

## Setup Steps

### 1. Create Docker Hub Access Token

1. Log into Docker Hub: https://hub.docker.com/
2. Go to **Account Settings** → **Security** → **Access Tokens**
3. Click **New Access Token**
4. Settings:
   - **Description**: `github-actions`
   - **Access permissions**: `Read, Write, Delete`
5. Click **Generate**
6. **Copy the token** (you won't see it again!)

### 2. Add Secret to GitHub

1. Go to your GitHub repository
2. Navigate to **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**
4. Add the secret:
   - **Name**: `DOCKERHUB_TOKEN`
   - **Value**: Paste the Docker Hub access token
5. Click **Add secret**

### 3. Verify Workflow Configuration

The workflow is already configured in `.github/workflows/docker-publish.yml`:

```yaml
env:
  DOCKER_HUB_USERNAME: cohandv
  IMAGE_NAME: cohandv/port-authorizing

- name: Update Docker Hub description
  if: github.event_name != 'pull_request' && github.ref == 'refs/heads/main'
  uses: peter-evans/dockerhub-description@v4
  with:
    username: ${{ env.DOCKER_HUB_USERNAME }}
    password: ${{ secrets.DOCKERHUB_TOKEN }}
    repository: ${{ env.IMAGE_NAME }}
    short-description: "Secure database access proxy with authentication and authorization"
    readme-filepath: ./DOCKER_HUB_README.md
```

### 4. Commit and Push

```bash
# Stage all changes
git add .

# Commit
git commit -m "feat: unified binary, organized docs, and Docker Hub automation"

# Push to main
git push origin main
```

### 5. Watch the Workflow

1. Go to **Actions** tab on GitHub
2. You'll see a new workflow run
3. Click on it to watch progress
4. It will:
   - Build the Docker image
   - Push to Docker Hub (`latest` tag)
   - Update the README on Docker Hub

### 6. Verify on Docker Hub

After the workflow completes:
1. Go to https://hub.docker.com/r/cohandv/port-authorizing
2. Check the **Overview** tab - README should be updated
3. Check the **Tags** tab - you should see `latest`

## Creating Versioned Releases

To create a versioned release:

```bash
# Tag the release
git tag -a v2.0.0 -m "Release 2.0.0 - Unified binary"

# Push the tag
git push origin v2.0.0
```

This will trigger the workflow and create tags:
- `cohandv/port-authorizing:v2.0.0`
- `cohandv/port-authorizing:2.0`
- `cohandv/port-authorizing:2`
- `cohandv/port-authorizing:latest`

## Workflow Triggers

The workflow runs on:

| Event | Trigger | Tags Created |
|-------|---------|--------------|
| Push to `main` | Automatic | `latest`, `main` |
| Tag `v1.2.3` | When tag is pushed | `v1.2.3`, `1.2`, `1`, `latest` |
| Pull Request | Builds but doesn't push | None (validation only) |

## What Gets Published

### Docker Image

- **Repository**: `cohandv/port-authorizing`
- **Architectures**: `linux/amd64`, `linux/arm64`
- **Size**: ~18MB (Alpine-based)
- **Base**: `alpine:latest`
- **Go Version**: 1.24

### Docker Hub README

- **Source**: `DOCKER_HUB_README.md` in repository
- **Updated**: Automatically on every push to `main`
- **Format**: Markdown with examples and usage

## Troubleshooting

### Workflow Fails at "Log in to Docker Hub"

**Error**: `Error: Cannot perform an interactive login from a non TTY device`

**Solution**: Make sure `DOCKERHUB_TOKEN` secret is set correctly in GitHub

### Workflow Fails at "Update Docker Hub description"

**Error**: `Error: 401 Unauthorized`

**Solutions**:
1. Check token has correct permissions (Read + Write)
2. Verify token hasn't expired
3. Regenerate token if needed

### Image Not Appearing on Docker Hub

**Check**:
1. Workflow completed successfully?
2. Repository exists on Docker Hub?
3. Repository is public? (if expecting public access)

### README Not Updating

**Check**:
1. Workflow ran for `main` branch (not PR)
2. `DOCKERHUB_TOKEN` has write permissions
3. File path is correct: `./DOCKER_HUB_README.md`

## Manual Updates

If you need to update the README manually:

### Using Script

```bash
export DOCKERHUB_TOKEN='your-token'
./scripts/update-dockerhub-readme.sh
```

### Using Docker Hub UI

1. Go to https://hub.docker.com/repository/docker/cohandv/port-authorizing/general
2. Click **Edit** in Description section
3. Copy content from `DOCKER_HUB_README.md`
4. Paste and save

## Security Best Practices

1. ✅ Use access tokens, not passwords
2. ✅ Limit token permissions (only what's needed)
3. ✅ Use GitHub Secrets (never commit tokens)
4. ✅ Rotate tokens periodically
5. ✅ Enable 2FA on Docker Hub
6. ✅ Review workflow permissions regularly

## Monitoring

### GitHub Actions

- View runs: https://github.com/yourusername/port-authorizing/actions
- Enable email notifications for failed runs
- Check logs for errors

### Docker Hub

- View pulls: https://hub.docker.com/r/cohandv/port-authorizing
- Monitor pull rate limits
- Check image scan results (if available)

## Rate Limits

### Docker Hub

- **Anonymous**: 100 pulls per 6 hours
- **Free account**: 200 pulls per 6 hours
- **Authenticated**: Unlimited for public images

### GitHub Actions

- **Public repos**: Unlimited minutes
- **Private repos**: 2000 minutes/month (free tier)

## Next Steps

After setup:

1. **Test the workflow**: Push to main and watch it run
2. **Create a release**: Tag and push a version
3. **Verify images work**: Pull and test
4. **Monitor usage**: Check Docker Hub analytics

## Support

If you encounter issues:

1. Check [GitHub Actions docs](https://docs.github.com/en/actions)
2. Check [Docker Hub docs](https://docs.docker.com/docker-hub/)
3. Review workflow logs in GitHub Actions tab
4. Open an issue in the repository

