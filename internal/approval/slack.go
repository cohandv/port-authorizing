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

// SlackProvider sends approval requests to Slack with interactive buttons
type SlackProvider struct {
	urlSource  config.ConfigSource
	resolver   *config.ConfigSourceResolver
	apiBaseURL string // Base URL of the API server for callbacks
	client     *http.Client
}

// NewSlackProvider creates a new Slack approval provider with dynamic URL resolution
func NewSlackProvider(urlSource config.ConfigSource, resolver *config.ConfigSourceResolver, apiBaseURL string) *SlackProvider {
	return &SlackProvider{
		urlSource:  urlSource,
		resolver:   resolver,
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
	// Resolve Slack webhook URL dynamically (with caching)
	webhookURL := s.urlSource.Value
	
	if s.resolver != nil && (s.urlSource.Type == config.ConfigSourceTypeConfigMap || s.urlSource.Type == config.ConfigSourceTypeSecret) {
		resolvedURL, exists, err := s.resolver.ResolveConfigSource(ctx, s.urlSource)
		if err != nil || !exists {
			log.Printf("Warning: Failed to resolve Slack webhook URL from %s (ref: %s): %v - using stored value", 
				s.urlSource.Type, s.urlSource.Ref, err)
		} else {
			webhookURL = resolvedURL
			log.Printf("DEBUG: Resolved Slack webhook URL from %s (ref: %s)", s.urlSource.Type, s.urlSource.Ref)
		}
	}
	
	// Check if Slack is disabled (empty URL)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL is empty - provider is disabled")
	}
	
	// Build Slack message with blocks
	message := s.buildSlackMessage(req)

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned non-success status: %d", resp.StatusCode)
	}

	return nil
}

// buildSlackMessage constructs a rich Slack message with interactive buttons
func (s *SlackProvider) buildSlackMessage(req *Request) slackMessage {
	// Build approval URLs
	approveURL := fmt.Sprintf("%s/api/approvals/%s/approve", s.apiBaseURL, req.ID)
	rejectURL := fmt.Sprintf("%s/api/approvals/%s/reject", s.apiBaseURL, req.ID)

	// Build title based on connection type
	title := s.buildTitle(req)

	// Build fields based on connection type
	fields := []slackTextBlock{
		{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*User:*\n%s", req.Username),
		},
		{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Connection:*\n%s", req.ConnectionName),
		},
		{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Type:*\n%s", req.ConnectionType),
		},
	}

	// Add type-specific fields
	fields = append(fields, s.getTypeSpecificFields(req)...)

	return slackMessage{
		Text: title,
		Blocks: []slackBlock{
			{
				Type: "header",
				Text: &slackTextBlock{
					Type: "plain_text",
					Text: "🔐 Command Approval Required for Environment: " + req.Environment,
				},
			},
			{
				Type: "section",
				Fields: fields,
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

// buildTitle creates a title based on connection type
func (s *SlackProvider) buildTitle(req *Request) string {
	switch req.ConnectionType {
	case "postgres":
		return fmt.Sprintf("🔐 Approval Required: SQL Query on %s", req.ConnectionName)
	case "redis":
		return fmt.Sprintf("🔐 Approval Required: Redis Command on %s", req.ConnectionName)
	case "http", "https":
		return fmt.Sprintf("🔐 Approval Required: %s %s", req.Method, req.Path)
	default:
		return fmt.Sprintf("🔐 Approval Required: %s", req.ConnectionName)
	}
}

// getTypeSpecificFields returns connection type-specific fields for Slack message
func (s *SlackProvider) getTypeSpecificFields(req *Request) []slackTextBlock {
	switch req.ConnectionType {
	case "postgres":
		if req.Query != "" {
			return []slackTextBlock{
				{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Query:*\n```%s```", req.Query),
				},
			}
		}
	case "redis":
		if req.Command != "" {
			return []slackTextBlock{
				{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Command:*\n`%s`", req.Command),
				},
			}
		}
	case "http", "https":
		fields := []slackTextBlock{}
		if req.Method != "" {
			methodEmoji := s.getMethodEmoji(req.Method)
			fields = append(fields, slackTextBlock{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Method:*\n%s %s", methodEmoji, req.Method),
			})
		}
		if req.Path != "" {
			fields = append(fields, slackTextBlock{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Path:*\n`%s`", req.Path),
			})
		}
		return fields
	}
	return []slackTextBlock{}
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
