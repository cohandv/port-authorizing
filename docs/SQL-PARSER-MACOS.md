# SQL Parser macOS Compilation Issue

## Overview

The SQL parsing feature uses **`pg_query_go`**, a Go wrapper for PostgreSQL's native parser (`libpg_query`). This library requires **CGO** (C bindings) and has a **known compatibility issue with macOS 15 (Sequoia)**.

## The Issue

On **macOS 15.x (Sequoia)**, compiling `pg_query_go` produces the following error:

```
# github.com/pganalyze/pg_query_go/v5/parser
src_port_snprintf.c:374:1: error: static declaration of 'strchrnul' follows non-static declaration
/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include/_string.h:198:9: note: previous declaration is here
```

This is caused by a **conflict between the PostgreSQL parser's internal function** (`strchrnul`) and the **macOS system headers** introduced in Sequoia.

## Why This Isn't a Problem

### 1. **Server Runs on Linux**
The Port Authorizing API server is designed to run in **Docker** (Alpine Linux) or on **Linux servers**. The SQL parser compiles and works perfectly in these environments.

### 2. **Local Development**
If you need to develop/test locally on macOS, use **Docker** for running tests:

```bash
# Test SQL parser in Docker (Alpine Linux)
docker run --rm -v $(pwd):/app -w /app golang:1.24-alpine sh -c '
  apk add --no-cache build-base git &&
  go test ./internal/security -v -run TestSQLAnalyzer
'
```

### 3. **CI/CD Works Fine**
GitHub Actions runs on **Ubuntu**, so all automated tests pass without issues.

## Workarounds for Local macOS Development

### Option 1: Test in Docker (RECOMMENDED)

Run the full test suite in Docker:

```bash
# Full test suite
make docker-test

# Or manually:
docker run --rm -v $(pwd):/app -w /app golang:1.24-alpine sh -c '
  apk add --no-cache build-base git make &&
  make test-unit
'
```

### Option 2: Test Individual Packages Without SQL Parser

You can test other packages without triggering SQL parser compilation:

```bash
# Test everything except security package
go test ./internal/api ./internal/auth ./internal/audit -v

# Or use build tags to conditionally exclude
go test -tags '!cgo' ./...
```

### Option 3: Switch to Pure-Go Parser (Alternative)

If macOS local testing is critical, consider using a pure-Go SQL parser:

- **`vitessio/vitess/go/vt/sqlparser`** - Vitess SQL parser (MySQL-compatible)
- **`xwb1989/sqlparser`** - Simple Go SQL parser

**Trade-off:** Less accurate PostgreSQL compatibility, but no CGO required.

## Production Deployment

### ✅ Docker (Recommended)

Build and run in Docker - **no issues**:

```bash
# Build Docker image
docker build -t port-authorizing:latest .

# Run server
docker-compose up -d
```

The Dockerfile uses **Alpine Linux**, where `pg_query_go` compiles successfully.

### ✅ Linux Server

Deploy directly on **Ubuntu/Debian/RHEL** - **no issues**:

```bash
# Install build dependencies
apt-get install -y build-essential

# Build server
make build

# Run
./bin/port-authorizing-api -config config.yaml
```

### ❌ macOS Server (Not Recommended)

If you must run the server on macOS 15:

1. Use **Rosetta 2** to run the Linux binary
2. Use Docker Desktop
3. Wait for upstream fix in `pg_query_go`

## Testing Strategy

### Local macOS Development

1. **Edit code** on macOS
2. **Run non-SQL tests** locally:
   ```bash
   go test ./internal/api ./internal/auth -v
   ```
3. **Run SQL parser tests** in Docker:
   ```bash
   make docker-test-security
   ```

### CI/CD (GitHub Actions)

All tests run automatically on **Ubuntu** - no issues:

```yaml
- name: Run tests
  run: go test ./... -v
```

### Pre-Deployment

Before deploying, test the full server in Docker:

```bash
# Build and run full stack
docker-compose up --build

# In another terminal, run integration tests
./test.sh
```

## Timeline for Fix

This is a **known issue** in `pg_query_go`:

- **Reported:** October 2024 (macOS Sequoia release)
- **Upstream:** https://github.com/pganalyze/pg_query_go/issues
- **Workaround:** Use Docker/Linux for testing

## Summary

| Environment | SQL Parser Works? | Solution |
|-------------|-------------------|----------|
| **macOS 15 (Sequoia)** | ❌ No (compile error) | Use Docker for testing |
| **macOS 14 and earlier** | ✅ Yes | Works normally |
| **Linux (Ubuntu/Debian/Alpine)** | ✅ Yes | Works normally |
| **Docker (Alpine)** | ✅ Yes | **RECOMMENDED** |
| **GitHub Actions (Ubuntu)** | ✅ Yes | Works normally |

**Bottom line:** This is a **local development inconvenience**, not a production issue. The API server always runs in Docker/Linux where SQL parsing works perfectly.

## Additional Resources

- **`pg_query_go` Documentation:** https://github.com/pganalyze/pg_query_go
- **PostgreSQL Parser (libpg_query):** https://github.com/pganalyze/libpg_query
- **Alternative Parsers:** https://github.com/xwb1989/sqlparser

