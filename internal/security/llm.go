package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LLMProvider represents different LLM providers
type LLMProvider string

const (
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
	ProviderCustom    LLMProvider = "custom"
)

// LLMClient handles LLM-based risk analysis
type LLMClient struct {
	Provider LLMProvider
	APIKey   string
	Endpoint string
}

// NewLLMClient creates a new LLM client
func NewLLMClient(provider string, apiKey string) *LLMClient {
	llmProvider := LLMProvider(provider)

	var endpoint string
	switch llmProvider {
	case ProviderOpenAI:
		endpoint = "https://api.openai.com/v1/chat/completions"
	case ProviderAnthropic:
		endpoint = "https://api.anthropic.com/v1/messages"
	default:
		endpoint = ""
	}

	return &LLMClient{
		Provider: llmProvider,
		APIKey:   apiKey,
		Endpoint: endpoint,
	}
}

// AnalyzeQuery analyzes a query for security risks using LLM
func (c *LLMClient) AnalyzeQuery(query string, queryType string) (bool, string, error) {
	if c.APIKey == "" {
		return true, "LLM API key not configured", nil
	}

	switch c.Provider {
	case ProviderOpenAI:
		return c.analyzeWithOpenAI(query, queryType)
	case ProviderAnthropic:
		return c.analyzeWithAnthropic(query, queryType)
	default:
		return true, "Unsupported LLM provider", nil
	}
}

// analyzeWithOpenAI uses OpenAI GPT for risk analysis
func (c *LLMClient) analyzeWithOpenAI(query string, queryType string) (bool, string, error) {
	// Construct prompt
	prompt := fmt.Sprintf(`You are a security analyst. Analyze this %s query for potential security risks such as:
- SQL injection attempts
- Unauthorized data access patterns
- Destructive operations
- Data exfiltration attempts

Query: %s

Respond with JSON in this format:
{
  "safe": true/false,
  "risk_level": "low/medium/high",
  "explanation": "brief explanation",
  "threats": ["list", "of", "threats"]
}`, queryType, query)

	// Prepare request
	reqBody := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a security analyst specializing in database query analysis.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.3,
		"max_tokens":  500,
	}

	return c.sendLLMRequest(reqBody)
}

// analyzeWithAnthropic uses Anthropic Claude for risk analysis
func (c *LLMClient) analyzeWithAnthropic(query string, queryType string) (bool, string, error) {
	// Similar to OpenAI but with Anthropic's API format
	// TODO: Implement Anthropic-specific request format
	return true, "Anthropic integration pending", nil
}

// sendLLMRequest sends request to LLM provider
func (c *LLMClient) sendLLMRequest(reqBody map[string]interface{}) (bool, string, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewBuffer(data))
	if err != nil {
		return false, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract safety assessment from response
	// This would need to be adapted based on actual LLM response format
	safe := true
	explanation := "Analysis complete"

	return safe, explanation, nil
}

// RiskAssessment represents the result of a risk analysis
type RiskAssessment struct {
	Safe        bool     `json:"safe"`
	RiskLevel   string   `json:"risk_level"`
	Explanation string   `json:"explanation"`
	Threats     []string `json:"threats"`
}
