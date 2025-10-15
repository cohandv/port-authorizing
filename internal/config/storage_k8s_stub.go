//go:build !k8s
// +build !k8s

package config

import "fmt"

// NewK8sBackend returns an error when K8s support is not compiled in
func NewK8sBackend(cfg *StorageConfig) (StorageBackend, error) {
	return nil, fmt.Errorf("Kubernetes backend not compiled in. Rebuild with -tags k8s to enable Kubernetes support")
}
