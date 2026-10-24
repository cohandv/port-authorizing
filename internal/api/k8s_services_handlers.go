package api

import (
	"net/http"
	"strings"

	"github.com/davidcohan/port-authorizing/internal/audit"
	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/davidcohan/port-authorizing/internal/proxy"
	"github.com/gorilla/mux"
)

// handleK8sServicesListServices returns the list of discovered K8s services
// This is the discovery endpoint - CLI calls this to get the service list, then opens
// individual proxy connections for each service using /api/proxy/{connID}
func (s *Server) handleK8sServicesListServices(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ContextKeyUsername).(string)
	vars := mux.Vars(r)
	connectionID := vars["connectionID"]

	// Validate connection exists and hasn't expired
	conn, err := s.connMgr.GetConnection(connectionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Connection not found or expired")
		return
	}

	// Verify ownership
	if conn.Username != username {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Verify connection type
	if conn.Config.Type != "k8s_services" {
		respondError(w, http.StatusBadRequest, "Connection is not a k8s_services type")
		return
	}

	// Create K8s service discovery
	discovery, err := proxy.NewK8sServiceDiscovery(
		conn.Config.K8sExcludeNamespaces,
		conn.Config.K8sIncludeNamespaces,
		conn.Config.K8sIgnoreServices,
	)
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_services_discovery_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		respondError(w, http.StatusInternalServerError, "Failed to create K8s service discovery: "+err.Error())
		return
	}

	// Discover services
	services, err := discovery.DiscoverServicesGroupedByNamespace(r.Context())
	if err != nil {
		_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_services_discovery_error", conn.Config.Name, map[string]interface{}{
			"connection_id": connectionID,
			"error":         err.Error(),
		})
		respondError(w, http.StatusInternalServerError, "Failed to discover services: "+err.Error())
		return
	}

	// Filter services based on expose annotations
	filteredServices := filterServicesForExpose(services)

	// Count totals for audit
	totalServices := 0
	for _, svcList := range filteredServices {
		totalServices += len(svcList)
	}

	_ = audit.Log(s.config.Logging.AuditLogPath, username, "k8s_services_discovery", conn.Config.Name, map[string]interface{}{
		"connection_id":    connectionID,
		"namespaces_count": len(filteredServices),
		"services_count":   totalServices,
	})

	// Return service list with port mappings info
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"services":      filteredServices,
		"connection_id": connectionID,
	})
}

// determineK8sPortProtocol determines the protocol for a K8s service port
// Priority order:
//  1. Explicit port mappings in connection config (highest priority)
//  2. Service annotations (cohandv.port-authorizer.io/port.$PORT.type)
//  3. Default to TCP (raw bytes, no filtering)
func determineK8sPortProtocol(mappings []config.K8sPortMapping, annotations map[string]string, portName string, port int32) string {
	// 1. Check explicit port mappings (connection config takes priority)
	for _, mapping := range mappings {
		// Match by port name (case-insensitive)
		if mapping.PortName != "" && strings.EqualFold(mapping.PortName, portName) {
			return strings.ToLower(mapping.Protocol)
		}
		// Match by port number
		if mapping.Port > 0 && mapping.Port == port {
			return strings.ToLower(mapping.Protocol)
		}
	}

	// 2. Check service annotations
	if annotationType := proxy.GetPortTypeFromAnnotations(annotations, portName, port); annotationType != "" {
		return annotationType
	}

	// 3. Default to TCP - no auto-detection
	return "tcp"
}

// filterServicesForExpose filters out ports with expose: "false" annotations
// Services with all ports filtered out are removed entirely
func filterServicesForExpose(services map[string][]proxy.DiscoveredService) map[string][]proxy.DiscoveredService {
	filtered := make(map[string][]proxy.DiscoveredService)

	for ns, svcList := range services {
		var filteredSvcs []proxy.DiscoveredService

		for _, svc := range svcList {
			// Filter ports
			var exposedPorts []proxy.ServicePort
			for _, port := range svc.Ports {
				if proxy.IsPortExposed(svc.Annotations, port.Name, port.Port) {
					exposedPorts = append(exposedPorts, port)
				}
			}

			// Only include service if it has at least one exposed port
			if len(exposedPorts) > 0 {
				filteredSvc := svc
				filteredSvc.Ports = exposedPorts
				filteredSvcs = append(filteredSvcs, filteredSvc)
			}
		}

		if len(filteredSvcs) > 0 {
			filtered[ns] = filteredSvcs
		}
	}

	return filtered
}
