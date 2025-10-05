package cli

import (
	"testing"
)

func TestNewLoginCmd(t *testing.T) {
	cmd := NewLoginCmd()

	if cmd == nil {
		t.Fatal("NewLoginCmd() returned nil")
	}

	if cmd.Use != "login" {
		t.Errorf("Use = %s, want 'login'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}

	// Check flags exist
	if cmd.Flags().Lookup("username") == nil {
		t.Error("username flag should be defined")
	}

	if cmd.Flags().Lookup("password") == nil {
		t.Error("password flag should be defined")
	}

	if cmd.Flags().Lookup("provider") == nil {
		t.Error("provider flag should be defined")
	}
}

func TestNewListCmd(t *testing.T) {
	cmd := NewListCmd()

	if cmd == nil {
		t.Fatal("NewListCmd() returned nil")
	}

	if cmd.Use != "list" {
		t.Errorf("Use = %s, want 'list'", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
}

func TestNewConnectCmd(t *testing.T) {
	cmd := NewConnectCmd()

	if cmd == nil {
		t.Fatal("NewConnectCmd() returned nil")
	}

	// The Use field includes arguments, so just check it's not empty
	if cmd.Use == "" {
		t.Error("Use should not be empty")
	}

	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}

	// Check flags exist
	if cmd.Flags().Lookup("local-port") == nil {
		t.Error("local-port flag should be defined")
	}
}

func BenchmarkNewLoginCmd(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewLoginCmd()
	}
}

func BenchmarkNewListCmd(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewListCmd()
	}
}

func BenchmarkNewConnectCmd(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewConnectCmd()
	}
}
