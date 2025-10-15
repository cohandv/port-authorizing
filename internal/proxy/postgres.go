package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/jackc/pgproto3/v2"
)

// simpleChunkReader wraps a net.Conn to implement pgproto3.ChunkReader
type simpleChunkReader struct {
	conn net.Conn
	buf  []byte
}

func newSimpleChunkReader(conn net.Conn) *simpleChunkReader {
	return &simpleChunkReader{
		conn: conn,
		buf:  make([]byte, 8192),
	}
}

func (r *simpleChunkReader) Next(n int) ([]byte, error) {
	if n > len(r.buf) {
		r.buf = make([]byte, n)
	}
	_, err := io.ReadFull(r.conn, r.buf[:n])
	if err != nil {
		return nil, err
	}
	return r.buf[:n], nil
}

// PostgresProxy handles PostgreSQL protocol proxying with query logging
type PostgresProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string // API username (for audit logging)
	connectionID string
	apiConfig    *config.Config // Full API config for user validation
}

// NewPostgresProxy creates a new PostgreSQL protocol-aware proxy
func NewPostgresProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, apiConfig *config.Config) *PostgresProxy {
	return &PostgresProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		apiConfig:    apiConfig,
	}
}

// HandleConnection handles a Postgres protocol connection
// Client connects with API credentials, proxy intercepts queries and forwards to backend
func (p *PostgresProxy) HandleConnection(clientConn net.Conn) error {
	defer func() { _ = clientConn.Close() }()

	// Create pgproto3 backend to handle client messages
	clientReader := newSimpleChunkReader(clientConn)
	backend := pgproto3.NewBackend(clientReader, clientConn)

	// Receive startup message from client
	startupMsg, err := backend.ReceiveStartupMessage()
	if err != nil {
		return fmt.Errorf("failed to receive startup message: %w", err)
	}

	var requestedDatabase string
	var clientUsername string

	switch msg := startupMsg.(type) {
	case *pgproto3.StartupMessage:
		clientUsername = msg.Parameters["user"]
		requestedDatabase = msg.Parameters["database"]

	case *pgproto3.SSLRequest:
		// Reject SSL for simplicity
		_, _ = clientConn.Write([]byte("N"))

		// Read the real startup message
		startupMsg, err = backend.ReceiveStartupMessage()
		if err != nil {
			return fmt.Errorf("failed to receive startup message after SSL: %w", err)
		}
		if sm, ok := startupMsg.(*pgproto3.StartupMessage); ok {
			clientUsername = sm.Parameters["user"]
			requestedDatabase = sm.Parameters["database"]
		}

	default:
		return fmt.Errorf("unexpected startup message type: %T", msg)
	}

	// Request password from client
	authMsg := &pgproto3.AuthenticationCleartextPassword{}
	buf, err := authMsg.Encode(nil)
	if err != nil {
		return fmt.Errorf("failed to encode auth message: %w", err)
	}
	if _, err := clientConn.Write(buf); err != nil {
		return fmt.Errorf("failed to request password: %w", err)
	}

	// Receive password
	pwdMsg, err := backend.Receive()
	if err != nil {
		return fmt.Errorf("failed to receive password: %w", err)
	}

	passwordMsg, ok := pwdMsg.(*pgproto3.PasswordMessage)
	if !ok {
		p.sendError(clientConn, "28P01", "expected password message")
		return fmt.Errorf("expected password message, got %T", pwdMsg)
	}

	// Validate API credentials
	if !p.validateAPICredentials(clientUsername, passwordMsg.Password) {
		p.sendError(clientConn, "28P01", "authentication failed: invalid API credentials")
		_ = audit.Log(p.auditLogPath, p.username, "postgres_auth_failed", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"client_user":   clientUsername,
			"reason":        "invalid_credentials",
		})
		return fmt.Errorf("invalid API credentials for user: %s", clientUsername)
	}

	// Send authentication OK, parameter status, and ready for query
	authOK := &pgproto3.AuthenticationOk{}
	buf, err = authOK.Encode(nil)
	if err != nil {
		return err
	}
	_, _ = clientConn.Write(buf)

	// Send some parameter status messages (postgres expects these)
	params := []struct{ name, value string }{
		{"server_version", "14.0"},
		{"server_encoding", "UTF8"},
		{"client_encoding", "UTF8"},
	}
	for _, param := range params {
		paramMsg := &pgproto3.ParameterStatus{Name: param.name, Value: param.value}
		buf, err = paramMsg.Encode(nil)
		if err == nil {
			_, _ = clientConn.Write(buf)
		}
	}

	// Send backend key data (dummy)
	keyData := &pgproto3.BackendKeyData{ProcessID: 12345, SecretKey: 67890}
	buf, err = keyData.Encode(nil)
	if err == nil {
		_, _ = clientConn.Write(buf)
	}

	// Send ready for query
	ready := &pgproto3.ReadyForQuery{TxStatus: 'I'}
	buf, err = ready.Encode(nil)
	if err != nil {
		return err
	}
	_, _ = clientConn.Write(buf)

	_ = audit.Log(p.auditLogPath, p.username, "postgres_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"client_user":   clientUsername,
		"database":      requestedDatabase,
		"status":        "authenticated",
	})

	// Use backend database from config
	backendDatabase := p.config.BackendDatabase
	if backendDatabase == "" {
		backendDatabase = requestedDatabase
	}

	// Connect to backend
	backendConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", p.config.Host, p.config.Port), 10*time.Second)
	if err != nil {
		p.sendError(clientConn, "08006", fmt.Sprintf("could not connect to backend: %v", err))
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backendConn.Close() }()

	// Create frontend to backend
	backendReader := newSimpleChunkReader(backendConn)
	frontend := pgproto3.NewFrontend(backendReader, backendConn)

	// Send startup to backend
	startupParams := map[string]string{
		"user":     p.config.BackendUsername,
		"database": backendDatabase,
	}
	startupBuf, err := (&pgproto3.StartupMessage{
		ProtocolVersion: 196608,
		Parameters:      startupParams,
	}).Encode(nil)
	if err != nil {
		return fmt.Errorf("failed to encode startup message: %w", err)
	}
	_, _ = backendConn.Write(startupBuf)

	// Handle backend authentication
	if err := p.authenticateToBackend(frontend, backendConn); err != nil {
		p.sendError(clientConn, "08006", "backend authentication failed")
		return fmt.Errorf("backend authentication failed: %w", err)
	}

	// Now do raw TCP bidirectional forwarding with query logging
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend (log queries)
	go func() {
		defer wg.Done()
		defer func() { _ = backendConn.Close() }()
		p.copyWithQueryLogging(clientConn, backendConn, true)
	}()

	// Backend -> Client (pass through)
	go func() {
		defer wg.Done()
		defer func() { _ = clientConn.Close() }()
		p.copyWithQueryLogging(backendConn, clientConn, false)
	}()

	wg.Wait()
	return nil
}

// authenticateToBackend handles backend authentication
func (p *PostgresProxy) authenticateToBackend(frontend *pgproto3.Frontend, conn net.Conn) error {
	for {
		msg, err := frontend.Receive()
		if err != nil {
			return fmt.Errorf("backend auth error: %w", err)
		}

		switch msg := msg.(type) {
		case *pgproto3.AuthenticationOk:
			continue

		case *pgproto3.AuthenticationCleartextPassword:
			pwdMsg := &pgproto3.PasswordMessage{Password: p.config.BackendPassword}
			buf, err := pwdMsg.Encode(nil)
			if err != nil {
				return err
			}
			_, _ = conn.Write(buf)

		case *pgproto3.AuthenticationMD5Password:
			return fmt.Errorf("MD5 authentication not supported")

		case *pgproto3.ReadyForQuery:
			return nil

		case *pgproto3.ParameterStatus, *pgproto3.BackendKeyData:
			continue

		case *pgproto3.ErrorResponse:
			return fmt.Errorf("backend error: %s", msg.Message)
		}
	}
}

// copyWithQueryLogging copies data between connections and optionally logs queries
func (p *PostgresProxy) copyWithQueryLogging(src, dst net.Conn, logQueries bool) {
	buf := make([]byte, 32*1024)

	for {
		n, err := src.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Try to log queries if this is client->backend traffic
			if logQueries {
				p.tryExtractQuery(data)
			}

			// Forward the data
			if _, err := dst.Write(data); err != nil {
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				_ = audit.Log(p.auditLogPath, p.username, "postgres_error", p.config.Name, map[string]interface{}{
					"connection_id": p.connectionID,
					"error":         err.Error(),
					"log_queries":   logQueries,
				})
			}
			return
		}
	}
}

// tryExtractQuery attempts to extract SQL queries from postgres protocol messages
func (p *PostgresProxy) tryExtractQuery(data []byte) {
	// Postgres simple query protocol: 'Q' followed by 4-byte length, then SQL string
	for i := 0; i < len(data); i++ {
		if data[i] == 'Q' && i+5 < len(data) {
			// Read length (4 bytes, big-endian)
			length := int(data[i+1])<<24 | int(data[i+2])<<16 | int(data[i+3])<<8 | int(data[i+4])

			// Check if we have the full message
			if i+1+length <= len(data) && length > 4 {
				// Extract query (skip 'Q' and 4-byte length)
				queryStart := i + 5
				queryEnd := i + 1 + length

				if queryEnd <= len(data) {
					queryBytes := data[queryStart:queryEnd]
					// Query is null-terminated
					query := string(bytes.TrimRight(queryBytes, "\x00"))

					if query != "" {
						p.logQuery(query)
					}
				}

				// Move past this message
				i += length
			}
		}
	}
}

// logQuery logs a SQL query
func (p *PostgresProxy) logQuery(query string) {
	if query == "" {
		return
	}

	_ = audit.Log(p.auditLogPath, p.username, "postgres_query", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"query":         query,
		"database":      p.config.BackendDatabase,
	})
}

// sendError sends an error to client
func (p *PostgresProxy) sendError(conn net.Conn, code, message string) {
	errMsg := &pgproto3.ErrorResponse{
		Severity: "FATAL",
		Code:     code,
		Message:  message,
	}
	buf, err := errMsg.Encode(nil)
	if err == nil {
		_, _ = conn.Write(buf)
	}
}

// captureAndForward captures messages for logging while forwarding
//
//nolint:unused // Reserved for future message capture/debugging
func captureAndForward(reader io.Reader, writer io.Writer, captureFunc func([]byte)) error {
	buf := make([]byte, 8192)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := buf[:n]
			if captureFunc != nil {
				// Make a copy for capture
				captured := make([]byte, n)
				copy(captured, data)
				captureFunc(captured)
			}
			if _, writeErr := writer.Write(data); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// parsePostgresMessage attempts to parse Postgres protocol messages for logging
//
//nolint:unused // Reserved for future message parsing enhancements
func parsePostgresMessage(data []byte) *string {
	if len(data) < 5 {
		return nil
	}

	msgType := data[0]
	// Simple query ('Q') is the most common
	if msgType == 'Q' {
		// Skip message type (1) and length (4)
		if len(data) > 5 {
			query := string(bytes.TrimRight(data[5:], "\x00"))
			return &query
		}
	}

	return nil
}

// validateAPICredentials validates username/password against API users
func (p *PostgresProxy) validateAPICredentials(username, password string) bool {
	if p.apiConfig == nil {
		return false
	}

	for _, user := range p.apiConfig.Auth.Users {
		if user.Username == username && user.Password == password {
			return true
		}
	}

	return false
}
