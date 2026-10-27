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

	// Split by top-level semicolons (comments dropped, literals kept intact)
	parts := p.splitBySemicolons(script, false)

	// standard_conforming_strings is per session and a client can turn it off (even via SELECT set_config(...)),
	// making \' an escape inside plain '...'. We can't know the mode, so validate both readings: whichever the
	// server uses, every statement it runs is in this list.
	if strings.Contains(script, `\`) {
		seen := make(map[string]bool, len(parts))
		for _, s := range parts {
			seen[s] = true
		}
		for _, s := range p.splitBySemicolons(script, true) {
			if !seen[s] {
				parts = append(parts, s)
			}
		}
	}

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

var dollarTag = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)

// splitBySemicolons splits a PostgreSQL script on top-level semicolons. Semicolons inside '...', E'...', "...",
// $tag$...$tag$ and (nested) comments do not split; comments are dropped. BEGIN/END are transaction statements in
// PostgreSQL, procedural bodies (DO, CREATE FUNCTION) are dollar-quoted. legacyStrings lexes plain '...' as with
// standard_conforming_strings = off, where a backslash escapes the next character.
func (p *PLSQLParser) splitBySemicolons(script string, legacyStrings bool) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			parts = append(parts, s)
		}
		cur.Reset()
	}
	// end returns the index just past a literal opened at i with closing quote q; unterminated runs to the end
	end := func(i int, q byte, backslash bool) int {
		for j := i + 1; j < len(script); j++ {
			switch {
			case backslash && script[j] == '\\':
				j++
			case script[j] == q && j+1 < len(script) && script[j+1] == q:
				j++
			case script[j] == q:
				return j + 1
			}
		}
		return len(script)
	}
	identBefore := func(i int) bool { return i > 0 && (isAlphaNumeric(rune(script[i-1])) || script[i-1] == '$') }

	for i := 0; i < len(script); {
		c := script[i]
		switch {
		case c == ';':
			flush()
			i++
		case strings.HasPrefix(script[i:], "--"):
			if j := strings.IndexByte(script[i:], '\n'); j >= 0 {
				i += j
			} else {
				i = len(script)
			}
		case strings.HasPrefix(script[i:], "/*"):
			// block comments nest in PostgreSQL: "/* a /* b */ c */" is one comment
			for depth := 0; i < len(script); {
				if strings.HasPrefix(script[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(script[i:], "*/") {
					i += 2
					if depth--; depth == 0 {
						break
					}
				} else {
					i++
				}
			}
			cur.WriteByte(' ')
		case c == '\'' || c == '"':
			// E'...' (escape string), or any '...' in legacy mode: backslash escapes the quote
			esc := c == '\'' && (legacyStrings || i > 0 && (script[i-1] == 'E' || script[i-1] == 'e') && !identBefore(i-1))
			j := end(i, c, esc)
			cur.WriteString(script[i:j])
			i = j
		case c == '$' && !identBefore(i):
			tag := dollarTag.FindString(script[i:])
			if tag == "" {
				cur.WriteByte(c)
				i++
				break
			}
			j := len(script)
			if k := strings.Index(script[i+len(tag):], tag); k >= 0 {
				j = i + len(tag) + k + len(tag)
			}
			cur.WriteString(script[i:j])
			i = j
		default:
			cur.WriteByte(c)
			i++
		}
	}
	flush()

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
