package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// WebhookProvider sends approval requests to a generic webhook endpoint
type WebhookProvider struct {
	urlSource config.ConfigSource
	resolver  *config.ConfigSourceResolver
	client    *http.Client
}

// NewWebhookProvider creates a new webhook approval provider with dynamic URL resolution
func NewWebhookProvider(urlSource config.ConfigSource, resolver *config.ConfigSourceResolver) *WebhookProvider {
	return &WebhookProvider{
		urlSource: urlSource,
		resolver:  resolver,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// webhookPayload is the payload sent to the webhook
type webhookPayload struct {
	RequestID      string            `json:"request_id"`
	Environment    string            `json:"environment"`
	Username       string            `json:"username"`
	ConnectionID   string            `json:"connection_id"`
	ConnectionType string            `json:"connection_type"` // "postgres", "redis", "http", "https", "tcp"
	ConnectionName string            `json:"connection_name"` // Name of the connection
	Method         string            `json:"method,omitempty"`          // HTTP method (for HTTP/HTTPS)
	Path           string            `json:"path,omitempty"`            // HTTP path (for HTTP/HTTPS)
	Query          string            `json:"query,omitempty"`           // SQL query (for Postgres)
	Command        string            `json:"command,omitempty"`         // Redis command (for Redis)
	Body           string            `json:"body,omitempty"`
	RequestedAt    string            `json:"requested_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	ApprovalURL    string            `json:"approval_url"` // URL to approve/reject
}

// SendApprovalRequest sends an approval request to the webhook
func (w *WebhookProvider) SendApprovalRequest(ctx context.Context, req *Request) error {
	// Resolve webhook URL dynamically (with caching)
	webhookURL := w.urlSource.Value
	
	if w.resolver != nil && (w.urlSource.Type == config.ConfigSourceTypeConfigMap || w.urlSource.Type == config.ConfigSourceTypeSecret) {
		resolvedURL, exists, err := w.resolver.ResolveConfigSource(ctx, w.urlSource)
		if err != nil || !exists {
			log.Printf("Warning: Failed to resolve webhook URL from %s (ref: %s): %v - using stored value", 
				w.urlSource.Type, w.urlSource.Ref, err)
		} else {
			webhookURL = resolvedURL
			log.Printf("DEBUG: Resolved webhook URL from %s (ref: %s)", w.urlSource.Type, w.urlSource.Ref)
		}
	}
	
	// Check if webhook is disabled (empty URL)
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is empty - provider is disabled")
	}
	
	payload := webhookPayload{
		RequestID:      req.ID,
		Username:       req.Username,
		ConnectionID:   req.ConnectionID,
		ConnectionType: req.ConnectionType,
		ConnectionName: req.ConnectionName,
		Method:         req.Method,
		Path:           req.Path,
		Query:          req.Query,
		Command:        req.Command,
		Body:           req.Body,
		RequestedAt:    req.RequestedAt.Format(time.RFC3339),
		Metadata:       req.Metadata,
		// The approval URL should be constructed from the API base URL
		// For now, we'll include the request ID and expect the webhook to call back
		ApprovalURL: fmt.Sprintf("/api/approvals/%s", req.ID),
		Environment: req.Environment,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Port-Authorizing-Approval/1.0")

	resp, err := w.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// GetProviderName returns the provider name
func (w *WebhookProvider) GetProviderName() string {
	return "webhook"
}
