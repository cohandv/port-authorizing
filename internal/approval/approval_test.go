package approval

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager(5 * time.Minute)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.defaultTimeout != 5*time.Minute {
		t.Errorf("defaultTimeout = %v, want %v", mgr.defaultTimeout, 5*time.Minute)
	}
}

func TestNewManager_DefaultTimeout(t *testing.T) {
	mgr := NewManager(0)

	if mgr.defaultTimeout != 5*time.Minute {
		t.Errorf("defaultTimeout = %v, want %v (default)", mgr.defaultTimeout, 5*time.Minute)
	}
}

func TestManager_RegisterProvider(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	provider := &mockProvider{name: "test"}

	mgr.RegisterProvider(provider)

	if len(mgr.providers) != 1 {
		t.Errorf("providers length = %d, want 1", len(mgr.providers))
	}
}

func TestManager_AddApprovalPattern(t *testing.T) {
	mgr := NewManager(5 * time.Minute)

	tests := []struct {
		name     string
		pattern  string
		tags     []string
		tagMatch string
		timeout  time.Duration
		wantErr  bool
	}{
		{
			name:     "valid pattern without tags",
			pattern:  "^DELETE /.*",
			tags:     nil,
			tagMatch: "",
			timeout:  5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "valid pattern with tags",
			pattern:  "^DELETE /.*",
			tags:     []string{"env:production"},
			tagMatch: "all",
			timeout:  5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "valid pattern with zero timeout uses default",
			pattern:  "^POST /.*",
			tags:     nil,
			tagMatch: "",
			timeout:  0,
			wantErr:  false,
		},
		{
			name:     "invalid regex pattern",
			pattern:  "[invalid",
			tags:     nil,
			tagMatch: "",
			timeout:  5 * time.Minute,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.AddApprovalPattern(tt.pattern, tt.tags, tt.tagMatch, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddApprovalPattern() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManager_RequiresApproval(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	_ = mgr.AddApprovalPattern("^DELETE /.*", nil, "", 3*time.Minute)
	_ = mgr.AddApprovalPattern("^POST /admin/.*", []string{"env:production"}, "all", 10*time.Minute)
	_ = mgr.AddApprovalPattern("^PUT /.*", []string{"team:backend", "env:production"}, "any", 5*time.Minute)

	tests := []struct {
		name           string
		method         string
		path           string
		connectionTags []string
		wantRequired   bool
		wantTimeout    time.Duration
	}{
		{
			name:           "DELETE matches pattern without tags",
			method:         "DELETE",
			path:           "/api/users/1",
			connectionTags: nil,
			wantRequired:   true,
			wantTimeout:    3 * time.Minute,
		},
		{
			name:           "POST to admin on production matches",
			method:         "POST",
			path:           "/admin/settings",
			connectionTags: []string{"env:production"},
			wantRequired:   true,
			wantTimeout:    10 * time.Minute,
		},
		{
			name:           "POST to admin on dev does not match",
			method:         "POST",
			path:           "/admin/settings",
			connectionTags: []string{"env:dev"},
			wantRequired:   false,
			wantTimeout:    0,
		},
		{
			name:           "PUT with any tag match - has team:backend",
			method:         "PUT",
			path:           "/api/users",
			connectionTags: []string{"team:backend"},
			wantRequired:   true,
			wantTimeout:    5 * time.Minute,
		},
		{
			name:           "PUT with any tag match - has env:production",
			method:         "PUT",
			path:           "/api/users",
			connectionTags: []string{"env:production"},
			wantRequired:   true,
			wantTimeout:    5 * time.Minute,
		},
		{
			name:           "PUT without matching tags",
			method:         "PUT",
			path:           "/api/users",
			connectionTags: []string{"env:dev"},
			wantRequired:   false,
			wantTimeout:    0,
		},
		{
			name:           "GET does not match",
			method:         "GET",
			path:           "/api/users",
			connectionTags: nil,
			wantRequired:   false,
			wantTimeout:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			required, timeout := mgr.RequiresApproval(tt.method, tt.path, tt.connectionTags)
			if required != tt.wantRequired {
				t.Errorf("RequiresApproval() required = %v, want %v", required, tt.wantRequired)
			}
			if timeout != tt.wantTimeout {
				t.Errorf("RequiresApproval() timeout = %v, want %v", timeout, tt.wantTimeout)
			}
		})
	}
}

func TestManager_RequestApproval_NoProviders(t *testing.T) {
	mgr := NewManager(5 * time.Minute)

	req := &Request{
		Username: "alice",
		Method:   "DELETE",
		Path:     "/api/users/1",
	}

	ctx := context.Background()
	_, err := mgr.RequestApproval(ctx, req, 1*time.Second)

	if err == nil {
		t.Error("RequestApproval() expected error with no providers, got nil")
	}
}

func TestManager_RequestApproval_Timeout(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	provider := &mockProvider{name: "test", delay: 2 * time.Second}
	mgr.RegisterProvider(provider)

	req := &Request{
		Username: "alice",
		Method:   "DELETE",
		Path:     "/api/users/1",
	}

	ctx := context.Background()
	resp, err := mgr.RequestApproval(ctx, req, 100*time.Millisecond)

	if err != nil {
		t.Fatalf("RequestApproval() unexpected error: %v", err)
	}

	if resp.Decision != DecisionTimeout {
		t.Errorf("RequestApproval() decision = %v, want %v", resp.Decision, DecisionTimeout)
	}
}

func TestManager_SubmitApproval(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	provider := &mockProvider{name: "test"}
	mgr.RegisterProvider(provider)

	req := &Request{
		Username: "alice",
		Method:   "DELETE",
		Path:     "/api/users/1",
	}

	ctx := context.Background()

	// Start approval request in goroutine
	respChan := make(chan *Response)
	go func() {
		resp, err := mgr.RequestApproval(ctx, req, 5*time.Second)
		if err != nil {
			t.Errorf("RequestApproval() error = %v", err)
			return
		}
		respChan <- resp
	}()

	// Wait a bit for the request to be registered
	time.Sleep(100 * time.Millisecond)

	// Get the request ID safely after it's been set
	mgr.mu.RLock()
	var requestID string
	for id := range mgr.pendingRequests {
		requestID = id
		break
	}
	mgr.mu.RUnlock()

	if requestID == "" {
		t.Fatal("No pending request found")
	}

	// Submit approval
	err := mgr.SubmitApproval(requestID, DecisionApproved, "bob", "looks good")
	if err != nil {
		t.Fatalf("SubmitApproval() error = %v", err)
	}

	// Get response
	select {
	case resp := <-respChan:
		if resp.Decision != DecisionApproved {
			t.Errorf("Decision = %v, want %v", resp.Decision, DecisionApproved)
		}
		if resp.ApprovedBy != "bob" {
			t.Errorf("ApprovedBy = %v, want bob", resp.ApprovedBy)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for approval response")
	}
}

func TestManager_SubmitApproval_NotFound(t *testing.T) {
	mgr := NewManager(5 * time.Minute)

	err := mgr.SubmitApproval("nonexistent-id", DecisionApproved, "bob", "test")

	if err == nil {
		t.Error("SubmitApproval() expected error for nonexistent ID, got nil")
	}
}

func TestManager_GetPendingRequest(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	provider := &mockProvider{name: "test", delay: 10 * time.Second}
	mgr.RegisterProvider(provider)

	req := &Request{
		Username: "alice",
		Method:   "DELETE",
		Path:     "/api/users/1",
	}

	ctx := context.Background()

	// Start approval request in goroutine
	go func() {
		mgr.RequestApproval(ctx, req, 30*time.Second)
	}()

	// Wait for request to be registered
	time.Sleep(100 * time.Millisecond)

	// Get the request ID safely after it's been set
	mgr.mu.RLock()
	var requestID string
	for id := range mgr.pendingRequests {
		requestID = id
		break
	}
	mgr.mu.RUnlock()

	if requestID == "" {
		t.Fatal("No pending request found")
	}

	// Get pending request
	pending, err := mgr.GetPendingRequest(requestID)
	if err != nil {
		t.Fatalf("GetPendingRequest() error = %v", err)
	}

	if pending.Username != "alice" {
		t.Errorf("Username = %v, want alice", pending.Username)
	}
}

func TestManager_GetPendingRequestsCount(t *testing.T) {
	mgr := NewManager(5 * time.Minute)
	provider := &mockProvider{name: "test", delay: 10 * time.Second}
	mgr.RegisterProvider(provider)

	ctx := context.Background()

	// Start 3 approval requests
	for i := 0; i < 3; i++ {
		req := &Request{
			Username: "alice",
			Method:   "DELETE",
			Path:     "/api/users/1",
		}
		go func() {
			mgr.RequestApproval(ctx, req, 30*time.Second)
		}()
	}

	// Wait for requests to be registered
	time.Sleep(200 * time.Millisecond)

	count := mgr.GetPendingRequestsCount()
	if count != 3 {
		t.Errorf("GetPendingRequestsCount() = %d, want 3", count)
	}
}

// mockProvider is a mock approval provider for testing
type mockProvider struct {
	name  string
	delay time.Duration
}

func (m *mockProvider) SendApprovalRequest(ctx context.Context, req *Request) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return nil
}

func (m *mockProvider) GetProviderName() string {
	return m.name
}

func BenchmarkManager_RequiresApproval(b *testing.B) {
	mgr := NewManager(5 * time.Minute)
	mgr.AddApprovalPattern("^DELETE /.*", nil, "", 5*time.Minute)
	mgr.AddApprovalPattern("^POST /admin/.*", []string{"env:production"}, "all", 5*time.Minute)
	mgr.AddApprovalPattern("^PUT /api/users/.*", nil, "", 5*time.Minute)

	tags := []string{"env:production"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.RequiresApproval("DELETE", "/api/users/123", tags)
	}
}
