# Docker Hub Automatic Publishing - Setup Checklist

## ✅ Pre-Setup (Already Done)

- [x] Created `DOCKER_HUB_README.md` with usage examples
- [x] Updated Dockerfile to use Go 1.24
- [x] Created GitHub Actions workflow (`.github/workflows/docker-publish.yml`)
- [x] Configured workflow to update Docker Hub README automatically
- [x] Built and tested Docker image locally

## 📋 Setup Steps (You Need To Do)

### Step 1: Create Docker Hub Access Token

1. **Go to Docker Hub**: https://hub.docker.com/settings/security
2. **Click**: "New Access Token"
3. **Fill in**:
   - Description: `github-actions`
   - Access permissions: ☑️ Read, Write, Delete
4. **Click**: "Generate"
5. **Copy the token** (you won't see it again!)
   ```
   Example: dckr_pat_abc123def456...
   ```

### Step 2: Add Secret to GitHub Repository

1. **Go to your GitHub repo**: https://github.com/yourusername/port-authorizing
2. **Navigate to**: Settings → Secrets and variables → Actions
3. **Click**: "New repository secret"
4. **Fill in**:
   - Name: `DOCKERHUB_TOKEN`
   - Secret: Paste the Docker Hub token
5. **Click**: "Add secret"

### Step 3: Commit and Push

```bash
# Check status
git status

# Stage all changes
git add .

# Commit
git commit -m "feat: unified binary, organized docs, and Docker Hub automation"

# Push to main (this triggers the workflow!)
git push origin main
```

### Step 4: Watch the Magic ✨

1. **Go to Actions tab**: https://github.com/yourusername/port-authorizing/actions
2. **Click on the running workflow** to see real-time logs
3. **Wait for completion** (~5-10 minutes)
4. **Check Docker Hub**: https://hub.docker.com/r/cohandv/port-authorizing

## 🎯 Expected Results

After the workflow completes:

### On Docker Hub
- ✅ New image: `cohandv/port-authorizing:latest`
- ✅ README updated with full documentation
- ✅ Multi-arch support: `linux/amd64`, `linux/arm64`
- ✅ Image size: ~18MB

### Test the Image
```bash
# Pull the image
docker pull cohandv/port-authorizing:latest

# Test it works
docker run --rm cohandv/port-authorizing:latest version

# Should show:
# Port Authorizing vX.X.X
# Build Time: 2025-10-05_XX:XX:XX
# Git Commit: xxxxxxx
```

## 🏷️ Creating Releases (Optional)

To create a versioned release:

```bash
# Tag the release
git tag -a v2.0.0 -m "Release 2.0.0 - Unified binary with OIDC support"

# Push the tag (triggers workflow again)
git push origin v2.0.0
```

This creates multiple tags:
- `cohandv/port-authorizing:v2.0.0` (exact version)
- `cohandv/port-authorizing:2.0` (minor version)
- `cohandv/port-authorizing:2` (major version)
- `cohandv/port-authorizing:latest` (latest stable)

## 🔍 Troubleshooting

### Workflow fails at "Log in to Docker Hub"
**Problem**: Secret not set or incorrect

**Solution**:
1. Verify `DOCKERHUB_TOKEN` secret exists in GitHub
2. Check token hasn't expired
3. Regenerate token if needed

### Workflow succeeds but README not updated
**Problem**: Token permissions insufficient

**Solution**:
1. Regenerate token with "Read, Write, Delete" permissions
2. Update `DOCKERHUB_TOKEN` secret in GitHub
3. Re-run the workflow

### Build fails with Go version error
**Problem**: Go version mismatch (already fixed!)

**Solution**: Dockerfile now uses `golang:1.24-alpine` ✅

## 📚 Documentation

- **Full Setup Guide**: [docker-hub-setup.md](docker-hub-setup.md)
- **GitHub Actions Guide**: [github-actions.md](github-actions.md)
- **Docker Hub README**: [../../DOCKER_HUB_README.md](../../DOCKER_HUB_README.md)

## 🎉 Success Indicators

You'll know everything works when:
- ✅ Workflow shows green checkmark in Actions tab
- ✅ Docker Hub shows new image with today's date
- ✅ README on Docker Hub matches `DOCKER_HUB_README.md`
- ✅ You can pull and run: `docker pull cohandv/port-authorizing:latest`
- ✅ Image works: `docker run --rm cohandv/port-authorizing:latest version`

## 🚀 Next Steps

After setup:
1. **Test locally**: `docker pull cohandv/port-authorizing:latest`
2. **Update your deployments** to use the new image
3. **Create a release tag** for version tracking
4. **Monitor Docker Hub** for pull statistics
5. **Share with your team!**

---

Need help? Check [docker-hub-setup.md](docker-hub-setup.md) for detailed troubleshooting.

