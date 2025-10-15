//go:build !k8s
<<<<<<< HEAD
// +build !k8s

package config

import "fmt"

// NewK8sBackend returns an error when K8s support is not compiled in
func NewK8sBackend(cfg *StorageConfig) (StorageBackend, error) {
	return nil, fmt.Errorf("Kubernetes backend not compiled in. Rebuild with -tags k8s to enable Kubernetes support")
=======

package config

import (
	"context"
	"fmt"
)

// K8sBackend stub implementation when k8s build tag is not enabled
type K8sBackend struct{}

// NewK8sBackend returns an error when Kubernetes support is not built
func NewK8sBackend(namespace, resourceType, resourceName string, maxVersions int) (*K8sBackend, error) {
	return nil, fmt.Errorf("kubernetes backend not available: rebuild with -tags k8s")
}

func (k *K8sBackend) Load(ctx context.Context) (*Config, error) {
	return nil, fmt.Errorf("kubernetes backend not available")
}

func (k *K8sBackend) Save(ctx context.Context, cfg *Config, comment string) error {
	return fmt.Errorf("kubernetes backend not available")
}

func (k *K8sBackend) ListVersions(ctx context.Context) ([]Version, error) {
	return nil, fmt.Errorf("kubernetes backend not available")
}

func (k *K8sBackend) LoadVersion(ctx context.Context, id string) (*Config, error) {
	return nil, fmt.Errorf("kubernetes backend not available")
}

func (k *K8sBackend) Rollback(ctx context.Context, id string) (*Config, error) {
	return nil, fmt.Errorf("kubernetes backend not available")
>>>>>>> e6f31f6 ([temp] test)
}
