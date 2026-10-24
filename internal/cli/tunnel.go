package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// fetchConnections retrieves the list of connections visible to the
// authenticated user from the API server.
func fetchConnections(apiURL, token string) ([]connectionInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/connections", apiURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list connections: %s", string(body))
	}

	var connections []connectionInfo
	if err := json.Unmarshal(body, &connections); err != nil {
		return nil, fmt.Errorf("failed to parse connections: %w", err)
	}
	return connections, nil
}

// connectToBackend performs the connect handshake for a named connection,
// returning the resulting proxy session details.
func connectToBackend(apiURL, token, name string) (*connectResponse, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/connect/%s", apiURL, name), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("connection failed: %s", string(body))
	}

	var connResp connectResponse
	if err := json.Unmarshal(body, &connResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &connResp, nil
}

// dialProxyWebSocket opens the websocket tunnel to the proxy for an
// already-established connectionID, bounded by a 10s handshake timeout.
// Shared by the per-local-connection dial in handleLocalConnection and by
// the pre-flight reachability check in runClientBinding, so both exercise
// exactly the same dial path.
func dialProxyWebSocket(apiURL, token, connectionID string) (*websocket.Conn, error) {
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = fmt.Sprintf("%s/api/proxy/%s", wsURL, connectionID)

	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing websocket URL: %w", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	wsConn, resp, err := dialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("HTTP %d: %w", resp.StatusCode, err)
		}
		return nil, err
	}
	return wsConn, nil
}
