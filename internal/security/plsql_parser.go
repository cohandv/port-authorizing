package security

import (
	"regexp"
	"strings"
)

// PLSQLSubQuery represents a parsed subquery from a PL/SQL script
type PLSQLSubQuery struct {
	Query     string `json:"query"`
	Type      string `json:"type"` // "sql", "plsql_block", "procedure", "function"
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
}

// PLSQLParser handles parsing of PL/SQL scripts
type PLSQLParser struct{}

// NewPLSQLParser creates a new PL/SQL parser
func NewPLSQLParser() *PLSQLParser {
	return &PLSQLParser{}
}

// ParseScript parses a PL/SQL script into individual subqueries
func (p *PLSQLParser) ParseScript(script string) []PLSQLSubQuery {
	var subqueries []PLSQLSubQuery

	// Remove comments and normalize whitespace
	normalizedScript := p.normalizeScript(script)

	// Split by semicolons and process each part
	parts := p.splitBySemicolons(normalizedScript)

	lineOffset := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Determine query type
		queryType := p.determineQueryType(part)

		// Find line numbers for this part
		lineStart, lineEnd := p.findLineNumbers(script, part, lineOffset)

		subquery := PLSQLSubQuery{
			Query:     part,
			Type:      queryType,
			LineStart: lineStart,
			LineEnd:   lineEnd,
		}

		subqueries = append(subqueries, subquery)
		lineOffset += len(part) + 1 // +1 for semicolon
	}

	return subqueries
}

// normalizeScript removes comments and normalizes whitespace
func (p *PLSQLParser) normalizeScript(script string) string {
	lines := strings.Split(script, "\n")
	var normalizedLines []string

	for _, line := range lines {
		// Remove single-line comments (-- comment)
		if commentIdx := strings.Index(line, "--"); commentIdx != -1 {
			line = line[:commentIdx]
		}

		// Remove multi-line comments (/* comment */)
		line = p.removeMultiLineComments(line)

		// Normalize whitespace
		line = strings.TrimSpace(line)

		if line != "" {
			normalizedLines = append(normalizedLines, line)
		}
	}

	return strings.Join(normalizedLines, "\n")
}

// removeMultiLineComments removes /* */ style comments
func (p *PLSQLParser) removeMultiLineComments(line string) string {
	// Simple regex to remove /* */ comments
	re := regexp.MustCompile(`/\*.*?\*/`)
	return re.ReplaceAllString(line, "")
}

// splitBySemicolons splits script by semicolons, respecting PL/SQL blocks
func (p *PLSQLParser) splitBySemicolons(script string) []string {
	var parts []string
	var current strings.Builder
	var inBlock bool
	var blockDepth int

	for i, char := range script {
		switch char {
		case ';':
			if !inBlock {
				// End of statement outside block
				part := strings.TrimSpace(current.String())
				if part != "" {
					parts = append(parts, part)
				}
				current.Reset()
			} else {
				current.WriteRune(char)
			}
		case 'B':
			// Check for BEGIN keyword
			if i+4 < len(script) && script[i:i+5] == "BEGIN" {
				// Make sure it's a word boundary
				if i == 0 || !isAlphaNumeric(rune(script[i-1])) {
					if i+5 >= len(script) || !isAlphaNumeric(rune(script[i+5])) {
						inBlock = true
						blockDepth++
					}
				}
			}
			current.WriteRune(char)
		case 'E':
			// Check for END keyword
			if i+2 < len(script) && script[i:i+3] == "END" {
				// Make sure it's a word boundary
				if i == 0 || !isAlphaNumeric(rune(script[i-1])) {
					if i+3 >= len(script) || !isAlphaNumeric(rune(script[i+3])) {
						if inBlock && blockDepth > 0 {
							blockDepth--
							if blockDepth == 0 {
								inBlock = false
							}
						}
					}
				}
			}
			current.WriteRune(char)
		default:
			current.WriteRune(char)
		}
	}

	// Add remaining content
	part := strings.TrimSpace(current.String())
	if part != "" {
		parts = append(parts, part)
	}

	return parts
}

// determineQueryType determines the type of query
func (p *PLSQLParser) determineQueryType(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))

	// Check for PL/SQL blocks
	if strings.HasPrefix(query, "BEGIN") && strings.HasSuffix(query, "END") {
		return "plsql_block"
	}

	// Check for procedure/function creation
	if strings.Contains(query, "CREATE PROCEDURE") || strings.Contains(query, "CREATE OR REPLACE PROCEDURE") {
		return "procedure"
	}
	if strings.Contains(query, "CREATE FUNCTION") || strings.Contains(query, "CREATE OR REPLACE FUNCTION") {
		return "function"
	}

	// Check for SQL statements
	if strings.HasPrefix(query, "SELECT") {
		return "sql"
	}
	if strings.HasPrefix(query, "INSERT") {
		return "sql"
	}
	if strings.HasPrefix(query, "UPDATE") {
		return "sql"
	}
	if strings.HasPrefix(query, "DELETE") {
		return "sql"
	}
	if strings.HasPrefix(query, "EXECUTE") {
		return "sql"
	}

	// Default to SQL
	return "sql"
}

// findLineNumbers finds the line numbers for a query part
func (p *PLSQLParser) findLineNumbers(originalScript, queryPart string, offset int) (int, int) {
	lines := strings.Split(originalScript, "\n")

	// Simple approximation - in a real implementation, you'd want more sophisticated line tracking
	lineStart := 1
	lineEnd := len(lines)

	// Try to find the query part in the original script
	for i, line := range lines {
		if strings.Contains(line, strings.TrimSpace(queryPart)[:min(20, len(strings.TrimSpace(queryPart)))]) {
			lineStart = i + 1
			lineEnd = i + 1
			break
		}
	}

	return lineStart, lineEnd
}

// isAlphaNumeric checks if a character is alphanumeric
func isAlphaNumeric(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_'
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
