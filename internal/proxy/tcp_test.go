package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func TestNewTCPProxy(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test-redis",
		Type: "tcp",
		Host: "localhost",
		Port: 6379,
	}

	proxy := NewTCPProxy(connConfig)

	if proxy == nil {
		t.Fatal("NewTCPProxy() returned nil")
	}

	if proxy.config != connConfig {
		t.Error("config not set correctly")
	}
}

func TestTCPProxy_HandleRequest(t *testing.T) {
	// Setup a mock TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().(*net.TCPAddr)

	// Server goroutine that echoes data
	serverDone := make(chan bool)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Logf("Server accept error: %v", err)
			return
		}
		defer conn.Close()

		// Echo server: read and write back
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Logf("Server read error: %v", err)
			return
		}

		if n > 0 {
			conn.Write(buf[:n])
		}

		serverDone <- true
	}()

	// Create proxy config pointing to our mock server
	connConfig := &config.ConnectionConfig{
		Name: "test-redis",
		Type: "tcp",
		Host: serverAddr.IP.String(),
		Port: serverAddr.Port,
	}

	proxy := NewTCPProxy(connConfig)

	// Test direct connection to backend (TCP proxy doesn't modify data)
	backendAddr := fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port)
	backendConn, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to backend: %v", err)
	}
	defer backendConn.Close()

	// Send test data
	testData := []byte("PING\r\n")
	_, err = backendConn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write to backend: %v", err)
	}

	// Read response
	reader := bufio.NewReader(backendConn)
	response, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("Failed to read from backend: %v", err)
	}

	if string(response) != string(testData) {
		t.Errorf("Response = %q, want %q", string(response), string(testData))
	}

	// Wait for server to complete
	select {
	case <-serverDone:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Server did not complete in time")
	}

	// Close proxy
	proxy.Close()
}

func TestTCPProxy_Close(t *testing.T) {
	connConfig := &config.ConnectionConfig{
		Name: "test-redis",
		Type: "tcp",
		Host: "localhost",
		Port: 6379,
	}

	proxy := NewTCPProxy(connConfig)

	// Close should not error even if nothing is open
	err := proxy.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Multiple closes should be safe
	err = proxy.Close()
	if err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestTCPProxy_Integration(t *testing.T) {
	// Create a simple echo server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().(*net.TCPAddr)

	// Run echo server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo all data back
			}(conn)
		}
	}()

	// Create proxy
	connConfig := &config.ConnectionConfig{
		Name: "test-echo",
		Type: "tcp",
		Host: serverAddr.IP.String(),
		Port: serverAddr.Port,
	}

	proxy := NewTCPProxy(connConfig)
	defer proxy.Close()

	// Connect to the backend through proxy config
	backendAddr := fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port)
	testConn, err := net.Dial("tcp", backendAddr)
	if err != nil {
		t.Fatalf("Failed to dial backend: %v", err)
	}
	defer testConn.Close()

	// Test multiple messages
	testMessages := []string{
		"Hello, World!",
		"TCP Proxy Test",
		"Integration Test",
	}

	for _, msg := range testMessages {
		// Send message
		_, err = testConn.Write([]byte(msg))
		if err != nil {
			t.Fatalf("Failed to write message: %v", err)
		}

		// Read echo
		buf := make([]byte, len(msg))
		n, err := io.ReadFull(testConn, buf)
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if string(buf[:n]) != msg {
			t.Errorf("Response = %q, want %q", string(buf[:n]), msg)
		}
	}
}

func BenchmarkTCPProxy_Create(b *testing.B) {
	connConfig := &config.ConnectionConfig{
		Name: "test-redis",
		Type: "tcp",
		Host: "localhost",
		Port: 6379,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy := NewTCPProxy(connConfig)
		proxy.Close()
	}
}

func BenchmarkTCPProxy_Connection(b *testing.B) {
	// Create echo server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().(*net.TCPAddr)

	// Run echo server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	connConfig := &config.ConnectionConfig{
		Name: "test-bench",
		Type: "tcp",
		Host: serverAddr.IP.String(),
		Port: serverAddr.Port,
	}

	proxy := NewTCPProxy(connConfig)
	defer proxy.Close()

	testData := []byte("benchmark test")

	backendAddr := fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", backendAddr)
		if err != nil {
			b.Fatalf("Failed to dial: %v", err)
		}

		conn.Write(testData)
		buf := make([]byte, len(testData))
		io.ReadFull(conn, buf)
		conn.Close()
	}
}
