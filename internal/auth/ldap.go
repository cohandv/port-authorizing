package auth

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/davidcohan/port-authorizing/internal/config"
	"github.com/go-ldap/ldap/v3"
)

// LDAPProvider implements LDAP authentication
type LDAPProvider struct {
	name          string
	url           string
	bindDN        string
	bindPassword  string
	userBaseDN    string
	userFilter    string
	groupBaseDN   string
	groupFilter   string
	useTLS        bool
	skipTLSVerify bool
}

// NewLDAPProvider creates a new LDAP provider
func NewLDAPProvider(cfg config.AuthProviderConfig) (*LDAPProvider, error) {
	url, ok := cfg.Config["url"]
	if !ok {
		return nil, fmt.Errorf("ldap url not configured")
	}

	bindDN, ok := cfg.Config["bind_dn"]
	if !ok {
		return nil, fmt.Errorf("bind_dn not configured")
	}

	bindPassword, ok := cfg.Config["bind_password"]
	if !ok {
		return nil, fmt.Errorf("bind_password not configured")
	}

	userBaseDN, ok := cfg.Config["user_base_dn"]
	if !ok {
		return nil, fmt.Errorf("user_base_dn not configured")
	}

	userFilter := cfg.Config["user_filter"]
	if userFilter == "" {
		userFilter = "(uid=%s)"
	}

	groupBaseDN := cfg.Config["group_base_dn"]
	groupFilter := cfg.Config["group_filter"]
	if groupFilter == "" {
		groupFilter = "(member=%s)"
	}

	useTLS := cfg.Config["use_tls"] == "true"
	skipTLSVerify := cfg.Config["skip_tls_verify"] == "true"

	return &LDAPProvider{
		name:          cfg.Name,
		url:           url,
		bindDN:        bindDN,
		bindPassword:  bindPassword,
		userBaseDN:    userBaseDN,
		userFilter:    userFilter,
		groupBaseDN:   groupBaseDN,
		groupFilter:   groupFilter,
		useTLS:        useTLS,
		skipTLSVerify: skipTLSVerify,
	}, nil
}

// Authenticate validates username and password against LDAP
func (p *LDAPProvider) Authenticate(credentials map[string]string) (*UserInfo, error) {
	username, ok := credentials["username"]
	if !ok {
		return nil, fmt.Errorf("username not provided")
	}

	password, ok := credentials["password"]
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	// Connect to LDAP
	var l *ldap.Conn
	var err error

	if p.useTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: p.skipTLSVerify,
		}
		//nolint:staticcheck // SA1019: Using DialTLS for compatibility
		l, err = ldap.DialTLS("tcp", p.url, tlsConfig)
	} else {
		//nolint:staticcheck // SA1019: Using Dial for compatibility
		l, err = ldap.Dial("tcp", p.url)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP: %w", err)
	}
	defer func() { _ = l.Close() }()

	// Bind as service account
	err = l.Bind(p.bindDN, p.bindPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to LDAP: %w", err)
	}

	// Search for user
	searchFilter := fmt.Sprintf(p.userFilter, ldap.EscapeFilter(username))
	searchRequest := ldap.NewSearchRequest(
		p.userBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		searchFilter,
		[]string{"dn", "cn", "mail", "uid"},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to search LDAP: %w", err)
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("multiple users found")
	}

	userDN := sr.Entries[0].DN
	email := sr.Entries[0].GetAttributeValue("mail")

	// Try to bind as the user to verify password
	err = l.Bind(userDN, password)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Get user groups if configured
	roles := []string{}
	if p.groupBaseDN != "" {
		// Bind back as service account to search groups
		err = l.Bind(p.bindDN, p.bindPassword)
		if err != nil {
			return nil, fmt.Errorf("failed to rebind as service account: %w", err)
		}

		groupFilter := fmt.Sprintf(p.groupFilter, ldap.EscapeFilter(userDN))
		groupSearchRequest := ldap.NewSearchRequest(
			p.groupBaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			groupFilter,
			[]string{"cn"},
			nil,
		)

		gsr, err := l.Search(groupSearchRequest)
		if err == nil {
			for _, entry := range gsr.Entries {
				groupName := entry.GetAttributeValue("cn")
				if groupName != "" {
					roles = append(roles, groupName)
				}
			}
		}
	}

	// Extract username from DN or use provided username
	finalUsername := username
	if strings.Contains(userDN, "uid=") {
		parts := strings.Split(userDN, ",")
		if len(parts) > 0 {
			uidPart := parts[0]
			if strings.HasPrefix(uidPart, "uid=") {
				finalUsername = strings.TrimPrefix(uidPart, "uid=")
			}
		}
	}

	return &UserInfo{
		Username: finalUsername,
		Email:    email,
		Roles:    roles,
		Metadata: map[string]string{
			"provider": p.name,
			"dn":       userDN,
		},
	}, nil
}

// Name returns the provider name
func (p *LDAPProvider) Name() string {
	return p.name
}

// Type returns the provider type
func (p *LDAPProvider) Type() string {
	return "ldap"
}
