package api

import (
	"io"
)

// truncateData truncates data to maxLen for preview/logging purposes
func truncateData(data []byte, maxLen int) string {
	if len(data) <= maxLen {
		return string(data)
	}
	return string(data[:maxLen]) + "... (truncated)"
}

// captureWriter wraps an io.Writer and captures written data up to maxSize
// Used for auditing and debugging protocol streams
//
//nolint:unused // Reserved for future audit/debugging features
type captureWriter struct {
	writer  io.Writer
	buffer  []byte
	maxSize int
}

//nolint:unused // Reserved for future audit/debugging features
func newCaptureWriter(w io.Writer, maxSize int) *captureWriter {
	return &captureWriter{
		writer:  w,
		buffer:  make([]byte, 0, maxSize),
		maxSize: maxSize,
	}
}

//nolint:unused // Reserved for future audit/debugging features
func (c *captureWriter) Write(p []byte) (n int, err error) {
	// Write to underlying writer
	n, err = c.writer.Write(p)

	// Capture data up to maxSize
	if len(c.buffer) < c.maxSize {
		remaining := c.maxSize - len(c.buffer)
		if len(p) <= remaining {
			c.buffer = append(c.buffer, p...)
		} else {
			c.buffer = append(c.buffer, p[:remaining]...)
		}
	}

	return n, err
}

//nolint:unused // Reserved for future audit/debugging features
func (c *captureWriter) GetData() []byte {
	return c.buffer
}

// captureReader wraps an io.Reader and captures read data up to maxSize
// Used for auditing and debugging protocol streams
//
//nolint:unused // Reserved for future audit/debugging features
type captureReader struct {
	reader  io.Reader
	buffer  []byte
	maxSize int
}

//nolint:unused // Reserved for future audit/debugging features
func newCaptureReader(r io.Reader, maxSize int) *captureReader {
	return &captureReader{
		reader:  r,
		buffer:  make([]byte, 0, maxSize),
		maxSize: maxSize,
	}
}

//nolint:unused // Reserved for future audit/debugging features
func (c *captureReader) Read(p []byte) (n int, err error) {
	// Read from underlying reader
	n, err = c.reader.Read(p)

	// Capture data up to maxSize
	if n > 0 && len(c.buffer) < c.maxSize {
		remaining := c.maxSize - len(c.buffer)
		if n <= remaining {
			c.buffer = append(c.buffer, p[:n]...)
		} else {
			c.buffer = append(c.buffer, p[:remaining]...)
		}
	}

	return n, err
}

//nolint:unused // Reserved for future audit/debugging features
func (c *captureReader) GetData() []byte {
	return c.buffer
}

