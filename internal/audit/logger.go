package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	mu       sync.Mutex
	logFiles = make(map[string]*os.File)
)

// LogEntry represents an audit log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Username  string                 `json:"username"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Log writes an audit log entry
func Log(logPath, username, action, resource string, metadata map[string]interface{}) error {
	mu.Lock()
	defer mu.Unlock()

	// Get or create log file
	logFile, exists := logFiles[logPath]
	if !exists {
		var err error
		// Support stdout as special case
		if logPath == "stdout" || logPath == "-" {
			logFile = os.Stdout
		} else {
			logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
		}
		logFiles[logPath] = logFile
	}

	// Create log entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Username:  username,
		Action:    action,
		Resource:  resource,
		Metadata:  metadata,
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Write to file
	if _, err := fmt.Fprintf(logFile, "%s\n", data); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// Close closes all open log files
func Close() {
	mu.Lock()
	defer mu.Unlock()

	for _, file := range logFiles {
		// Don't close stdout
		if file != os.Stdout {
			file.Close()
		}
	}
	logFiles = make(map[string]*os.File)
}
