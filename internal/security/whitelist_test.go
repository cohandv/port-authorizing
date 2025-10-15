package security

import (
	"testing"
)

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		query     string
		wantErr   bool
	}{
		{
			name:      "SELECT allowed with SELECT pattern",
			query:     "SELECT * FROM users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   false,
		},
		{
			name:      "SELECT with WHERE allowed",
			query:     "SELECT id, name FROM users WHERE active = true",
			whitelist: []string{"^SELECT.*"},
			wantErr:   false,
		},
		{
			name:      "DELETE blocked with SELECT-only whitelist",
			query:     "DELETE FROM users WHERE id = 1",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true,
		},
		{
			name:      "UPDATE blocked with SELECT-only whitelist",
			query:     "UPDATE users SET active = false",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true,
		},
		{
			name:      "INSERT blocked with SELECT-only whitelist",
			query:     "INSERT INTO users (name) VALUES ('test')",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true,
		},
		{
			name:      "DROP blocked with SELECT-only whitelist",
			query:     "DROP TABLE users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true,
		},
		{
			name:      "multiple patterns - SELECT matches",
			query:     "SELECT * FROM users",
			whitelist: []string{"^SELECT.*", "^EXPLAIN.*"},
			wantErr:   false,
		},
		{
			name:      "multiple patterns - EXPLAIN matches",
			query:     "EXPLAIN SELECT * FROM users",
			whitelist: []string{"^SELECT.*", "^EXPLAIN.*"},
			wantErr:   false,
		},
		{
			name:      "empty whitelist allows all",
			query:     "DELETE FROM users",
			whitelist: []string{},
			wantErr:   false,
		},
		{
			name:      "nil whitelist allows all",
			query:     "DROP DATABASE production",
			whitelist: nil,
			wantErr:   false,
		},
		{
			name:      "case insensitive - lowercase query",
			query:     "select * from users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true, // Note: ValidateQuery doesn't do case-insensitive matching
		},
		{
			name:      "case insensitive - mixed case query",
			query:     "SeLeCt * FrOm users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true, // Note: ValidateQuery doesn't do case-insensitive matching
		},
		{
			name:      "wildcard pattern allows all",
			query:     "DELETE FROM users",
			whitelist: []string{".*"},
			wantErr:   false,
		},
		{
			name:      "specific table pattern",
			query:     "SELECT * FROM users",
			whitelist: []string{"^SELECT.* FROM users.*"},
			wantErr:   false,
		},
		{
			name:      "specific table pattern - wrong table",
			query:     "SELECT * FROM products",
			whitelist: []string{"^SELECT.* FROM users.*"},
			wantErr:   true,
		},
		{
			name:      "query with leading whitespace",
			query:     "  SELECT * FROM users",
			whitelist: []string{"^SELECT.*"},
			wantErr:   true, // Pattern expects SELECT at start, not whitespace
		},
		{
			name:      "query with trailing semicolon",
			query:     "SELECT * FROM users;",
			whitelist: []string{"^SELECT.*"},
			wantErr:   false,
		},
		{
			name:      "multiline query",
			query:     "SELECT id, name\nFROM users\nWHERE active = true",
			whitelist: []string{"^SELECT.*"},
			wantErr:   false,
		},
		{
			name:      "GET request for HTTP proxy",
			query:     "GET /api/users HTTP/1.1",
			whitelist: []string{"^GET .*"},
			wantErr:   false,
		},
		{
			name:      "POST blocked for HTTP proxy",
			query:     "POST /api/users HTTP/1.1",
			whitelist: []string{"^GET .*"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(tt.whitelist, tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuery() error = %v, wantErr %v\nQuery: %s\nWhitelist: %v",
					err, tt.wantErr, tt.query, tt.whitelist)
			}
		})
	}
}

func TestValidateQuery_InvalidRegex(t *testing.T) {
	// Test with invalid regex pattern
	query := "SELECT * FROM users"
	whitelist := []string{"[invalid"}

	// Should not panic and should return error for invalid regex
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ValidateQuery() panicked with invalid regex: %v", r)
		}
	}()

	err := ValidateQuery(whitelist, query)
	if err == nil {
		t.Error("ValidateQuery() with invalid regex should return error")
	}
}

func BenchmarkValidateQuery_SinglePattern(b *testing.B) {
	query := "SELECT * FROM users WHERE id = 1"
	whitelist := []string{"^SELECT.*"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateQuery(whitelist, query)
	}
}

func BenchmarkValidateQuery_MultiplePatterns(b *testing.B) {
	query := "SELECT * FROM users WHERE id = 1"
	whitelist := []string{"^INSERT.*", "^UPDATE.*", "^DELETE.*", "^SELECT.*", "^EXPLAIN.*"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateQuery(whitelist, query)
	}
}

func BenchmarkValidateQuery_NoMatch(b *testing.B) {
	query := "DROP TABLE users"
	whitelist := []string{"^SELECT.*", "^EXPLAIN.*"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateQuery(whitelist, query)
	}
}
