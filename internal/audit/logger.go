package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	avgLogEntrySize = 256 // Average size of a log entry in bytes (conservative estimate)
)

var (
	mu              sync.Mutex
	logFiles        = make(map[string]*os.File)
	recentLogs      []LogEntry
	maxMemoryBytes  int64 = 1 * 1024 * 1024 // Default: 1MB
	currentMemBytes int64 = 0
	memoryEnabled   bool  = true
)

// LogEntry represents an audit log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Username  string                 `json:"username"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ConfigureMemoryBuffer sets the maximum memory for the in-memory audit buffer
// memoryMB: Maximum memory in megabytes (0 to disable, default 1MB)
func ConfigureMemoryBuffer(memoryMB int) {
	mu.Lock()
	defer mu.Unlock()

	if memoryMB <= 0 {
		// Disable in-memory buffer
		memoryEnabled = false
		recentLogs = nil
		currentMemBytes = 0
		maxMemoryBytes = 0
		return
	}

	memoryEnabled = true
	maxMemoryBytes = int64(memoryMB) * 1024 * 1024

	// Clear existing logs if they exceed new limit
	if currentMemBytes > maxMemoryBytes {
		recentLogs = nil
		currentMemBytes = 0
	}
}

// GetMemoryStats returns current memory usage statistics
func GetMemoryStats() (currentMB float64, maxMB float64, entryCount int, enabled bool) {
	mu.Lock()
	defer mu.Unlock()

	return float64(currentMemBytes) / (1024 * 1024),
		float64(maxMemoryBytes) / (1024 * 1024),
		len(recentLogs),
		memoryEnabled
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

	// Also store in memory buffer for web UI (if enabled)
	if memoryEnabled {
		entrySize := int64(len(data))

		// Ensure we don't exceed memory limit
		for currentMemBytes+entrySize > maxMemoryBytes && len(recentLogs) > 0 {
			// Remove oldest entry
			oldestSize := int64(avgLogEntrySize) // Use average for removed entries
			recentLogs = recentLogs[1:]
			currentMemBytes -= oldestSize
			if currentMemBytes < 0 {
				currentMemBytes = 0
			}
		}

		// Add new entry if we have room
		if currentMemBytes+entrySize <= maxMemoryBytes {
			recentLogs = append(recentLogs, entry)
			currentMemBytes += entrySize
		}
	}

	return nil
}

// GetRecentLogs returns recent audit logs from memory
// Returns empty slice if memory buffer is disabled
func GetRecentLogs(limit int) []LogEntry {
	mu.Lock()
	defer mu.Unlock()

	if !memoryEnabled || len(recentLogs) == 0 {
		return []LogEntry{}
	}

	if limit <= 0 || limit > len(recentLogs) {
		limit = len(recentLogs)
	}

	// Return last N entries (most recent)
	start := len(recentLogs) - limit
	if start < 0 {
		start = 0
	}

	// Create a copy to avoid race conditions
	result := make([]LogEntry, limit)
	copy(result, recentLogs[start:])
	return result
}

// Close closes all open log files
func Close() {
	mu.Lock()
	defer mu.Unlock()

	for _, file := range logFiles {
		// Don't close stdout
		if file != os.Stdout {
			_ = file.Close()
		}
	}
	logFiles = make(map[string]*os.File)
}
