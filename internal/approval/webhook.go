package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookProvider sends approval requests to a generic webhook endpoint
type WebhookProvider struct {
	webhookURL string
	client     *http.Client
}

// NewWebhookProvider creates a new webhook approval provider
func NewWebhookProvider(webhookURL string) *WebhookProvider {
	return &WebhookProvider{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// webhookPayload is the payload sent to the webhook
type webhookPayload struct {
	RequestID    string            `json:"request_id"`
	Username     string            `json:"username"`
	ConnectionID string            `json:"connection_id"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Body         string            `json:"body,omitempty"`
	RequestedAt  string            `json:"requested_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ApprovalURL  string            `json:"approval_url"` // URL to approve/reject
}

// SendApprovalRequest sends an approval request to the webhook
func (w *WebhookProvider) SendApprovalRequest(ctx context.Context, req *Request) error {
	payload := webhookPayload{
		RequestID:    req.ID,
		Username:     req.Username,
		ConnectionID: req.ConnectionID,
		Method:       req.Method,
		Path:         req.Path,
		Body:         req.Body,
		RequestedAt:  req.RequestedAt.Format(time.RFC3339),
		Metadata:     req.Metadata,
		// The approval URL should be constructed from the API base URL
		// For now, we'll include the request ID and expect the webhook to call back
		ApprovalURL: fmt.Sprintf("/api/approvals/%s", req.ID),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", w.webhookURL, bytes.NewBuffer(jsonData))
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
