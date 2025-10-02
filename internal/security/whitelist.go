package security

import (
	"fmt"
	"regexp"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// Validator handles security validation
type Validator struct {
	config *config.SecurityConfig
}

// NewValidator creates a new security validator
func NewValidator(config *config.SecurityConfig) *Validator {
	return &Validator{config: config}
}

// ValidateQuery checks if a query matches whitelist patterns
func ValidateQuery(whitelist []string, query string) error {
	// If no whitelist is configured, allow all
	if len(whitelist) == 0 {
		return nil
	}

	// Check if query matches any whitelist pattern
	for _, pattern := range whitelist {
		matched, err := regexp.MatchString(pattern, query)
		if err != nil {
			return fmt.Errorf("invalid whitelist pattern %s: %w", pattern, err)
		}
		if matched {
			return nil // Query is whitelisted
		}
	}

	return fmt.Errorf("query does not match any whitelist pattern")
}

// AnalyzeRisk performs risk analysis on a query (placeholder for LLM integration)
func (v *Validator) AnalyzeRisk(query string) (bool, string, error) {
	if !v.config.EnableLLMAnalysis {
		return true, "LLM analysis disabled", nil
	}

	// TODO: Implement LLM integration
	// This would call an LLM API (OpenAI, Anthropic, etc.) to analyze the query
	// and determine if it's potentially malicious or risky

	// For now, return a placeholder
	return true, "LLM analysis not yet implemented", nil
}

// ValidateWithRisk validates a query with both whitelist and LLM analysis
func (v *Validator) ValidateWithRisk(whitelist []string, query string) error {
	// First check whitelist
	if err := ValidateQuery(whitelist, query); err != nil {
		return err
	}

	// Then perform risk analysis if enabled
	if v.config.EnableLLMAnalysis {
		safe, reason, err := v.AnalyzeRisk(query)
		if err != nil {
			return fmt.Errorf("risk analysis failed: %w", err)
		}
		if !safe {
			return fmt.Errorf("risk analysis blocked query: %s", reason)
		}
	}

	return nil
}
