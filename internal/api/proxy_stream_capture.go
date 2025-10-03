package api

import (
	"io"
	"sync"
)

// captureWriter wraps an io.Writer and captures written data
type captureWriter struct {
	writer  io.Writer
	data    []byte
	maxSize int
	mu      sync.Mutex
}

// newCaptureWriter creates a new capture writer with max size limit
func newCaptureWriter(w io.Writer, maxSize int) *captureWriter {
	return &captureWriter{
		writer:  w,
		data:    make([]byte, 0),
		maxSize: maxSize,
	}
}

func (c *captureWriter) Write(p []byte) (n int, err error) {
	// Write to underlying writer
	n, err = c.writer.Write(p)

	// Capture data (up to maxSize)
	c.mu.Lock()
	if len(c.data) < c.maxSize {
		remaining := c.maxSize - len(c.data)
		if len(p) <= remaining {
			c.data = append(c.data, p...)
		} else {
			c.data = append(c.data, p[:remaining]...)
		}
	}
	c.mu.Unlock()

	return n, err
}

func (c *captureWriter) GetData() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data
}

// captureReader wraps an io.Reader and captures read data
type captureReader struct {
	reader  io.Reader
	data    []byte
	maxSize int
	mu      sync.Mutex
}

// newCaptureReader creates a new capture reader with max size limit
func newCaptureReader(r io.Reader, maxSize int) *captureReader {
	return &captureReader{
		reader:  r,
		data:    make([]byte, 0),
		maxSize: maxSize,
	}
}

func (c *captureReader) Read(p []byte) (n int, err error) {
	// Read from underlying reader
	n, err = c.reader.Read(p)

	// Capture data (up to maxSize)
	c.mu.Lock()
	if len(c.data) < c.maxSize && n > 0 {
		remaining := c.maxSize - len(c.data)
		if n <= remaining {
			c.data = append(c.data, p[:n]...)
		} else {
			c.data = append(c.data, p[:remaining]...)
		}
	}
	c.mu.Unlock()

	return n, err
}

func (c *captureReader) GetData() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data
}

// truncateData truncates data to a reasonable size for logging
func truncateData(data []byte, maxLen int) string {
	if len(data) == 0 {
		return ""
	}

	if len(data) <= maxLen {
		return string(data)
	}

	return string(data[:maxLen]) + "... (truncated)"
}

