package security

import (
	"testing"
)

func TestValidateQuery_ComplexPatterns(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		query     string
		wantErr   bool
	}{
		{
			name:      "allow specific table",
			whitelist: []string{"^SELECT.*FROM logs.*"},
			query:     "SELECT * FROM logs",
			wantErr:   false,
		},
		{
			name:      "block different table",
			whitelist: []string{"^SELECT.*FROM logs.*"},
			query:     "SELECT * FROM users",
			wantErr:   true,
		},
		{
			name:      "allow with WHERE clause",
			whitelist: []string{"^SELECT.*WHERE.*"},
			query:     "SELECT * FROM users WHERE id=1",
			wantErr:   false,
		},
		{
			name:      "block without WHERE clause",
			whitelist: []string{"^SELECT.*WHERE.*"},
			query:     "SELECT * FROM users",
			wantErr:   true,
		},
		{
			name:      "allow JOIN queries",
			whitelist: []string{"^SELECT.*JOIN.*"},
			query:     "SELECT * FROM users JOIN orders ON users.id = orders.user_id",
			wantErr:   false,
		},
		{
			name:      "block non-JOIN queries",
			whitelist: []string{"^SELECT.*JOIN.*"},
			query:     "SELECT * FROM users",
			wantErr:   true,
		},
		{
			name:      "allow ORDER BY",
			whitelist: []string{"^SELECT.*ORDER BY.*"},
			query:     "SELECT * FROM users ORDER BY name",
			wantErr:   false,
		},
		{
			name:      "allow LIMIT",
			whitelist: []string{"^SELECT.*LIMIT.*"},
			query:     "SELECT * FROM users LIMIT 10",
			wantErr:   false,
		},
		{
			name:      "allow aggregate functions",
			whitelist: []string{"^SELECT (COUNT|SUM|AVG|MIN|MAX)\\(.*"},
			query:     "SELECT COUNT(*) FROM users",
			wantErr:   false,
		},
		{
			name:      "allow specific columns",
			whitelist: []string{"^SELECT (id|name|email).*FROM users"},
			query:     "SELECT id, name FROM users",
			wantErr:   false,
		},
		{
			name:      "block SELECT *",
			whitelist: []string{"^SELECT (id|name|email).*FROM users"},
			query:     "SELECT * FROM users",
			wantErr:   true,
		},
		{
			name:      "allow subqueries",
			whitelist: []string{"^SELECT.*\\(SELECT.*\\).*"},
			query:     "SELECT * FROM users WHERE id IN (SELECT user_id FROM orders)",
			wantErr:   false,
		},
		{
			name:      "allow DISTINCT",
			whitelist: []string{"^SELECT DISTINCT.*"},
			query:     "SELECT DISTINCT name FROM users",
			wantErr:   false,
		},
		{
			name:      "allow GROUP BY",
			whitelist: []string{"^SELECT.*GROUP BY.*"},
			query:     "SELECT role, COUNT(*) FROM users GROUP BY role",
			wantErr:   false,
		},
		{
			name:      "allow HAVING",
			whitelist: []string{"^SELECT.*GROUP BY.*HAVING.*"},
			query:     "SELECT role, COUNT(*) FROM users GROUP BY role HAVING COUNT(*) > 5",
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

func TestValidateQuery_HTTPPatterns(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		query     string
		wantErr   bool
	}{
		{
			name:      "allow GET with path",
			whitelist: []string{"^GET /api/users.*"},
			query:     "GET /api/users/123",
			wantErr:   false,
		},
		{
			name:      "block GET to different path",
			whitelist: []string{"^GET /api/users.*"},
			query:     "GET /api/orders",
			wantErr:   true,
		},
		{
			name:      "allow POST to specific endpoint",
			whitelist: []string{"^POST /api/users$"},
			query:     "POST /api/users",
			wantErr:   false,
		},
		{
			name:      "block POST to subpath",
			whitelist: []string{"^POST /api/users$"},
			query:     "POST /api/users/123",
			wantErr:   true,
		},
		{
			name:      "allow with query parameters",
			whitelist: []string{"^GET /api/users\\?.*"},
			query:     "GET /api/users?page=1&limit=10",
			wantErr:   false,
		},
		{
			name:      "allow any method to path",
			whitelist: []string{"^(GET|POST|PUT|DELETE) /api/admin.*"},
			query:     "PUT /api/admin/settings",
			wantErr:   false,
		},
		{
			name:      "allow versioned API",
			whitelist: []string{"^GET /api/v[0-9]+/.*"},
			query:     "GET /api/v1/users",
			wantErr:   false,
		},
		{
			name:      "allow numeric ID in path",
			whitelist: []string{"^GET /api/users/[0-9]+$"},
			query:     "GET /api/users/123",
			wantErr:   false,
		},
		{
			name:      "block non-numeric ID",
			whitelist: []string{"^GET /api/users/[0-9]+$"},
			query:     "GET /api/users/abc",
			wantErr:   true,
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

func TestValidateQuery_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		query     string
		wantErr   bool
	}{
		{
			name:      "very long query",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users WHERE " + string(make([]byte, 10000)),
			wantErr:   false,
		},
		{
			name:      "unicode characters",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users WHERE name='José'",
			wantErr:   false,
		},
		{
			name:      "special SQL characters",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users WHERE email LIKE '%@example.com'",
			wantErr:   false,
		},
		{
			name:      "escaped quotes",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users WHERE name='O\\'Brien'",
			wantErr:   false,
		},
		{
			name:      "comments in SQL",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users -- This is a comment",
			wantErr:   false,
		},
		{
			name:      "multiple statements",
			whitelist: []string{"^SELECT.*"},
			query:     "SELECT * FROM users; DROP TABLE users;",
			wantErr:   false, // First statement matches
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

func BenchmarkValidateQuery_ComplexPattern(b *testing.B) {
	whitelist := []string{
		"^SELECT (id|name|email|created_at) FROM (users|customers) WHERE id IN \\(SELECT.*\\) ORDER BY.*LIMIT [0-9]+$",
	}
	query := "SELECT id, name, email FROM users WHERE id IN (SELECT user_id FROM orders) ORDER BY name LIMIT 100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateQuery(whitelist, query)
	}
}

