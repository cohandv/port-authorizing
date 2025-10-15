package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/authorization"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// Server represents the API server
type Server struct {
	config      *config.Config
	router      *mux.Router
	httpServer  *http.Server
	connMgr     *proxy.ConnectionManager
	authSvc     *AuthService
	authz       *authorization.Authorizer
	approvalMgr *approval.Manager
}

// NewServer creates a new API server instance
func NewServer(cfg *config.Config) (*Server, error) {
	authSvc, err := NewAuthService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	// Initialize approval manager
	approvalMgr := approval.NewManager(5 * time.Minute) // Default 5 minute timeout

	// Configure approval providers if enabled
	if cfg.Approval != nil && cfg.Approval.Enabled {
		if cfg.Approval.Webhook != nil && cfg.Approval.Webhook.URL != "" {
			webhookProvider := approval.NewWebhookProvider(cfg.Approval.Webhook.URL)
			approvalMgr.RegisterProvider(webhookProvider)
		}

		if cfg.Approval.Slack != nil && cfg.Approval.Slack.WebhookURL != "" {
			slackProvider := approval.NewSlackProvider(
				cfg.Approval.Slack.WebhookURL,
				cfg.Server.BaseURL,
			)
			approvalMgr.RegisterProvider(slackProvider)
		}

		// Add approval patterns
		for _, pattern := range cfg.Approval.Patterns {
			timeout := time.Duration(pattern.TimeoutSeconds) * time.Second
			if err := approvalMgr.AddApprovalPattern(pattern.Pattern, pattern.Tags, pattern.TagMatch, timeout); err != nil {
				return nil, fmt.Errorf("failed to add approval pattern: %w", err)
			}
		}
	}

	s := &Server{
		config:      cfg,
		router:      mux.NewRouter(),
		connMgr:     proxy.NewConnectionManager(cfg.Server.MaxConnectionDuration),
		authSvc:     authSvc,
		authz:       authorization.NewAuthorizer(cfg),
		approvalMgr: approvalMgr,
	}

	s.setupRoutes()
	return s, nil
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// Apply CORS middleware to all routes (allow all origins)
	s.router.Use(s.corsMiddleware)

	// Public routes
	s.router.HandleFunc("/api/info", s.handleServerInfo).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/login", s.handleLogin).Methods("POST", "OPTIONS")
	s.router.HandleFunc("/api/health", s.handleHealth).Methods("GET", "OPTIONS")

	// OIDC authentication routes (public)
	s.router.HandleFunc("/api/auth/oidc/login", s.handleOIDCLogin).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/auth/oidc/callback", s.handleOIDCCallback).Methods("GET", "OPTIONS")

	// Protected routes (require authentication)
	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(s.authMiddleware)
	api.HandleFunc("/connections", s.handleListConnections).Methods("GET", "OPTIONS")
	api.HandleFunc("/connect/{name}", s.handleConnect).Methods("POST", "OPTIONS")

	// Transparent proxy endpoint - accepts TCP connection and forwards to target
	api.HandleFunc("/proxy/{connectionID}", s.handleProxyStream).Methods("POST", "GET", "PUT", "DELETE", "CONNECT", "PATCH", "OPTIONS")

	// Approval endpoints (can be accessed via webhook callbacks, so don't require auth)
	s.router.HandleFunc("/api/approvals/{request_id}/approve", s.handleApproveRequest).Methods("GET", "POST", "OPTIONS")
	s.router.HandleFunc("/api/approvals/{request_id}/reject", s.handleRejectRequest).Methods("GET", "POST", "OPTIONS")

	// Admin endpoint for pending approvals (requires auth)
	api.HandleFunc("/approvals/pending", s.handleGetPendingApprovals).Methods("GET", "OPTIONS")
}

// corsMiddleware adds CORS headers to all responses (allow all origins)
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow all origins
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Allow common HTTP methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, CONNECT")

		// Allow common headers
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")

		// Allow credentials
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Cache preflight response for 24 hours
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Expose custom headers to JavaScript
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Close all active connections
	s.connMgr.CloseAll()

	return s.httpServer.Shutdown(ctx)
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}
