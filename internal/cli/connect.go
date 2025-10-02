package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [connection-name]",
	Short: "Connect to a service via proxy",
	Long:  "Establish a local proxy connection to a remote service through the API",
	Args:  cobra.ExactArgs(1),
	RunE:  runConnect,
}

var (
	localPort int
	duration  string
)

func init() {
	connectCmd.Flags().IntVarP(&localPort, "local-port", "l", 0, "Local port to listen on (required)")
	connectCmd.Flags().StringVarP(&duration, "duration", "d", "1h", "Connection duration (e.g., 30m, 1h, 2h)")
	connectCmd.MarkFlagRequired("local-port")
}

type connectRequest struct {
	Duration string `json:"duration"`
}

type connectResponse struct {
	ConnectionID string `json:"connection_id"`
	ExpiresAt    string `json:"expires_at"`
	ProxyURL     string `json:"proxy_url"`
}

func runConnect(cmd *cobra.Command, args []string) error {
	connectionName := args[0]

	token, err := loadToken()
	if err != nil {
		return fmt.Errorf("not logged in. Please run 'login' first: %w", err)
	}

	// Request connection from API
	reqBody := connectRequest{Duration: duration}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/connect/%s", apiURL, connectionName), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

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
	fmt.Println("\nStarting local proxy server...")

	// Start local proxy server
	if err := startLocalProxy(localPort, connResp.ConnectionID, token); err != nil {
		return fmt.Errorf("failed to start local proxy: %w", err)
	}

	return nil
}

func startLocalProxy(port int, connectionID, token string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	defer listener.Close()

	fmt.Printf("✓ Proxy server listening on localhost:%d\n", port)
	fmt.Println("Press Ctrl+C to stop")

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

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
	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			return nil
		case conn := <-connChan:
			go handleLocalConnection(conn, connectionID, token)
		}
	}
}

func handleLocalConnection(localConn net.Conn, connectionID, token string) {
	defer localConn.Close()

	// Read data from local connection
	buf := make([]byte, 32*1024)
	n, err := localConn.Read(buf)
	if err != nil {
		fmt.Printf("Error reading from local connection: %v\n", err)
		return
	}

	// Forward to API proxy
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/proxy/%s", apiURL, connectionID), bytes.NewBuffer(buf[:n]))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error forwarding request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Forward response back to local connection
	io.Copy(localConn, resp.Body)
}

func loadToken() (string, error) {
	configPath := os.ExpandEnv(configPath)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}

	token, ok := config["token"].(string)
	if !ok {
		return "", fmt.Errorf("token not found in config")
	}

	return token, nil
}
