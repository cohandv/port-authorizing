package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackProvider sends approval requests to Slack with interactive buttons
type SlackProvider struct {
	webhookURL string
	apiBaseURL string // Base URL of the API server for callbacks
	client     *http.Client
}

// NewSlackProvider creates a new Slack approval provider
func NewSlackProvider(webhookURL, apiBaseURL string) *SlackProvider {
	return &SlackProvider{
		webhookURL: webhookURL,
		apiBaseURL: apiBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// slackMessage represents a Slack message with blocks
type slackMessage struct {
	Text        string        `json:"text"`
	Blocks      []slackBlock  `json:"blocks"`
	Attachments []interface{} `json:"attachments,omitempty"`
}

type slackBlock struct {
	Type      string                 `json:"type"`
	Text      *slackTextBlock        `json:"text,omitempty"`
	Fields    []slackTextBlock       `json:"fields,omitempty"`
	Elements  []interface{}          `json:"elements,omitempty"`
	Accessory map[string]interface{} `json:"accessory,omitempty"`
}

type slackTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackButton struct {
	Type  string         `json:"type"`
	Text  slackTextBlock `json:"text"`
	Value string         `json:"value"`
	URL   string         `json:"url,omitempty"`
	Style string         `json:"style,omitempty"` // primary, danger
}

// SendApprovalRequest sends an approval request to Slack with interactive buttons
func (s *SlackProvider) SendApprovalRequest(ctx context.Context, req *Request) error {
	// Build Slack message with blocks
	message := s.buildSlackMessage(req)

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Slack webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// buildSlackMessage constructs a rich Slack message with interactive buttons
func (s *SlackProvider) buildSlackMessage(req *Request) slackMessage {
	// Determine emoji based on HTTP method
	methodEmoji := s.getMethodEmoji(req.Method)

	// Build approval URLs
	approveURL := fmt.Sprintf("%s/api/approvals/%s/approve", s.apiBaseURL, req.ID)
	rejectURL := fmt.Sprintf("%s/api/approvals/%s/reject", s.apiBaseURL, req.ID)

	return slackMessage{
		Text: fmt.Sprintf("🔐 Approval Required: %s %s", req.Method, req.Path),
		Blocks: []slackBlock{
			{
				Type: "header",
				Text: &slackTextBlock{
					Type: "plain_text",
					Text: "🔐 Command Approval Required",
				},
			},
			{
				Type: "section",
				Fields: []slackTextBlock{
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("*User:*\n%s", req.Username),
					},
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("*Connection:*\n%s", req.ConnectionID),
					},
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("*Method:*\n%s %s", methodEmoji, req.Method),
					},
					{
						Type: "mrkdwn",
						Text: fmt.Sprintf("*Path:*\n`%s`", req.Path),
					},
				},
			},
			{
				Type: "section",
				Text: &slackTextBlock{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Time:*\n%s", req.RequestedAt.Format(time.RFC1123)),
				},
			},
			{
				Type: "divider",
			},
			{
				Type: "section",
				Text: &slackTextBlock{
					Type: "mrkdwn",
					Text: "*Click a button below to approve or reject this request:*",
				},
			},
			{
				Type: "actions",
				Elements: []interface{}{
					slackButton{
						Type: "button",
						Text: slackTextBlock{
							Type: "plain_text",
							Text: "✅ Approve",
						},
						URL:   approveURL,
						Style: "primary",
						Value: "approve",
					},
					slackButton{
						Type: "button",
						Text: slackTextBlock{
							Type: "plain_text",
							Text: "❌ Reject",
						},
						URL:   rejectURL,
						Style: "danger",
						Value: "reject",
					},
				},
			},
			{
				Type: "context",
				Elements: []interface{}{
					slackTextBlock{
						Type: "mrkdwn",
						Text: fmt.Sprintf("Request ID: `%s` | This request will timeout in 5 minutes", req.ID),
					},
				},
			},
		},
	}
}

// getMethodEmoji returns an emoji for the HTTP method
func (s *SlackProvider) getMethodEmoji(method string) string {
	switch method {
	case "GET":
		return "📖"
	case "POST":
		return "➕"
	case "PUT":
		return "✏️"
	case "PATCH":
		return "🔧"
	case "DELETE":
		return "🗑️"
	default:
		return "📡"
	}
}

// GetProviderName returns the provider name
func (s *SlackProvider) GetProviderName() string {
	return "slack"
}
