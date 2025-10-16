package api

import (
	"net/http"
)

// adminMiddleware checks if the user has the admin role
func (s *Server) adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get roles from context (set by authMiddleware)
		rolesInterface := r.Context().Value(ContextKeyRoles)
		if rolesInterface == nil {
			respondError(w, http.StatusForbidden, "Admin role required")
			return
		}

		roles, ok := rolesInterface.([]string)
		if !ok {
			respondError(w, http.StatusForbidden, "Invalid roles")
			return
		}

		// Check if user has admin role
		hasAdmin := false
		for _, role := range roles {
			if role == "admin" {
				hasAdmin = true
				break
			}
		}

		if !hasAdmin {
			respondError(w, http.StatusForbidden, "Admin role required")
			return
		}

		next.ServeHTTP(w, r)
	})
}
