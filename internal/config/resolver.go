package config

import (
	"context"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ConfigSourceResolver resolves ConfigSource values from ConfigMaps/Secrets
type ConfigSourceResolver struct {
	client    *kubernetes.Clientset
	namespace string
	cache     *ConfigSourceCache
}

// NewConfigSourceResolver creates a new resolver with default 1 minute cache TTL
// namespace can be empty if not running in Kubernetes
func NewConfigSourceResolver(namespace string) (*ConfigSourceResolver, error) {
	return NewConfigSourceResolverWithTTL(namespace, 1*time.Minute)
}

// NewConfigSourceResolverWithTTL creates a new resolver with custom cache TTL
func NewConfigSourceResolverWithTTL(namespace string, cacheTTL time.Duration) (*ConfigSourceResolver, error) {
	// Try to create K8s client, but don't fail if not in K8s
	config, err := rest.InClusterConfig()
	if err != nil {
		// Try loading from kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			// Not in Kubernetes - return resolver that will log errors
			return &ConfigSourceResolver{
				namespace: namespace,
				cache:     NewConfigSourceCache(cacheTTL),
			}, nil
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		// Can't create client - return resolver that will log errors
		return &ConfigSourceResolver{
			namespace: namespace,
			cache:     NewConfigSourceCache(cacheTTL),
		}, nil
	}

	return &ConfigSourceResolver{
		client:    client,
		namespace: namespace,
		cache:     NewConfigSourceCache(cacheTTL),
	}, nil
}

// ResolveConfigSource resolves a ConfigSource value with caching
// Returns the resolved value, whether the source exists, and any error
func (r *ConfigSourceResolver) ResolveConfigSource(ctx context.Context, source ConfigSource) (string, bool, error) {
	// Plain value - return as-is
	if source.Type == ConfigSourceTypePlain || source.Type == "" {
		return source.Value, true, nil
	}

	// Need Kubernetes client for ConfigMap/Secret resolution
	if r.client == nil {
		log.Printf("Warning: ConfigSource requires Kubernetes client but not available (type: %s, ref: %s)", source.Type, source.Ref)
		return source.Value, false, fmt.Errorf("kubernetes client not available")
	}

	// Determine namespace for cache key
	namespace := r.namespace
	if namespace == "" {
		namespace = "default"
	}

	// Check cache first
	cacheKey := GetCacheKey(namespace, source)
	if cachedValue, exists, cachedErr, found := r.cache.Get(cacheKey); found {
		return cachedValue, exists, cachedErr
	}

	// Determine resource name
	resourceName := source.Ref
	if resourceName == "" {
		resourceName = source.RefName
	}
	if resourceName == "" {
		return source.Value, false, fmt.Errorf("ConfigSource ref or ref_name is required for type %s", source.Type)
	}

	// Determine key name in ConfigMap/Secret
	keyName := source.RefName
	if keyName == "" || keyName == resourceName {
		keyName = "value"
	}

	// Fetch from Kubernetes and cache the result
	var value string
	var exists bool
	var fetchErr error

	if source.Type == ConfigSourceTypeConfigMap {
		cm, err := r.client.CoreV1().ConfigMaps(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Warning: Failed to read ConfigMap %s/%s: %v", namespace, resourceName, err)
			fetchErr = err
			exists = false
			value = source.Value
		} else {
			val, ok := cm.Data[keyName]
			if !ok {
				// Try "value" as fallback
				if keyName != "value" {
					val, ok = cm.Data["value"]
				}
				if !ok {
					log.Printf("Warning: Key %s not found in ConfigMap %s/%s", keyName, namespace, resourceName)
					fetchErr = fmt.Errorf("key %s not found in ConfigMap", keyName)
					exists = false
					value = source.Value
				} else {
					value = val
					exists = true
				}
			} else {
				value = val
				exists = true
			}
		}
	} else if source.Type == ConfigSourceTypeSecret {
		secret, err := r.client.CoreV1().Secrets(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Warning: Failed to read Secret %s/%s: %v", namespace, resourceName, err)
			fetchErr = err
			exists = false
			value = source.Value
		} else {
			valueBytes, ok := secret.Data[keyName]
			if !ok {
				// Try "value" as fallback
				if keyName != "value" {
					valueBytes, ok = secret.Data["value"]
				}
				if !ok {
					log.Printf("Warning: Key %s not found in Secret %s/%s", keyName, namespace, resourceName)
					fetchErr = fmt.Errorf("key %s not found in Secret", keyName)
					exists = false
					value = source.Value
				} else {
					value = string(valueBytes)
					exists = true
				}
			} else {
				value = string(valueBytes)
				exists = true
			}
		}
	} else {
		fetchErr = fmt.Errorf("unknown ConfigSource type: %s", source.Type)
		exists = false
		value = source.Value
	}

	// Cache the result (even errors are cached to avoid hammering K8s API)
	r.cache.Set(cacheKey, value, exists, fetchErr)

	return value, exists, fetchErr
}

// ConfigErrors tracks missing ConfigMaps/Secrets
type ConfigErrors struct {
	MissingConfigMaps []string                          `json:"missing_configmaps"`
	MissingSecrets    []string                          `json:"missing_secrets"`
	Warnings          []string                          `json:"warnings"`
	Connections       map[string]*ConnectionConfigErrors `json:"connections,omitempty"`
}

// ConnectionConfigErrors tracks errors for a specific connection
type ConnectionConfigErrors struct {
	MissingConfigMaps []string `json:"missing_configmaps"`
	MissingSecrets    []string `json:"missing_secrets"`
	Warnings          []string `json:"warnings"`
}

// ResolveConfig resolves all ConfigSource fields in a Config
func (r *ConfigSourceResolver) ResolveConfig(ctx context.Context, cfg *Config) (*ConfigErrors, error) {
	errors := &ConfigErrors{
		MissingConfigMaps: []string{},
		MissingSecrets:    []string{},
		Warnings:          []string{},
		Connections:       make(map[string]*ConnectionConfigErrors),
	}

	// Resolve JWT Secret - CRITICAL: fail if not found
	if cfg.Auth.JWTSecret.Type != ConfigSourceTypePlain && cfg.Auth.JWTSecret.Type != "" {
		value, exists, err := r.ResolveConfigSource(ctx, cfg.Auth.JWTSecret)
		if err == nil && exists {
			cfg.Auth.JWTSecret.Value = value
		} else {
			ref := cfg.Auth.JWTSecret.Ref
			if ref == "" {
				ref = cfg.Auth.JWTSecret.RefName
			}
			// JWT Secret is critical - return error instead of warning
			if cfg.Auth.JWTSecret.Type == ConfigSourceTypeConfigMap {
				return nil, fmt.Errorf("CRITICAL: JWT Secret ConfigMap %s not found - cannot start without valid JWT secret", ref)
			} else if cfg.Auth.JWTSecret.Type == ConfigSourceTypeSecret {
				return nil, fmt.Errorf("CRITICAL: JWT Secret %s not found - cannot start without valid JWT secret", ref)
			}
		}
	}
	
	// Validate JWT Secret has a value
	if cfg.Auth.JWTSecret.Value == "" {
		return nil, fmt.Errorf("CRITICAL: JWT Secret is empty - cannot start without valid JWT secret")
	}

	// Resolve connection backend credentials
	for i := range cfg.Connections {
		conn := &cfg.Connections[i]
		connErrors := &ConnectionConfigErrors{
			MissingConfigMaps: []string{},
			MissingSecrets:    []string{},
			Warnings:          []string{},
		}

		if conn.BackendUsername.Type != ConfigSourceTypePlain && conn.BackendUsername.Type != "" {
			value, exists, err := r.ResolveConfigSource(ctx, conn.BackendUsername)
			if err == nil && exists {
				conn.BackendUsername.Value = value
			} else {
				ref := conn.BackendUsername.Ref
				if ref == "" {
					ref = conn.BackendUsername.RefName
				}
				if conn.BackendUsername.Type == ConfigSourceTypeConfigMap {
					connErrors.MissingConfigMaps = append(connErrors.MissingConfigMaps, ref)
					errors.MissingConfigMaps = append(errors.MissingConfigMaps, ref)
				} else if conn.BackendUsername.Type == ConfigSourceTypeSecret {
					connErrors.MissingSecrets = append(connErrors.MissingSecrets, ref)
					errors.MissingSecrets = append(errors.MissingSecrets, ref)
				}
			}
		}

		if conn.BackendPassword.Type != ConfigSourceTypePlain && conn.BackendPassword.Type != "" {
			value, exists, err := r.ResolveConfigSource(ctx, conn.BackendPassword)
			if err == nil && exists {
				conn.BackendPassword.Value = value
			} else {
				ref := conn.BackendPassword.Ref
				if ref == "" {
					ref = conn.BackendPassword.RefName
				}
				if conn.BackendPassword.Type == ConfigSourceTypeConfigMap {
					connErrors.MissingConfigMaps = append(connErrors.MissingConfigMaps, ref)
					errors.MissingConfigMaps = append(errors.MissingConfigMaps, ref)
				} else if conn.BackendPassword.Type == ConfigSourceTypeSecret {
					connErrors.MissingSecrets = append(connErrors.MissingSecrets, ref)
					errors.MissingSecrets = append(errors.MissingSecrets, ref)
				}
			}
		}

		if len(connErrors.MissingConfigMaps) > 0 || len(connErrors.MissingSecrets) > 0 {
			errors.Connections[conn.Name] = connErrors
		}
	}

	// Resolve Slack Webhook URL (non-critical - warning only)
	if cfg.Approval != nil && cfg.Approval.Slack != nil {
		if cfg.Approval.Slack.WebhookURL.Type != ConfigSourceTypePlain && cfg.Approval.Slack.WebhookURL.Type != "" {
			value, exists, err := r.ResolveConfigSource(ctx, cfg.Approval.Slack.WebhookURL)
			if err == nil && exists {
				cfg.Approval.Slack.WebhookURL.Value = value
			} else {
				ref := cfg.Approval.Slack.WebhookURL.Ref
				if ref == "" {
					ref = cfg.Approval.Slack.WebhookURL.RefName
				}
				if cfg.Approval.Slack.WebhookURL.Type == ConfigSourceTypeConfigMap {
					errors.MissingConfigMaps = append(errors.MissingConfigMaps, ref)
					errors.Warnings = append(errors.Warnings, fmt.Sprintf("Slack Webhook URL: ConfigMap %s not found", ref))
				} else if cfg.Approval.Slack.WebhookURL.Type == ConfigSourceTypeSecret {
					errors.MissingSecrets = append(errors.MissingSecrets, ref)
					errors.Warnings = append(errors.Warnings, fmt.Sprintf("Slack Webhook URL: Secret %s not found", ref))
				}
			}
		}
	}
	
	// Resolve generic Webhook URL (non-critical - warning only)
	if cfg.Approval != nil && cfg.Approval.Webhook != nil {
		if cfg.Approval.Webhook.URL.Type != ConfigSourceTypePlain && cfg.Approval.Webhook.URL.Type != "" {
			value, exists, err := r.ResolveConfigSource(ctx, cfg.Approval.Webhook.URL)
			if err == nil && exists {
				cfg.Approval.Webhook.URL.Value = value
			} else {
				ref := cfg.Approval.Webhook.URL.Ref
				if ref == "" {
					ref = cfg.Approval.Webhook.URL.RefName
				}
				if cfg.Approval.Webhook.URL.Type == ConfigSourceTypeConfigMap {
					errors.MissingConfigMaps = append(errors.MissingConfigMaps, ref)
					errors.Warnings = append(errors.Warnings, fmt.Sprintf("Approval Webhook URL: ConfigMap %s not found", ref))
				} else if cfg.Approval.Webhook.URL.Type == ConfigSourceTypeSecret {
					errors.MissingSecrets = append(errors.MissingSecrets, ref)
					errors.Warnings = append(errors.Warnings, fmt.Sprintf("Approval Webhook URL: Secret %s not found", ref))
				}
			}
		}
	}

	// Resolve OIDC client_secret if stored as ConfigSource - CRITICAL if provider is enabled
	for i := range cfg.Auth.Providers {
		provider := &cfg.Auth.Providers[i]
		if provider.Type == "oidc" {
			if provider.ClientSecret.Type != ConfigSourceTypePlain && provider.ClientSecret.Type != "" {
				value, exists, err := r.ResolveConfigSource(ctx, provider.ClientSecret)
				if err == nil && exists {
					// Update the Config map with resolved value
					if provider.Config == nil {
						provider.Config = make(map[string]string)
					}
					provider.Config["client_secret"] = value
					provider.ClientSecret.Value = value
				} else {
					// If provider is enabled, this is critical
					if provider.Enabled {
						ref := provider.ClientSecret.Ref
						if ref == "" {
							ref = provider.ClientSecret.RefName
						}
						if provider.ClientSecret.Type == ConfigSourceTypeConfigMap {
							return nil, fmt.Errorf("CRITICAL: OIDC provider '%s' is enabled but client_secret ConfigMap %s not found", provider.Name, ref)
						} else if provider.ClientSecret.Type == ConfigSourceTypeSecret {
							return nil, fmt.Errorf("CRITICAL: OIDC provider '%s' is enabled but client_secret Secret %s not found", provider.Name, ref)
						}
					} else {
						// Provider disabled, just log warning
						ref := provider.ClientSecret.Ref
						if ref == "" {
							ref = provider.ClientSecret.RefName
						}
						if provider.ClientSecret.Type == ConfigSourceTypeConfigMap {
							errors.MissingConfigMaps = append(errors.MissingConfigMaps, ref)
							errors.Warnings = append(errors.Warnings, fmt.Sprintf("OIDC provider '%s' (disabled): ConfigMap %s not found", provider.Name, ref))
						} else if provider.ClientSecret.Type == ConfigSourceTypeSecret {
							errors.MissingSecrets = append(errors.MissingSecrets, ref)
							errors.Warnings = append(errors.Warnings, fmt.Sprintf("OIDC provider '%s' (disabled): Secret %s not found", provider.Name, ref))
						}
					}
				}
			}
			
			// Validate client_secret has a value if provider is enabled
			if provider.Enabled {
				clientSecret := provider.ClientSecret.Value
				if clientSecret == "" && provider.Config != nil {
					clientSecret = provider.Config["client_secret"]
				}
				if clientSecret == "" {
					return nil, fmt.Errorf("CRITICAL: OIDC provider '%s' is enabled but client_secret is empty", provider.Name)
				}
			}
		}
	}

	return errors, nil
}

// ListConfigMaps returns a list of ConfigMap names in the resolver's namespace
func (r *ConfigSourceResolver) ListConfigMaps(ctx context.Context) ([]string, error) {
	namespace := r.namespace
	if namespace == "" {
		namespace = "default"
	}
	return r.ListConfigMapsInNamespace(ctx, namespace)
}

// ListConfigMapsInNamespace returns a list of ConfigMap names in the specified namespace
func (r *ConfigSourceResolver) ListConfigMapsInNamespace(ctx context.Context, namespace string) ([]string, error) {
	if r.client == nil {
		return []string{}, fmt.Errorf("kubernetes client not available")
	}

	if namespace == "" {
		namespace = "default"
	}

	configMapList, err := r.client.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{}, err
	}

	names := make([]string, 0, len(configMapList.Items))
	for _, cm := range configMapList.Items {
		names = append(names, cm.Name)
	}

	return names, nil
}

// ListSecrets returns a list of Secret names in the resolver's namespace
func (r *ConfigSourceResolver) ListSecrets(ctx context.Context) ([]string, error) {
	namespace := r.namespace
	if namespace == "" {
		namespace = "default"
	}
	return r.ListSecretsInNamespace(ctx, namespace)
}

// ListSecretsInNamespace returns a list of Secret names in the specified namespace
func (r *ConfigSourceResolver) ListSecretsInNamespace(ctx context.Context, namespace string) ([]string, error) {
	if r.client == nil {
		return []string{}, fmt.Errorf("kubernetes client not available")
	}

	if namespace == "" {
		namespace = "default"
	}

	secretList, err := r.client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{}, err
	}

	names := make([]string, 0, len(secretList.Items))
	for _, secret := range secretList.Items {
		names = append(names, secret.Name)
	}

	return names, nil
}

