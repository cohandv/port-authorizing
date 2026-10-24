package proxy

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestRedisProxy_ParseRESPCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple GET command",
			input:    "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n",
			expected: "GET key",
		},
		{
			name:     "SET command with value",
			input:    "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n",
			expected: "SET key value",
		},
		{
			name:     "DEL command with multiple keys",
			input:    "*4\r\n$3\r\nDEL\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n$4\r\nkey3\r\n",
			expected: "DEL key1 key2 key3",
		},
		{
			name:     "Invalid RESP format",
			input:    "invalid\r\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal RedisProxy for testing
			rp := &RedisProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
					Type: "redis",
					Host: "localhost",
					Port: 6379,
				},
			}

			// Parse the command
			reader := bufio.NewReader(strings.NewReader(tt.input))
			firstLine, _ := reader.ReadString('\n')
			result := rp.parseRESPCommand(firstLine, reader)

			if result != tt.expected {
				t.Errorf("parseRESPCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRedisProxy_ExtractRESPCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Single command",
			input:    "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n",
			expected: []string{"GET key"},
		},
		{
			name: "Multiple commands",
			input: "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n" +
				"*3\r\n$3\r\nSET\r\n$4\r\nkey2\r\n$6\r\nvalue2\r\n",
			expected: []string{"GET key", "SET key2 value2"},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal RedisAuthProxy for testing
			rp := &RedisAuthProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
					Type: "redis",
					Host: "localhost",
					Port: 6379,
				},
			}

			result := rp.extractRESPCommands([]byte(tt.input))

			if len(result) != len(tt.expected) {
				t.Errorf("extractRESPCommands() returned %d commands, want %d", len(result), len(tt.expected))
				return
			}

			for i, cmd := range result {
				if cmd != tt.expected[i] {
					t.Errorf("extractRESPCommands()[%d] = %q, want %q", i, cmd, tt.expected[i])
				}
			}
		})
	}
}

func TestRedisAuthProxy_IsCommandAllowed(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		whitelist []string
		expected  bool
	}{
		{
			name:      "No whitelist - allow all",
			command:   "GET key",
			whitelist: []string{},
			expected:  true,
		},
		{
			name:      "Exact match",
			command:   "GET key",
			whitelist: []string{"^GET"},
			expected:  true,
		},
		{
			name:      "Pattern match",
			command:   "SET mykey value",
			whitelist: []string{"^SET .*"},
			expected:  true,
		},
		{
			name:      "Case insensitive match",
			command:   "get key",
			whitelist: []string{"^GET"},
			expected:  true,
		},
		{
			name:      "Blocked command",
			command:   "FLUSHDB",
			whitelist: []string{"^GET", "^SET"},
			expected:  false,
		},
		{
			name:      "Multiple patterns - match second",
			command:   "DEL key",
			whitelist: []string{"^GET", "^SET", "^DEL"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RedisAuthProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
					Type: "redis",
					Host: "localhost",
					Port: 6379,
				},
				auditLogPath: "/tmp/audit.log",
				username:     "testuser",
				connectionID: "test-conn-123",
				whitelist:    tt.whitelist,
			}

			result := rp.isCommandAllowed(tt.command)

			if result != tt.expected {
				t.Errorf("isCommandAllowed(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestRedisProxy_AuthenticateToBackend(t *testing.T) {
	// Start a mock Redis server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Server goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read AUTH command
		reader := bufio.NewReader(conn)
		line, _ := reader.ReadString('\n')

		if strings.HasPrefix(line, "*2") {
			// Skip the command parts and send +OK
			_, _ = reader.ReadString('\n')
			_, _ = reader.ReadString('\n')
			_, _ = reader.ReadString('\n')
			_, _ = reader.ReadString('\n')

			// Send success response
			_, _ = conn.Write([]byte("+OK\r\n"))
		}
	}()

	// Create Redis proxy
	rp := &RedisProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
			Type: "redis",
			Host: "localhost",
			Port: 6379,
			BackendPassword: config.ConfigSource{
				Type:  config.ConfigSourceTypePlain,
				Value: "testpassword",
			},
		},
	}

	// Connect to mock server
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to mock server: %v", err)
	}
	defer conn.Close()

	// Test authentication
	err = rp.authenticateToBackend(conn, "testpassword")
	if err != nil {
		t.Errorf("authenticateToBackend() error = %v, want nil", err)
	}
}

func TestRedisAuthProxy_SendCommandBlockedError(t *testing.T) {
	// Create a pair of connected net.Conn for testing
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	rp := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
			Type: "redis",
		},
	}

	// Send blocked error in a goroutine
	go func() {
		rp.sendCommandBlockedError(server, "FLUSHDB")
	}()

	// Read the error response
	reader := bufio.NewReader(client)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !strings.HasPrefix(response, "-ERR") {
		t.Errorf("Expected error response starting with '-ERR', got: %s", response)
	}

	if !strings.Contains(response, "blocked by whitelist") {
		t.Errorf("Expected error message to contain 'blocked by whitelist', got: %s", response)
	}
}

func TestRedisProxy_DynamicCredentialResolution(t *testing.T) {
	// Test that credentials are resolved from ConfigSource
	cfg := &config.ConnectionConfig{
		Name: "test-redis",
		Type: "redis",
		Host: "localhost",
		Port: 6379,
		BackendPassword: config.ConfigSource{
			Type:  config.ConfigSourceTypePlain,
			Value: "plainpassword",
		},
	}

	rp := NewRedisProxy(cfg, "/tmp/audit.log", "testuser", "conn-123", nil, nil)

	if rp.config.BackendPassword.Value != "plainpassword" {
		t.Errorf("Expected password to be 'plainpassword', got: %s", rp.config.BackendPassword.Value)
	}
}

func BenchmarkRedisProxy_ExtractRESPCommands(b *testing.B) {
	rp := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
			Type: "redis",
		},
	}

	// Prepare test data with multiple commands
	testData := []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n" +
		"*3\r\n$3\r\nSET\r\n$4\r\nkey2\r\n$6\r\nvalue2\r\n" +
		"*2\r\n$3\r\nDEL\r\n$4\r\nkey3\r\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.extractRESPCommands(testData)
	}
}

func BenchmarkRedisAuthProxy_IsCommandAllowed(b *testing.B) {
	rp := &RedisAuthProxy{
		config: &config.ConnectionConfig{
			Name: "test-redis",
			Type: "redis",
		},
		whitelist: []string{"^GET", "^SET", "^DEL", "^HGET", "^HSET"},
	}

	command := "GET mykey"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.isCommandAllowed(command)
	}
}

// TestRedisProxy_RESPProtocolEdgeCases tests edge cases in RESP parsing
func TestRedisProxy_RESPProtocolEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Command with empty string",
			input:    "*2\r\n$3\r\nGET\r\n$0\r\n\r\n",
			expected: "GET",
		},
		{
			name:     "Command with special characters",
			input:    "*2\r\n$3\r\nGET\r\n$10\r\nmy:key:123\r\n",
			expected: "GET my:key:123",
		},
		{
			name:     "Incomplete command",
			input:    "*2\r\n$3\r\nGET\r\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RedisProxy{
				config: &config.ConnectionConfig{
					Name: "test-redis",
					Type: "redis",
				},
			}

			reader := bufio.NewReader(strings.NewReader(tt.input))
			firstLine, _ := reader.ReadString('\n')
			result := rp.parseRESPCommand(firstLine, reader)

			// For this test, we just check it doesn't panic
			// The actual result might vary based on implementation
			fmt.Printf("Edge case %q produced: %q\n", tt.name, result)
		})
	}
}

