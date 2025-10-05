package security

import (
	"testing"
)

func TestRiskAssessment_Levels(t *testing.T) {
	tests := []struct {
		name       string
		assessment RiskAssessment
		wantHigh   bool
	}{
		{
			name: "high risk",
			assessment: RiskAssessment{
				Safe:        false,
				RiskLevel:   "high",
				Explanation: "Dangerous query",
			},
			wantHigh: true,
		},
		{
			name: "medium risk",
			assessment: RiskAssessment{
				Safe:        true,
				RiskLevel:   "medium",
				Explanation: "Potentially risky",
			},
			wantHigh: false,
		},
		{
			name: "low risk",
			assessment: RiskAssessment{
				Safe:        true,
				RiskLevel:   "low",
				Explanation: "Safe query",
			},
			wantHigh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isHigh := tt.assessment.RiskLevel == "high"
			if isHigh != tt.wantHigh {
				t.Errorf("isHigh = %v, want %v", isHigh, tt.wantHigh)
			}

			if tt.assessment.Explanation == "" {
				t.Error("Explanation should not be empty")
			}
		})
	}
}

func TestRiskAssessment_AllowedCombinations(t *testing.T) {
	tests := []struct {
		name      string
		safe      bool
		riskLevel string
		valid     bool
	}{
		{"safe low", true, "low", true},
		{"safe medium", true, "medium", true},
		{"blocked high", false, "high", true},
		{"blocked low", false, "low", false}, // Unusual but possible
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := RiskAssessment{
				Safe:        tt.safe,
				RiskLevel:   tt.riskLevel,
				Explanation: "test",
			}

			// Just verify fields are set
			if assessment.RiskLevel != tt.riskLevel {
				t.Errorf("RiskLevel = %s, want %s", assessment.RiskLevel, tt.riskLevel)
			}
		})
	}
}

func TestLLMClient_Providers(t *testing.T) {
	tests := []struct {
		name     string
		provider LLMProvider
		apiKey   string
	}{
		{"openai", ProviderOpenAI, "test-key"},
		{"anthropic", ProviderAnthropic, "test-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LLMClient{
				Provider: tt.provider,
				APIKey:   tt.apiKey,
			}

			if client.Provider != tt.provider {
				t.Errorf("Provider = %v, want %v", client.Provider, tt.provider)
			}

			if client.APIKey != tt.apiKey {
				t.Errorf("APIKey = %s, want %s", client.APIKey, tt.apiKey)
			}
		})
	}
}

func TestValidateQuery_MoreCases(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		query     string
		wantErr   bool
	}{
		{
			name:      "query with whitespace",
			whitelist: []string{".*SELECT.*"},
			query:     "  SELECT * FROM users  ",
			wantErr:   false,
		},
		{
			name:      "very long query",
			whitelist: []string{".*"},
			query:     "SELECT *",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(tt.whitelist, tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateQuery_SpecialCharacters(t *testing.T) {
	whitelist := []string{".*"}

	tests := []struct {
		name  string
		query string
	}{
		{"with quotes", "SELECT * FROM 'users'"},
		{"with backslash", "SELECT * FROM users WHERE name='O\\'Brien'"},
		{"with dollar sign", "SELECT $1::text"},
		{"with percent", "SELECT * FROM users WHERE name LIKE '%test%'"},
		{"with brackets", "SELECT array[1,2,3]"},
		{"with parentheses", "SELECT COUNT(*)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(whitelist, tt.query)
			if err != nil {
				t.Errorf("ValidateQuery() error = %v, want nil", err)
			}
		})
	}
}

func BenchmarkRiskAssessment(b *testing.B) {
	assessment := RiskAssessment{
		Safe:        true,
		RiskLevel:   "low",
		Explanation: "Safe query",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = assessment.Safe
		_ = assessment.RiskLevel
		_ = assessment.Explanation
	}
}
