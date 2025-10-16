package proxy

import (
	"bytes"
	"strings"
	"testing"
)

func TestRESPParser_ParseCommand_Simple(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCmd     string
		wantArgs    []string
		wantErr     bool
		errContains string
	}{
		{
			name:     "simple GET command",
			input:    "*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n",
			wantCmd:  "GET",
			wantArgs: []string{"key"},
			wantErr:  false,
		},
		{
			name:     "SET command with value",
			input:    "*3\r\n$3\r\nSET\r\n$5\r\nmykey\r\n$7\r\nmyvalue\r\n",
			wantCmd:  "SET",
			wantArgs: []string{"mykey", "myvalue"},
			wantErr:  false,
		},
		{
			name:     "DEL command with multiple keys",
			input:    "*4\r\n$3\r\nDEL\r\n$4\r\nkey1\r\n$4\r\nkey2\r\n$4\r\nkey3\r\n",
			wantCmd:  "DEL",
			wantArgs: []string{"key1", "key2", "key3"},
			wantErr:  false,
		},
		{
			name:     "HGET command",
			input:    "*3\r\n$4\r\nHGET\r\n$6\r\nmyhash\r\n$5\r\nfield\r\n",
			wantCmd:  "HGET",
			wantArgs: []string{"myhash", "field"},
			wantErr:  false,
		},
		{
			name:     "command with lowercase (should uppercase)",
			input:    "*2\r\n$3\r\nget\r\n$3\r\nkey\r\n",
			wantCmd:  "GET",
			wantArgs: []string{"key"},
			wantErr:  false,
		},
		{
			name:     "KEYS command",
			input:    "*2\r\n$4\r\nKEYS\r\n$1\r\n*\r\n",
			wantCmd:  "KEYS",
			wantArgs: []string{"*"},
			wantErr:  false,
		},
		{
			name:        "invalid array count",
			input:       "*0\r\n",
			wantErr:     true,
			errContains: "invalid command array count",
		},
		{
			name:        "not an array",
			input:       "+OK\r\n",
			wantErr:     true,
			errContains: "expected array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewRESPParser(strings.NewReader(tt.input))
			cmd, err := parser.ParseCommand()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cmd.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd.Command, tt.wantCmd)
			}

			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("args length = %d, want %d", len(cmd.Args), len(tt.wantArgs))
			}

			for i, arg := range cmd.Args {
				if arg != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}

			// Verify raw bytes match input
			if string(cmd.Raw) != tt.input {
				t.Errorf("raw bytes don't match input:\ngot:  %q\nwant: %q", string(cmd.Raw), tt.input)
			}
		})
	}
}

func TestRESPParser_ParseCommand_ComplexValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "SET with JSON value",
			input:    "*3\r\n$3\r\nSET\r\n$4\r\nuser\r\n$15\r\n{\"name\":\"test\"}\r\n",
			wantCmd:  "SET",
			wantArgs: []string{"user", `{"name":"test"}`},
		},
		{
			name:     "key with colon prefix",
			input:    "*2\r\n$3\r\nGET\r\n$11\r\nmyapp:users\r\n",
			wantCmd:  "GET",
			wantArgs: []string{"myapp:users"},
		},
		{
			name:     "value with special characters",
			input:    "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$11\r\nhello\nworld\r\n",
			wantCmd:  "SET",
			wantArgs: []string{"key", "hello\nworld"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewRESPParser(strings.NewReader(tt.input))
			cmd, err := parser.ParseCommand()

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cmd.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", cmd.Command, tt.wantCmd)
			}

			if len(cmd.Args) != len(tt.wantArgs) {
				t.Fatalf("args length = %d, want %d", len(cmd.Args), len(tt.wantArgs))
			}

			for i, arg := range cmd.Args {
				if arg != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestMatchesRedisPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		cmd     *RedisCommand
		want    bool
	}{
		{
			name:    "exact command match",
			pattern: "GET *",
			cmd:     &RedisCommand{Command: "GET", Args: []string{"key"}},
			want:    true,
		},
		{
			name:    "command mismatch",
			pattern: "GET *",
			cmd:     &RedisCommand{Command: "SET", Args: []string{"key", "value"}},
			want:    false,
		},
		{
			name:    "key prefix match",
			pattern: "GET myapp:*",
			cmd:     &RedisCommand{Command: "GET", Args: []string{"myapp:users"}},
			want:    true,
		},
		{
			name:    "key prefix mismatch",
			pattern: "GET myapp:*",
			cmd:     &RedisCommand{Command: "GET", Args: []string{"otherapp:users"}},
			want:    false,
		},
		{
			name:    "wildcard matches any",
			pattern: "SET *",
			cmd:     &RedisCommand{Command: "SET", Args: []string{"anykey", "value"}},
			want:    true,
		},
		{
			name:    "multiple args pattern",
			pattern: "HGET myhash *",
			cmd:     &RedisCommand{Command: "HGET", Args: []string{"myhash", "field1"}},
			want:    true,
		},
		{
			name:    "multiple args mismatch",
			pattern: "HGET myhash *",
			cmd:     &RedisCommand{Command: "HGET", Args: []string{"otherhash", "field1"}},
			want:    false,
		},
		{
			name:    "command only pattern",
			pattern: "KEYS",
			cmd:     &RedisCommand{Command: "KEYS", Args: []string{"*"}},
			want:    true,
		},
		{
			name:    "command only pattern with args",
			pattern: "PING",
			cmd:     &RedisCommand{Command: "PING", Args: []string{}},
			want:    true,
		},
		{
			name:    "all wildcards",
			pattern: "HGET * *",
			cmd:     &RedisCommand{Command: "HGET", Args: []string{"anyhash", "anyfield"}},
			want:    true,
		},
		{
			name:    "case insensitive command",
			pattern: "get *",
			cmd:     &RedisCommand{Command: "GET", Args: []string{"key"}},
			want:    true,
		},
		{
			name:    "pattern has more args than command",
			pattern: "GET key value",
			cmd:     &RedisCommand{Command: "GET", Args: []string{"key"}},
			want:    false,
		},
		{
			name:    "exact key match",
			pattern: "DEL myapp:session:123",
			cmd:     &RedisCommand{Command: "DEL", Args: []string{"myapp:session:123"}},
			want:    true,
		},
		{
			name:    "exact key mismatch",
			pattern: "DEL myapp:session:123",
			cmd:     &RedisCommand{Command: "DEL", Args: []string{"myapp:session:456"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRedisPattern(tt.pattern, tt.cmd)
			if got != tt.want {
				t.Errorf("matchesRedisPattern(%q, %v) = %v, want %v", tt.pattern, tt.cmd, got, tt.want)
			}
		})
	}
}

func TestMatchesGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"myapp:*", "myapp:users", true},
		{"myapp:*", "myapp:", true},
		{"myapp:*", "otherapp:users", false},
		{"*:users", "myapp:users", true},
		{"*:users", "users", false},
		{"exact", "exact", true},
		{"exact", "different", false},
		{"prefix*suffix", "prefixMIDDLEsuffix", true},
		{"prefix*suffix", "prefix", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			got := matchesGlobPattern(tt.pattern, tt.s)
			if got != tt.want {
				t.Errorf("matchesGlobPattern(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

func TestRedisCommand_String(t *testing.T) {
	tests := []struct {
		name string
		cmd  *RedisCommand
		want string
	}{
		{
			name: "command with args",
			cmd:  &RedisCommand{Command: "GET", Args: []string{"key"}},
			want: "GET key",
		},
		{
			name: "command without args",
			cmd:  &RedisCommand{Command: "PING", Args: []string{}},
			want: "PING",
		},
		{
			name: "command with multiple args",
			cmd:  &RedisCommand{Command: "SET", Args: []string{"key", "value"}},
			want: "SET key value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cmd.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRESPParser_ErrorConditions(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{
			name:        "truncated array count",
			input:       "*2\r",
			errContains: "failed to read array count",
		},
		{
			name:        "truncated bulk string length",
			input:       "*1\r\n$5\r",
			errContains: "failed to read bulk string length",
		},
		{
			name:        "truncated bulk string data",
			input:       "*1\r\n$5\r\nhel",
			errContains: "failed to read bulk string data",
		},
		{
			name:        "missing CRLF after bulk string",
			input:       "*1\r\n$5\r\nhelloX",
			errContains: "failed to read bulk string CRLF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewRESPParser(strings.NewReader(tt.input))
			_, err := parser.ParseCommand()

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errContains)
			}

			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
			}
		})
	}
}

func BenchmarkRESPParser_ParseCommand(b *testing.B) {
	input := "*3\r\n$3\r\nSET\r\n$4\r\nmykey\r\n$7\r\nmyvalue\r\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewRESPParser(bytes.NewReader([]byte(input)))
		_, err := parser.ParseCommand()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMatchesRedisPattern(b *testing.B) {
	pattern := "GET myapp:*"
	cmd := &RedisCommand{Command: "GET", Args: []string{"myapp:users:123"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchesRedisPattern(pattern, cmd)
	}
}
