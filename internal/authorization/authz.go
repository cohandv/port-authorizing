package authorization

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/davidcohan/port-authorizing/internal/config"
)

// Authorizer handles authorization decisions
type Authorizer struct {
	config      *config.Config
	policies    map[string][]*config.RolePolicy // role -> policies
	connections map[string]*config.ConnectionConfig
}

// NewAuthorizer creates a new authorizer
func NewAuthorizer(cfg *config.Config) *Authorizer {
	// Index policies by role
	policyMap := make(map[string][]*config.RolePolicy)
	for i := range cfg.Policies {
		policy := &cfg.Policies[i]
		for _, role := range policy.Roles {
			policyMap[role] = append(policyMap[role], policy)
		}
	}

	// Index connections by name
	connMap := make(map[string]*config.ConnectionConfig)
	for i := range cfg.Connections {
		conn := &cfg.Connections[i]
		connMap[conn.Name] = conn
	}

	return &Authorizer{
		config:      cfg,
		policies:    policyMap,
		connections: connMap,
	}
}

// CanAccessConnection checks if user with given roles can access a connection
func (a *Authorizer) CanAccessConnection(roles []string, connectionName string) bool {
	conn, exists := a.connections[connectionName]
	if !exists {
		return false
	}

	// Check if any role grants access
	for _, role := range roles {
		if a.roleCanAccessConnection(role, conn) {
			return true
		}
	}

	return false
}

// GetWhitelistForConnection returns the whitelist patterns for a user's roles on a connection
func (a *Authorizer) GetWhitelistForConnection(roles []string, connectionName string) []string {
	conn, exists := a.connections[connectionName]
	if !exists {
		return nil
	}

	// Legacy: if connection has direct whitelist and no tags, use it
	//nolint:staticcheck // SA1019: Supporting deprecated Whitelist field for backwards compatibility
	if len(conn.Whitelist) > 0 && len(conn.Tags) == 0 {
		//nolint:staticcheck // SA1019: Supporting deprecated Whitelist field for backwards compatibility
		return conn.Whitelist
	}

	// Collect whitelists from all matching policies
	whitelistMap := make(map[string]bool)
	for _, role := range roles {
		policies, exists := a.policies[role]
		if !exists {
			continue
		}

		for _, policy := range policies {
			if a.policyMatchesConnection(policy, conn) {
				for _, pattern := range policy.Whitelist {
					whitelistMap[pattern] = true
				}
			}
		}
	}

	// Convert map to slice
	whitelist := make([]string, 0, len(whitelistMap))
	for pattern := range whitelistMap {
		whitelist = append(whitelist, pattern)
	}

	return whitelist
}

// roleCanAccessConnection checks if a specific role can access a connection
func (a *Authorizer) roleCanAccessConnection(role string, conn *config.ConnectionConfig) bool {
	policies, exists := a.policies[role]
	if !exists {
		return false
	}

	// If connection has no tags, check for policies with no tags (legacy mode)
	if len(conn.Tags) == 0 {
		for _, policy := range policies {
			if len(policy.Tags) == 0 {
				return true
			}
		}
		return false
	}

	// Check if any policy matches this connection's tags
	for _, policy := range policies {
		if a.policyMatchesConnection(policy, conn) {
			return true
		}
	}

	return false
}

// policyMatchesConnection checks if a policy's tags match a connection's tags
func (a *Authorizer) policyMatchesConnection(policy *config.RolePolicy, conn *config.ConnectionConfig) bool {
	if len(policy.Tags) == 0 {
		return false
	}

	if len(conn.Tags) == 0 {
		return false
	}

	tagMatch := policy.TagMatch
	if tagMatch == "" {
		tagMatch = "all"
	}

	// Convert connection tags to a set for faster lookup
	connTags := make(map[string]bool)
	for _, tag := range conn.Tags {
		connTags[tag] = true
	}

	if tagMatch == "any" {
		// At least one policy tag must match
		for _, policyTag := range policy.Tags {
			if connTags[policyTag] {
				return true
			}
		}
		return false
	}

	// Default: "all" - all policy tags must be present in connection
	for _, policyTag := range policy.Tags {
		if !connTags[policyTag] {
			return false
		}
	}
	return true
}

// ValidatePattern checks if a query/request matches whitelist patterns
func (a *Authorizer) ValidatePattern(query string, whitelist []string) error {
	if len(whitelist) == 0 {
		// No whitelist means everything is allowed
		return nil
	}

	for _, pattern := range whitelist {
		matched, err := regexp.MatchString(pattern, query)
		if err != nil {
			return fmt.Errorf("invalid whitelist pattern: %s", pattern)
		}
		if matched {
			return nil
		}
	}

	return fmt.Errorf("query does not match any whitelist pattern")
}

// ListAccessibleConnections returns all connections a user with given roles can access
func (a *Authorizer) ListAccessibleConnections(roles []string) []string {
	accessible := make(map[string]bool)

	for connName, conn := range a.connections {
		for _, role := range roles {
			if a.roleCanAccessConnection(role, conn) {
				accessible[connName] = true
				break
			}
		}
	}

	result := make([]string, 0, len(accessible))
	for connName := range accessible {
		result = append(result, connName)
	}

	return result
}

// GetConnectionInfo returns connection configuration (without sensitive data)
func (a *Authorizer) GetConnectionInfo(connectionName string) map[string]interface{} {
	conn, exists := a.connections[connectionName]
	if !exists {
		return nil
	}

	return map[string]interface{}{
		"name":     conn.Name,
		"type":     conn.Type,
		"host":     conn.Host,
		"port":     conn.Port,
		"scheme":   conn.Scheme,
		"tags":     conn.Tags,
		"metadata": conn.Metadata,
	}
}

// Helper function to check if string slice contains a value
//nolint:unused // Reserved for future tag matching logic
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
