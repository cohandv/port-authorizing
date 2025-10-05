# Testing Guide

This document describes the testing strategy and how to run tests for Port Authorizing.

## Test Structure

The project follows Go's standard testing conventions with tests located alongside the code they test using the `_test.go` suffix.

### Test Coverage by Package

| Package | Coverage | Description |
|---------|----------|-------------|
| `internal/authorization` | 90.3% | Authorization logic, role-based access control, and policy matching |
| `internal/config` | 76.5% | Configuration loading and validation |
| `internal/audit` | 66.7% | Audit logging functionality |
| `internal/security` | 15.0% | Query validation and whitelist enforcement |
| `internal/auth` | 7.4% | Authentication providers (local, OIDC, SAML2, LDAP) |
| **Total** | **~7.6%** | **Overall coverage** |

*Note: API, CLI, proxy, and server packages currently have integration tests but no unit tests.*

## Running Tests

### Quick Test Commands

```bash
# Run all tests
make test

# Run unit tests only (internal packages)
make test-unit

# Run tests with verbose output
make test-verbose

# Generate coverage report (HTML)
make test-coverage
```

### Manual Test Commands

```bash
# Run all tests with coverage
go test ./... -cover

# Run specific package tests
go test ./internal/auth -v
go test ./internal/authorization -v
go test ./internal/security -v

# Run with coverage profile
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Run specific test
go test -run TestLocalProvider_Authenticate ./internal/auth -v

# Run benchmarks
go test -bench=. ./internal/security
go test -bench=. ./internal/authorization
```

## Test Packages

### 1. Authentication Tests (`internal/auth`)

**File:** `internal/auth/local_test.go`

Tests the local authentication provider:
- Valid/invalid credentials
- Missing username/password
- User roles assignment
- Multiple users support

**Key Tests:**
- `TestLocalProvider_Authenticate` - Comprehensive credential validation
- `TestNewLocalProvider` - Provider initialization
- `TestLocalProvider_Name/Type` - Provider metadata

### 2. Authorization Tests (`internal/authorization`)

**File:** `internal/authorization/authz_test.go`

Tests role-based access control and policy matching:
- Connection access based on roles
- Tag-based policy matching (`any` vs `all`)
- Whitelist aggregation from multiple policies
- Legacy whitelist support

**Key Tests:**
- `TestAuthorizer_CanAccessConnection` - 10 test cases covering various role/tag scenarios
- `TestAuthorizer_GetWhitelistForConnection` - Whitelist merging logic
- `TestAuthorizer_ListAccessibleConnections` - User connection listing
- `TestAuthorizer_ValidatePattern` - Query pattern validation

**Benchmark Tests:**
- `BenchmarkCanAccessConnection` - Authorization performance
- `BenchmarkGetWhitelistForConnection` - Whitelist lookup performance

### 3. Security Tests (`internal/security`)

**File:** `internal/security/whitelist_test.go`

Tests query validation against whitelist patterns:
- SELECT, INSERT, UPDATE, DELETE, DROP statements
- Case sensitivity
- Multiple patterns
- Invalid regex handling
- HTTP method validation (for HTTP proxy)

**Key Tests:**
- `TestValidateQuery` - 20 test cases for various query types
- `TestValidateQuery_InvalidRegex` - Error handling

**Benchmark Tests:**
- `BenchmarkValidateQuery_SinglePattern`
- `BenchmarkValidateQuery_MultiplePatterns`
- `BenchmarkValidateQuery_NoMatch`

### 4. Configuration Tests (`internal/config`)

**File:** `internal/config/config_test.go`

Tests configuration loading and validation:
- YAML parsing
- Duration parsing (1h, 24h)
- Connection and policy structures
- Invalid configuration handling

**Key Tests:**
- `TestLoadConfig` - Valid configuration loading
- `TestLoadConfig_NonExistentFile` - Error handling
- `TestLoadConfig_InvalidYAML` - Malformed input handling
- `TestRolePolicy_Validation` - Policy structure validation

### 5. Audit Logging Tests (`internal/audit`)

**File:** `internal/audit/logger_test.go`

Tests audit log functionality:
- JSON log formatting
- Multiple log entries
- Concurrent logging
- Empty/nil metadata handling

**Key Tests:**
- `TestLog` - Basic logging functionality
- `TestLog_MultipleEntries` - Multiple entries verification
- `TestLog_EmptyDetails` - Nil metadata handling
- `TestLog_NonExistentDirectory` - Error resilience

**Benchmark Tests:**
- `BenchmarkLog` - Logging performance

## Test Data

Tests use temporary files and in-memory data structures to avoid dependencies on external resources. The `testing` package's `os.CreateTemp()` is used for file-based tests.

### Example Test Configuration

```yaml
server:
  port: 8080
  max_connection_duration: 1h

auth:
  jwt_secret: "test-secret"
  token_expiry: 24h
  users:
    - username: admin
      password: admin123
      roles: [admin]

connections:
  - name: test-db
    type: postgres
    host: localhost
    port: 5432
    tags: [env:test]

policies:
  - name: admin-all
    roles: [admin]
    tags: [env:test]
    whitelist: [".*"]
```

## Integration Tests

Integration tests are performed using Docker Compose and the `test.sh` script:

```bash
# Run end-to-end tests
make test-e2e

# Or directly
./test.sh
```

These tests verify:
- Full authentication flows (local, OIDC, SAML2, LDAP)
- Proxy functionality (HTTP, PostgreSQL, TCP)
- Connection management
- Query logging and blocking
- Audit logging

## Continuous Integration

Tests are automatically run on:
- Every pull request
- Pushes to `main` branch
- Release builds

See `.github/workflows/release.yml` for CI configuration.

## Writing New Tests

### Test File Naming

- Unit tests: `<package>_test.go` in the same directory as the code
- Test data: Use `testdata/` subdirectory if needed

### Test Structure

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:    "descriptive test case name",
            input:   ...,
            want:    ...,
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionUnderTest(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("FunctionUnderTest() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("FunctionUnderTest() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Benchmark Tests

```go
func BenchmarkFunctionName(b *testing.B) {
    // Setup
    input := setupInput()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        FunctionUnderTest(input)
    }
}
```

## Code Coverage Goals

- **Critical packages** (auth, authorization, security): Target 80%+
- **Configuration and utilities**: Target 70%+
- **API/CLI handlers**: Target 50%+ (integration tests cover most scenarios)
- **Overall project**: Target 50%+

Current overall coverage: ~7.6% (unit tests only)

## Test Maintenance

- Keep tests simple and focused
- Use table-driven tests for multiple scenarios
- Mock external dependencies
- Clean up temporary resources (files, connections)
- Update tests when changing functionality
- Add tests for bug fixes

## Future Test Improvements

- [ ] Add API handler unit tests
- [ ] Add CLI command tests
- [ ] Add PostgreSQL proxy tests
- [ ] Add HTTP proxy tests
- [ ] Add connection manager tests
- [ ] Increase overall coverage to 50%+
- [ ] Add property-based testing for authorization logic
- [ ] Add fuzz testing for query validation
- [ ] Add load/stress tests for concurrent connections

## Troubleshooting

### Tests Hanging

If tests hang, check for:
- Unclosed connections or files
- Infinite loops in proxy logic
- Deadlocks in concurrent code

### Flaky Tests

If tests are flaky:
- Check for race conditions (run with `-race` flag)
- Verify timing assumptions (avoid tight timing constraints)
- Ensure proper cleanup in `defer` statements

### Coverage Not Updating

```bash
# Clean coverage cache
go clean -testcache

# Regenerate coverage
make test-coverage
```

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Table Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Go Coverage](https://go.dev/blog/cover)

