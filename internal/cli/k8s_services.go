package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// K8sDiscoveredService represents a discovered K8s service
type K8sDiscoveredService struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	ClusterIP   string            `json:"cluster_ip"`
	Ports       []K8sServicePort  `json:"ports"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// K8sServicePort represents a service port
type K8sServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
}

// k8sServicesResponse is the response from the services discovery endpoint
type k8sServicesResponse struct {
	Services     map[string][]K8sDiscoveredService `json:"services"`
	ConnectionID string                            `json:"connection_id"`
}

// servicePortMapping tracks local port to service mapping
type servicePortMapping struct {
	LocalPort int
	Namespace string
	Service   string
	Port      int32
	PortName  string
}

// k8sServiceConnection manages a connection to a specific K8s service
type k8sServiceConnection struct {
	mapping  servicePortMapping
	listener net.Listener
	wsConn   *websocket.Conn
	done     chan struct{}
	ready    chan bool // Signals when connection is established
}

// runK8sServicesConnect handles k8s_services connection type
func runK8sServicesConnect(apiURL, token, connectionID, expiresAt string, startPort int) error {
	fmt.Println("🔍 Discovering K8s services...")

	// Step 1: Fetch service list via HTTP
	services, err := fetchK8sServicesList(apiURL, token, connectionID)
	if err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	if len(services) == 0 {
		fmt.Println("⚠️  No services found")
		return nil
	}

	// Display services
	displayServiceList(services)

	// Build port mappings
	var portMappings []servicePortMapping
	currentPort := startPort + 1 // Reserve startPort for index page

	// Sort namespaces for consistent ordering
	namespaces := make([]string, 0, len(services))
	for ns := range services {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for _, ns := range namespaces {
		svcList := services[ns]
		for _, svc := range svcList {
			for _, port := range svc.Ports {
				portMappings = append(portMappings, servicePortMapping{
					LocalPort: currentPort,
					Namespace: svc.Namespace,
					Service:   svc.Name,
					Port:      port.Port,
					PortName:  port.Name,
				})
				currentPort++
			}
		}
	}

	// Step 2: Open parallel connections for each service
	fmt.Printf("\n📡 Opening %d connections...\n", len(portMappings))

	connections := make([]*k8sServiceConnection, 0, len(portMappings))
	var wg sync.WaitGroup
	var connMu sync.Mutex
	var failedCount int

	for _, mapping := range portMappings {
		wg.Add(1)
		go func(m servicePortMapping) {
			defer wg.Done()

			conn, err := openK8sServiceConnection(apiURL, token, connectionID, m)
			connMu.Lock()
			if err != nil {
				fmt.Printf("  ❌ %s/%s:%d - %v\n", m.Namespace, m.Service, m.Port, err)
				failedCount++
			} else {
				connections = append(connections, conn)
				fmt.Printf("  ✓ %s/%s:%d → localhost:%d\n", m.Namespace, m.Service, m.Port, m.LocalPort)
			}
			connMu.Unlock()
		}(mapping)
	}

	wg.Wait()

	if len(connections) == 0 {
		return fmt.Errorf("failed to establish any connections")
	}

	if failedCount > 0 {
		fmt.Printf("\n⚠️  %d connection(s) failed, %d succeeded\n", failedCount, len(connections))
	}

	// Step 3: Start the index page server
	go startK8sIndexServer(startPort, services, portMappings)

	fmt.Printf("\n✅ K8s Services Proxy Ready!\n")
	fmt.Printf("   Index page: http://localhost:%d\n", startPort)
	fmt.Printf("   Expires at: %s\n", expiresAt)
	fmt.Println("\nPress Ctrl+C to disconnect")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n\nDisconnecting...")

	// Close all connections
	for _, conn := range connections {
		conn.Close()
	}

	return nil
}

// fetchK8sServicesList fetches the service list from the API
func fetchK8sServicesList(apiURL, token, connectionID string) (map[string][]K8sDiscoveredService, error) {
	url := fmt.Sprintf("%s/api/k8s-services/%s/services", apiURL, connectionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var result k8sServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Services, nil
}

// openK8sServiceConnection opens a WebSocket connection to proxy a specific K8s service
func openK8sServiceConnection(apiURL, token, connectionID string, mapping servicePortMapping) (*k8sServiceConnection, error) {
	// Start local listener first
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", mapping.LocalPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", mapping.LocalPort, err)
	}

	conn := &k8sServiceConnection{
		mapping:  mapping,
		listener: listener,
		done:     make(chan struct{}),
		ready:    make(chan bool, 1),
	}

	// Start accepting connections in background
	go conn.acceptConnections(apiURL, token, connectionID)

	return conn, nil
}

// acceptConnections accepts local connections and proxies them
func (c *k8sServiceConnection) acceptConnections(apiURL, token, connectionID string) {
	for {
		select {
		case <-c.done:
			return
		default:
		}

		localConn, err := c.listener.Accept()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
				continue
			}
		}

		// Handle each connection in a goroutine
		go c.handleConnection(localConn, apiURL, token, connectionID)
	}
}

// handleConnection handles a single local connection by opening a WebSocket to the API
func (c *k8sServiceConnection) handleConnection(localConn net.Conn, apiURL, token, connectionID string) {
	defer localConn.Close()

	// Build WebSocket URL with query params
	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL = fmt.Sprintf("%s/api/proxy/%s?namespace=%s&service=%s&port=%d&port_name=%s",
		wsURL, connectionID,
		c.mapping.Namespace,
		c.mapping.Service,
		c.mapping.Port,
		c.mapping.PortName,
	)

	// Create WebSocket connection
	headers := http.Header{}
	headers.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	wsConn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("WebSocket error for %s/%s:%d: %s\n", c.mapping.Namespace, c.mapping.Service, c.mapping.Port, string(body))
		}
		return
	}
	defer wsConn.Close()

	// Bidirectional proxy
	var wg sync.WaitGroup
	wg.Add(2)

	// Local -> WebSocket
	go func() {
		defer wg.Done()
		buf := make([]byte, 32768)
		for {
			n, err := localConn.Read(buf)
			if err != nil {
				_ = wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if n > 0 {
				if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					return
				}
			}
		}
	}()

	// WebSocket -> Local
	go func() {
		defer wg.Done()
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if _, err := localConn.Write(data); err != nil {
				return
			}
		}
	}()

	wg.Wait()
}

// Close closes the service connection
func (c *k8sServiceConnection) Close() {
	close(c.done)
	c.listener.Close()
	if c.wsConn != nil {
		c.wsConn.Close()
	}
}

// displayServiceList displays the discovered services
func displayServiceList(services map[string][]K8sDiscoveredService) {
	totalServices := 0
	totalPorts := 0
	for _, svcList := range services {
		totalServices += len(svcList)
		for _, svc := range svcList {
			totalPorts += len(svc.Ports)
		}
	}

	fmt.Printf("\n✓ Discovered %d services (%d ports) in %d namespaces\n", totalServices, totalPorts, len(services))
}

// startK8sIndexServer starts an HTTP server for the index page
func startK8sIndexServer(port int, services map[string][]K8sDiscoveredService, portMappings []servicePortMapping) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := generateK8sIndexHTML(services, portMappings)
		_, _ = w.Write([]byte(html))
	})

	mux.HandleFunc("/api/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"services":      services,
			"port_mappings": portMappings,
		})
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("k8s index server error: %v\n", err)
	}
}

// generateK8sIndexHTML generates the HTML index page
func generateK8sIndexHTML(services map[string][]K8sDiscoveredService, portMappings []servicePortMapping) string {
	// Build a map from namespace/service/port to local port
	portMap := make(map[string]int)
	for _, m := range portMappings {
		key := fmt.Sprintf("%s/%s/%d", m.Namespace, m.Service, m.Port)
		portMap[key] = m.LocalPort
	}

	// Collect all services into a flat list
	type serviceItem struct {
		Namespace string
		Name      string
		Port      int32
		PortName  string
		LocalPort int
	}
	var items []serviceItem

	// Sort namespaces
	namespaces := make([]string, 0, len(services))
	for ns := range services {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for _, ns := range namespaces {
		svcList := services[ns]
		for _, svc := range svcList {
			for _, port := range svc.Ports {
				key := fmt.Sprintf("%s/%s/%d", svc.Namespace, svc.Name, port.Port)
				if localPort, ok := portMap[key]; ok {
					items = append(items, serviceItem{
						Namespace: svc.Namespace,
						Name:      svc.Name,
						Port:      port.Port,
						PortName:  port.Name,
						LocalPort: localPort,
					})
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>K8s Services - Port Authorizing</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            color: #e0e0e0;
            padding: 2rem;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 {
            font-size: 2rem;
            font-weight: 600;
            margin-bottom: 1.5rem;
            color: #fff;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        h1::before { content: '☸️'; font-size: 1.5rem; }
        .stats {
            display: flex;
            gap: 2rem;
            margin-bottom: 2rem;
            flex-wrap: wrap;
        }
        .stat {
            background: rgba(255,255,255,0.05);
            padding: 1rem 1.5rem;
            border-radius: 8px;
            border: 1px solid rgba(255,255,255,0.1);
        }
        .stat-value { font-size: 1.5rem; font-weight: 600; color: #4fc3f7; }
        .stat-label { font-size: 0.85rem; color: #888; margin-top: 0.25rem; }
        .services-list {
            display: grid;
            gap: 0.75rem;
        }
        .service-item {
            background: rgba(255,255,255,0.03);
            border: 1px solid rgba(255,255,255,0.08);
            border-radius: 8px;
            padding: 1rem 1.25rem;
            display: flex;
            align-items: center;
            justify-content: space-between;
            transition: all 0.2s ease;
        }
        .service-item:hover {
            background: rgba(255,255,255,0.06);
            border-color: rgba(79, 195, 247, 0.3);
        }
        .service-info { display: flex; align-items: center; gap: 1rem; flex: 1; }
        .service-name {
            font-weight: 500;
            color: #4fc3f7;
            text-decoration: none;
            font-size: 1rem;
        }
        .service-name:hover { text-decoration: underline; }
        .namespace-tag {
            background: rgba(156, 39, 176, 0.2);
            color: #ce93d8;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 500;
        }
        .port-tag {
            background: rgba(76, 175, 80, 0.2);
            color: #81c784;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
        }
        .actions { display: flex; gap: 0.5rem; }
        .doc-link {
            color: #90caf9;
            text-decoration: none;
            font-size: 0.85rem;
            padding: 0.4rem 0.8rem;
            border: 1px solid rgba(144, 202, 249, 0.3);
            border-radius: 4px;
            transition: all 0.2s;
        }
        .doc-link:hover {
            background: rgba(144, 202, 249, 0.1);
            border-color: rgba(144, 202, 249, 0.5);
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>K8s Services</h1>
        <div class="stats">
            <div class="stat">
                <div class="stat-value">`)
	sb.WriteString(fmt.Sprintf("%d", len(items)))
	sb.WriteString(`</div>
                <div class="stat-label">Service Ports</div>
            </div>
            <div class="stat">
                <div class="stat-value">`)
	sb.WriteString(fmt.Sprintf("%d", len(services)))
	sb.WriteString(`</div>
                <div class="stat-label">Namespaces</div>
            </div>
        </div>
        <div class="services-list">`)

	for _, item := range items {
		localURL := fmt.Sprintf("http://localhost:%d", item.LocalPort)
		docsURL := fmt.Sprintf("http://localhost:%d/docs", item.LocalPort)

		sb.WriteString(fmt.Sprintf(`
            <div class="service-item">
                <div class="service-info">
                    <a href="%s" class="service-name" target="_blank">%s</a>
                    <span class="namespace-tag">%s</span>
                    <span class="port-tag">:%d</span>
                </div>
                <div class="actions">
                    <a href="%s" class="doc-link" target="_blank">📄 Docs</a>
                </div>
            </div>`,
			localURL, item.Name, item.Namespace, item.Port, docsURL))
	}

	sb.WriteString(`
        </div>
    </div>
</body>
</html>`)

	return sb.String()
}
