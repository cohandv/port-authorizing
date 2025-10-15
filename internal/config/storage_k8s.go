//go:build k8s
// +build k8s

package config

import (
	"context"
<<<<<<< HEAD
	"encoding/json"
=======
>>>>>>> e6f31f6 ([temp] test)
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
<<<<<<< HEAD
)

// K8sBackend implements storage backed by Kubernetes ConfigMap or Secret
=======
	"k8s.io/client-go/tools/clientcmd"
)

// K8sBackend implements StorageBackend using Kubernetes ConfigMaps or Secrets
>>>>>>> e6f31f6 ([temp] test)
type K8sBackend struct {
	client       *kubernetes.Clientset
	namespace    string
	resourceType string // "configmap" or "secret"
	resourceName string
	maxVersions  int
}

// NewK8sBackend creates a new Kubernetes-based storage backend
<<<<<<< HEAD
func NewK8sBackend(cfg *StorageConfig) (*K8sBackend, error) {
	// Create in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
=======
func NewK8sBackend(namespace, resourceType, resourceName string, maxVersions int) (*K8sBackend, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if resourceName == "" {
		return nil, fmt.Errorf("resource name is required")
	}
	if resourceType != "configmap" && resourceType != "secret" {
		return nil, fmt.Errorf("resource type must be 'configmap' or 'secret'")
	}

	// Try in-cluster config first, then fall back to kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		// Try loading from kubeconfig
		kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubernetes config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
>>>>>>> e6f31f6 ([temp] test)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

<<<<<<< HEAD
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
=======
	return &K8sBackend{
		client:       client,
		namespace:    namespace,
		resourceType: resourceType,
		resourceName: resourceName,
		maxVersions:  maxVersions,
	}, nil
}

// Load reads the configuration from Kubernetes ConfigMap/Secret
func (k *K8sBackend) Load(ctx context.Context) (*Config, error) {
	data, err := k.readResource(ctx, k.resourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to Kubernetes ConfigMap/Secret with versioning
func (k *K8sBackend) Save(ctx context.Context, cfg *Config, comment string) error {
	// Marshal config
>>>>>>> e6f31f6 ([temp] test)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

<<<<<<< HEAD
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
=======
	// Create version backup
	if err := k.createVersionBackup(ctx, comment); err != nil {
		return fmt.Errorf("failed to create version backup: %w", err)
	}

	// Update main resource
	if err := k.writeResource(ctx, k.resourceName, string(data), comment); err != nil {
		return fmt.Errorf("failed to write resource: %w", err)
	}

	// Rotate old versions
	if err := k.rotateVersions(ctx); err != nil {
		return fmt.Errorf("failed to rotate versions: %w", err)
	}

	return nil
}

// createVersionBackup creates a versioned backup resource
func (k *K8sBackend) createVersionBackup(ctx context.Context, comment string) error {
	// Read current config
	currentData, err := k.readResource(ctx, k.resourceName)
	if err != nil {
		// No current config, nothing to backup
		return nil
	}

	// Create backup with timestamp
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("%s-%s", k.resourceName, timestamp)

	return k.writeResource(ctx, backupName, currentData, comment)
}

// rotateVersions removes old version backups
func (k *K8sBackend) rotateVersions(ctx context.Context) error {
	if k.maxVersions <= 0 {
		return nil
	}

	versions, err := k.listVersionResources(ctx)
	if err != nil {
		return err
	}

	// Sort by name (timestamp) descending
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})

	// Remove old versions
	for i := k.maxVersions; i < len(versions); i++ {
		if err := k.deleteResource(ctx, versions[i]); err != nil {
			return err
		}
	}

	return nil
}

// ListVersions returns list of available configuration versions
func (k *K8sBackend) ListVersions(ctx context.Context) ([]Version, error) {
	versionNames, err := k.listVersionResources(ctx)
	if err != nil {
		return nil, err
	}

	// Sort by timestamp (newest first)
	sort.Slice(versionNames, func(i, j int) bool {
		return versionNames[i] > versionNames[j]
	})

	versions := []Version{
		{
			ID:        "current",
			Timestamp: time.Now(),
			Comment:   "Current configuration",
		},
	}

	// Parse version metadata
	for _, name := range versionNames {
		metadata, err := k.getResourceMetadata(ctx, name)
		if err != nil {
			continue
		}

		version := Version{
			ID:        name,
			Timestamp: metadata.Timestamp,
			Comment:   metadata.Comment,
			Author:    metadata.Author,
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// LoadVersion loads a specific configuration version
func (k *K8sBackend) LoadVersion(ctx context.Context, id string) (*Config, error) {
	var data string
	var err error

	if id == "current" {
		data, err = k.readResource(ctx, k.resourceName)
	} else {
		data, err = k.readResource(ctx, id)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read version %s: %w", id, err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Rollback restores a previous configuration version
func (k *K8sBackend) Rollback(ctx context.Context, id string) (*Config, error) {
	cfg, err := k.LoadVersion(ctx, id)
	if err != nil {
		return nil, err
	}

	comment := fmt.Sprintf("Rolled back to version %s", id)
	if err := k.Save(ctx, cfg, comment); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Helper methods for ConfigMap/Secret operations

func (k *K8sBackend) readResource(ctx context.Context, name string) (string, error) {
	if k.resourceType == "configmap" {
		cm, err := k.client.CoreV1().ConfigMaps(k.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		return cm.Data["config.yaml"], nil
	}

	// Secret
	secret, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(secret.Data["config.yaml"]), nil
}

func (k *K8sBackend) writeResource(ctx context.Context, name, data, comment string) error {
	annotations := map[string]string{
		"port-authorizing.io/comment":   comment,
		"port-authorizing.io/timestamp": time.Now().Format(time.RFC3339),
	}

	if k.resourceType == "configmap" {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   k.namespace,
				Annotations: annotations,
			},
			Data: map[string]string{
				"config.yaml": data,
			},
		}

		existing, err := k.client.CoreV1().ConfigMaps(k.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			// Create new
			_, err = k.client.CoreV1().ConfigMaps(k.namespace).Create(ctx, cm, metav1.CreateOptions{})
			return err
		}

		// Update existing
		cm.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
		_, err = k.client.CoreV1().ConfigMaps(k.namespace).Update(ctx, cm, metav1.UpdateOptions{})
		return err
	}

	// Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   k.namespace,
			Annotations: annotations,
		},
		Data: map[string][]byte{
			"config.yaml": []byte(data),
		},
	}

	existing, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// Create new
		_, err = k.client.CoreV1().Secrets(k.namespace).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}

	// Update existing
	secret.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	_, err = k.client.CoreV1().Secrets(k.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

func (k *K8sBackend) deleteResource(ctx context.Context, name string) error {
	if k.resourceType == "configmap" {
		return k.client.CoreV1().ConfigMaps(k.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return k.client.CoreV1().Secrets(k.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *K8sBackend) listVersionResources(ctx context.Context) ([]string, error) {
	prefix := k.resourceName + "-"
	var names []string

	if k.resourceType == "configmap" {
		list, err := k.client.CoreV1().ConfigMaps(k.namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, cm := range list.Items {
			if cm.Name != k.resourceName && len(cm.Name) > len(prefix) && cm.Name[:len(prefix)] == prefix {
				names = append(names, cm.Name)
			}
		}
	} else {
		list, err := k.client.CoreV1().Secrets(k.namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, secret := range list.Items {
			if secret.Name != k.resourceName && len(secret.Name) > len(prefix) && secret.Name[:len(prefix)] == prefix {
				names = append(names, secret.Name)
			}
		}
	}

	return names, nil
}

type versionMetadata struct {
	Timestamp time.Time
	Comment   string
	Author    string
}

func (k *K8sBackend) getResourceMetadata(ctx context.Context, name string) (*versionMetadata, error) {
	var annotations map[string]string

	if k.resourceType == "configmap" {
		cm, err := k.client.CoreV1().ConfigMaps(k.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		annotations = cm.Annotations
	} else {
		secret, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		annotations = secret.Annotations
	}

	metadata := &versionMetadata{
		Comment: annotations["port-authorizing.io/comment"],
		Author:  annotations["port-authorizing.io/author"],
	}

	if ts := annotations["port-authorizing.io/timestamp"]; ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			metadata.Timestamp = t
		}
	}

	return metadata, nil
>>>>>>> e6f31f6 ([temp] test)
}
