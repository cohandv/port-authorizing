package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [connection-name]",
	Short: "Connect to a service via proxy",
	Long:  "Establish a local proxy connection to a remote service through the API. Duration is controlled by API server configuration.",
	Args:  cobra.ExactArgs(1),
	RunE:  runConnect,
}

var (
	localPort int
)

func init() {
	connectCmd.Flags().IntVarP(&localPort, "local-port", "l", 0, "Local port to listen on (required)")
	_ = connectCmd.MarkFlagRequired("local-port")
}

type connectResponse struct {
	ConnectionID string `json:"connection_id"`
	ExpiresAt    string `json:"expires_at"`
	ProxyURL     string `json:"proxy_url"`
	Type         string `json:"type,omitempty"`     // Connection type (postgres, http, tcp)
	Database     string `json:"database,omitempty"` // For postgres connections
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Get current context
	ctx, err := GetCurrentContext()
	if err != nil {
		return fmt.Errorf("not logged in: %w. Please run 'login' first", err)
	}

	apiURL := ctx.APIURL
	token := ctx.Token

	// Allow override from command line flag
	if flagURL, _ := cmd.Root().PersistentFlags().GetString("api-url"); flagURL != "" {
		apiURL = flagURL
	}

	connectionName := args[0]

	// Validate token is still valid
	if err := validateToken(token); err != nil {
		return fmt.Errorf("authentication expired or invalid: %w\nPlease login again: ./port-authorizing-cli login", err)
	}

	// Request connection from API (duration is set by server config)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/connect/%s", apiURL, connectionName), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("connection failed: %s", string(body))
	}

	var connResp connectResponse
	if err := json.Unmarshal(body, &connResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("✓ Connection established: %s\n", connectionName)
	fmt.Printf("  Connection ID: %s\n", connResp.ConnectionID)
	fmt.Printf("  Expires at: %s\n", connResp.ExpiresAt)
	fmt.Printf("  Local port: %d\n", localPort)
	fmt.Printf("  Server will auto-disconnect at expiry\n")

	// Show connection examples based on service type
	if connResp.Type == "postgres" {
		// Extract username from token for display
		username, _ := getUsernameFromToken(token)
		fmt.Printf("\n📝 PostgreSQL Connection Info:\n")
		fmt.Printf("  ⚠️  IMPORTANT: You MUST connect with your authenticated username\n")
		fmt.Printf("  • Username: %s (required - no other username will work)\n", username)
		fmt.Printf("  • Password: <your API password>\n")
		fmt.Printf("  • Database: %s\n", connResp.Database)
		fmt.Printf("\n  Connection string:\n")
		fmt.Printf("  psql -h localhost -p %d -U %s -d %s\n", localPort, username, connResp.Database)
		fmt.Printf("  or\n")
		fmt.Printf("  postgresql://%s:<password>@localhost:%d/%s\n", username, localPort, connResp.Database)
		fmt.Printf("\n  🔒 Backend credentials are hidden - managed by server.\n")
		fmt.Printf("  🔒 All queries logged with your username.\n")
	}

	fmt.Println("\nStarting local proxy server...")

	// Start local proxy server with expiry time
	if err := startLocalProxy(localPort, connResp.ConnectionID, token, connResp.ExpiresAt, apiURL); err != nil {
		return fmt.Errorf("failed to start local proxy: %w", err)
	}

	return nil
}

func startLocalProxy(port int, connectionID, token string, expiresAt string, apiURL string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	defer func() { _ = listener.Close() }()

	fmt.Printf("✓ Proxy server listening on localhost:%d\n", port)
	fmt.Printf("Connection will expire at: %s\n", expiresAt)
	fmt.Println("Press Ctrl+C to stop")

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Parse expiry time
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		fmt.Printf("Warning: could not parse expiry time: %v\n", err)
	} else {
		// Start timeout monitor
		go func() {
			timeUntilExpiry := time.Until(expiry)
			if timeUntilExpiry > 0 {
				<-time.After(timeUntilExpiry)
				fmt.Printf("\n⏱  Connection timeout reached at %s\n", expiresAt)
				fmt.Println("Server has disconnected the connection.")
				fmt.Println("Run 'connect' again to establish a new connection.")
				os.Exit(0)
			}
		}()
	}

	// Accept connections in goroutine
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

	// Main loop
	// Create closure to capture apiURL
	handleConnection := func(conn net.Conn) {
		handleLocalConnection(conn, connectionID, token, apiURL)
	}

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			return nil
		case conn := <-connChan:
			go handleConnection(conn)
		}
	}
}

func handleLocalConnection(localConn net.Conn, connectionID, token, apiURL string) {
	defer func() { _ = localConn.Close() }()

	// Convert HTTP URL to WebSocket URL
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = fmt.Sprintf("%s/api/proxy/%s", wsURL, connectionID)

	// Parse URL and add auth header
	u, err := url.Parse(wsURL)
	if err != nil {
		fmt.Printf("Error parsing WebSocket URL: %v\n", err)
		return
	}

	// Create WebSocket connection with auth header
	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	// Establish WebSocket connection to API server
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	wsConn, resp, err := dialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			fmt.Printf("Error connecting to API (HTTP %d): %v\n", resp.StatusCode, err)
		} else {
			fmt.Printf("Error connecting to API: %v\n", err)
		}
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
		fmt.Printf("Connection error: %v\n", err)
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
