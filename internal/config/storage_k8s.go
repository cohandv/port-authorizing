//go:build k8s
// +build k8s

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// K8sBackend implements storage backed by Kubernetes ConfigMap or Secret
type K8sBackend struct {
	client       *kubernetes.Clientset
	namespace    string
	resourceType string // "configmap" or "secret"
	resourceName string
	maxVersions  int
}

// NewK8sBackend creates a new Kubernetes-based storage backend
func NewK8sBackend(cfg *StorageConfig) (*K8sBackend, error) {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	if cfg.ResourceType == "" {
		cfg.ResourceType = "configmap"
	}
	if cfg.ResourceName == "" {
		cfg.ResourceName = "port-authorizing-config"
	}
	if cfg.Versions == 0 {
		cfg.Versions = 5
	}

	return &K8sBackend{
		client:       clientset,
		namespace:    cfg.Namespace,
		resourceType: cfg.ResourceType,
		resourceName: cfg.ResourceName,
		maxVersions:  cfg.Versions,
	}, nil
}

// Load loads the current configuration from ConfigMap or Secret
func (kb *K8sBackend) Load() (*Config, error) {
	ctx := context.Background()

	var data []byte
	var err error

	if kb.resourceType == "secret" {
		secret, err := kb.client.CoreV1().Secrets(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret: %w", err)
		}
		data = secret.Data["config.yaml"]
	} else {
		configMap, err := kb.client.CoreV1().ConfigMaps(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get configmap: %w", err)
		}
		data = []byte(configMap.Data["config.yaml"])
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no configuration data found")
	}

	var config Config
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}
	if config.Server.MaxConnectionDuration == 0 {
		config.Server.MaxConnectionDuration = 2 * time.Hour
	}
	if config.Auth.TokenExpiry == 0 {
		config.Auth.TokenExpiry = 24 * time.Hour
	}
	if config.Logging.LogLevel == "" {
		config.Logging.LogLevel = "info"
	}
	if config.Logging.AuditLogPath == "" {
		config.Logging.AuditLogPath = "audit.log"
	}

	return &config, nil
}

// Save saves the configuration to ConfigMap or Secret with versioning in annotations
func (kb *K8sBackend) Save(cfg *Config, comment string) error {
	ctx := context.Background()

	// Marshal configuration to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create version metadata
	version := Version{
		ID:        time.Now().Format("20060102-150405"),
		Timestamp: time.Now(),
		Comment:   comment,
	}

	if kb.resourceType == "secret" {
		return kb.saveSecret(ctx, data, version)
	}
	return kb.saveConfigMap(ctx, data, version)
}

// saveConfigMap saves to a ConfigMap
func (kb *K8sBackend) saveConfigMap(ctx context.Context, data []byte, version Version) error {
	configMap, err := kb.client.CoreV1().ConfigMaps(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        kb.resourceName,
				Namespace:   kb.namespace,
				Annotations: make(map[string]string),
			},
			Data: make(map[string]string),
		}
	}

	// Rotate versions in annotations
	kb.rotateVersionAnnotations(configMap.Annotations, configMap.Data["config.yaml"])

	// Update current config
	configMap.Data["config.yaml"] = string(data)

	// Add version metadata to annotations
	configMap.Annotations["config-version"] = version.ID
	configMap.Annotations["config-timestamp"] = version.Timestamp.Format(time.RFC3339)
	configMap.Annotations["config-comment"] = version.Comment

	// Update or create
	if configMap.CreationTimestamp.IsZero() {
		_, err = kb.client.CoreV1().ConfigMaps(kb.namespace).Create(ctx, configMap, metav1.CreateOptions{})
	} else {
		_, err = kb.client.CoreV1().ConfigMaps(kb.namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	}

	return err
}

// saveSecret saves to a Secret
func (kb *K8sBackend) saveSecret(ctx context.Context, data []byte, version Version) error {
	secret, err := kb.client.CoreV1().Secrets(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
	if err != nil {
		// Create new Secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        kb.resourceName,
				Namespace:   kb.namespace,
				Annotations: make(map[string]string),
			},
			Data: make(map[string][]byte),
		}
	}

	// Rotate versions in annotations
	kb.rotateVersionAnnotations(secret.Annotations, string(secret.Data["config.yaml"]))

	// Update current config
	secret.Data["config.yaml"] = data

	// Add version metadata to annotations
	secret.Annotations["config-version"] = version.ID
	secret.Annotations["config-timestamp"] = version.Timestamp.Format(time.RFC3339)
	secret.Annotations["config-comment"] = version.Comment

	// Update or create
	if secret.CreationTimestamp.IsZero() {
		_, err = kb.client.CoreV1().Secrets(kb.namespace).Create(ctx, secret, metav1.CreateOptions{})
	} else {
		_, err = kb.client.CoreV1().Secrets(kb.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}

	return err
}

// rotateVersionAnnotations rotates version history in annotations
func (kb *K8sBackend) rotateVersionAnnotations(annotations map[string]string, currentData string) {
	// Get current version metadata
	currentVersion := annotations["config-version"]
	currentTimestamp := annotations["config-timestamp"]
	currentComment := annotations["config-comment"]

	if currentVersion == "" {
		return
	}

	// Shift existing versions
	for i := kb.maxVersions - 1; i > 0; i-- {
		prevKey := fmt.Sprintf("config-v%d", i)
		nextKey := fmt.Sprintf("config-v%d", i+1)

		if val, exists := annotations[prevKey]; exists {
			annotations[nextKey] = val
		} else {
			delete(annotations, nextKey)
		}
	}

	// Save current as v1
	v1Data := map[string]string{
		"version":   currentVersion,
		"timestamp": currentTimestamp,
		"comment":   currentComment,
		"data":      currentData,
	}
	v1JSON, _ := json.Marshal(v1Data)
	annotations["config-v1"] = string(v1JSON)

	// Clean up old versions beyond maxVersions
	for i := kb.maxVersions + 1; i <= kb.maxVersions+10; i++ {
		delete(annotations, fmt.Sprintf("config-v%d", i))
	}
}

// ListVersions returns a list of available configuration versions from annotations
func (kb *K8sBackend) ListVersions() ([]Version, error) {
	ctx := context.Background()
	var annotations map[string]string

	if kb.resourceType == "secret" {
		secret, err := kb.client.CoreV1().Secrets(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret: %w", err)
		}
		annotations = secret.Annotations
	} else {
		configMap, err := kb.client.CoreV1().ConfigMaps(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get configmap: %w", err)
		}
		annotations = configMap.Annotations
	}

	versions := []Version{}

	// Add current version
	if v := annotations["config-version"]; v != "" {
		ts, _ := time.Parse(time.RFC3339, annotations["config-timestamp"])
		versions = append(versions, Version{
			ID:        "current",
			Timestamp: ts,
			Comment:   annotations["config-comment"],
		})
	}

	// Parse versioned configs from annotations
	for i := 1; i <= kb.maxVersions; i++ {
		key := fmt.Sprintf("config-v%d", i)
		vJSON := annotations[key]
		if vJSON == "" {
			continue
		}

		var vData map[string]string
		if err := json.Unmarshal([]byte(vJSON), &vData); err != nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339, vData["timestamp"])
		versions = append(versions, Version{
			ID:        fmt.Sprintf("v%d", i),
			Timestamp: ts,
			Comment:   vData["comment"],
		})
	}

	// Sort by timestamp descending
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Timestamp.After(versions[j].Timestamp)
	})

	return versions, nil
}

// LoadVersion loads a specific version from annotations
func (kb *K8sBackend) LoadVersion(id string) (*Config, error) {
	if id == "current" {
		return kb.Load()
	}

	ctx := context.Background()
	var annotations map[string]string

	if kb.resourceType == "secret" {
		secret, err := kb.client.CoreV1().Secrets(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get secret: %w", err)
		}
		annotations = secret.Annotations
	} else {
		configMap, err := kb.client.CoreV1().ConfigMaps(kb.namespace).Get(ctx, kb.resourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get configmap: %w", err)
		}
		annotations = configMap.Annotations
	}

	// Extract version number
	var versionNum int
	if _, err := fmt.Sscanf(id, "v%d", &versionNum); err != nil {
		return nil, fmt.Errorf("invalid version ID: %s", id)
	}

	key := fmt.Sprintf("config-v%d", versionNum)
	vJSON := annotations[key]
	if vJSON == "" {
		return nil, fmt.Errorf("version not found: %s", id)
	}

	var vData map[string]string
	if err := json.Unmarshal([]byte(vJSON), &vData); err != nil {
		return nil, fmt.Errorf("failed to parse version data: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal([]byte(vData["data"]), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// Rollback rolls back to a specific version
func (kb *K8sBackend) Rollback(id string) error {
	// Load the specified version
	cfg, err := kb.LoadVersion(id)
	if err != nil {
		return fmt.Errorf("failed to load version %s: %w", id, err)
	}

	// Save it as the current configuration
	comment := fmt.Sprintf("Rolled back to version %s", id)
	return kb.Save(cfg, comment)
}
