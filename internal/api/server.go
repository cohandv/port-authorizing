package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/davidcohan/port-authorizing/internal/approval"
	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/authorization"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// Server represents the API server
type Server struct {
	config         *config.Config
	configMu       sync.RWMutex
	storageBackend config.StorageBackend
	router         *mux.Router
	httpServer     *http.Server
	connMgr        *proxy.ConnectionManager
	authSvc        *AuthService
	authz          *authorization.Authorizer
	approvalMgr    *approval.Manager
	resolver       *config.ConfigSourceResolver
}

// NewServer creates a new API server instance
func NewServer(cfg *config.Config) (*Server, error) {
	// Configure audit system
	audit.SetEnvironment(cfg.Environment)
	
	memoryMB := cfg.Logging.AuditMemoryMB
	if memoryMB == 0 {
		memoryMB = 1 // Default to 1MB
	}
	audit.ConfigureMemoryBuffer(memoryMB)

	// Initialize storage backend
	storageBackend, err := config.NewStorageBackend(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Initialize ConfigSourceResolver for dynamic credential resolution
	resolverNamespace := ""
	if cfg.Storage != nil {
		resolverNamespace = cfg.Storage.Namespace
	}
	cacheTTL := cfg.Server.Cache.TTL
	if cacheTTL == 0 {
		cacheTTL = 1 * time.Minute
	}
	resolver, err := config.NewConfigSourceResolverWithTTL(resolverNamespace, cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create config resolver: %w", err)
	}

	authSvc, err := NewAuthService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	// Initialize approval manager
	approvalMgr := approval.NewManager(5*time.Minute, cfg.Environment) // Default 5 minute timeout

	// Configure approval providers if enabled
	if cfg.Approval != nil && cfg.Approval.Enabled {
		providersConfigured := 0

		// Register webhook provider if configured (even if currently empty - allows dynamic enable/disable)
		if cfg.Approval.Webhook != nil {
			webhookProvider := approval.NewWebhookProvider(cfg.Approval.Webhook.URL, resolver)
			approvalMgr.RegisterProvider(webhookProvider)
			if cfg.Approval.Webhook.URL.Value != "" {
				providersConfigured++
				fmt.Printf("✓ Registered webhook approval provider (type: %s, has_value: true)\n", cfg.Approval.Webhook.URL.Type)
			} else {
				fmt.Printf("⚠ Registered webhook approval provider (type: %s, has_value: false) - will resolve dynamically\n", cfg.Approval.Webhook.URL.Type)
			}
		}

		// Register Slack provider if configured (even if currently empty - allows dynamic enable/disable)
		if cfg.Approval.Slack != nil {
			slackProvider := approval.NewSlackProvider(cfg.Approval.Slack.WebhookURL, resolver, cfg.Server.BaseURL)
			approvalMgr.RegisterProvider(slackProvider)
			if cfg.Approval.Slack.WebhookURL.Value != "" {
				providersConfigured++
				fmt.Printf("✓ Registered Slack approval provider (type: %s, has_value: true)\n", cfg.Approval.Slack.WebhookURL.Type)
			} else {
				fmt.Printf("⚠ Registered Slack approval provider (type: %s, has_value: false) - will resolve dynamically\n", cfg.Approval.Slack.WebhookURL.Type)
			}
		}

		if providersConfigured == 0 && cfg.Approval.Webhook == nil && cfg.Approval.Slack == nil {
			fmt.Printf("⚠ WARNING: Approval workflow is ENABLED but NO approval providers are configured!\n")
			fmt.Printf("   Approval requests will fail with 'no approval providers configured'.\n")
			fmt.Printf("   Please configure at least one provider (Webhook or Slack) with a valid URL.\n")
		}

		// Add approval patterns
		for _, pattern := range cfg.Approval.Patterns {
			timeout := time.Duration(pattern.TimeoutSeconds) * time.Second
			if err := approvalMgr.AddApprovalPattern(pattern.Pattern, pattern.Tags, pattern.TagMatch, pattern.Roles, pattern.RoleMatch, timeout); err != nil {
				return nil, fmt.Errorf("failed to add approval pattern: %w", err)
			}
		}
	}

	s := &Server{
		config:         cfg,
		storageBackend: storageBackend,
		router:         mux.NewRouter(),
		connMgr:        proxy.NewConnectionManager(cfg.Server.MaxConnectionDuration),
		authSvc:        authSvc,
		authz:          authorization.NewAuthorizer(cfg),
		approvalMgr:    approvalMgr,
		resolver:       resolver,
	}

	s.setupRoutes()
	return s, nil
}

// ReloadConfig reloads the configuration and updates server components
// while preserving existing connections
func (s *Server) ReloadConfig(newCfg *config.Config) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	// Reconfigure audit system
	audit.SetEnvironment(newCfg.Environment)
	
	memoryMB := newCfg.Logging.AuditMemoryMB
	if memoryMB == 0 {
		memoryMB = 1 // Default to 1MB
	}
	audit.ConfigureMemoryBuffer(memoryMB)

	// Recreate auth service
	authSvc, err := NewAuthService(newCfg)
	if err != nil {
		return fmt.Errorf("failed to create auth service: %w", err)
	}

	// Recreate authorizer
	authz := authorization.NewAuthorizer(newCfg)

	// Recreate ConfigSourceResolver for dynamic credential resolution
	resolverNamespace := ""
	if newCfg.Storage != nil {
		resolverNamespace = newCfg.Storage.Namespace
	}
	cacheTTL := newCfg.Server.Cache.TTL
	if cacheTTL == 0 {
		cacheTTL = 1 * time.Minute
	}
	newResolver, err := config.NewConfigSourceResolverWithTTL(resolverNamespace, cacheTTL)
	if err != nil {
		return fmt.Errorf("failed to create config resolver: %w", err)
	}

	// Recreate approval manager
	approvalMgr := approval.NewManager(5*time.Minute, newCfg.Environment)
	if newCfg.Approval != nil && newCfg.Approval.Enabled {
		providersConfigured := 0

		// Register webhook provider if configured (even if currently empty - allows dynamic enable/disable)
		if newCfg.Approval.Webhook != nil {
			webhookProvider := approval.NewWebhookProvider(newCfg.Approval.Webhook.URL, newResolver)
			approvalMgr.RegisterProvider(webhookProvider)
			if newCfg.Approval.Webhook.URL.Value != "" {
				providersConfigured++
				fmt.Printf("✓ Registered webhook approval provider (type: %s, has_value: true)\n", newCfg.Approval.Webhook.URL.Type)
			} else {
				fmt.Printf("⚠ Registered webhook approval provider (type: %s, has_value: false) - will resolve dynamically\n", newCfg.Approval.Webhook.URL.Type)
			}
		}

		// Register Slack provider if configured (even if currently empty - allows dynamic enable/disable)
		if newCfg.Approval.Slack != nil {
			slackProvider := approval.NewSlackProvider(newCfg.Approval.Slack.WebhookURL, newResolver, newCfg.Server.BaseURL)
			approvalMgr.RegisterProvider(slackProvider)
			if newCfg.Approval.Slack.WebhookURL.Value != "" {
				providersConfigured++
				fmt.Printf("✓ Registered Slack approval provider (type: %s, has_value: true)\n", newCfg.Approval.Slack.WebhookURL.Type)
			} else {
				fmt.Printf("⚠ Registered Slack approval provider (type: %s, has_value: false) - will resolve dynamically\n", newCfg.Approval.Slack.WebhookURL.Type)
			}
		}

		if providersConfigured == 0 && newCfg.Approval.Webhook == nil && newCfg.Approval.Slack == nil {
			fmt.Printf("⚠ WARNING: Approval workflow is ENABLED but NO approval providers are configured!\n")
			fmt.Printf("   Approval requests will fail with 'no approval providers configured'.\n")
			fmt.Printf("   Please configure at least one provider (Webhook or Slack) with a valid URL.\n")
		}

		for _, pattern := range newCfg.Approval.Patterns {
			timeout := time.Duration(pattern.TimeoutSeconds) * time.Second
			if err := approvalMgr.AddApprovalPattern(pattern.Pattern, pattern.Tags, pattern.TagMatch, pattern.Roles, pattern.RoleMatch, timeout); err != nil {
				return fmt.Errorf("failed to add approval pattern: %w", err)
			}
		}
	}

	// Update server fields
	// Note: We intentionally preserve connMgr to keep existing connections alive
	s.config = newCfg
	s.authSvc = authSvc
	s.authz = authz
	s.approvalMgr = approvalMgr
	s.resolver = newResolver

	return nil
}

// GetConfig returns the current configuration (thread-safe)
func (s *Server) GetConfig() *config.Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}

// LoadConfigFromStorage loads the latest configuration from the storage backend
func (s *Server) LoadConfigFromStorage() (*config.Config, error) {
	return s.storageBackend.Load(context.Background())
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
	s.router.HandleFunc("/api/auth/oidc/ws", s.handleOIDCWebSocket).Methods("GET")
	s.router.HandleFunc("/api/auth/oidc/login", s.handleOIDCLogin).Methods("GET", "OPTIONS")
	s.router.HandleFunc("/api/auth/oidc/callback", s.handleOIDCCallback).Methods("GET", "OPTIONS")

	// Protected routes (require authentication)
	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(s.authMiddleware)
	api.HandleFunc("/connections", s.handleListConnections).Methods("GET", "OPTIONS")
	api.HandleFunc("/connect/{name}", s.handleConnect).Methods("POST", "OPTIONS")

	// Transparent proxy endpoint - accepts TCP connection and forwards to target
	api.HandleFunc("/proxy/{connectionID}", s.handleProxyStream).Methods("POST", "GET", "PUT", "DELETE", "CONNECT", "PATCH", "OPTIONS")

	// K8s Services discovery endpoint
	// CLI calls this to get service list, then opens individual proxy connections via /api/proxy/{connID}
	api.HandleFunc("/k8s-services/{connectionID}/services", s.handleK8sServicesListServices).Methods("GET", "OPTIONS")

	// Approval endpoints (can be accessed via webhook callbacks, so don't require auth)
	s.router.HandleFunc("/api/approvals/{request_id}/approve", s.handleApproveRequest).Methods("GET", "POST", "OPTIONS")
	s.router.HandleFunc("/api/approvals/{request_id}/reject", s.handleRejectRequest).Methods("GET", "POST", "OPTIONS")

	// Admin endpoint for pending approvals (requires auth)
	api.HandleFunc("/approvals/pending", s.handleGetPendingApprovals).Methods("GET", "OPTIONS")

	// Admin API endpoints (require auth + admin role) - MUST come before /admin/ prefix
	adminAPI := s.router.PathPrefix("/admin/api").Subrouter()
	adminAPI.Use(s.authMiddleware, s.adminMiddleware)

	// Configuration management
	adminAPI.HandleFunc("/config", s.handleGetConfig).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/config", s.handleUpdateConfig).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/config/errors", s.handleGetConfigErrors).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/config/versions", s.handleListConfigVersions).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/config/versions/{id}", s.handleGetConfigVersion).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/config/rollback/{id}", s.handleRollbackConfig).Methods("POST", "OPTIONS")

	// Connection management
	adminAPI.HandleFunc("/connections", s.handleListAllConnections).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/connections", s.handleCreateConnection).Methods("POST", "OPTIONS")
	adminAPI.HandleFunc("/connections/{name}", s.handleUpdateConnection).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/connections/{name}", s.handleDeleteConnection).Methods("DELETE", "OPTIONS")

	// User management
	adminAPI.HandleFunc("/users", s.handleListUsers).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/users", s.handleCreateUser).Methods("POST", "OPTIONS")
	adminAPI.HandleFunc("/users/{username}", s.handleUpdateUser).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/users/{username}", s.handleDeleteUser).Methods("DELETE", "OPTIONS")

	// Policy management
	adminAPI.HandleFunc("/policies", s.handleListPolicies).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/policies", s.handleCreatePolicy).Methods("POST", "OPTIONS")
	adminAPI.HandleFunc("/policies/{name}", s.handleUpdatePolicy).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/policies/{name}", s.handleDeletePolicy).Methods("DELETE", "OPTIONS")

	// Policy tester
	adminAPI.HandleFunc("/policy-test", s.handlePolicyTest).Methods("POST", "OPTIONS")

	// Approval management
	adminAPI.HandleFunc("/approvals", s.handleGetApprovalConfig).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/approvals/enabled", s.handleUpdateApprovalEnabled).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/approvals/providers", s.handleUpdateApprovalProviders).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/approvals/patterns", s.handleCreateApprovalPattern).Methods("POST", "OPTIONS")
	adminAPI.HandleFunc("/approvals/patterns/{index}", s.handleUpdateApprovalPattern).Methods("PUT", "OPTIONS")
	adminAPI.HandleFunc("/approvals/patterns/{index}", s.handleDeleteApprovalPattern).Methods("DELETE", "OPTIONS")

	// Audit logs
	adminAPI.HandleFunc("/audit/logs", s.handleGetAuditLogs).Methods("GET", "OPTIONS")
	adminAPI.HandleFunc("/audit/stats", s.handleGetAuditStats).Methods("GET", "OPTIONS")

	// System status
	adminAPI.HandleFunc("/status", s.handleGetSystemStatus).Methods("GET", "OPTIONS")

	// Kubernetes resources (ConfigMaps/Secrets listing)
	adminAPI.HandleFunc("/k8s/resources", s.handleListK8sResources).Methods("GET", "OPTIONS")

	// Admin UI routes (HTML/CSS/JS served without auth - auth handled by JavaScript)
	// These MUST come after /admin/api routes to avoid conflicts
	s.router.HandleFunc("/admin", s.handleAdminUI).Methods("GET")
	s.router.HandleFunc("/admin/", s.handleAdminUI).Methods("GET")
	s.router.PathPrefix("/admin/").HandlerFunc(s.handleAdminUI).Methods("GET")
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
