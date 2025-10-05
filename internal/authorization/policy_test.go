package authorization

import (
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestGetWhitelistForConnection_ComplexScenarios(t *testing.T) {
	authz := &Authorizer{
		config: &config.Config{},
		policies: map[string][]*config.RolePolicy{
			"developer": {
				{
					Name:      "dev-read",
					Roles:     []string{"developer"},
					Tags:      []string{"env:dev"},
					TagMatch:  "any",
					Whitelist: []string{"^SELECT.*", "^EXPLAIN.*"},
				},
				{
					Name:      "dev-staging",
					Roles:     []string{"developer"},
					Tags:      []string{"env:staging"},
					TagMatch:  "any",
					Whitelist: []string{"^SELECT.*", "^INSERT.*"},
				},
			},
			"admin": {
				{
					Name:      "admin-all",
					Roles:     []string{"admin"},
					Tags:      []string{"env:dev", "env:staging", "env:prod"},
					TagMatch:  "any",
					Whitelist: []string{".*"},
				},
			},
		},
		connections: map[string]*config.ConnectionConfig{
			"dev-db": {
				Name: "dev-db",
				Tags: []string{"env:dev"},
			},
			"staging-db": {
				Name: "staging-db",
				Tags: []string{"env:staging"},
			},
		},
	}

	tests := []struct {
		name       string
		roles      []string
		connection string
		wantCount  int
	}{
		{
			name:       "developer dev db",
			roles:      []string{"developer"},
			connection: "dev-db",
			wantCount:  2, // SELECT, EXPLAIN
		},
		{
			name:       "developer staging db",
			roles:      []string{"developer"},
			connection: "staging-db",
			wantCount:  2, // SELECT, INSERT
		},
		{
			name:       "admin any db",
			roles:      []string{"admin"},
			connection: "dev-db",
			wantCount:  1, // .*
		},
		{
			name:       "multiple roles",
			roles:      []string{"developer", "admin"},
			connection: "dev-db",
			wantCount:  3, // SELECT, EXPLAIN, .*
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			whitelist := authz.GetWhitelistForConnection(tt.roles, tt.connection)
			if len(whitelist) != tt.wantCount {
				t.Errorf("whitelist count = %d, want %d", len(whitelist), tt.wantCount)
			}
		})
	}
}

func TestListAccessibleConnections_Filtering(t *testing.T) {
	authz := &Authorizer{
		config: &config.Config{},
		policies: map[string][]*config.RolePolicy{
			"developer": {
				{
					Name:      "dev-only",
					Roles:     []string{"developer"},
					Tags:      []string{"env:dev"},
					TagMatch:  "any",
					Whitelist: []string{"^SELECT.*"},
				},
			},
			"admin": {
				{
					Name:      "admin-all",
					Roles:     []string{"admin"},
					Tags:      []string{"env:dev", "env:prod"},
					TagMatch:  "any",
					Whitelist: []string{".*"},
				},
			},
		},
		connections: map[string]*config.ConnectionConfig{
			"dev-db": {
				Name: "dev-db",
				Tags: []string{"env:dev"},
			},
			"prod-db": {
				Name: "prod-db",
				Tags: []string{"env:prod"},
			},
			"staging-db": {
				Name: "staging-db",
				Tags: []string{"env:staging"},
			},
		},
	}

	tests := []struct {
		name      string
		roles     []string
		wantCount int
	}{
		{
			name:      "developer sees only dev",
			roles:     []string{"developer"},
			wantCount: 1, // dev-db
		},
		{
			name:      "admin sees dev and prod",
			roles:     []string{"admin"},
			wantCount: 2, // dev-db, prod-db
		},
		{
			name:      "no roles sees nothing",
			roles:     []string{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conns := authz.ListAccessibleConnections(tt.roles)
			if len(conns) != tt.wantCount {
				t.Errorf("connections count = %d, want %d", len(conns), tt.wantCount)
			}
		})
	}
}

func TestGetConnectionInfo_Details(t *testing.T) {
	authz := &Authorizer{
		config: &config.Config{},
		policies: map[string][]*config.RolePolicy{
			"developer": {
				{
					Name:      "dev-policy",
					Roles:     []string{"developer"},
					Tags:      []string{"env:dev"},
					TagMatch:  "any",
					Whitelist: []string{"^SELECT.*"},
					Metadata: map[string]string{
						"description": "Dev access",
					},
				},
			},
		},
		connections: map[string]*config.ConnectionConfig{
			"dev-db": {
				Name: "dev-db",
				Type: "postgres",
				Host: "localhost",
				Port: 5432,
				Tags: []string{"env:dev"},
				Metadata: map[string]string{
					"owner": "dev-team",
				},
			},
		},
	}

	info := authz.GetConnectionInfo("dev-db")

	if info == nil {
		t.Fatal("GetConnectionInfo() should return connection info")
	}

	if info["name"] != "dev-db" {
		t.Errorf("Name = %v, want dev-db", info["name"])
	}

	if info["type"] != "postgres" {
		t.Errorf("Type = %v, want postgres", info["type"])
	}
}
