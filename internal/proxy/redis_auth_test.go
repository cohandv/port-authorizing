package proxy

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// mockConn is a mock network connection for testing
type mockRedisConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockRedisConn(responses ...string) *mockRedisConn {
	buf := bytes.NewBuffer(nil)
	for _, resp := range responses {
		buf.WriteString(resp)
	}
	return &mockRedisConn{
		readBuf:  buf,
		writeBuf: bytes.NewBuffer(nil),
		closed:   false,
	}
}

func (m *mockRedisConn) Read(b []byte) (n int, err error) {
	return m.readBuf.Read(b)
}

func (m *mockRedisConn) Write(b []byte) (n int, err error) {
	return m.writeBuf.Write(b)
}

func (m *mockRedisConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockRedisConn) LocalAddr() net.Addr                { return nil }
func (m *mockRedisConn) RemoteAddr() net.Addr               { return nil }
func (m *mockRedisConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockRedisConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockRedisConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *mockRedisConn) getWrittenData() string {
	return m.writeBuf.String()
}

func TestAuthenticateToBackend_ACL_Success(t *testing.T) {
	// Test Redis 6.0+ ACL authentication with username + password
	conn := newMockRedisConn("+OK\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "myuser", "mypassword")
	if err != nil {
		t.Errorf("authenticateToBackend() with ACL failed: %v", err)
	}
	
	// Verify the AUTH command sent
	written := conn.getWrittenData()
	// Should be: *3\r\n$4\r\nAUTH\r\n$6\r\nmyuser\r\n$10\r\nmypassword\r\n
	expected := "*3\r\n$4\r\nAUTH\r\n$6\r\nmyuser\r\n$10\r\nmypassword\r\n"
	if written != expected {
		t.Errorf("Wrong AUTH command sent.\nGot:  %q\nWant: %q", written, expected)
	}
}

func TestAuthenticateToBackend_Legacy_Success(t *testing.T) {
	// Test Redis < 6.0 legacy authentication with password only
	conn := newMockRedisConn("+OK\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "", "mypassword")
	if err != nil {
		t.Errorf("authenticateToBackend() with legacy auth failed: %v", err)
	}
	
	// Verify the AUTH command sent
	written := conn.getWrittenData()
	// Should be: *2\r\n$4\r\nAUTH\r\n$10\r\nmypassword\r\n
	expected := "*2\r\n$4\r\nAUTH\r\n$10\r\nmypassword\r\n"
	if written != expected {
		t.Errorf("Wrong AUTH command sent.\nGot:  %q\nWant: %q", written, expected)
	}
}

func TestAuthenticateToBackend_ACL_Fallback_WrongNumberOfArgs(t *testing.T) {
	// Test fallback from ACL to legacy when Redis doesn't support ACL
	// First response: error for ACL (username+password)
	// Second response: success for legacy (password-only)
	conn := newMockRedisConn(
		"-ERR wrong number of arguments for 'auth' command\r\n",
		"+OK\r\n",
	)
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "myuser", "mypassword")
	if err != nil {
		t.Errorf("authenticateToBackend() fallback failed: %v", err)
	}
	
	// Verify both AUTH commands were sent
	written := conn.getWrittenData()
	
	// Should contain both ACL and legacy attempts
	if !strings.Contains(written, "*3\r\n$4\r\nAUTH") {
		t.Error("Expected ACL auth attempt not found in written data")
	}
	if !strings.Contains(written, "*2\r\n$4\r\nAUTH") {
		t.Error("Expected legacy auth attempt not found in written data")
	}
}

func TestAuthenticateToBackend_ACL_Fallback_WRONGPASS(t *testing.T) {
	// Test fallback when WRONGPASS is returned (might be legacy Redis)
	conn := newMockRedisConn(
		"-ERR invalid password\r\n",
		"+OK\r\n",
	)
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "myuser", "mypassword")
	if err != nil {
		t.Errorf("authenticateToBackend() fallback on WRONGPASS failed: %v", err)
	}
}

func TestAuthenticateToBackend_ACL_FailNoFallback(t *testing.T) {
	// Test ACL failure that should NOT trigger fallback (e.g., actual wrong credentials)
	conn := newMockRedisConn("-ERR some other auth error\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "myuser", "wrongpassword")
	if err == nil {
		t.Error("Expected authentication to fail with wrong credentials")
	}
	
	if !strings.Contains(err.Error(), "ACL authentication failed") {
		t.Errorf("Expected 'ACL authentication failed' error, got: %v", err)
	}
}

func TestAuthenticateToBackend_Legacy_Failure(t *testing.T) {
	// Test legacy auth failure (no fallback needed)
	conn := newMockRedisConn("-ERR invalid password\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "", "wrongpassword")
	if err == nil {
		t.Error("Expected authentication to fail with wrong password")
	}
	
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("Expected 'authentication failed' in error, got: %v", err)
	}
}

func TestAuthenticateToBackend_BothFailed(t *testing.T) {
	// Test both ACL and legacy auth fail
	conn := newMockRedisConn(
		"-ERR wrong number of arguments for 'auth' command\r\n",
		"-ERR invalid password\r\n",
	)
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	err := proxy.authenticateToBackend(conn, "myuser", "wrongpassword")
	if err == nil {
		t.Error("Expected authentication to fail when both ACL and legacy fail")
	}
	
	if !strings.Contains(err.Error(), "both ACL and legacy authentication failed") {
		t.Errorf("Expected 'both ACL and legacy authentication failed' error, got: %v", err)
	}
}

func TestSendAuthCommand_Success(t *testing.T) {
	conn := newMockRedisConn("+OK\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	authCmd := "*2\r\n$4\r\nAUTH\r\n$8\r\ntestpass\r\n"
	err := proxy.sendAuthCommand(conn, authCmd)
	if err != nil {
		t.Errorf("sendAuthCommand() failed: %v", err)
	}
	
	written := conn.getWrittenData()
	if written != authCmd {
		t.Errorf("Wrong command sent.\nGot:  %q\nWant: %q", written, authCmd)
	}
}

func TestSendAuthCommand_Error(t *testing.T) {
	conn := newMockRedisConn("-ERR Client sent AUTH, but no password is set\r\n")
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	authCmd := "*2\r\n$4\r\nAUTH\r\n$8\r\ntestpass\r\n"
	err := proxy.sendAuthCommand(conn, authCmd)
	if err == nil {
		t.Error("Expected sendAuthCommand to fail with error response")
	}
	
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("Expected 'authentication failed' in error, got: %v", err)
	}
}

func TestSendAuthCommand_ResponseTooLong(t *testing.T) {
	// Create a response that's way too long (over 1024 bytes)
	longResponse := "-ERR " + strings.Repeat("x", 2000) + "\r\n"
	conn := newMockRedisConn(longResponse)
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	authCmd := "*2\r\n$4\r\nAUTH\r\n$4\r\ntest\r\n"
	err := proxy.sendAuthCommand(conn, authCmd)
	if err == nil {
		t.Error("Expected sendAuthCommand to fail with response too long")
	}
	
	if !strings.Contains(err.Error(), "AUTH response too long") {
		t.Errorf("Expected 'AUTH response too long' error, got: %v", err)
	}
}

func TestSendError(t *testing.T) {
	conn := newMockRedisConn()
	
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	proxy.sendError(conn, "Test error message")
	
	written := conn.getWrittenData()
	expected := "-ERR Test error message\r\n"
	if written != expected {
		t.Errorf("Wrong error sent.\nGot:  %q\nWant: %q", written, expected)
	}
}

func TestAuthenticateToBackend_DifferentPasswordLengths(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{
			name:     "short password",
			username: "",
			password: "a",
		},
		{
			name:     "medium password",
			username: "",
			password: "password123",
		},
		{
			name:     "long password",
			username: "",
			password: strings.Repeat("x", 100),
		},
		{
			name:     "ACL with short username",
			username: "u",
			password: "p",
		},
		{
			name:     "ACL with long username",
			username: strings.Repeat("user", 10),
			password: strings.Repeat("pass", 10),
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := newMockRedisConn("+OK\r\n")
			
			proxy := &RedisAuthProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
				},
			}
			
			err := proxy.authenticateToBackend(conn, tt.username, tt.password)
			if err != nil {
				t.Errorf("authenticateToBackend() failed: %v", err)
			}
			
			// Verify command format is correct
			written := conn.getWrittenData()
			
			if tt.username != "" {
				// ACL format: *3\r\n$4\r\nAUTH\r\n$<userlen>\r\n<user>\r\n$<passlen>\r\n<pass>\r\n
				expectedPrefix := fmt.Sprintf("*3\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
					len(tt.username), tt.username, len(tt.password), tt.password)
				if written != expectedPrefix {
					t.Errorf("Wrong ACL command.\nGot:  %q\nWant: %q", written, expectedPrefix)
				}
			} else {
				// Legacy format: *2\r\n$4\r\nAUTH\r\n$<passlen>\r\n<pass>\r\n
				expectedPrefix := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n",
					len(tt.password), tt.password)
				if written != expectedPrefix {
					t.Errorf("Wrong legacy command.\nGot:  %q\nWant: %q", written, expectedPrefix)
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkAuthenticateToBackend_ACL(b *testing.B) {
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := newMockRedisConn("+OK\r\n")
		_ = proxy.authenticateToBackend(conn, "testuser", "testpassword")
	}
}

func BenchmarkAuthenticateToBackend_Legacy(b *testing.B) {
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := newMockRedisConn("+OK\r\n")
		_ = proxy.authenticateToBackend(conn, "", "testpassword")
	}
}

func BenchmarkAuthenticateToBackend_Fallback(b *testing.B) {
	proxy := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
		},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := newMockRedisConn(
			"-ERR wrong number of arguments for 'auth' command\r\n",
			"+OK\r\n",
		)
		_ = proxy.authenticateToBackend(conn, "testuser", "testpassword")
	}
}

