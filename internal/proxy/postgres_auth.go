package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/security"
	"github.com/xdg-go/scram"
)

// pgSSLRequestCode is the fixed protocol code Postgres uses to ask a server
// whether it supports SSL/TLS before sending the real startup message.
const pgSSLRequestCode = 80877103

// PostgresAuthProxy handles postgres with credential substitution
type PostgresAuthProxy struct {
	config       *config.ConnectionConfig
	auditLogPath string
	username     string
	connectionID string
	roles        []string
	apiConfig    *config.Config
	whitelist    []string
	approvalMgr  *approval.Manager
	resolver     *config.ConfigSourceResolver
	txStatus     atomic.Uint32 // last ReadyForQuery status from backend ('I', 'T', 'E')
}

// NewPostgresAuthProxy creates a postgres proxy with auth handling
func NewPostgresAuthProxy(cfg *config.ConnectionConfig, auditLogPath, username, connectionID string, roles []string, apiConfig *config.Config, whitelist []string, resolver *config.ConfigSourceResolver) *PostgresAuthProxy {
	return &PostgresAuthProxy{
		config:       cfg,
		auditLogPath: auditLogPath,
		username:     username,
		connectionID: connectionID,
		roles:        roles,
		apiConfig:    apiConfig,
		whitelist:    whitelist,
		approvalMgr:  nil, // Will be set later if approvals are enabled
		resolver:     resolver,
	}
}

// SetApprovalManager sets the approval manager for this proxy
func (p *PostgresAuthProxy) SetApprovalManager(mgr *approval.Manager) {
	p.approvalMgr = mgr
}

// HandleConnection handles the full postgres connection with auth
func (p *PostgresAuthProxy) HandleConnection(clientConn net.Conn) error {
	defer func() { _ = clientConn.Close() }()

	// Read startup message from client (might be SSL request first)
	startupMsg, err := p.readStartupMessage(clientConn)
	if err != nil {
		return fmt.Errorf("failed to read startup message: %w", err)
	}

	// Check if this is an SSL request (protocol 80877103)
	if len(startupMsg) >= 8 {
		protocol := binary.BigEndian.Uint32(startupMsg[4:8])
		if protocol == 80877103 {
			// Reject SSL - send 'N'
			_, _ = clientConn.Write([]byte{'N'})

			// Now read the real startup message
			startupMsg, err = p.readStartupMessage(clientConn)
			if err != nil {
				return fmt.Errorf("failed to read startup message after SSL: %w", err)
			}
		}
	}

	// Parse parameters from startup
	params, database := p.parseStartupParams(startupMsg)
	clientUser := params["user"]

	// Request password from client (cleartext for simplicity)
	if err := p.sendAuthRequest(clientConn); err != nil {
		return err
	}

	// Read password from client (but don't validate it - JWT already authenticated the user)
	_, err = p.readPassword(clientConn)
	if err != nil {
		return err
	}

	// SECURITY: Enforce that psql username matches authenticated API username
	if clientUser != p.username {
		p.sendAuthError(clientConn, "Username mismatch: you must connect as your authenticated user")
		_ = audit.Log(p.auditLogPath, p.username, "postgres_auth_failed", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"client_user":   clientUser,
			"expected_user": p.username,
			"reason":        "username_mismatch",
		})
		return fmt.Errorf("username mismatch: client=%s, authenticated=%s", clientUser, p.username)
	}

	// NOTE: Password validation is SKIPPED because authentication already happened at the API/JWT level
	// The connection was established with a valid JWT token, so the user is already authenticated.
	// We accept any password here since the real authentication is the JWT token.
	// This allows OIDC/SAML users (who don't have local passwords) to connect.
	_ = audit.Log(p.auditLogPath, p.username, "postgres_client_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"client_user":   clientUser,
		"note":          "password validation skipped - already authenticated via JWT",
	})

	// Resolve backend credentials dynamically (with caching)
	ctx := context.Background()
	backendUsername := p.config.BackendUsername.Value
	backendPassword := p.config.BackendPassword.Value

	// Resolve username if it's from ConfigMap/Secret
	if p.resolver != nil && (p.config.BackendUsername.Type == config.ConfigSourceTypeConfigMap || p.config.BackendUsername.Type == config.ConfigSourceTypeSecret) {
		resolvedUsername, exists, err := p.resolver.ResolveConfigSource(ctx, p.config.BackendUsername)
		if err != nil || !exists {
			log.Printf("Warning: Failed to resolve backend username for connection %s: %v (using stored value)", p.config.Name, err)
		} else {
			backendUsername = resolvedUsername
			log.Printf("DEBUG: Resolved backend username for connection %s (type: %s)", p.config.Name, p.config.BackendUsername.Type)
		}
	}

	// Resolve password if it's from ConfigMap/Secret
	if p.resolver != nil && (p.config.BackendPassword.Type == config.ConfigSourceTypeConfigMap || p.config.BackendPassword.Type == config.ConfigSourceTypeSecret) {
		resolvedPassword, exists, err := p.resolver.ResolveConfigSource(ctx, p.config.BackendPassword)
		if err != nil || !exists {
			log.Printf("Warning: Failed to resolve backend password for connection %s: %v (using stored value)", p.config.Name, err)
		} else {
			backendPassword = resolvedPassword
			log.Printf("DEBUG: Resolved backend password for connection %s (type: %s)", p.config.Name, p.config.BackendPassword.Type)
		}
	}

	// Connect to backend with BACKEND credentials (tries TLS first, falls back to plain)
	backendAddr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	backendConn, err := p.dialBackend(backendAddr)
	if err != nil {
		p.sendAuthError(clientConn, "Backend connection failed")
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer func() { _ = backendConn.Close() }()

	// Send startup to backend with BACKEND username
	backendDB := p.config.BackendDatabase
	if backendDB == "" {
		backendDB = database
	}
	if err := p.sendBackendStartup(backendConn, backendUsername, backendDB); err != nil {
		return err
	}

	// Handle backend authentication
	if err := p.handleBackendAuth(backendConn, backendPassword); err != nil {
		p.sendAuthError(clientConn, "Backend authentication failed")
		return fmt.Errorf("backend auth failed: %w", err)
	}

	// Send success to client
	if err := p.sendAuthSuccess(clientConn); err != nil {
		return err
	}

	_ = audit.Log(p.auditLogPath, p.username, "postgres_auth", p.config.Name, map[string]interface{}{
		"connection_id": p.connectionID,
		"client_user":   clientUser,
		"database":      database,
		"status":        "authenticated",
	})

	// Now do transparent bidirectional forwarding with query logging
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer func() { _ = backendConn.Close() }()
		p.forwardClient(clientConn, backendConn)
	}()

	go func() {
		defer wg.Done()
		defer func() { _ = clientConn.Close() }()
		p.forwardBackend(backendConn, clientConn)
	}()

	wg.Wait()
	return nil
}

// readStartupMessage reads the postgres startup message
func (p *PostgresAuthProxy) readStartupMessage(conn net.Conn) ([]byte, error) {
	// First 4 bytes are length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length < 8 || length > 10000 {
		return nil, fmt.Errorf("invalid startup message length: %d", length)
	}

	// Read the rest of the message (length includes itself)
	msgBuf := make([]byte, length-4)
	if _, err := io.ReadFull(conn, msgBuf); err != nil {
		return nil, err
	}

	// Combine length + message
	fullMsg := append(lenBuf, msgBuf...)
	return fullMsg, nil
}

// parseStartupParams extracts parameters from startup message
func (p *PostgresAuthProxy) parseStartupParams(msg []byte) (map[string]string, string) {
	params := make(map[string]string)
	database := ""

	// Skip length (4 bytes) and protocol version (4 bytes)
	data := msg[8:]

	// Parse null-terminated key-value pairs
	for len(data) > 0 {
		// Find null terminator
		nullIdx := bytes.IndexByte(data, 0)
		if nullIdx == -1 {
			break
		}
		key := string(data[:nullIdx])
		data = data[nullIdx+1:]

		if len(data) == 0 {
			break
		}

		nullIdx = bytes.IndexByte(data, 0)
		if nullIdx == -1 {
			break
		}
		value := string(data[:nullIdx])
		data = data[nullIdx+1:]

		params[key] = value
		if key == "database" {
			database = value
		}
	}

	return params, database
}

// sendAuthRequest sends cleartext password request
func (p *PostgresAuthProxy) sendAuthRequest(conn net.Conn) error {
	// AuthenticationCleartextPassword: 'R' + length(8) + type(3)
	msg := []byte{'R', 0, 0, 0, 8, 0, 0, 0, 3}
	_, err := conn.Write(msg)
	return err
}

// readPassword reads password message from client
func (p *PostgresAuthProxy) readPassword(conn net.Conn) (string, error) {
	// Read message type
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, typeBuf); err != nil {
		return "", err
	}

	if typeBuf[0] != 'p' {
		return "", fmt.Errorf("expected password message, got: %c", typeBuf[0])
	}

	// Read length
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "", err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if length < 5 || length > 1000 {
		return "", fmt.Errorf("invalid password length: %d", length)
	}

	// Read password (null-terminated)
	pwdBuf := make([]byte, length-4)
	if _, err := io.ReadFull(conn, pwdBuf); err != nil {
		return "", err
	}

	password := string(bytes.TrimRight(pwdBuf, "\x00"))
	return password, nil
}

// sendBackendStartup sends startup message to backend with backend credentials
func (p *PostgresAuthProxy) sendBackendStartup(conn net.Conn, username, database string) error {
	// Build startup message
	var buf bytes.Buffer

	// Protocol version 3.0
	_ = binary.Write(&buf, binary.BigEndian, uint32(196608))

	// Parameters
	buf.WriteString("user")
	buf.WriteByte(0)
	buf.WriteString(username)
	buf.WriteByte(0)

	buf.WriteString("database")
	buf.WriteByte(0)
	buf.WriteString(database)
	buf.WriteByte(0)

	keys := make([]string, 0, len(p.config.BackendParams))
	for k := range p.config.BackendParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte(0)
		buf.WriteString(p.config.BackendParams[k])
		buf.WriteByte(0)
	}

	// End marker
	buf.WriteByte(0)

	// Prepend length
	msgLen := buf.Len() + 4
	fullMsg := make([]byte, 4)
	binary.BigEndian.PutUint32(fullMsg, uint32(msgLen))
	fullMsg = append(fullMsg, buf.Bytes()...)

	_, err := conn.Write(fullMsg)
	return err
}

// dialBackend connects to the Postgres backend, opportunistically upgrading to
// TLS via the standard Postgres SSLRequest negotiation. If the backend doesn't
// support TLS, or the handshake fails (e.g. self-signed/unknown CA), it falls
// back to a plain connection automatically.
func (p *PostgresAuthProxy) dialBackend(addr string) (net.Conn, error) {
	timeout := 10 * time.Second

	// Explicit opt-out: tls.enabled=false skips negotiation entirely
	if p.config.TLS != nil && !p.config.TLS.Enabled {
		return net.DialTimeout("tcp", addr, timeout)
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}

	upgraded, err := p.tryUpgradeTLS(conn, timeout)
	if err != nil {
		log.Printf("Warning: TLS to Postgres backend %s failed, falling back to plain connection: %v", p.config.Name, err)
		_ = conn.Close()
		return net.DialTimeout("tcp", addr, timeout)
	}

	return upgraded, nil
}

// tryUpgradeTLS performs the Postgres SSLRequest handshake and, if the backend
// supports it, upgrades conn to TLS. If the backend declines, the original
// plain conn is returned unchanged. A deadline bounds the whole negotiation
// (SSLRequest round-trip + TLS handshake) so a backend that accepts the TCP
// connection and then stalls can't hang the goroutine indefinitely; it's
// cleared before handing the connection back for normal use.
func (p *PostgresAuthProxy) tryUpgradeTLS(conn net.Conn, timeout time.Duration) (net.Conn, error) {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("failed to set negotiation deadline: %w", err)
	}

	req := make([]byte, 8)
	binary.BigEndian.PutUint32(req[0:4], 8)
	binary.BigEndian.PutUint32(req[4:8], pgSSLRequestCode)
	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("failed to send SSLRequest: %w", err)
	}

	resp := make([]byte, 1)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, fmt.Errorf("failed to read SSLRequest response: %w", err)
	}

	switch resp[0] {
	case 'N':
		// Backend does not support TLS - continue on the plain connection
		if err := conn.SetDeadline(time.Time{}); err != nil {
			return nil, fmt.Errorf("failed to clear negotiation deadline: %w", err)
		}
		return conn, nil
	case 'E':
		return nil, fmt.Errorf("backend rejected SSLRequest: %s", readErrorResponseMessage(conn))
	case 'S':
		// Backend supports TLS - proceed to handshake below
	default:
		return nil, fmt.Errorf("unexpected SSLRequest response byte %q", resp[0])
	}

	// RootCAs is left nil unless CACert is set below, which makes crypto/tls fall
	// back to the OS trust store (x509.SystemCertPool()). The Dockerfile installs
	// AWS RDS's CA bundle into that trust store, so RDS certs verify by default.
	tlsConfig := &tls.Config{ServerName: p.config.Host}
	if p.config.TLS != nil {
		if p.config.TLS.ServerName != "" {
			tlsConfig.ServerName = p.config.TLS.ServerName
		}
		tlsConfig.InsecureSkipVerify = p.config.TLS.SkipVerify

		if p.config.TLS.CACert != "" {
			caCert, err := os.ReadFile(p.config.TLS.CACert)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
			tlsConfig.RootCAs = caCertPool
		}

		if p.config.TLS.ClientCert != "" && p.config.TLS.ClientKey != "" {
			clientCert, err := tls.LoadX509KeyPair(p.config.TLS.ClientCert, p.config.TLS.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{clientCert}
		}
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("failed to clear negotiation deadline: %w", err)
	}

	log.Printf("DEBUG: Established TLS connection to Postgres backend %s (server=%s)", p.config.Name, tlsConfig.ServerName)
	return tlsConn, nil
}

// readErrorResponseMessage reads a Postgres ErrorResponse body (the type byte
// 'E' has already been consumed) and returns its human-readable message field,
// falling back to a generic description if the body can't be read or parsed.
func readErrorResponseMessage(conn net.Conn) string {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "malformed ErrorResponse (failed to read length)"
	}
	length := binary.BigEndian.Uint32(lenBuf)
	if length < 4 {
		return "malformed ErrorResponse (invalid length)"
	}

	body := make([]byte, length-4)
	if _, err := io.ReadFull(conn, body); err != nil {
		return "malformed ErrorResponse (failed to read body)"
	}

	for i := 0; i < len(body); {
		fieldType := body[i]
		i++
		if fieldType == 0 {
			break
		}
		end := i
		for end < len(body) && body[end] != 0 {
			end++
		}
		if fieldType == 'M' {
			return string(body[i:end])
		}
		i = end + 1
	}
	return "no message field in ErrorResponse"
}

// handleBackendAuth handles backend authentication flow
func (p *PostgresAuthProxy) handleBackendAuth(conn net.Conn, password string) error {
	reader := bufio.NewReader(conn)

	for {
		// Read message type
		msgType, err := reader.ReadByte()
		if err != nil {
			return err
		}

		// Read length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(lenBuf)

		// Read message body
		body := make([]byte, length-4)
		if _, err := io.ReadFull(reader, body); err != nil {
			return err
		}

		switch msgType {
		case 'R': // Authentication request
			if len(body) >= 4 {
				authType := binary.BigEndian.Uint32(body[:4])
				switch authType {
				case 0:
					// Auth OK, continue
				case 3:
					// Cleartext password requested
					if err := p.sendBackendPassword(conn, password); err != nil {
						return err
					}
				case 5:
					// MD5 password requested
					if len(body) < 8 {
						return fmt.Errorf("invalid MD5 auth message")
					}
					salt := body[4:8]
					if err := p.sendBackendPasswordMD5(conn, password, p.config.BackendUsername.Value, salt); err != nil {
						return err
					}
				case 10:
					// SCRAM-SHA-256 requested
					if err := p.handleSCRAMAuth(conn, reader, password, p.config.BackendUsername.Value); err != nil {
						return err
					}
				default:
					return fmt.Errorf("unsupported auth type: %d", authType)
				}
			}

		case 'Z': // ReadyForQuery
			return nil // Backend ready

		case 'E': // ErrorResponse
			return fmt.Errorf("backend auth error: %s", string(body))

		case 'S', 'K': // ParameterStatus, BackendKeyData
			// Ignore these
			continue
		}
	}
}

// sendBackendPassword sends password to backend
func (p *PostgresAuthProxy) sendBackendPassword(conn net.Conn, password string) error {
	var buf bytes.Buffer
	buf.WriteByte('p')                                                // Message type
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(password)+5)) // Length
	buf.WriteString(password)
	buf.WriteByte(0)

	_, err := conn.Write(buf.Bytes())
	return err
}

// sendBackendPasswordMD5 sends MD5-hashed password to backend
func (p *PostgresAuthProxy) sendBackendPasswordMD5(conn net.Conn, password, username string, salt []byte) error {
	// PostgreSQL MD5 auth: "md5" + md5(md5(password + username) + salt)

	// First hash: md5(password + username)
	hasher := md5.New()
	hasher.Write([]byte(password))
	hasher.Write([]byte(username))
	hash1 := hex.EncodeToString(hasher.Sum(nil))

	// Second hash: md5(hash1 + salt)
	hasher.Reset()
	hasher.Write([]byte(hash1))
	hasher.Write(salt)
	hash2 := hex.EncodeToString(hasher.Sum(nil))

	// Final format: "md5" + hash2
	finalHash := "md5" + hash2

	var buf bytes.Buffer
	buf.WriteByte('p')                                                 // Message type
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(finalHash)+5)) // Length
	buf.WriteString(finalHash)
	buf.WriteByte(0)

	_, err := conn.Write(buf.Bytes())
	return err
}

// handleSCRAMAuth handles SCRAM-SHA-256 authentication
func (p *PostgresAuthProxy) handleSCRAMAuth(conn net.Conn, reader *bufio.Reader, password, username string) error {
	// Create SCRAM client
	client, err := scram.SHA256.NewClient(username, password, "")
	if err != nil {
		return fmt.Errorf("failed to create SCRAM client: %w", err)
	}

	conv := client.NewConversation()

	// Generate initial client message
	clientFirst, err := conv.Step("")
	if err != nil {
		return fmt.Errorf("SCRAM step 1 failed: %w", err)
	}

	// Send SASL Initial Response
	var buf bytes.Buffer
	buf.WriteByte('p') // PasswordMessage type

	// Calculate total message length
	mechanism := "SCRAM-SHA-256"
	// Length = 4 (length itself) + len(mechanism) + 1 (null) + 4 (clientFirst length) + len(clientFirst)
	totalLen := 4 + len(mechanism) + 1 + 4 + len(clientFirst)

	_ = binary.Write(&buf, binary.BigEndian, int32(totalLen))
	buf.WriteString(mechanism)
	buf.WriteByte(0) // Null terminator
	_ = binary.Write(&buf, binary.BigEndian, int32(len(clientFirst)))
	buf.WriteString(clientFirst)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return err
	}

	// Read server-first-message
	msgType, err := reader.ReadByte()
	if err != nil {
		return err
	}

	if msgType == 'E' {
		// Error response
		return fmt.Errorf("SCRAM auth error from server")
	}

	if msgType != 'R' {
		return fmt.Errorf("unexpected message type during SCRAM: %c", msgType)
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	body := make([]byte, length-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}

	// Check if this is AuthenticationSASLContinue (type 11)
	if len(body) < 4 || binary.BigEndian.Uint32(body[:4]) != 11 {
		return fmt.Errorf("expected SASL Continue")
	}

	serverFirst := string(body[4:])

	// Process server-first and generate client-final
	clientFinal, err := conv.Step(serverFirst)
	if err != nil {
		return fmt.Errorf("SCRAM step 2 failed: %w", err)
	}

	// Send client-final-message
	buf.Reset()
	buf.WriteByte('p')
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(clientFinal)+4))
	buf.WriteString(clientFinal)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return err
	}

	// Read server-final-message
	msgType, err = reader.ReadByte()
	if err != nil {
		return err
	}

	if msgType == 'E' {
		return fmt.Errorf("SCRAM auth failed")
	}

	if msgType != 'R' {
		return fmt.Errorf("unexpected message type during SCRAM final: %c", msgType)
	}

	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return err
	}
	length = binary.BigEndian.Uint32(lenBuf)

	body = make([]byte, length-4)
	if _, err := io.ReadFull(reader, body); err != nil {
		return err
	}

	// Check if this is AuthenticationSASLFinal (type 12)
	if len(body) < 4 || binary.BigEndian.Uint32(body[:4]) != 12 {
		return fmt.Errorf("expected SASL Final")
	}

	serverFinal := string(body[4:])

	// Validate server signature
	if _, err := conv.Step(serverFinal); err != nil {
		return fmt.Errorf("SCRAM validation failed: %w", err)
	}

	return nil
}

// sendAuthSuccess sends authentication success to client
func (p *PostgresAuthProxy) sendAuthSuccess(conn net.Conn) error {
	var buf bytes.Buffer

	// AuthenticationOk
	buf.WriteByte('R')
	_ = binary.Write(&buf, binary.BigEndian, int32(8))
	_ = binary.Write(&buf, binary.BigEndian, int32(0))

	// ParameterStatus messages
	params := map[string]string{
		"server_version":  "14.0",
		"server_encoding": "UTF8",
		"client_encoding": "UTF8",
		"DateStyle":       "ISO, MDY",
		"TimeZone":        "UTC",
	}

	for key, value := range params {
		buf.WriteByte('S')
		paramData := key + "\x00" + value + "\x00"
		_ = binary.Write(&buf, binary.BigEndian, int32(len(paramData)+4))
		buf.WriteString(paramData)
	}

	// BackendKeyData (dummy)
	buf.WriteByte('K')
	_ = binary.Write(&buf, binary.BigEndian, int32(12))
	_ = binary.Write(&buf, binary.BigEndian, int32(12345)) // process ID
	_ = binary.Write(&buf, binary.BigEndian, int32(67890)) // secret key

	// ReadyForQuery
	buf.WriteByte('Z')
	_ = binary.Write(&buf, binary.BigEndian, int32(5))
	buf.WriteByte('I') // Idle status

	_, err := conn.Write(buf.Bytes())
	return err
}

// sendAuthError sends authentication error to client
func (p *PostgresAuthProxy) sendAuthError(conn net.Conn, message string) {
	// ErrorResponse message
	var buf bytes.Buffer
	buf.WriteByte('E')

	// Build error fields
	fields := fmt.Sprintf("SFATAL\x00C28P01\x00M%s\x00\x00", message)
	length := len(fields) + 4

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(length))

	buf.Write(lenBuf)
	buf.WriteString(fields)

	_, _ = conn.Write(buf.Bytes())
}

// validateAPICredentials checks API username/password
//
//nolint:unused // Reserved for future API credential validation
func (p *PostgresAuthProxy) validateAPICredentials(username, password string) bool {
	if p.apiConfig == nil {
		return false
	}

	for _, user := range p.apiConfig.Auth.Users {
		if user.Username.Value == username && user.Password.Value == password {
			return true
		}
	}
	return false
}

// Message size caps: client queries are bounded tighter, backend rows can be large (bytea/text up to 1 GB)
const (
	maxClientMessage  = 64 << 20
	maxBackendMessage = 1 << 30
)

// readMessage reads one typed protocol message (type, int32 length incl. itself, body) and returns it whole
func readMessage(r *bufio.Reader, maxLen uint32) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n < 4 || n > maxLen {
		return 0, nil, fmt.Errorf("invalid message length %d", n)
	}
	msg := make([]byte, 1+n)
	copy(msg, hdr[:])
	_, err := io.ReadFull(r, msg[5:])
	return hdr[0], msg, err
}

// forwardClient forwards client messages one by one, validating every Query/Parse. A blocked Query is answered
// with ErrorResponse+ReadyForQuery; a blocked Parse drops the rest of the extended-protocol batch up to Sync.
// ponytail: with pipelined batches the injected error can overtake backend results still in flight; fine for psql/IDEs
func (p *PostgresAuthProxy) forwardClient(src, dst net.Conn) {
	r := bufio.NewReaderSize(src, 32*1024)
	w := bufio.NewWriterSize(dst, 32*1024)
	var skipToSync bool
	var blockedQuery string
	for {
		t, msg, err := readMessage(r, maxClientMessage)
		if err != nil {
			return
		}
		if skipToSync {
			if t == 'S' {
				p.sendQueryBlockedError(src, blockedQuery)
				skipToSync = false
			}
			continue
		}
		if t == 'Q' || t == 'P' {
			if blocked, query := p.validateAndLogQuery(msg); blocked {
				if t == 'P' {
					skipToSync, blockedQuery = true, query
					continue
				}
				p.sendQueryBlockedError(src, query)
				continue
			}
		}
		if _, err := w.Write(msg); err != nil {
			return
		}
		if r.Buffered() == 0 && w.Flush() != nil {
			return
		}
	}
}

// forwardBackend forwards backend messages, remembering the transaction status of each ReadyForQuery
func (p *PostgresAuthProxy) forwardBackend(src, dst net.Conn) {
	r := bufio.NewReaderSize(src, 32*1024)
	w := bufio.NewWriterSize(dst, 32*1024)
	for {
		t, msg, err := readMessage(r, maxBackendMessage)
		if err != nil {
			return
		}
		if t == 'Z' && len(msg) == 6 {
			p.txStatus.Store(uint32(msg[5]))
		}
		if _, err := w.Write(msg); err != nil {
			return
		}
		if r.Buffered() == 0 && w.Flush() != nil {
			return
		}
	}
}

// validateAndLogQuery extracts queries, validates against whitelist, checks approval, and logs
// Returns (blocked, query) where blocked=true if query should be blocked
func (p *PostgresAuthProxy) validateAndLogQuery(data []byte) (bool, string) {
	for i := 0; i < len(data); i++ {
		// Check for both Simple Query ('Q') and Extended Query Parse ('P') messages
		msgType := data[i]
		if (msgType == 'Q' || msgType == 'P') && i+5 < len(data) {
			length := int(binary.BigEndian.Uint32(data[i+1 : i+5]))

			if i+1+length <= len(data) && length > 4 {
				var query string

				switch msgType {
				case 'Q':
					// Simple Query: query string starts at i+5
					queryBytes := data[i+5 : i+1+length]
					query = string(bytes.TrimRight(queryBytes, "\x00"))
				case 'P':
					// Parse message: format is statement_name (null-terminated) + query (null-terminated)
					// Skip statement name to get to query
					nameStart := i + 5
					nameEnd := nameStart
					for nameEnd < i+1+length && data[nameEnd] != 0 {
						nameEnd++
					}
					if nameEnd < i+1+length {
						nameEnd++ // Skip null terminator
						// the query is null-terminated; parameter type OIDs follow it
						queryBytes := data[nameEnd : i+1+length]
						if end := bytes.IndexByte(queryBytes, 0); end >= 0 {
							queryBytes = queryBytes[:end]
						}
						query = string(queryBytes)
					}
				}

				if query != "" {
					// Check whitelist first
					allowed := p.isQueryAllowed(query)

					// Log the query with whitelist result
					_ = audit.Log(p.auditLogPath, p.username, "postgres_query", p.config.Name, map[string]interface{}{
						"connection_id": p.connectionID,
						"query":         query,
						"database":      p.config.BackendDatabase,
						"allowed":       allowed,
						"whitelist":     len(p.whitelist) > 0,
						"message_type":  string(msgType),
					})

					if !allowed {
						// Log blocked query
						_ = audit.Log(p.auditLogPath, p.username, "postgres_query_blocked", p.config.Name, map[string]interface{}{
							"connection_id": p.connectionID,
							"query":         query,
							"reason":        "whitelist_violation",
						})
						return true, query
					}

					// Check if approval is required for this query
					if p.approvalMgr != nil {
						normalizedQuery := strings.TrimSpace(query)
						requiresApproval, timeout := p.approvalMgr.RequiresApproval(normalizedQuery, "", p.config.Tags, p.roles)
						if requiresApproval {
							// Request approval
							approvalReq := &approval.Request{
								Username:       p.username,
								ConnectionID:   p.connectionID,
								ConnectionType: p.config.Type,
								ConnectionName: p.config.Name,
								Query:          normalizedQuery,
								Environment:    p.apiConfig.Environment,
								Metadata: map[string]string{
									"connection_name": p.config.Name,
									"connection_type": p.config.Type,
									"database":        p.config.BackendDatabase,
								},
							}

							// Log approval request
							_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_requested", p.config.Name, map[string]interface{}{
								"connection_id": p.connectionID,
								"query":         query,
								"database":      p.config.BackendDatabase,
								"timeout":       timeout.String(),
							})

							// Wait for approval with timeout
							ctx, cancel := context.WithTimeout(context.Background(), timeout)
							defer cancel()

							approvalResp, err := p.approvalMgr.RequestApproval(ctx, approvalReq, timeout)
							if err != nil {
								// Log approval error
								_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_error", p.config.Name, map[string]interface{}{
									"connection_id": p.connectionID,
									"query":         query,
									"error":         err.Error(),
								})
								return true, query
							}

							// Check approval decision
							if approvalResp.Decision != approval.DecisionApproved {
								// Log rejection/timeout
								_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_rejected", p.config.Name, map[string]interface{}{
									"connection_id": p.connectionID,
									"query":         query,
									"decision":      approvalResp.Decision,
									"reason":        approvalResp.Reason,
									"rejected_by":   approvalResp.ApprovedBy,
								})
								return true, query
							}

							// Log approval success
							_ = audit.Log(p.auditLogPath, p.username, "postgres_approval_granted", p.config.Name, map[string]interface{}{
								"connection_id": p.connectionID,
								"query":         query,
								"database":      p.config.BackendDatabase,
								"approved_by":   approvalResp.ApprovedBy,
							})
						}
					}
				}

				i += length
			}
		}
	}
	return false, ""
}

// isQueryAllowed checks if a query matches the whitelist patterns (case-insensitive)
// For PL/SQL scripts, validates each subquery individually
func (p *PostgresAuthProxy) isQueryAllowed(query string) bool {
	// If no whitelist, allow everything (backward compatibility)
	if len(p.whitelist) == 0 {
		return true
	}

	// Validate each top-level statement: "SELECT 1; DROP ..." must not pass on the first one
	result := security.NewSubqueryValidator().ValidateScript(query, p.whitelist)
	if result.TotalQueries > 1 {
		_ = audit.Log(p.auditLogPath, p.username, "plsql_subquery_validation", p.config.Name, map[string]interface{}{
			"connection_id": p.connectionID,
			"total_queries": result.TotalQueries,
			"allowed_count": result.AllowedCount,
			"blocked_count": result.BlockedCount,
			"is_allowed":    result.IsAllowed,
		})
	}
	return result.IsAllowed
}

// sendQueryBlockedError sends a proper PostgreSQL error response to the client for blocked queries
func (p *PostgresAuthProxy) sendQueryBlockedError(conn net.Conn, query string) {
	// Truncate query if too long
	displayQuery := query
	if r := []rune(displayQuery); len(r) > 100 {
		displayQuery = string(r[:100]) + "..."
	}

	// Build PostgreSQL ErrorResponse message
	var buf bytes.Buffer

	// Message type 'E' for ErrorResponse
	buf.WriteByte('E')

	// Build error fields according to PostgreSQL protocol
	// S = Severity, C = SQLSTATE code, M = Message
	var fields bytes.Buffer
	fields.WriteString("SERROR\x00") // Severity: ERROR
	fields.WriteString("C42501\x00") // SQLSTATE: insufficient_privilege
	fields.WriteString(fmt.Sprintf("MQuery blocked by whitelist policy: %s\x00", displayQuery))
	fields.WriteString("HCheck your role's whitelist patterns in the configuration.\x00") // Hint
	fields.WriteByte(0)                                                                   // Null terminator for fields

	// Write message length (includes the length field itself)
	msgLength := uint32(4 + fields.Len())
	_ = binary.Write(&buf, binary.BigEndian, msgLength)

	// Write fields
	buf.Write(fields.Bytes())

	// Send complete error message to client
	// ReadyForQuery with the backend's real transaction status (the backend never saw the query);
	// one Write so it can't interleave with backend output
	status := byte(p.txStatus.Load())
	if status == 0 {
		status = 'I'
	}
	buf.WriteByte('Z')
	_ = binary.Write(&buf, binary.BigEndian, int32(5))
	buf.WriteByte(status)

	_, _ = conn.Write(buf.Bytes())
}
