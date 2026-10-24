package config

import (
	"context"
	"testing"
)

func TestNewConfigSourceResolver(t *testing.T) {
	// Should not fail even if not in Kubernetes
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}
	if resolver == nil {
		t.Fatal("Expected non-nil resolver")
	}
}

func TestResolveConfigSource_PlainType(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	tests := []struct {
		name   string
		source ConfigSource
		want   string
	}{
		{
			name: "plain type with value",
			source: ConfigSource{
				Type:  ConfigSourceTypePlain,
				Value: "test-value",
			},
			want: "test-value",
		},
		{
			name: "empty type defaults to plain",
			source: ConfigSource{
				Type:  "",
				Value: "default-value",
			},
			want: "default-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists, err := resolver.ResolveConfigSource(context.Background(), tt.source)
			if err != nil {
				t.Errorf("ResolveConfigSource() error = %v", err)
				return
			}
			if !exists {
				t.Error("Expected source to exist")
			}
			if got != tt.want {
				t.Errorf("ResolveConfigSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveConfigSource_ConfigMapWithoutK8s(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	source := ConfigSource{
		Type:    ConfigSourceTypeConfigMap,
		Ref:     "test-configmap",
		RefName: "test-key",
		Value:   "fallback-value",
	}

	// Should return fallback value when K8s client not available
	got, exists, err := resolver.ResolveConfigSource(context.Background(), source)
	if err == nil {
		t.Error("Expected error when K8s client not available")
	}
	if exists {
		t.Error("Expected source to not exist")
	}
	if got != "fallback-value" {
		t.Errorf("Expected fallback value, got %v", got)
	}
}

func TestResolveConfigSource_SecretWithoutK8s(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	source := ConfigSource{
		Type:    ConfigSourceTypeSecret,
		Ref:     "test-secret",
		RefName: "test-key",
		Value:   "fallback-secret",
	}

	// Should return fallback value when K8s client not available
	got, exists, err := resolver.ResolveConfigSource(context.Background(), source)
	if err == nil {
		t.Error("Expected error when K8s client not available")
	}
	if exists {
		t.Error("Expected source to not exist")
	}
	if got != "fallback-secret" {
		t.Errorf("Expected fallback value, got %v", got)
	}
}

func TestResolveConfig_JWTSecret(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	tests := []struct {
		name      string
		config    *Config
		wantValue string
		wantError bool
	}{
		{
			name: "plain JWT secret",
			config: &Config{
				Auth: AuthConfig{
					JWTSecret: ConfigSource{
						Type:  ConfigSourceTypePlain,
						Value: "test-secret",
					},
				},
			},
			wantValue: "test-secret",
			wantError: false,
		},
		{
			name: "configmap JWT secret without K8s - should fail hard",
			config: &Config{
				Auth: AuthConfig{
					JWTSecret: ConfigSource{
						Type:    ConfigSourceTypeConfigMap,
						Ref:     "jwt-secrets",
						RefName: "jwt-secret",
					},
				},
			},
			wantValue: "",
			wantError: true, // JWT Secret is critical - must fail if not found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, err := resolver.ResolveConfig(context.Background(), tt.config)
			if (err != nil) != tt.wantError {
				t.Errorf("ResolveConfig() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError {
				// Only check value if no error expected
				if tt.config.Auth.JWTSecret.Value != tt.wantValue {
					t.Errorf("JWTSecret.Value = %v, want %v", tt.config.Auth.JWTSecret.Value, tt.wantValue)
				}
				if errors == nil {
					t.Error("Expected non-nil errors")
				}
			}
		})
	}
}

func TestResolveConfig_ConnectionCredentials(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	config := &Config{
		Auth: AuthConfig{
			JWTSecret: ConfigSource{
				Type:  ConfigSourceTypePlain,
				Value: "test-jwt",
			},
		},
		Connections: []ConnectionConfig{
			{
				Name: "test-postgres",
				Type: "postgres",
				Host: "localhost",
				Port: 5432,
				BackendUsername: ConfigSource{
					Type:  ConfigSourceTypePlain,
					Value: "postgres",
				},
				BackendPassword: ConfigSource{
					Type:  ConfigSourceTypePlain,
					Value: "password",
				},
			},
			{
				Name: "prod-postgres",
				Type: "postgres",
				Host: "prod.example.com",
				Port: 5432,
				BackendUsername: ConfigSource{
					Type:    ConfigSourceTypeSecret,
					Ref:     "prod-credentials",
					RefName: "username",
					Value:   "fallback-user",
				},
				BackendPassword: ConfigSource{
					Type:    ConfigSourceTypeSecret,
					Ref:     "prod-credentials",
					RefName: "password",
					Value:   "fallback-pass",
				},
			},
		},
	}

	errors, err := resolver.ResolveConfig(context.Background(), config)
	if err != nil {
		t.Errorf("ResolveConfig() unexpected error = %v", err)
	}

	// Check plain credentials resolved correctly
	if config.Connections[0].BackendUsername.Value != "postgres" {
		t.Errorf("Plain username = %v, want postgres", config.Connections[0].BackendUsername.Value)
	}
	if config.Connections[0].BackendPassword.Value != "password" {
		t.Errorf("Plain password = %v, want password", config.Connections[0].BackendPassword.Value)
	}

	// Check fallback values used for Secret type
	if config.Connections[1].BackendUsername.Value != "fallback-user" {
		t.Errorf("Fallback username = %v, want fallback-user", config.Connections[1].BackendUsername.Value)
	}
	if config.Connections[1].BackendPassword.Value != "fallback-pass" {
		t.Errorf("Fallback password = %v, want fallback-pass", config.Connections[1].BackendPassword.Value)
	}

	// Check that errors were tracked
	if errors == nil {
		t.Fatal("Expected non-nil errors")
	}
	if errors.Connections == nil {
		t.Fatal("Expected non-nil connection errors")
	}
	if connErr, ok := errors.Connections["prod-postgres"]; !ok {
		t.Error("Expected error for prod-postgres connection")
	} else {
		if len(connErr.MissingSecrets) == 0 {
			t.Error("Expected missing secrets to be tracked")
		}
	}
}

func TestResolveConfig_SlackWebhookURL(t *testing.T) {
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	tests := []struct {
		name      string
		config    *Config
		wantValue string
		hasErrors bool
	}{
		{
			name: "plain webhook URL",
			config: &Config{
				Auth: AuthConfig{
					JWTSecret: ConfigSource{
						Type:  ConfigSourceTypePlain,
						Value: "jwt",
					},
				},
				Approval: &ApprovalConfig{
					Slack: &SlackApprovalConfig{
						WebhookURL: ConfigSource{
							Type:  ConfigSourceTypePlain,
							Value: "https://hooks.slack.com/services/TEST",
						},
					},
				},
			},
			wantValue: "https://hooks.slack.com/services/TEST",
			hasErrors: false,
		},
		{
			name: "secret webhook URL",
			config: &Config{
				Auth: AuthConfig{
					JWTSecret: ConfigSource{
						Type:  ConfigSourceTypePlain,
						Value: "jwt",
					},
				},
				Approval: &ApprovalConfig{
					Slack: &SlackApprovalConfig{
						WebhookURL: ConfigSource{
							Type:    ConfigSourceTypeSecret,
							Ref:     "slack-secrets",
							RefName: "webhook-url",
							Value:   "https://fallback.url",
						},
					},
				},
			},
			wantValue: "https://fallback.url",
			hasErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, err := resolver.ResolveConfig(context.Background(), tt.config)
			if err != nil {
				t.Errorf("ResolveConfig() unexpected error = %v", err)
			}
			if tt.config.Approval.Slack.WebhookURL.Value != tt.wantValue {
				t.Errorf("WebhookURL.Value = %v, want %v", 
					tt.config.Approval.Slack.WebhookURL.Value, tt.wantValue)
			}
			if tt.hasErrors && (errors == nil || len(errors.MissingSecrets) == 0) {
				t.Error("Expected missing secrets error")
			}
		})
	}
}

func TestConfigErrors_Structure(t *testing.T) {
	errors := &ConfigErrors{
		MissingConfigMaps: []string{"cm1", "cm2"},
		MissingSecrets:    []string{"secret1"},
		Warnings:          []string{"warning1"},
		Connections:       make(map[string]*ConnectionConfigErrors),
	}

	errors.Connections["test"] = &ConnectionConfigErrors{
		MissingConfigMaps: []string{"cm3"},
		MissingSecrets:    []string{"secret2"},
	}

	if len(errors.MissingConfigMaps) != 2 {
		t.Errorf("Expected 2 missing ConfigMaps, got %d", len(errors.MissingConfigMaps))
	}
	if len(errors.MissingSecrets) != 1 {
		t.Errorf("Expected 1 missing Secret, got %d", len(errors.MissingSecrets))
	}
	if len(errors.Connections) != 1 {
		t.Errorf("Expected 1 connection error, got %d", len(errors.Connections))
	}
}

func TestListConfigMaps(t *testing.T) {
	// Create resolver (may or may not have K8s client depending on environment)
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	// If no K8s client, should return error
	// If K8s client available, should return list (may be empty)
	configMaps, err := resolver.ListConfigMaps(context.Background())
	
	if resolver.client == nil {
		// No K8s client - should error
		if err == nil {
			t.Error("Expected error when K8s client not available")
		}
		if len(configMaps) != 0 {
			t.Errorf("Expected empty list without K8s client, got %d items", len(configMaps))
		}
	} else {
		// K8s client available - should succeed (list may be empty)
		if err != nil {
			t.Logf("ListConfigMaps returned error (may be expected if no access): %v", err)
		}
		t.Logf("Found %d ConfigMaps in namespace", len(configMaps))
	}
}

func TestListSecrets(t *testing.T) {
	// Create resolver (may or may not have K8s client depending on environment)
	resolver, err := NewConfigSourceResolver("")
	if err != nil {
		t.Fatalf("NewConfigSourceResolver failed: %v", err)
	}

	// If no K8s client, should return error
	// If K8s client available, should return list (may be empty)
	secrets, err := resolver.ListSecrets(context.Background())
	
	if resolver.client == nil {
		// No K8s client - should error
		if err == nil {
			t.Error("Expected error when K8s client not available")
		}
		if len(secrets) != 0 {
			t.Errorf("Expected empty list without K8s client, got %d items", len(secrets))
		}
	} else {
		// K8s client available - should succeed (list may be empty)
		if err != nil {
			t.Logf("ListSecrets returned error (may be expected if no access): %v", err)
		}
		t.Logf("Found %d Secrets in namespace", len(secrets))
	}
}

