package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// runOIDCLogin performs browser-based OIDC authentication
func runOIDCLogin(apiURL string) error {
	fmt.Println("🔐 Starting browser-based OIDC authentication...")
	fmt.Println("")

	// Generate state for CSRF protection
	state, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	// Start local callback server
	callbackChan := make(chan *loginResponse, 1)
	errorChan := make(chan error, 1)

	server := startCallbackServer(callbackChan, errorChan, state)
	defer server.Shutdown(context.Background())

	// Build authorization URL (API will redirect to Keycloak)
	authURL := fmt.Sprintf("%s/api/auth/oidc/login?state=%s&cli_callback=http://localhost:8089/callback",
		apiURL, state)

	// Open browser
	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If browser doesn't open, visit: %s\n", authURL)
	fmt.Println("")

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("⚠️  Failed to open browser: %v\n", err)
		fmt.Println("Please open the URL manually.")
	}

	fmt.Println("⏳ Waiting for authentication in browser...")
	fmt.Println("   (Window will close automatically after login)")
	fmt.Println("")

	// Wait for callback or timeout
	select {
	case loginResp := <-callbackChan:
		// Save token
		if err := saveToken(loginResp.Token); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}

		fmt.Println("✓ Authentication successful!")
		if loginResp.User.Username != "" {
			fmt.Printf("  User: %s", loginResp.User.Username)
			if loginResp.User.Email != "" {
				fmt.Printf(" (%s)", loginResp.User.Email)
			}
			fmt.Println()
		}
		if len(loginResp.User.Roles) > 0 {
			fmt.Printf("  Roles: %v\n", loginResp.User.Roles)
		}
		fmt.Printf("  Token expires at: %s\n", loginResp.ExpiresAt)
		return nil

	case err := <-errorChan:
		return fmt.Errorf("authentication failed: %w", err)

	case <-time.After(5 * time.Minute):
		return fmt.Errorf("authentication timed out after 5 minutes")
	}
}

func startCallbackServer(callbackChan chan *loginResponse, errorChan chan error, expectedState string) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Verify state to prevent CSRF
		state := r.URL.Query().Get("state")
		if state != expectedState {
			errorChan <- fmt.Errorf("invalid state parameter")
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}

		// Check for error
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			errorChan <- fmt.Errorf("%s: %s", errParam, errDesc)
			http.Error(w, errDesc, http.StatusBadRequest)
			return
		}

		// Get token data from query parameter
		tokenData := r.URL.Query().Get("token_data")
		if tokenData == "" {
			errorChan <- fmt.Errorf("no token data received")
			http.Error(w, "No token data", http.StatusBadRequest)
			return
		}

		// Decode token data (it's base64 encoded JSON)
		decoded, err := base64.URLEncoding.DecodeString(tokenData)
		if err != nil {
			errorChan <- fmt.Errorf("failed to decode token data: %w", err)
			http.Error(w, "Invalid token data", http.StatusBadRequest)
			return
		}

		// Parse login response
		var loginResp loginResponse
		if err := json.Unmarshal(decoded, &loginResp); err != nil {
			errorChan <- fmt.Errorf("failed to parse token data: %w", err)
			http.Error(w, "Invalid token format", http.StatusBadRequest)
			return
		}

		// Send success page
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Login Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Arial, sans-serif;
            text-align: center;
            padding: 50px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            margin: 0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            background: white;
            color: #333;
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            max-width: 400px;
        }
        .success {
            color: #2ecc71;
            font-size: 64px;
            margin-bottom: 20px;
        }
        h1 {
            margin: 0 0 10px 0;
            font-size: 24px;
        }
        .message {
            font-size: 16px;
            margin-top: 20px;
            color: #666;
        }
        .countdown {
            font-size: 14px;
            color: #999;
            margin-top: 30px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="success">✓</div>
        <h1>Authentication Successful!</h1>
        <p class="message">You can close this window and return to the terminal.</p>
        <p class="countdown">This window will close automatically in <span id="seconds">3</span> seconds...</p>
    </div>
    <script>
        var seconds = 3;
        var countdown = setInterval(function() {
            seconds--;
            document.getElementById('seconds').textContent = seconds;
            if (seconds <= 0) {
                clearInterval(countdown);
                window.close();
            }
        }, 1000);
    </script>
</body>
</html>
		`)

		callbackChan <- &loginResp
	})

	server := &http.Server{
		Addr:    ":8089",
		Handler: mux,
	}

	go server.ListenAndServe()
	return server
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default: // linux, freebsd, etc.
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
