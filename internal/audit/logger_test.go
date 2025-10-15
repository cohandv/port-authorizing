package audit

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLog(t *testing.T) {
	// Create a temporary log file
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Test logging
	username := "testuser"
	action := "login"
	resource := "api"
	metadata := map[string]interface{}{
		"ip":     "127.0.0.1",
		"result": "success",
	}

	err = Log(tmpFile.Name(), username, action, resource, metadata)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Give it a moment to write
	time.Sleep(100 * time.Millisecond)

	// Read the log file
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logLine := string(content)
	if logLine == "" {
		t.Error("Log file is empty")
		return
	}

	// Parse the JSON log entry
	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	// Validate log entry fields
	if logEntry["username"] != username {
		t.Errorf("username = %v, want %v", logEntry["username"], username)
	}

	if logEntry["action"] != action {
		t.Errorf("action = %v, want %v", logEntry["action"], action)
	}

	if logEntry["resource"] != resource {
		t.Errorf("resource = %v, want %v", logEntry["resource"], resource)
	}

	if logEntry["timestamp"] == nil {
		t.Error("timestamp should not be nil")
	}

	metadataMap, ok := logEntry["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("metadata is not a map")
	}

	if metadataMap["ip"] != "127.0.0.1" {
		t.Errorf("metadata.ip = %v, want '127.0.0.1'", metadataMap["ip"])
	}

	if metadataMap["result"] != "success" {
		t.Errorf("metadata.result = %v, want 'success'", metadataMap["result"])
	}
}

func TestLog_MultipleEntries(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Log multiple entries
	actions := []string{"login", "connect", "query", "disconnect"}
	for _, action := range actions {
		if err := Log(tmpFile.Name(), "user1", action, "postgres-test", map[string]interface{}{
			"action": action,
		}); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Read the log file
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Count the number of log lines
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != len(actions) {
		t.Errorf("Log contains %d lines, want %d", len(lines), len(actions))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestLog_NonExistentDirectory(t *testing.T) {
	// Test logging to a non-existent directory
	// Should not panic, but may fail silently or create directory
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Log() panicked when logging to non-existent directory: %v", r)
		}
	}()

	_ = Log("/nonexistent/directory/audit.log", "user", "action", "target", nil)
}

func TestLog_EmptyDetails(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Log with nil metadata
	if err := Log(tmpFile.Name(), "user", "action", "resource", nil); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Read and verify
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if logEntry["username"] != "user" {
		t.Errorf("username = %v, want 'user'", logEntry["username"])
	}
}

func TestClose(t *testing.T) {
	// Create temp files
	tmpFile1, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file 1: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile1.Name()) }()
	_ = tmpFile1.Close()

	tmpFile2, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file 2: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile2.Name()) }()
	_ = tmpFile2.Close()

	// Log to multiple files
	_ = Log(tmpFile1.Name(), "user1", "action1", "resource1", nil)
	_ = Log(tmpFile2.Name(), "user2", "action2", "resource2", nil)

	// Close should close all open files
	Close()

	// After Close, we should be able to log again (reopens files)
	if err := Log(tmpFile1.Name(), "user3", "action3", "resource3", nil); err != nil {
		t.Errorf("Log() after Close() error = %v", err)
	}
}

func TestLog_ReopenAfterClose(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Log, close, log again
	_ = Log(tmpFile.Name(), "user1", "action1", "resource1", nil)
	Close()
	_ = Log(tmpFile.Name(), "user2", "action2", "resource2", nil)

	// Read the log file
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Should have 2 log entries
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("Log contains %d lines, want 2", len(lines))
	}
}

func TestLog_ConcurrentWrites(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_ = Log(tmpFile.Name(), "user", "action", "resource", map[string]interface{}{
				"id": id,
			})
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all writes completed
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 10 {
		t.Errorf("Log contains %d lines, want 10", len(lines))
	}
}

func BenchmarkLog(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "audit-*.log")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	details := map[string]interface{}{
		"ip":     "127.0.0.1",
		"result": "success",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Log(tmpFile.Name(), "user", "action", "target", details)
	}
}
