package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RedisCommand represents a parsed Redis command
type RedisCommand struct {
	Command string   // e.g., "GET", "SET", "HGET"
	Args    []string // Command arguments (keys, values, etc.)
	Raw     []byte   // Raw bytes for forwarding to backend
}

// RESPParser parses Redis Serialization Protocol (RESP)
type RESPParser struct {
	reader *bufio.Reader
}

// NewRESPParser creates a new RESP parser
func NewRESPParser(reader io.Reader) *RESPParser {
	return &RESPParser{
		reader: bufio.NewReader(reader),
	}
}

// ParseCommand reads and parses a Redis command from the client
// Returns the parsed command and raw bytes for forwarding
func (p *RESPParser) ParseCommand() (*RedisCommand, error) {
	var rawBuf bytes.Buffer

	// Read the type byte
	typeByte, err := p.reader.ReadByte()
	if err != nil {
		return nil, err
	}
	rawBuf.WriteByte(typeByte)

	// Commands are sent as arrays (*<count>)
	if typeByte != '*' {
		return nil, fmt.Errorf("expected array, got %c", typeByte)
	}

	// Read array count
	count, line, err := p.readInteger()
	if err != nil {
		return nil, fmt.Errorf("failed to read array count: %w", err)
	}
	rawBuf.Write(line)

	if count <= 0 {
		return nil, fmt.Errorf("invalid command array count: %d", count)
	}

	// Parse array elements (command + args)
	elements := make([]string, count)
	for i := int64(0); i < count; i++ {
		element, rawBytes, err := p.parseElement()
		if err != nil {
			return nil, fmt.Errorf("failed to parse element %d: %w", i, err)
		}
		rawBuf.Write(rawBytes)
		elements[i] = element
	}

	// First element is the command, rest are arguments
	cmd := &RedisCommand{
		Command: strings.ToUpper(elements[0]),
		Args:    elements[1:],
		Raw:     rawBuf.Bytes(),
	}

	return cmd, nil
}

// parseElement parses a single RESP element
func (p *RESPParser) parseElement() (string, []byte, error) {
	var rawBuf bytes.Buffer

	typeByte, err := p.reader.ReadByte()
	if err != nil {
		return "", nil, err
	}
	rawBuf.WriteByte(typeByte)

	switch typeByte {
	case '$': // Bulk string
		length, line, err := p.readInteger()
		if err != nil {
			return "", nil, fmt.Errorf("failed to read bulk string length: %w", err)
		}
		rawBuf.Write(line)

		if length == -1 {
			// Null bulk string
			return "", rawBuf.Bytes(), nil
		}

		if length < 0 {
			return "", nil, fmt.Errorf("invalid bulk string length: %d", length)
		}

		// Read the string data
		data := make([]byte, length)
		_, err = io.ReadFull(p.reader, data)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read bulk string data: %w", err)
		}
		rawBuf.Write(data)

		// Read trailing \r\n
		crlf := make([]byte, 2)
		_, err = io.ReadFull(p.reader, crlf)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read bulk string CRLF: %w", err)
		}
		rawBuf.Write(crlf)

		if crlf[0] != '\r' || crlf[1] != '\n' {
			return "", nil, fmt.Errorf("expected CRLF after bulk string, got %v", crlf)
		}

		return string(data), rawBuf.Bytes(), nil

	case '+': // Simple string
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return "", nil, fmt.Errorf("failed to read simple string: %w", err)
		}
		rawBuf.WriteString(line)
		return strings.TrimSuffix(line, "\r\n"), rawBuf.Bytes(), nil

	case '-': // Error
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return "", nil, fmt.Errorf("failed to read error: %w", err)
		}
		rawBuf.WriteString(line)
		return strings.TrimSuffix(line, "\r\n"), rawBuf.Bytes(), nil

	case ':': // Integer
		num, line, err := p.readInteger()
		if err != nil {
			return "", nil, fmt.Errorf("failed to read integer: %w", err)
		}
		rawBuf.Write(line)
		return strconv.FormatInt(num, 10), rawBuf.Bytes(), nil

	default:
		return "", nil, fmt.Errorf("unknown RESP type: %c", typeByte)
	}
}

// readInteger reads an integer value followed by \r\n
// Returns the integer value and the raw bytes read
func (p *RESPParser) readInteger() (int64, []byte, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return 0, nil, err
	}

	// Trim \r\n for parsing
	trimmed := strings.TrimSuffix(line, "\r\n")
	trimmed = strings.TrimSuffix(trimmed, "\n") // Handle just \n if no \r

	num, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse integer: %w", err)
	}

	// Return raw bytes as they were read (including \r\n or \n)
	rawBytes := []byte(line)
	return num, rawBytes, nil
}

// String returns a string representation of the command for logging
func (c *RedisCommand) String() string {
	if len(c.Args) == 0 {
		return c.Command
	}
	return fmt.Sprintf("%s %s", c.Command, strings.Join(c.Args, " "))
}
