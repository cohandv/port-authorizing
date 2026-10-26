package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [connection-name port] [connection-name port]...",
	Short: "Connect to one or more services via proxy",
	Long: `Establish local proxy connections to remote services through the API.

Examples:
  # Single connection
  port-authorizing connect mydb 5432
  
  # Single connection (backwards compatible)
  port-authorizing connect mydb -l 5432
  
  # Multiple connections
  port-authorizing connect redis1 6379 postgres1 5432 api1 8080
  
  # Connect to all connections with a tag
  port-authorizing connect --tag env:dev --base-port 8000
  port-authorizing connect --tag type:cache --base-port 6379
  
  # Connect to all connections with ANY of the specified tags (OR logic)
  port-authorizing connect --tag env:dev,type:cache --base-port 8000`,
	Args: cobra.MinimumNArgs(0),
	RunE: runConnect,
}

var (
	localPort int // Deprecated, kept for backwards compatibility
	tagFilter string
	basePort  int
)

func init() {
	connectCmd.Flags().IntVarP(&localPort, "local-port", "l", 0, "Local port (deprecated: use positional args)")
	connectCmd.Flags().StringVar(&tagFilter, "tag", "", "Connect to all connections with tag(s) - comma-separated for OR (e.g., 'env:dev' or 'env:dev,type:database')")
	connectCmd.Flags().IntVar(&basePort, "base-port", 0, "Starting port for tag-based connections (increments for each connection)")
}

type connectResponse struct {
	ConnectionID string `json:"connection_id"`
	ExpiresAt    string `json:"expires_at"`
	ProxyURL     string `json:"proxy_url"`
	Type         string `json:"type,omitempty"`     // Connection type (postgres, http, tcp)
	Database     string `json:"database,omitempty"` // For postgres connections
}

type connectionTarget struct {
	Name string
	Port int
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Get context (--context if passed, else the current context)
	ctx, err := resolveContext(cmd)
	if err != nil {
		return fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}

	apiURL := resolveAPIURL(cmd, ctx.Name, ctx.APIURL)
	token := ctx.Token

	// Validate token is still valid
	if err := validateToken(token); err != nil {
		return fmt.Errorf("authentication expired or invalid: %w\nPlease login again: ./port-authorizing-cli login", err)
	}

	var targets []connectionTarget

	// Mode 1: Tag-based connection
	if tagFilter != "" {
		if basePort == 0 {
			return fmt.Errorf("--base-port is required when using --tag")
		}
		
		// Parse comma-separated tags
		tags := strings.Split(tagFilter, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		
		if len(tags) == 1 {
			fmt.Printf("🔍 Finding connections with tag: %s\n", tags[0])
		} else {
			fmt.Printf("🔍 Finding connections with tags: %s (OR logic)\n", strings.Join(tags, ", "))
		}
		
		tagTargets, err := getConnectionsByTags(apiURL, token, tags, basePort)
		if err != nil {
			return err
		}
		if len(tagTargets) == 0 {
			return fmt.Errorf("no connections found with tag(s): %s", tagFilter)
		}
		targets = tagTargets
		fmt.Printf("✓ Found %d connection(s)\n\n", len(targets))
	} else if len(args) == 1 && localPort > 0 {
		// Mode 2: Backwards compatibility - single connection with -l flag
		targets = append(targets, connectionTarget{Name: args[0], Port: localPort})
	} else if len(args) > 0 {
		// Mode 3: Multiple connection pairs
		if len(args)%2 != 0 {
			return fmt.Errorf("arguments must be in pairs: connection-name port [connection-name port]...")
		}
		
		for i := 0; i < len(args); i += 2 {
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("invalid port '%s' for connection '%s': %w", args[i+1], args[i], err)
			}
			targets = append(targets, connectionTarget{Name: args[i], Port: port})
		}
	} else {
		return fmt.Errorf("provide connection(s) or use --tag flag.\n\nExamples:\n  connect mydb 5432\n  connect mydb -l 5432 (backwards compatible)\n  connect redis1 6379 postgres1 5432\n  connect --tag env:dev --base-port 8000")
	}

	// Start all connections concurrently
	return startMultipleConnections(targets, apiURL, token)
}

func getConnectionsByTags(apiURL, token string, tags []string, basePort int) ([]connectionTarget, error) {
	connections, err := fetchConnections(apiURL, token)
	if err != nil {
		return nil, err
	}

	// Filter by tags (OR logic - connection must have at least one of the specified tags)
	var targets []connectionTarget
	port := basePort
	for _, conn := range connections {
		matched := false
		for _, connTag := range conn.Tags {
			for _, filterTag := range tags {
				if connTag == filterTag {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			targets = append(targets, connectionTarget{Name: conn.Name, Port: port})
			port++
		}
	}

	return targets, nil
}

func startMultipleConnections(targets []connectionTarget, apiURL, token string) error {
	if len(targets) == 0 {
		return fmt.Errorf("no connections to start")
	}

	// Print summary
	fmt.Printf("🚀 Starting %d connection(s):\n", len(targets))
	for _, t := range targets {
		fmt.Printf("  • %s → localhost:%d\n", t.Name, t.Port)
	}
	fmt.Println()

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Error channel for all connections
	errChan := make(chan error, len(targets))

	// Start each connection in its own goroutine
	for _, target := range targets {
		go func(t connectionTarget) {
			if err := startSingleConnection(ctx, t, apiURL, token); err != nil {
				errChan <- fmt.Errorf("%s: %w", t.Name, err)
			}
		}(target)
	}

	// Wait for interrupt or error
	select {
	case <-sigChan:
		fmt.Println("\n\n🛑 Interrupt received, shutting down all connections...")
		cancel()
		return nil
	case err := <-errChan:
		fmt.Printf("\n❌ Connection error: %v\n", err)
		cancel()
		return err
	}
}

func startSingleConnection(ctx context.Context, target connectionTarget, apiURL, token string) error {
	connResp, err := connectToBackend(apiURL, token, target.Name)
	if err != nil {
		return err
	}

	// Handle K8s services connection type differently
	if connResp.Type == "k8s_services" {
		fmt.Println("\n🔍 K8s Services Discovery Mode")
		fmt.Printf("  Base port for forwarding: %d\n", target.Port)
		// For k8s_services, we don't use context-based cancellation since it has its own lifecycle
		return runK8sServicesConnect(apiURL, token, connResp.ConnectionID, connResp.ExpiresAt, target.Port)
	}

	fmt.Printf("✓ %s connected (expires: %s)\n", target.Name, connResp.ExpiresAt)

	// Show connection info for postgres
	if connResp.Type == "postgres" {
		username, _ := getUsernameFromToken(token)
		fmt.Printf("  └─ psql -h localhost -p %d -U %s -d %s\n", target.Port, username, connResp.Database)
	}

	// Start local proxy server with context
	return startLocalProxyWithContext(ctx, target.Port, connResp.ConnectionID, token, connResp.ExpiresAt, apiURL)
}

func startLocalProxyWithContext(ctx context.Context, port int, connectionID, token string, expiresAt string, apiURL string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	defer func() { _ = listener.Close() }()

	// Parse expiry time
	expiryTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		fmt.Printf("Warning: Could not parse expiry time for port %d: %v\n", port, err)
	}

	// Channel for expiry
	var expiryChan <-chan time.Time
	if !expiryTime.IsZero() {
		expiryChan = time.After(time.Until(expiryTime))
	}

	// Accept connections in a goroutine
	connChan := make(chan net.Conn)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connChan <- conn
		}
	}()

	// Track active connections
	var activeConns []net.Conn
	var mu sync.Mutex

	// Handle connections
	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, c := range activeConns {
				_ = c.Close()
			}
			mu.Unlock()
			return nil

		case <-expiryChan:
			fmt.Printf("\n⏰ Connection expired for port %d\n", port)
			mu.Lock()
			for _, c := range activeConns {
				_ = c.Close()
			}
			mu.Unlock()
			return nil

		case conn := <-connChan:
			mu.Lock()
			activeConns = append(activeConns, conn)
			mu.Unlock()

			go func(c net.Conn) {
				defer func() {
					_ = c.Close()
					mu.Lock()
					// Remove from active connections
					for i, ac := range activeConns {
						if ac == c {
							activeConns = append(activeConns[:i], activeConns[i+1:]...)
							break
						}
					}
					mu.Unlock()
				}()

				handleLocalConnection(c, connectionID, token, apiURL)
			}(conn)
		}
	}
}

func handleLocalConnection(localConn net.Conn, connectionID, token, apiURL string) {
	defer func() { _ = localConn.Close() }()

	wsConn, err := dialProxyWebSocket(apiURL, token, connectionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to API: %v\n", err)
		return
	}
	defer func() { _ = wsConn.Close() }()

	// Setup ping/pong to keep connection alive (prevent ALB timeout)
	_ = wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.SetPongHandler(func(string) error {
		_ = wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping sender (every 30 seconds)
	done := make(chan error, 3)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
					done <- err
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Forward data from local connection to WebSocket (Local App → API → Backend)
	go func() {
		for {
			buf := make([]byte, 32768) // 32KB buffer
			n, err := localConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					done <- fmt.Errorf("local read error: %w", err)
				}
				done <- nil
				return
			}

			// Send binary data over WebSocket
			if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				done <- fmt.Errorf("websocket write error: %w", err)
				return
			}
		}
	}()

	// Forward data from WebSocket to local connection (Backend → API → Local App)
	go func() {
		for {
			messageType, data, err := wsConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					done <- fmt.Errorf("websocket read error: %w", err)
				}
				done <- nil
				return
			}

			// Only process binary messages (skip ping/pong/text)
			if messageType == websocket.BinaryMessage {
				if _, err := localConn.Write(data); err != nil {
					done <- fmt.Errorf("local write error: %w", err)
					return
				}
			}
		}
	}()

	// Wait for any goroutine to finish or error
	err = <-done
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
	}
}

// validateToken checks if JWT token is still valid
func validateToken(token string) error {
	// Split JWT token (format: header.payload.signature)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid token format")
	}

	// Decode payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("failed to decode token: %w", err)
	}

	// Parse payload JSON
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("failed to parse token claims: %w", err)
	}

	// Check if token is expired
	if claims.Exp == 0 {
		return fmt.Errorf("token has no expiration")
	}

	expiryTime := time.Unix(claims.Exp, 0)
	if time.Now().After(expiryTime) {
		return fmt.Errorf("token expired at %s", expiryTime.Format(time.RFC3339))
	}

	return nil
}

// getUsernameFromToken extracts the username from a JWT token
func getUsernameFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode token: %w", err)
	}

	var claims struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to parse token claims: %w", err)
	}

	return claims.Username, nil
}
