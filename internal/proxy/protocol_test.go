package proxy

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewProtocol(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.ConnectionConfig
		wantErr bool
		wantNil bool
	}{
		{
			name: "create HTTP proxy",
			config: &config.ConnectionConfig{
				Name:   "test-http",
				Type:   "http",
				Host:   "localhost",
				Port:   8080,
				Scheme: "http",
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "create HTTPS proxy",
			config: &config.ConnectionConfig{
				Name:   "test-https",
				Type:   "https",
				Host:   "localhost",
				Port:   443,
				Scheme: "https",
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "create TCP proxy",
			config: &config.ConnectionConfig{
				Name: "test-tcp",
				Type: "tcp",
				Host: "localhost",
				Port: 6379,
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "postgres type returns error (handled separately)",
			config: &config.ConnectionConfig{
				Name: "test-postgres",
				Type: "postgres",
				Host: "localhost",
				Port: 5432,
			},
			wantErr: true,
			wantNil: false,
		},
		{
			name: "unsupported protocol type",
			config: &config.ConnectionConfig{
				Name: "test-unsupported",
				Type: "unsupported",
				Host: "localhost",
				Port: 1234,
			},
			wantErr: true,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProtocol(tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewProtocol() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantNil && got != nil {
				t.Error("NewProtocol() should return nil for postgres type")
			}

			if !tt.wantNil && !tt.wantErr && got == nil {
				t.Error("NewProtocol() returned nil but expected protocol")
			}
		})
	}
}

func BenchmarkNewProtocol_HTTP(b *testing.B) {
	config := &config.ConnectionConfig{
		Name:   "test-http",
		Type:   "http",
		Host:   "localhost",
		Port:   8080,
		Scheme: "http",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewProtocol(config)
	}
}

func BenchmarkNewProtocol_TCP(b *testing.B) {
	config := &config.ConnectionConfig{
		Name: "test-tcp",
		Type: "tcp",
		Host: "localhost",
		Port: 6379,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewProtocol(config)
	}
}
