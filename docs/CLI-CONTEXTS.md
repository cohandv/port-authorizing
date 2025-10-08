# CLI Contexts - Managing Multiple API Servers

## Overview

The CLI now supports **multiple contexts** (like kubectl), allowing you to manage connections to different API servers (production, staging, local, etc.) with seamless switching.

## What Changed

### Before (Single API)
```bash
# Had to manually specify API URL every time
./port-authorizing login -u admin -p pass --api-url http://prod.example.com
./port-authorizing list --api-url http://prod.example.com

# Or it defaulted to localhost:8080
./port-authorizing login -u admin -p pass  # Always localhost
```

### After (Multiple Contexts)
```bash
# Login creates a context automatically
./port-authorizing login -u admin -p pass --api-url https://prod.example.com

# Subsequent commands use the saved API URL!
./port-authorizing list  # Uses https://prod.example.com automatically!
./port-authorizing connect mydb -l 5432  # Uses prod API
```

## Quick Start

### 1. Login to Different Environments

```bash
# Login to production (creates "default" context)
./port-authorizing login -u admin -p prod123 \
  --api-url https://api.prod.example.com

# Login to staging (creates "staging" context)
./port-authorizing login -u admin -p stage123 \
  --api-url https://api.staging.example.com \
  --context staging

# Login to local (creates "local" context)
./port-authorizing login -u admin -p local123 \
  --context local
  # API URL defaults to http://localhost:8080
```

### 2. List Your Contexts

```bash
$ ./port-authorizing context list

Available Contexts:
===================
CURRENT   NAME      API URL                          AUTHENTICATED
-------   ----      -------                          -------------
*         default   https://api.prod.example.com     Yes
          staging   https://api.staging.example.com  Yes
          local     http://localhost:8080            Yes
```

### 3. Switch Contexts

```bash
# Switch to staging
$ ./port-authorizing context use staging
✓ Switched to context 'staging'

# Now all commands use staging API
$ ./port-authorizing list  # Lists staging connections
```

### 4. Check Current Context

```bash
$ ./port-authorizing context current
Current context: staging
API URL: https://api.staging.example.com
Status: Authenticated
```

## All Context Commands

| Command | Description |
|---------|-------------|
| `context list` | Show all contexts |
| `context current` | Show current context |
| `context use <name>` | Switch to a different context |
| `context rename <old> <new>` | Rename a context |
| `context delete <name>` | Delete a context |

## Configuration File

Contexts are stored in `~/.port-auth/config.json`:

```json
{
  "contexts": [
    {
      "name": "production",
      "api_url": "https://api.prod.example.com",
      "token": "eyJhbGciOiJIUzI1NiIs..."
    },
    {
      "name": "staging",
      "api_url": "https://api.staging.example.com",
      "token": "eyJhbGciOiJIUzI1NiIs..."
    }
  ],
  "current_context": "production"
}
```

## Legacy Compatibility

The CLI automatically migrates old config format:

**Old format (still supported):**
```json
{
  "api_url": "http://localhost:8080",
  "token": "eyJhbG..."
}
```

**Auto-migrated to:**
```json
{
  "contexts": [
    {
      "name": "default",
      "api_url": "http://localhost:8080",
      "token": "eyJhbG..."
    }
  ],
  "current_context": "default"
}
```

## Common Workflows

### Production + Staging Setup

```bash
# Setup production
./port-authorizing login -u admin -p prod-pass \
  --api-url https://api.prod.example.com \
  --context production

# Setup staging
./port-authorizing login -u admin -p stage-pass \
  --api-url https://api.staging.example.com \
  --context staging

# Work on staging
./port-authorizing context use staging
./port-authorizing connect test-db -l 5433

# Quick check on production
./port-authorizing context use production
./port-authorizing list
```

### Multiple Teams/Projects

```bash
# Team Alpha project
./port-authorizing login -u user@alpha.com -p pass \
  --api-url https://api.team-alpha.com \
  --context team-alpha

# Team Beta project
./port-authorizing login -u user@beta.com -p pass \
  --api-url https://api.team-beta.com \
  --context team-beta

# Switch between teams
./port-authorizing context use team-alpha
./port-authorizing context use team-beta
```

### Local Development

```bash
# Default local setup
./port-authorizing login -u admin -p admin123

# Automatically uses http://localhost:8080
# No need to specify --api-url!
```

## Override API URL

You can still override the API URL from command line:

```bash
# Context says staging, but override to production
./port-authorizing list --api-url https://api.prod.example.com
```

## Deleting Contexts

```bash
# Delete old context
$ ./port-authorizing context delete old-staging
✓ Deleted context 'old-staging'

# Cannot delete current context
$ ./port-authorizing context delete production
Error: cannot delete current context. Switch to another context first

# Switch first, then delete
$ ./port-authorizing context use staging
$ ./port-authorizing context delete production
✓ Deleted context 'production'
```

## Renaming Contexts

```bash
# Rename for clarity
$ ./port-authorizing context rename default prod
✓ Renamed context 'default' to 'prod'

$ ./port-authorizing context rename local dev
✓ Renamed context 'local' to 'dev'
```

## Benefits

✅ **No more `--api-url` flags everywhere**
✅ **Switch between environments instantly**
✅ **Each context remembers its own token**
✅ **Clear visibility of all configured servers**
✅ **Compatible with existing workflows**
✅ **Works like kubectl contexts**

## Troubleshooting

### "No contexts configured"

```bash
$ ./port-authorizing list
Error: no contexts configured. Please run 'login' first
```

**Solution:** Run `login` to create your first context

### "Context not found"

```bash
$ ./port-authorizing context use nonexistent
Error: context 'nonexistent' not found
```

**Solution:** Run `context list` to see available contexts

### "Not authenticated"

```bash
$ ./port-authorizing context list
CURRENT   NAME      API URL                     AUTHENTICATED
-------   ----      -------                     -------------
*         prod      https://api.prod.example.com  No
```

**Solution:** Token expired, run `login` again:
```bash
./port-authorizing login -u admin -p pass
```

## Migration Guide

If you have existing scripts using `--api-url`:

**Before:**
```bash
#!/bin/bash
API_URL="https://api.prod.example.com"
./port-authorizing login -u admin -p $PASS --api-url $API_URL
./port-authorizing list --api-url $API_URL
./port-authorizing connect db -l 5432 --api-url $API_URL
```

**After (recommended):**
```bash
#!/bin/bash
# Login once with context
./port-authorizing login -u admin -p $PASS \
  --api-url https://api.prod.example.com \
  --context production

# Use context for all commands
./port-authorizing context use production
./port-authorizing list
./port-authorizing connect db -l 5432
```

**After (alternate - keep old style):**
```bash
#!/bin/bash
# Old style still works!
API_URL="https://api.prod.example.com"
./port-authorizing login -u admin -p $PASS --api-url $API_URL
./port-authorizing list --api-url $API_URL  # Override still works
./port-authorizing connect db -l 5432 --api-url $API_URL
```

## Summary

**New CLI context system solves both original issues:**

1. ✅ **API URL is now saved and reused** - no more repetitive `--api-url` flags
2. ✅ **Multiple APIs supported** - kubectl-style contexts for different environments

**The CLI is now much more user-friendly for teams managing multiple environments!** 🚀

