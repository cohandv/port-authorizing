package proxy

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sServiceDiscovery handles discovery of Kubernetes services
type K8sServiceDiscovery struct {
	client            kubernetes.Interface
	excludeNamespaces []string
	includeNamespaces []string // If non-empty, only include these namespaces
	ignoreServices    []string // Regex patterns to ignore service names
}

// DiscoveredService represents a Kubernetes service discovered across namespaces
type DiscoveredService struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // ClusterIP, NodePort, LoadBalancer
	ClusterIP   string            `json:"cluster_ip"`
	Ports       []ServicePort     `json:"ports"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ServicePort represents a port exposed by a service
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// Annotation keys for port-authorizer configuration
// Example annotations:
//
//	cohandv.port-authorizer.io/port.8080.type: http
//	cohandv.port-authorizer.io/port.8080.expose: "false"
//	cohandv.port-authorizer.io/port.http.type: http
//	cohandv.port-authorizer.io/port.http.expose: "true"
const (
	AnnotationPrefix = "cohandv.port-authorizer.io/port."
)

// GetPortTypeFromAnnotations returns the protocol type for a port from annotations
// It checks both by port number and port name
// Returns empty string if no annotation found
func GetPortTypeFromAnnotations(annotations map[string]string, portName string, port int32) string {
	if annotations == nil {
		return ""
	}

	// Check by port number first (more specific)
	key := fmt.Sprintf("%s%d.type", AnnotationPrefix, port)
	if val, ok := annotations[key]; ok {
		return strings.ToLower(strings.TrimSpace(val))
	}

	// Check by port name
	if portName != "" {
		key = fmt.Sprintf("%s%s.type", AnnotationPrefix, portName)
		if val, ok := annotations[key]; ok {
			return strings.ToLower(strings.TrimSpace(val))
		}
	}

	return ""
}

// IsPortExposed checks if a port should be exposed based on annotations
// Returns true if no annotation found (default is to expose)
func IsPortExposed(annotations map[string]string, portName string, port int32) bool {
	if annotations == nil {
		return true
	}

	// Check by port number first
	key := fmt.Sprintf("%s%d.expose", AnnotationPrefix, port)
	if val, ok := annotations[key]; ok {
		return strings.ToLower(strings.TrimSpace(val)) == "true"
	}

	// Check by port name
	if portName != "" {
		key = fmt.Sprintf("%s%s.expose", AnnotationPrefix, portName)
		if val, ok := annotations[key]; ok {
			return strings.ToLower(strings.TrimSpace(val)) == "true"
		}
	}

	// Default: expose all ports
	return true
}

// DefaultExcludedNamespaces are system namespaces excluded by default
var DefaultExcludedNamespaces = []string{
	"kube-system",
	"kube-public",
	"kube-node-lease",
	"cert-manager",
	"ingress-nginx",
	"istio-system",
	"monitoring",
	"calico-system",
	"tigera-operator",
}

// NewK8sServiceDiscovery creates a new K8s service discovery instance
func NewK8sServiceDiscovery(excludeNamespaces, includeNamespaces, ignoreServices []string) (*K8sServiceDiscovery, error) {
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

	// If no exclude namespaces specified, use defaults
	if len(excludeNamespaces) == 0 && len(includeNamespaces) == 0 {
		excludeNamespaces = DefaultExcludedNamespaces
	}

	return &K8sServiceDiscovery{
		client:            client,
		excludeNamespaces: excludeNamespaces,
		includeNamespaces: includeNamespaces,
		ignoreServices:    ignoreServices,
	}, nil
}

// NewK8sServiceDiscoveryWithClient creates a discovery instance with a pre-configured client (for testing)
func NewK8sServiceDiscoveryWithClient(client kubernetes.Interface, excludeNamespaces, includeNamespaces, ignoreServices []string) *K8sServiceDiscovery {
	return &K8sServiceDiscovery{
		client:            client,
		excludeNamespaces: excludeNamespaces,
		includeNamespaces: includeNamespaces,
		ignoreServices:    ignoreServices,
	}
}

// DiscoverServices discovers all services across allowed namespaces
func (d *K8sServiceDiscovery) DiscoverServices(ctx context.Context) ([]DiscoveredService, error) {
	// Get all namespaces
	namespaces, err := d.getNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var services []DiscoveredService

	// Discover services in each namespace
	for _, ns := range namespaces {
		svcList, err := d.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Log error but continue with other namespaces
			continue
		}

		for _, svc := range svcList.Items {
			// Skip headless services (no ClusterIP)
			if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
				continue
			}

			// Skip services matching ignore patterns
			if d.isServiceIgnored(svc.Name) {
				continue
			}

			discovered := DiscoveredService{
				Namespace:   svc.Namespace,
				Name:        svc.Name,
				Type:        string(svc.Spec.Type),
				ClusterIP:   svc.Spec.ClusterIP,
				Labels:      svc.Labels,
				Annotations: svc.Annotations,
				Ports:       make([]ServicePort, 0, len(svc.Spec.Ports)),
			}

			for _, port := range svc.Spec.Ports {
				sp := ServicePort{
					Name:       port.Name,
					Port:       port.Port,
					TargetPort: port.TargetPort.String(),
					Protocol:   string(port.Protocol),
					NodePort:   port.NodePort,
				}
				discovered.Ports = append(discovered.Ports, sp)
			}

			services = append(services, discovered)
		}
	}

	// Sort by namespace, then name
	sort.Slice(services, func(i, j int) bool {
		if services[i].Namespace != services[j].Namespace {
			return services[i].Namespace < services[j].Namespace
		}
		return services[i].Name < services[j].Name
	})

	return services, nil
}

// DiscoverServicesGroupedByNamespace returns services grouped by namespace
func (d *K8sServiceDiscovery) DiscoverServicesGroupedByNamespace(ctx context.Context) (map[string][]DiscoveredService, error) {
	services, err := d.DiscoverServices(ctx)
	if err != nil {
		return nil, err
	}

	grouped := make(map[string][]DiscoveredService)
	for _, svc := range services {
		grouped[svc.Namespace] = append(grouped[svc.Namespace], svc)
	}

	return grouped, nil
}

// GetService returns a specific service by namespace and name
func (d *K8sServiceDiscovery) GetService(ctx context.Context, namespace, name string) (*DiscoveredService, error) {
	// Verify namespace is allowed
	if !d.isNamespaceAllowed(namespace) {
		return nil, fmt.Errorf("namespace %s is not allowed", namespace)
	}

	svc, err := d.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s/%s: %w", namespace, name, err)
	}

	discovered := &DiscoveredService{
		Namespace:   svc.Namespace,
		Name:        svc.Name,
		Type:        string(svc.Spec.Type),
		ClusterIP:   svc.Spec.ClusterIP,
		Labels:      svc.Labels,
		Annotations: svc.Annotations,
		Ports:       make([]ServicePort, 0, len(svc.Spec.Ports)),
	}

	for _, port := range svc.Spec.Ports {
		sp := ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
			NodePort:   port.NodePort,
		}
		discovered.Ports = append(discovered.Ports, sp)
	}

	return discovered, nil
}

// GetServiceEndpoint returns the address and port for a service
// It returns the FQDN (service.namespace.svc.cluster.local) which works both
// inside the cluster and via VPN/tunnels that resolve K8s DNS
func (d *K8sServiceDiscovery) GetServiceEndpoint(ctx context.Context, namespace, name string, portName string) (string, int32, error) {
	svc, err := d.GetService(ctx, namespace, name)
	if err != nil {
		return "", 0, err
	}

	// Build the FQDN: service.namespace.svc.cluster.local
	// This works inside K8s cluster and also via VPN/tunnels with K8s DNS
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace)

	// Find the port
	for _, port := range svc.Ports {
		if portName == "" || port.Name == portName || fmt.Sprintf("%d", port.Port) == portName {
			return fqdn, port.Port, nil
		}
	}

	// If no port name specified and there's only one port, use it
	if portName == "" && len(svc.Ports) == 1 {
		return fqdn, svc.Ports[0].Port, nil
	}

	return "", 0, fmt.Errorf("port %s not found in service %s/%s", portName, namespace, name)
}

// getNamespaces returns list of namespaces to search based on inclusion/exclusion rules
func (d *K8sServiceDiscovery) getNamespaces(ctx context.Context) ([]string, error) {
	// If include list is specified, use it directly
	if len(d.includeNamespaces) > 0 {
		return d.includeNamespaces, nil
	}

	// Otherwise, list all namespaces and apply exclusions
	nsList, err := d.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	excludeMap := make(map[string]bool)
	for _, ns := range d.excludeNamespaces {
		excludeMap[strings.ToLower(ns)] = true
	}

	var namespaces []string
	for _, ns := range nsList.Items {
		if !excludeMap[strings.ToLower(ns.Name)] {
			namespaces = append(namespaces, ns.Name)
		}
	}

	return namespaces, nil
}

// isServiceIgnored checks if a service name matches any ignore pattern
func (d *K8sServiceDiscovery) isServiceIgnored(serviceName string) bool {
	for _, pattern := range d.ignoreServices {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			continue // Skip invalid patterns
		}
		if re.MatchString(serviceName) {
			return true
		}
	}
	return false
}

// isNamespaceAllowed checks if a namespace is allowed based on inclusion/exclusion rules
func (d *K8sServiceDiscovery) isNamespaceAllowed(namespace string) bool {
	// If include list is specified, namespace must be in it
	if len(d.includeNamespaces) > 0 {
		for _, ns := range d.includeNamespaces {
			if strings.EqualFold(ns, namespace) {
				return true
			}
		}
		return false
	}

	// Otherwise, check exclusion list
	for _, ns := range d.excludeNamespaces {
		if strings.EqualFold(ns, namespace) {
			return false
		}
	}

	return true
}

// GetEndpoints returns the endpoints (pod IPs) for a service
func (d *K8sServiceDiscovery) GetEndpoints(ctx context.Context, namespace, name string) ([]string, error) {
	endpoints, err := d.client.CoreV1().Endpoints(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints for %s/%s: %w", namespace, name, err)
	}

	var addresses []string
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			addresses = append(addresses, addr.IP)
		}
	}

	return addresses, nil
}
