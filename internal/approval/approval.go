package approval

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Decision represents an approval decision
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
	DecisionTimeout  Decision = "timeout"
)

// Request represents a request pending approval
type Request struct {
	ID           string
	Username     string
	ConnectionID string
	Method       string
	Path         string
	Body         string
	RequestedAt  time.Time
	Metadata     map[string]string
}

// Response represents an approval response
type Response struct {
	RequestID   string
	Decision    Decision
	ApprovedBy  string
	Reason      string
	RespondedAt time.Time
}

// Provider defines the interface for approval providers (Slack, webhook, etc)
type Provider interface {
	// SendApprovalRequest sends an approval request and returns immediately
	SendApprovalRequest(ctx context.Context, req *Request) error

	// GetProviderName returns the name of the provider
	GetProviderName() string
}

// Manager manages pending approval requests
type Manager struct {
	providers       []Provider
	pendingRequests map[string]*pendingRequest
	mu              sync.RWMutex
	defaultTimeout  time.Duration
	patterns        []*approvalPattern
}

type pendingRequest struct {
	Request  *Request
	Response chan *Response
	Timer    *time.Timer
}

type approvalPattern struct {
	Pattern  *regexp.Regexp
	Tags     []string
	TagMatch string // "all" or "any"
	Timeout  time.Duration
}

// NewManager creates a new approval manager
func NewManager(defaultTimeout time.Duration) *Manager {
	if defaultTimeout == 0 {
		defaultTimeout = 5 * time.Minute // Default 5 minute timeout
	}

	return &Manager{
		providers:       []Provider{},
		pendingRequests: make(map[string]*pendingRequest),
		defaultTimeout:  defaultTimeout,
		patterns:        []*approvalPattern{},
	}
}

// RegisterProvider registers an approval provider
func (m *Manager) RegisterProvider(provider Provider) {
	m.providers = append(m.providers, provider)
}

// AddApprovalPattern adds a pattern that requires approval
// Pattern format: "^METHOD /path/pattern$"
// Patterns are case-insensitive by default
func (m *Manager) AddApprovalPattern(pattern string, tags []string, tagMatch string, timeout time.Duration) error {
	// Make pattern case-insensitive (like whitelist patterns)
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return fmt.Errorf("invalid approval pattern: %w", err)
	}

	if timeout == 0 {
		timeout = m.defaultTimeout
	}

	if tagMatch == "" {
		tagMatch = "all"
	}

	m.patterns = append(m.patterns, &approvalPattern{
		Pattern:  re,
		Tags:     tags,
		TagMatch: tagMatch,
		Timeout:  timeout,
	})

	return nil
}

// RequiresApproval checks if a request requires approval
// If connectionTags is nil or empty, only patterns without tags are considered
func (m *Manager) RequiresApproval(method, path string, connectionTags []string) (bool, time.Duration) {
	if len(m.patterns) == 0 {
		return false, 0
	}

	requestStr := fmt.Sprintf("%s %s", method, path)

	for _, pattern := range m.patterns {
		// Check if request matches pattern
		if !pattern.Pattern.MatchString(requestStr) {
			continue
		}

		// If pattern has no tags, it applies to all connections
		if len(pattern.Tags) == 0 {
			return true, pattern.Timeout
		}

		// Check tag matching
		if m.matchesTags(connectionTags, pattern.Tags, pattern.TagMatch) {
			return true, pattern.Timeout
		}
	}

	return false, 0
}

// matchesTags checks if connection tags match the required tags
func (m *Manager) matchesTags(connectionTags, requiredTags []string, matchMode string) bool {
	if len(requiredTags) == 0 {
		return true // No tags required = match all
	}

	if len(connectionTags) == 0 {
		return false // Connection has no tags but pattern requires some
	}

	// Create a set of connection tags for faster lookup
	tagSet := make(map[string]bool)
	for _, tag := range connectionTags {
		tagSet[tag] = true
	}

	if matchMode == "any" {
		// Match if ANY required tag is present
		for _, reqTag := range requiredTags {
			if tagSet[reqTag] {
				return true
			}
		}
		return false
	}

	// Default: "all" - Match if ALL required tags are present
	for _, reqTag := range requiredTags {
		if !tagSet[reqTag] {
			return false
		}
	}
	return true
}

// RequestApproval sends an approval request to all providers and waits for a response
func (m *Manager) RequestApproval(ctx context.Context, req *Request, timeout time.Duration) (*Response, error) {
	if len(m.providers) == 0 {
		return nil, fmt.Errorf("no approval providers configured")
	}

	// Generate unique request ID
	req.ID = uuid.New().String()
	req.RequestedAt = time.Now()

	// Create response channel
	respChan := make(chan *Response, 1)

	// Create timeout timer
	timer := time.NewTimer(timeout)

	// Store pending request
	m.mu.Lock()
	m.pendingRequests[req.ID] = &pendingRequest{
		Request:  req,
		Response: respChan,
		Timer:    timer,
	}
	m.mu.Unlock()

	// Clean up after we're done
	defer func() {
		m.mu.Lock()
		delete(m.pendingRequests, req.ID)
		m.mu.Unlock()
		timer.Stop()
	}()

	// Send approval request to all providers
	for _, provider := range m.providers {
		if err := provider.SendApprovalRequest(ctx, req); err != nil {
			// Log error but continue with other providers
			fmt.Printf("Error sending approval request to %s: %v\n", provider.GetProviderName(), err)
		}
	}

	// Wait for response or timeout
	select {
	case response := <-respChan:
		return response, nil
	case <-timer.C:
		return &Response{
			RequestID:   req.ID,
			Decision:    DecisionTimeout,
			Reason:      "approval request timed out",
			RespondedAt: time.Now(),
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubmitApproval processes an approval response (called by callback endpoints)
func (m *Manager) SubmitApproval(requestID string, decision Decision, approvedBy, reason string) error {
	m.mu.Lock()
	pending, exists := m.pendingRequests[requestID]
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("approval request not found or already processed: %s", requestID)
	}

	response := &Response{
		RequestID:   requestID,
		Decision:    decision,
		ApprovedBy:  approvedBy,
		Reason:      reason,
		RespondedAt: time.Now(),
	}

	// Send response (non-blocking)
	select {
	case pending.Response <- response:
		return nil
	default:
		return fmt.Errorf("failed to deliver approval response")
	}
}

// GetPendingRequest retrieves a pending approval request by ID
func (m *Manager) GetPendingRequest(requestID string) (*Request, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pending, exists := m.pendingRequests[requestID]
	if !exists {
		return nil, fmt.Errorf("approval request not found: %s", requestID)
	}

	return pending.Request, nil
}

// GetPendingRequestsCount returns the number of pending approval requests
func (m *Manager) GetPendingRequestsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pendingRequests)
}
