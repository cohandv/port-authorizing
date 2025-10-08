package security

import (
	"testing"
)

func TestSQLAnalyzer_AnalyzeQuery(t *testing.T) {
	analyzer := NewSQLAnalyzer()

	tests := []struct {
		name           string
		query          string
		wantValid      bool
		wantOperations []SQLOperation
		wantTables     []string
		wantError      bool
	}{
		{
			name:           "simple SELECT",
			query:          "SELECT * FROM users",
			wantValid:      true,
			wantOperations: []SQLOperation{OpSelect},
			wantTables:     []string{"users"},
			wantError:      false,
		},
		{
			name:           "SELECT with WHERE",
			query:          "SELECT id, name FROM users WHERE role = 'admin'",
			wantValid:      true,
			wantOperations: []SQLOperation{OpSelect},
			wantTables:     []string{"users"},
			wantError:      false,
		},
		{
			name:           "JOIN query",
			query:          "SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id",
			wantValid:      true,
			wantOperations: []SQLOperation{OpSelect},
			wantTables:     []string{"users", "orders"},
			wantError:      false,
		},
		{
			name:           "INSERT statement",
			query:          "INSERT INTO logs (message, level) VALUES ('test', 'info')",
			wantValid:      true,
			wantOperations: []SQLOperation{OpInsert},
			wantTables:     []string{"logs"},
			wantError:      false,
		},
		{
			name:           "UPDATE statement",
			query:          "UPDATE users SET active = true WHERE id = 1",
			wantValid:      true,
			wantOperations: []SQLOperation{OpUpdate},
			wantTables:     []string{"users"},
			wantError:      false,
		},
		{
			name:           "DELETE statement",
			query:          "DELETE FROM sessions WHERE expired_at < NOW()",
			wantValid:      true,
			wantOperations: []SQLOperation{OpDelete},
			wantTables:     []string{"sessions"},
			wantError:      false,
		},
		{
			name:           "TRUNCATE statement",
			query:          "TRUNCATE TABLE temp_data",
			wantValid:      true,
			wantOperations: []SQLOperation{OpTruncate},
			wantTables:     []string{"temp_data"},
			wantError:      false,
		},
		{
			name:           "DROP TABLE",
			query:          "DROP TABLE old_logs",
			wantValid:      true,
			wantOperations: []SQLOperation{OpDrop},
			wantTables:     []string{"old_logs"},
			wantError:      false,
		},
		{
			name:           "SQL injection attempt - multiple statements",
			query:          "SELECT * FROM users; DROP TABLE users;",
			wantValid:      true, // Both statements parse individually
			wantOperations: []SQLOperation{OpSelect, OpDrop},
			wantTables:     []string{"users"},
			wantError:      false,
		},
		{
			name:           "invalid SQL",
			query:          "SELCT * FORM users",
			wantValid:      false,
			wantOperations: []SQLOperation{},
			wantTables:     []string{},
			wantError:      true,
		},
		{
			name:           "CREATE TABLE",
			query:          "CREATE TABLE test_table (id INT, name VARCHAR(50))",
			wantValid:      true,
			wantOperations: []SQLOperation{OpCreate},
			wantTables:     []string{},
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.AnalyzeQuery(tt.query)

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}

			if (result.Error != nil) != tt.wantError {
				t.Errorf("Error = %v, wantError %v", result.Error, tt.wantError)
			}

			if len(result.Operations) != len(tt.wantOperations) {
				t.Errorf("Operations count = %d, want %d. Got: %v", len(result.Operations), len(tt.wantOperations), result.Operations)
			} else {
				for i, op := range result.Operations {
					if op != tt.wantOperations[i] {
						t.Errorf("Operation[%d] = %v, want %v", i, op, tt.wantOperations[i])
					}
				}
			}

			if len(result.Tables) != len(tt.wantTables) {
				t.Errorf("Tables count = %d, want %d. Got: %v", len(result.Tables), len(tt.wantTables), result.Tables)
			} else {
				for _, wantTable := range tt.wantTables {
					found := false
					for _, table := range result.Tables {
						if table == wantTable {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected table '%s' not found in %v", wantTable, result.Tables)
					}
				}
			}
		})
	}
}

func TestSQLAnalyzer_CheckTablePermissions(t *testing.T) {
	analyzer := NewSQLAnalyzer()

	tests := []struct {
		name        string
		query       string
		permissions []TablePermission
		wantAllowed bool
		wantReason  string
	}{
		{
			name:  "allowed SELECT on users",
			query: "SELECT * FROM users",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpSelect},
					Tables:     []string{"users"},
				},
			},
			wantAllowed: true,
		},
		{
			name:  "denied DELETE on users",
			query: "DELETE FROM users WHERE id = 1",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpSelect},
					Tables:     []string{"users"},
				},
			},
			wantAllowed: false,
			wantReason:  "operation DELETE not allowed on table 'users'",
		},
		{
			name:  "allowed SELECT on multiple tables with wildcard",
			query: "SELECT * FROM users JOIN orders ON users.id = orders.user_id",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpSelect},
					Tables:     []string{"*"},
				},
			},
			wantAllowed: true,
		},
		{
			name:  "allowed INSERT on logs_* pattern",
			query: "INSERT INTO logs_2024 (message) VALUES ('test')",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpInsert},
					Tables:     []string{"logs_*"},
				},
			},
			wantAllowed: true,
		},
		{
			name:  "denied INSERT on non-matching pattern",
			query: "INSERT INTO users (name) VALUES ('test')",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpInsert},
					Tables:     []string{"logs_*"},
				},
			},
			wantAllowed: false,
			wantReason:  "operation INSERT not allowed on table 'users'",
		},
		{
			name:  "allowed with suffix wildcard",
			query: "DELETE FROM temp_sessions",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpDelete},
					Tables:     []string{"*_sessions", "*_temp"},
				},
			},
			wantAllowed: true,
		},
		{
			name:  "multiple permissions - first allows",
			query: "SELECT * FROM users",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpSelect},
					Tables:     []string{"users", "orders"},
				},
				{
					Operations: []SQLOperation{OpInsert, OpUpdate},
					Tables:     []string{"logs"},
				},
			},
			wantAllowed: true,
		},
		{
			name:  "SQL injection blocked by operation check",
			query: "SELECT * FROM users; DROP TABLE users;",
			permissions: []TablePermission{
				{
					Operations: []SQLOperation{OpSelect},
					Tables:     []string{"*"},
				},
			},
			wantAllowed: false,
			wantReason:  "operation DROP not allowed on table 'users'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzer.AnalyzeQuery(tt.query)
			if !analysis.Valid {
				t.Fatalf("Query failed to parse: %v", analysis.Error)
			}

			allowed, reason := analyzer.CheckTablePermissions(analysis, tt.permissions)

			if allowed != tt.wantAllowed {
				t.Errorf("CheckTablePermissions() allowed = %v, want %v", allowed, tt.wantAllowed)
			}

			if !tt.wantAllowed && tt.wantReason != "" && reason != tt.wantReason {
				t.Errorf("CheckTablePermissions() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}

func TestMatchTablePattern(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		pattern   string
		want      bool
	}{
		{"exact match", "users", "users", true},
		{"wildcard all", "users", "*", true},
		{"prefix match", "logs_2024", "logs_*", true},
		{"prefix no match", "users", "logs_*", false},
		{"suffix match", "temp_sessions", "*_sessions", true},
		{"suffix no match", "users", "*_sessions", false},
		{"no match", "users", "orders", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTablePattern(tt.tableName, tt.pattern)
			if got != tt.want {
				t.Errorf("matchTablePattern(%q, %q) = %v, want %v", tt.tableName, tt.pattern, got, tt.want)
			}
		})
	}
}

func BenchmarkSQLAnalyzer_AnalyzeQuery(b *testing.B) {
	analyzer := NewSQLAnalyzer()
	query := "SELECT u.id, u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE u.active = true"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.AnalyzeQuery(query)
	}
}

func BenchmarkSQLAnalyzer_CheckTablePermissions(b *testing.B) {
	analyzer := NewSQLAnalyzer()
	query := "SELECT * FROM users WHERE id = 1"
	analysis := analyzer.AnalyzeQuery(query)

	permissions := []TablePermission{
		{
			Operations: []SQLOperation{OpSelect, OpInsert},
			Tables:     []string{"users", "orders", "products"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.CheckTablePermissions(analysis, permissions)
	}
}
