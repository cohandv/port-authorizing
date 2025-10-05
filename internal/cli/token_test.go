package cli

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test saveToken and loadToken
func TestSaveAndLoadToken(t *testing.T) {
	tmpDir := t.TempDir()

	// Set config path for testing
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	token := "test.token.here"

	err := saveToken(token)
	if err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}

	loadedToken, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken() error = %v", err)
	}

	if loadedToken != token {
		t.Errorf("loadToken() = %s, want %s", loadedToken, token)
	}
}

func TestLoadToken_NotExist(t *testing.T) {
	tmpDir := t.TempDir()

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := loadToken()
	if err == nil {
		t.Error("loadToken() should fail when file doesn't exist")
	}
}

func TestValidateToken(t *testing.T) {
	// Create a valid JWT token
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name: "valid token not expired",
			token: func() string {
				payload := map[string]interface{}{
					"username": "test",
					"exp":      time.Now().Add(1 * time.Hour).Unix(),
				}
				payloadJSON, _ := json.Marshal(payload)
				payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
				return header + "." + payloadB64 + ".signature"
			}(),
			wantErr: false,
		},
		{
			name: "expired token",
			token: func() string {
				payload := map[string]interface{}{
					"username": "test",
					"exp":      time.Now().Add(-1 * time.Hour).Unix(),
				}
				payloadJSON, _ := json.Marshal(payload)
				payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
				return header + "." + payloadB64 + ".signature"
			}(),
			wantErr: true,
		},
		{
			name: "token without exp",
			token: func() string {
				payload := map[string]interface{}{
					"username": "test",
				}
				payloadJSON, _ := json.Marshal(payload)
				payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
				return header + "." + payloadB64 + ".signature"
			}(),
			wantErr: true,
		},
		{
			name:    "invalid format - no dots",
			token:   "invalidtoken",
			wantErr: true,
		},
		{
			name:    "invalid format - only 2 parts",
			token:   "header.payload",
			wantErr: true,
		},
		{
			name:    "invalid base64 in payload",
			token:   header + ".!!!invalid!!!.signature",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToken() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetUsernameFromToken(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	tests := []struct {
		name     string
		token    string
		wantUser string
		wantErr  bool
	}{
		{
			name: "valid token with username",
			token: func() string {
				payload := map[string]interface{}{
					"username": "testuser",
					"exp":      time.Now().Add(1 * time.Hour).Unix(),
				}
				payloadJSON, _ := json.Marshal(payload)
				payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
				return header + "." + payloadB64 + ".signature"
			}(),
			wantUser: "testuser",
			wantErr:  false,
		},
		{
			name: "valid token with different username",
			token: func() string {
				payload := map[string]interface{}{
					"username": "admin",
					"roles":    []string{"admin"},
				}
				payloadJSON, _ := json.Marshal(payload)
				payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
				return header + "." + payloadB64 + ".signature"
			}(),
			wantUser: "admin",
			wantErr:  false,
		},
		{
			name:     "invalid format",
			token:    "invalid",
			wantUser: "",
			wantErr:  true,
		},
		{
			name:     "invalid base64",
			token:    header + ".!!!invalid!!!.signature",
			wantUser: "",
			wantErr:  true,
		},
		{
			name:     "empty token",
			token:    "",
			wantUser: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, err := getUsernameFromToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("getUsernameFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if username != tt.wantUser {
				t.Errorf("getUsernameFromToken() = %s, want %s", username, tt.wantUser)
			}
		})
	}
}

func TestSaveToken_InvalidPath(t *testing.T) {
	// Set HOME to a file (not a directory) to cause mkdir to fail
	tmpFile := filepath.Join(t.TempDir(), "notadir")
	os.WriteFile(tmpFile, []byte("test"), 0644)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpFile)
	defer os.Setenv("HOME", oldHome)

	err := saveToken("test-token")
	if err == nil {
		t.Error("saveToken() should fail when HOME is not a valid directory")
	}
}

func TestLoadToken_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".port-auth")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	// Write invalid JSON
	os.WriteFile(configFile, []byte("invalid json{"), 0600)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := loadToken()
	if err == nil {
		t.Error("loadToken() should fail with invalid JSON")
	}
}

func TestLoadToken_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".port-auth")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	// Write JSON without token field
	os.WriteFile(configFile, []byte(`{"other":"field"}`), 0600)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	_, err := loadToken()
	if err == nil {
		t.Error("loadToken() should fail when token field is missing")
	}
}

func BenchmarkSaveToken(b *testing.B) {
	tmpDir := b.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	token := "test.token.here"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		saveToken(token)
	}
}

func BenchmarkLoadToken(b *testing.B) {
	tmpDir := b.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	saveToken("test.token.here")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loadToken()
	}
}

func BenchmarkValidateToken(b *testing.B) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := map[string]interface{}{
		"username": "test",
		"exp":      time.Now().Add(1 * time.Hour).Unix(),
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	token := header + "." + payloadB64 + ".signature"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateToken(token)
	}
}

func BenchmarkGetUsernameFromToken(b *testing.B) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := map[string]interface{}{
		"username": "testuser",
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	token := header + "." + payloadB64 + ".signature"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getUsernameFromToken(token)
	}
}
