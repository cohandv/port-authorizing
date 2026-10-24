package proxy

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewK8sServiceDiscoveryWithClient(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		name              string
		excludeNamespaces []string
		includeNamespaces []string
	}{
		{
			name:              "with default excludes",
			excludeNamespaces: nil,
			includeNamespaces: nil,
		},
		{
			name:              "with custom excludes",
			excludeNamespaces: []string{"kube-system", "kube-public"},
			includeNamespaces: nil,
		},
		{
			name:              "with include list",
			excludeNamespaces: nil,
			includeNamespaces: []string{"default", "app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discovery := NewK8sServiceDiscoveryWithClient(client, tt.excludeNamespaces, tt.includeNamespaces, nil)

			if discovery == nil {
				t.Fatal("expected non-nil discovery")
			}

			if discovery.client != client {
				t.Error("client not set correctly")
			}
		})
	}
}

func TestDiscoverServices(t *testing.T) {
	ctx := context.Background()

	// Create fake client with test data
	client := fake.NewSimpleClientset(
		// Namespaces
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-namespace"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},

		// Services
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-service",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-service",
				Namespace: "app-namespace",
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.2",
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					},
					{
						Name:       "grpc",
						Port:       9090,
						TargetPort: intstr.FromInt(9090),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		},
		// Headless service (should be skipped)
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-service",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "None",
				Ports: []corev1.ServicePort{
					{
						Port:     80,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		},
		// System service (should be excluded)
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-dns",
				Namespace: "kube-system",
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.10",
				Ports: []corev1.ServicePort{
					{
						Port:     53,
						Protocol: corev1.ProtocolUDP,
					},
				},
			},
		},
	)

	discovery := NewK8sServiceDiscoveryWithClient(client, []string{"kube-system"}, nil, nil)

	services, err := discovery.DiscoverServices(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find 2 services (web-service in default, api-service in app-namespace)
	// Headless service and kube-system services should be excluded
	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
		for _, s := range services {
			t.Logf("  - %s/%s", s.Namespace, s.Name)
		}
	}

	// Verify services are sorted by namespace then name
	if len(services) >= 2 {
		if services[0].Namespace != "app-namespace" || services[0].Name != "api-service" {
			t.Errorf("expected first service to be app-namespace/api-service, got %s/%s",
				services[0].Namespace, services[0].Name)
		}
		if services[1].Namespace != "default" || services[1].Name != "web-service" {
			t.Errorf("expected second service to be default/web-service, got %s/%s",
				services[1].Namespace, services[1].Name)
		}
	}
}

func TestDiscoverServicesGroupedByNamespace(t *testing.T) {
	ctx := context.Background()

	client := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2"}},

		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns1"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc2", Namespace: "ns1"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.2",
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc3", Namespace: "ns2"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.3",
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
	)

	discovery := NewK8sServiceDiscoveryWithClient(client, nil, []string{"ns1", "ns2"}, nil)

	grouped, err := discovery.DiscoverServicesGroupedByNamespace(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(grouped) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(grouped))
	}

	if len(grouped["ns1"]) != 2 {
		t.Errorf("expected 2 services in ns1, got %d", len(grouped["ns1"]))
	}

	if len(grouped["ns2"]) != 1 {
		t.Errorf("expected 1 service in ns2, got %d", len(grouped["ns2"]))
	}
}

func TestGetService(t *testing.T) {
	ctx := context.Background()

	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		},
	)

	discovery := NewK8sServiceDiscoveryWithClient(client, nil, []string{"default"}, nil)

	tests := []struct {
		name      string
		namespace string
		service   string
		wantErr   bool
	}{
		{
			name:      "existing service",
			namespace: "default",
			service:   "test-service",
			wantErr:   false,
		},
		{
			name:      "non-existent service",
			namespace: "default",
			service:   "not-found",
			wantErr:   true,
		},
		{
			name:      "excluded namespace",
			namespace: "kube-system",
			service:   "test-service",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := discovery.GetService(ctx, tt.namespace, tt.service)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if svc.Name != tt.service {
				t.Errorf("expected service name %s, got %s", tt.service, svc.Name)
			}

			if svc.Namespace != tt.namespace {
				t.Errorf("expected namespace %s, got %s", tt.namespace, svc.Namespace)
			}
		})
	}
}

func TestGetServiceEndpoint(t *testing.T) {
	ctx := context.Background()

	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-port-service",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
					{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP},
					{Name: "grpc", Port: 9090, Protocol: corev1.ProtocolTCP},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "single-port-service",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.2",
				Ports: []corev1.ServicePort{
					{Port: 8080, Protocol: corev1.ProtocolTCP},
				},
			},
		},
	)

	discovery := NewK8sServiceDiscoveryWithClient(client, nil, []string{"default"}, nil)

	tests := []struct {
		name      string
		namespace string
		service   string
		portName  string
		wantAddr  string // Now expects FQDN instead of ClusterIP
		wantPort  int32
		wantErr   bool
	}{
		{
			name:      "named port - http",
			namespace: "default",
			service:   "multi-port-service",
			portName:  "http",
			wantAddr:  "multi-port-service.default.svc.cluster.local",
			wantPort:  80,
			wantErr:   false,
		},
		{
			name:      "named port - https",
			namespace: "default",
			service:   "multi-port-service",
			portName:  "https",
			wantAddr:  "multi-port-service.default.svc.cluster.local",
			wantPort:  443,
			wantErr:   false,
		},
		{
			name:      "port by number",
			namespace: "default",
			service:   "multi-port-service",
			portName:  "9090",
			wantAddr:  "multi-port-service.default.svc.cluster.local",
			wantPort:  9090,
			wantErr:   false,
		},
		{
			name:      "single port - no name",
			namespace: "default",
			service:   "single-port-service",
			portName:  "",
			wantAddr:  "single-port-service.default.svc.cluster.local",
			wantPort:  8080,
			wantErr:   false,
		},
		{
			name:      "multi-port - no name specified - uses first match",
			namespace: "default",
			service:   "multi-port-service",
			portName:  "",
			wantAddr:  "multi-port-service.default.svc.cluster.local",
			wantPort:  80,
			wantErr:   false,
		},
		{
			name:      "non-existent port",
			namespace: "default",
			service:   "multi-port-service",
			portName:  "unknown",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, port, err := discovery.GetServiceEndpoint(ctx, tt.namespace, tt.service, tt.portName)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if addr != tt.wantAddr {
				t.Errorf("expected address %s, got %s", tt.wantAddr, addr)
			}

			if port != tt.wantPort {
				t.Errorf("expected port %d, got %d", tt.wantPort, port)
			}
		})
	}
}

func TestIsNamespaceAllowed(t *testing.T) {
	tests := []struct {
		name              string
		excludeNamespaces []string
		includeNamespaces []string
		checkNamespace    string
		want              bool
	}{
		{
			name:              "include list - allowed",
			includeNamespaces: []string{"default", "app"},
			checkNamespace:    "default",
			want:              true,
		},
		{
			name:              "include list - not allowed",
			includeNamespaces: []string{"default", "app"},
			checkNamespace:    "kube-system",
			want:              false,
		},
		{
			name:              "exclude list - allowed",
			excludeNamespaces: []string{"kube-system"},
			checkNamespace:    "default",
			want:              true,
		},
		{
			name:              "exclude list - not allowed",
			excludeNamespaces: []string{"kube-system"},
			checkNamespace:    "kube-system",
			want:              false,
		},
		{
			name:              "case insensitive include",
			includeNamespaces: []string{"Default"},
			checkNamespace:    "default",
			want:              true,
		},
		{
			name:              "case insensitive exclude",
			excludeNamespaces: []string{"Kube-System"},
			checkNamespace:    "kube-system",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discovery := NewK8sServiceDiscoveryWithClient(
				fake.NewSimpleClientset(),
				tt.excludeNamespaces,
				tt.includeNamespaces,
				nil,
			)

			got := discovery.isNamespaceAllowed(tt.checkNamespace)
			if got != tt.want {
				t.Errorf("isNamespaceAllowed(%s) = %v, want %v", tt.checkNamespace, got, tt.want)
			}
		})
	}
}

func TestDefaultExcludedNamespaces(t *testing.T) {
	// Verify default excluded namespaces are set
	expected := []string{
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

	if len(DefaultExcludedNamespaces) != len(expected) {
		t.Errorf("expected %d default excluded namespaces, got %d",
			len(expected), len(DefaultExcludedNamespaces))
	}

	for _, ns := range expected {
		found := false
		for _, defaultNs := range DefaultExcludedNamespaces {
			if defaultNs == ns {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s to be in default excluded namespaces", ns)
		}
	}
}

func TestGetPortTypeFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		portName    string
		port        int32
		want        string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			portName:    "http",
			port:        80,
			want:        "",
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			portName:    "http",
			port:        80,
			want:        "",
		},
		{
			name: "match by port number",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.8080.type": "http",
			},
			portName: "web",
			port:     8080,
			want:     "http",
		},
		{
			name: "match by port name",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.http.type": "http",
			},
			portName: "http",
			port:     80,
			want:     "http",
		},
		{
			name: "port number takes priority over name",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.8080.type": "https",
				"cohandv.port-authorizer.io/port.http.type": "http",
			},
			portName: "http",
			port:     8080,
			want:     "https",
		},
		{
			name: "case insensitive protocol value",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.80.type": "HTTP",
			},
			portName: "",
			port:     80,
			want:     "http",
		},
		{
			name: "trim whitespace",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.80.type": "  postgres  ",
			},
			portName: "",
			port:     80,
			want:     "postgres",
		},
		{
			name: "no matching annotation",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.443.type": "https",
			},
			portName: "http",
			port:     80,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPortTypeFromAnnotations(tt.annotations, tt.portName, tt.port)
			if got != tt.want {
				t.Errorf("GetPortTypeFromAnnotations() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsPortExposed(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		portName    string
		port        int32
		want        bool
	}{
		{
			name:        "nil annotations - default expose",
			annotations: nil,
			portName:    "http",
			port:        80,
			want:        true,
		},
		{
			name:        "empty annotations - default expose",
			annotations: map[string]string{},
			portName:    "http",
			port:        80,
			want:        true,
		},
		{
			name: "explicitly exposed by port number",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.8080.expose": "true",
			},
			portName: "web",
			port:     8080,
			want:     true,
		},
		{
			name: "explicitly hidden by port number",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.8080.expose": "false",
			},
			portName: "web",
			port:     8080,
			want:     false,
		},
		{
			name: "explicitly exposed by port name",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.metrics.expose": "true",
			},
			portName: "metrics",
			port:     9090,
			want:     true,
		},
		{
			name: "explicitly hidden by port name",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.admin.expose": "false",
			},
			portName: "admin",
			port:     8081,
			want:     false,
		},
		{
			name: "port number annotation takes priority",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.8080.expose": "false",
				"cohandv.port-authorizer.io/port.http.expose": "true",
			},
			portName: "http",
			port:     8080,
			want:     false,
		},
		{
			name: "case insensitive true",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.80.expose": "TRUE",
			},
			portName: "",
			port:     80,
			want:     true,
		},
		{
			name: "case insensitive false",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.80.expose": "FALSE",
			},
			portName: "",
			port:     80,
			want:     false,
		},
		{
			name: "non-boolean value treated as false",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.80.expose": "yes",
			},
			portName: "",
			port:     80,
			want:     false,
		},
		{
			name: "no matching annotation - default expose",
			annotations: map[string]string{
				"cohandv.port-authorizer.io/port.443.expose": "false",
			},
			portName: "http",
			port:     80,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPortExposed(tt.annotations, tt.portName, tt.port)
			if got != tt.want {
				t.Errorf("IsPortExposed() = %v, want %v", got, tt.want)
			}
		})
	}
}
