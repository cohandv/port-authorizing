package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/davidcohan/port-authorizing/internal/auth"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// ContextKeyUsername is the context key for storing username
	ContextKeyUsername ContextKey = "username"
	// ContextKeyRoles is the context key for storing user roles
	ContextKeyRoles ContextKey = "roles"
)

// AuthService handles authentication operations
type AuthService struct {
	config      *config.Config
	authManager *auth.Manager
}

// NewAuthService creates a new authentication service
func NewAuthService(cfg *config.Config) (*AuthService, error) {
	authMgr, err := auth.NewManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth manager: %w", err)
	}

	return &AuthService{
		config:      cfg,
		authManager: authMgr,
	}, nil
}

// Claims represents JWT token claims
type Claims struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	Email    string   `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// LoginRequest represents login credentials
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents login response
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserInfo  `json:"user"`
}

// UserInfo in response
type UserInfo struct {
	Username string   `json:"username"`
	Email    string   `json:"email,omitempty"`
	Roles    []string `json:"roles"`
}

// handleLogin handles user login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Authenticate user via auth manager
	credentials := map[string]string{
		"username": req.Username,
		"password": req.Password,
	}

	userInfo, err := s.authSvc.authManager.Authenticate(credentials)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Generate JWT token
	token, expiresAt, err := s.authSvc.generateToken(userInfo)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User: UserInfo{
			Username: userInfo.Username,
			Email:    userInfo.Email,
			Roles:    userInfo.Roles,
		},
	})
}

// generateToken creates a new JWT token
func (a *AuthService) generateToken(userInfo *auth.UserInfo) (string, time.Time, error) {
	expiresAt := time.Now().Add(a.config.Auth.TokenExpiry)
	claims := &Claims{
		Username: userInfo.Username,
		Roles:    userInfo.Roles,
		Email:    userInfo.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(a.config.Auth.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// validateToken validates and parses a JWT token
func (a *AuthService) validateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.config.Auth.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// authMiddleware validates JWT tokens
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "Missing authorization header")
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondError(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		claims, err := s.authSvc.validateToken(parts[1])
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		// Add username and roles to context
		ctx := context.WithValue(r.Context(), ContextKeyUsername, claims.Username)
		ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
