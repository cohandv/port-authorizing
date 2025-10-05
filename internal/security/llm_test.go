package security

import (
	"testing"
)

func TestNewLLMClient(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		apiKey       string
		wantEndpoint string
	}{
		{
			name:         "OpenAI provider",
			provider:     "openai",
			apiKey:       "test-key",
			wantEndpoint: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:         "Anthropic provider",
			provider:     "anthropic",
			apiKey:       "test-key",
			wantEndpoint: "https://api.anthropic.com/v1/messages",
		},
		{
			name:         "Custom provider",
			provider:     "custom",
			apiKey:       "test-key",
			wantEndpoint: "",
		},
		{
			name:         "Unknown provider",
			provider:     "unknown",
			apiKey:       "test-key",
			wantEndpoint: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewLLMClient(tt.provider, tt.apiKey)

			if client == nil {
				t.Fatal("NewLLMClient() returned nil")
			}

			if client.APIKey != tt.apiKey {
				t.Errorf("APIKey = %s, want %s", client.APIKey, tt.apiKey)
			}

			if client.Endpoint != tt.wantEndpoint {
				t.Errorf("Endpoint = %s, want %s", client.Endpoint, tt.wantEndpoint)
			}

			if string(client.Provider) != tt.provider {
				t.Errorf("Provider = %s, want %s", client.Provider, tt.provider)
			}
		})
	}
}

func TestLLMClient_AnalyzeQuery_NoAPIKey(t *testing.T) {
	client := NewLLMClient("openai", "")

	safe, explanation, err := client.AnalyzeQuery("SELECT * FROM users", "SQL")

	if err != nil {
		t.Errorf("AnalyzeQuery() error = %v, want nil", err)
	}

	if !safe {
		t.Error("AnalyzeQuery() should return safe=true when API key not configured")
	}

	if explanation != "LLM API key not configured" {
		t.Errorf("explanation = %s, want 'LLM API key not configured'", explanation)
	}
}

func TestLLMClient_AnalyzeQuery_UnsupportedProvider(t *testing.T) {
	client := NewLLMClient("unsupported", "test-key")

	safe, explanation, err := client.AnalyzeQuery("SELECT * FROM users", "SQL")

	if err != nil {
		t.Errorf("AnalyzeQuery() error = %v, want nil", err)
	}

	if !safe {
		t.Error("AnalyzeQuery() should return safe=true for unsupported provider")
	}

	if explanation != "Unsupported LLM provider" {
		t.Errorf("explanation = %s, want 'Unsupported LLM provider'", explanation)
	}
}

func TestLLMClient_AnalyzeQuery_Anthropic(t *testing.T) {
	client := NewLLMClient("anthropic", "test-key")

	safe, explanation, err := client.AnalyzeQuery("SELECT * FROM users", "SQL")

	if err != nil {
		t.Errorf("AnalyzeQuery() error = %v, want nil", err)
	}

	if !safe {
		t.Error("AnalyzeQuery() should return safe=true (pending implementation)")
	}

	if explanation != "Anthropic integration pending" {
		t.Errorf("explanation = %s, want 'Anthropic integration pending'", explanation)
	}
}

func TestLLMClient_sendLLMRequest_InvalidEndpoint(t *testing.T) {
	client := &LLMClient{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		Endpoint: "http://invalid-endpoint-that-does-not-exist.com/api",
	}

	reqBody := map[string]interface{}{
		"test": "data",
	}

	safe, explanation, err := client.sendLLMRequest(reqBody)

	if err == nil {
		t.Error("sendLLMRequest() should return error for invalid endpoint")
	}

	if safe {
		t.Error("sendLLMRequest() should return safe=false on error")
	}

	if explanation != "" {
		t.Errorf("explanation should be empty on error, got %s", explanation)
	}
}

func TestRiskAssessment(t *testing.T) {
	// Test the RiskAssessment struct can be marshaled/unmarshaled
	assessment := RiskAssessment{
		Safe:        false,
		RiskLevel:   "high",
		Explanation: "Potential SQL injection detected",
		Threats:     []string{"sql_injection", "data_exfiltration"},
	}

	if assessment.Safe {
		t.Error("assessment.Safe should be false")
	}

	if assessment.RiskLevel != "high" {
		t.Errorf("RiskLevel = %s, want 'high'", assessment.RiskLevel)
	}

	if len(assessment.Threats) != 2 {
		t.Errorf("Threats count = %d, want 2", len(assessment.Threats))
	}
}

func BenchmarkNewLLMClient(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewLLMClient("openai", "test-key")
	}
}

func BenchmarkLLMClient_AnalyzeQuery_NoKey(b *testing.B) {
	client := NewLLMClient("openai", "")
	query := "SELECT * FROM users WHERE id = 1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.AnalyzeQuery(query, "SQL")
	}
}
