package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/davidcohan/port-authorizing/internal/authorization"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// Server represents the API server
type Server struct {
	config     *config.Config
	router     *mux.Router
	httpServer *http.Server
	connMgr    *proxy.ConnectionManager
	authSvc    *AuthService
	authz      *authorization.Authorizer
}

// NewServer creates a new API server instance
func NewServer(cfg *config.Config) (*Server, error) {
	authSvc, err := NewAuthService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	s := &Server{
		config:  cfg,
		router:  mux.NewRouter(),
		connMgr: proxy.NewConnectionManager(cfg.Server.MaxConnectionDuration),
		authSvc: authSvc,
		authz:   authorization.NewAuthorizer(cfg),
	}

	s.setupRoutes()
	return s, nil
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// Public routes
	s.router.HandleFunc("/api/login", s.handleLogin).Methods("POST")
	s.router.HandleFunc("/api/health", s.handleHealth).Methods("GET")
	
	// OIDC authentication routes (public)
	s.router.HandleFunc("/api/auth/oidc/login", s.handleOIDCLogin).Methods("GET")
	s.router.HandleFunc("/api/auth/oidc/callback", s.handleOIDCCallback).Methods("GET")

	// Protected routes (require authentication)
	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(s.authMiddleware)
	api.HandleFunc("/connections", s.handleListConnections).Methods("GET")
	api.HandleFunc("/connect/{name}", s.handleConnect).Methods("POST")

	// Transparent proxy endpoint - accepts TCP connection and forwards to target
	api.HandleFunc("/proxy/{connectionID}", s.handleProxyStream).Methods("POST", "GET", "PUT", "DELETE", "CONNECT")
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
