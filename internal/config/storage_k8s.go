package config

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sBackend implements StorageBackend using Kubernetes ConfigMaps or Secrets
type K8sBackend struct {
	client       *kubernetes.Clientset
	namespace    string
	resourceType string // "configmap" or "secret"
	resourceName string
	maxVersions  int
}

// NewK8sBackend creates a new Kubernetes-based storage backend
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
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

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
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

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
		cm.ResourceVersion = existing.ResourceVersion
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
	secret.ResourceVersion = existing.ResourceVersion
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
}
