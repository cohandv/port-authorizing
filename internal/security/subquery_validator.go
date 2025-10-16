package security

import (
	"fmt"
	"regexp"
	"strings"
)

// SubqueryValidationResult represents the result of validating a subquery
type SubqueryValidationResult struct {
	Subquery    PLSQLSubQuery `json:"subquery"`
	IsAllowed   bool          `json:"is_allowed"`
	BlockedBy   string        `json:"blocked_by,omitempty"`  // Pattern that blocked it
	MatchedBy   string        `json:"matched_by,omitempty"`  // Pattern that allowed it
	Error       string        `json:"error,omitempty"`       // Validation error
	RiskLevel   string        `json:"risk_level"`            // "low", "medium", "high"
	Suggestions []string      `json:"suggestions,omitempty"` // Security suggestions
}

// ScriptValidationResult represents the result of validating an entire script
type ScriptValidationResult struct {
	Script       string                     `json:"script"`
	IsAllowed    bool                       `json:"is_allowed"`
	Subqueries   []SubqueryValidationResult `json:"subqueries"`
	TotalQueries int                        `json:"total_queries"`
	AllowedCount int                        `json:"allowed_count"`
	BlockedCount int                        `json:"blocked_count"`
	RiskSummary  map[string]int             `json:"risk_summary"` // Count by risk level
}

// SubqueryValidator handles validation of PL/SQL subqueries
type SubqueryValidator struct {
	parser *PLSQLParser
}

// NewSubqueryValidator creates a new subquery validator
func NewSubqueryValidator() *SubqueryValidator {
	return &SubqueryValidator{
		parser: NewPLSQLParser(),
	}
}

// ValidateScript validates an entire PL/SQL script against whitelist patterns
func (v *SubqueryValidator) ValidateScript(script string, whitelist []string) ScriptValidationResult {
	// Parse the script into subqueries
	subqueries := v.parser.ParseScript(script)

	result := ScriptValidationResult{
		Script:       script,
		Subqueries:   make([]SubqueryValidationResult, 0, len(subqueries)),
		TotalQueries: len(subqueries),
		RiskSummary:  make(map[string]int),
	}

	allAllowed := true

	// Validate each subquery
	for _, subquery := range subqueries {
		subResult := v.ValidateSubquery(subquery, whitelist)
		result.Subqueries = append(result.Subqueries, subResult)

		if subResult.IsAllowed {
			result.AllowedCount++
		} else {
			result.BlockedCount++
			allAllowed = false
		}

		// Count risk levels
		result.RiskSummary[subResult.RiskLevel]++
	}

	result.IsAllowed = allAllowed
	return result
}

// ValidateSubquery validates a single subquery against whitelist patterns
func (v *SubqueryValidator) ValidateSubquery(subquery PLSQLSubQuery, whitelist []string) SubqueryValidationResult {
	result := SubqueryValidationResult{
		Subquery:  subquery,
		RiskLevel: v.assessRiskLevel(subquery),
	}

	// If no whitelist, allow everything (backward compatibility)
	if len(whitelist) == 0 {
		result.IsAllowed = true
		result.MatchedBy = "no_whitelist"
		return result
	}

	// Check each whitelist pattern
	for _, pattern := range whitelist {
		// Compile with case-insensitive flag
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			result.Error = fmt.Sprintf("invalid whitelist pattern: %s", pattern)
			continue
		}

		if re.MatchString(subquery.Query) {
			result.IsAllowed = true
			result.MatchedBy = pattern
			return result
		}
	}

	// If no pattern matched, it's blocked
	result.IsAllowed = false
	result.BlockedBy = "no_matching_pattern"

	// Add security suggestions
	result.Suggestions = v.generateSuggestions(subquery)

	return result
}

// assessRiskLevel assesses the risk level of a subquery
func (v *SubqueryValidator) assessRiskLevel(subquery PLSQLSubQuery) string {
	query := strings.ToUpper(subquery.Query)

	// High risk operations
	highRiskPatterns := []string{
		"DROP", "TRUNCATE", "DELETE FROM", "UPDATE.*SET",
		"ALTER TABLE", "CREATE USER", "GRANT", "REVOKE",
		"EXECUTE.*SP_", "EXEC.*SP_", "xp_cmdshell",
	}

	for _, pattern := range highRiskPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(query) {
			return "high"
		}
	}

	// Medium risk operations
	mediumRiskPatterns := []string{
		"INSERT INTO", "CREATE TABLE", "CREATE INDEX",
		"ALTER SESSION", "SET.*=", "EXECUTE",
	}

	for _, pattern := range mediumRiskPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(query) {
			return "medium"
		}
	}

	// Low risk operations
	lowRiskPatterns := []string{
		"SELECT", "SHOW", "DESCRIBE", "EXPLAIN",
		"BEGIN", "COMMIT", "ROLLBACK",
	}

	for _, pattern := range lowRiskPatterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(query) {
			return "low"
		}
	}

	// Default to medium for unknown operations
	return "medium"
}

// generateSuggestions generates security suggestions for blocked queries
func (v *SubqueryValidator) generateSuggestions(subquery PLSQLSubQuery) []string {
	var suggestions []string
	query := strings.ToUpper(subquery.Query)

	// Suggest specific patterns based on query type
	if strings.HasPrefix(query, "SELECT") {
		suggestions = append(suggestions, "Consider adding pattern: ^SELECT.*")
	}

	if strings.HasPrefix(query, "INSERT") {
		suggestions = append(suggestions, "Consider adding pattern: ^INSERT INTO.*")
	}

	if strings.HasPrefix(query, "UPDATE") {
		suggestions = append(suggestions, "Consider adding pattern: ^UPDATE.*SET.*")
	}

	if strings.HasPrefix(query, "DELETE") {
		suggestions = append(suggestions, "Consider adding pattern: ^DELETE FROM.*")
	}

	if strings.HasPrefix(query, "BEGIN") {
		suggestions = append(suggestions, "Consider adding pattern: ^BEGIN.*")
	}

	// General suggestions
	if subquery.Type == "plsql_block" {
		suggestions = append(suggestions, "For PL/SQL blocks, consider: ^BEGIN.*END.*")
	}

	if strings.Contains(query, "WHERE") {
		suggestions = append(suggestions, "Ensure WHERE clauses are properly restricted")
	}

	return suggestions
}
