package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}),
	)
	if err != nil {
		t.Fatalf("failed to load key pair: %v", err)
	}
	return cert
}

// generateCASignedCert creates a self-signed CA and a leaf certificate for
// 127.0.0.1 signed by that CA. It returns the leaf ready for tls.Server and
// a filesystem path to the CA's PEM-encoded certificate, for use as
// TLSConfig.CACert.
func generateCASignedCert(t *testing.T) (tls.Certificate, string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}
	leafTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create leaf certificate: %v", err)
	}
	leafCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)}),
	)
	if err != nil {
		t.Fatalf("failed to load leaf key pair: %v", err)
	}

	caCertPath := filepath.Join(t.TempDir(), "ca.pem")
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := os.WriteFile(caCertPath, caCertPEM, 0o600); err != nil {
		t.Fatalf("failed to write CA cert: %v", err)
	}

	return leafCert, caCertPath
}

// readSSLRequest returns an error rather than calling t.Fatalf directly:
// it's called from server goroutines, and testing.T requires FailNow to run
// on the test goroutine itself. Callers must forward the error themselves.
func readSSLRequest(conn net.Conn) error {
	buf := make([]byte, 8)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("failed to read SSLRequest: %w", err)
	}
	if code := binary.BigEndian.Uint32(buf[4:8]); code != pgSSLRequestCode {
		return fmt.Errorf("expected SSLRequest code %d, got %d", pgSSLRequestCode, code)
	}
	return nil
}

// TestDialBackend_DeclinesTLS covers the backend saying "N" (no TLS support):
// dialBackend must keep using the plain connection, not redial.
func TestDialBackend_DeclinesTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if err := readSSLRequest(conn); err != nil {
			errCh <- err
			return
		}
		_, _ = conn.Write([]byte{'N'})
		_, _ = conn.Write([]byte("plain-ok"))
	}()

	p := &PostgresAuthProxy{config: &config.ConnectionConfig{Name: "t", Host: "127.0.0.1"}}
	conn, err := p.dialBackend(listener.Addr().String())
	if err != nil {
		t.Fatalf("dialBackend failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, ok := conn.(*tls.Conn); ok {
		t.Fatalf("expected plain connection, got TLS")
	}

	buf := make([]byte, len("plain-ok"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read from backend: %v", err)
	}
	if string(buf) != "plain-ok" {
		t.Fatalf("unexpected payload: %s", buf)
	}

	select {
	case err := <-errCh:
		t.Fatalf("server goroutine: %v", err)
	default:
	}
}

// TestDialBackend_UpgradesToTLS covers the backend saying "S": dialBackend
// must complete a real TLS handshake and hand back a *tls.Conn.
func TestDialBackend_UpgradesToTLS(t *testing.T) {
	cert := generateSelfSignedCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if err := readSSLRequest(conn); err != nil {
			errCh <- err
			return
		}
		_, _ = conn.Write([]byte{'S'})

		tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
		defer func() { _ = tlsConn.Close() }()
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		_, _ = tlsConn.Write([]byte("tls-ok"))
	}()

	p := &PostgresAuthProxy{config: &config.ConnectionConfig{
		Name: "t",
		Host: "127.0.0.1",
		TLS:  &config.TLSConfig{Enabled: true, SkipVerify: true},
	}}
	conn, err := p.dialBackend(listener.Addr().String())
	if err != nil {
		t.Fatalf("dialBackend failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, ok := conn.(*tls.Conn); !ok {
		t.Fatalf("expected TLS connection, got %T", conn)
	}

	buf := make([]byte, len("tls-ok"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read from backend: %v", err)
	}
	if string(buf) != "tls-ok" {
		t.Fatalf("unexpected payload: %s", buf)
	}

	select {
	case err := <-errCh:
		t.Fatalf("server goroutine: %v", err)
	default:
	}
}

// TestDialBackend_FallsBackOnHandshakeFailure covers a backend that claims TLS
// support ("S") but then breaks the handshake (e.g. untrusted cert): dialBackend
// must fall back to a fresh plain connection instead of failing outright.
func TestDialBackend_FallsBackOnHandshakeFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	errCh := make(chan error, 1)
	go func() {
		// First connection: claims TLS, then aborts the handshake.
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		if err := readSSLRequest(conn); err != nil {
			errCh <- err
			return
		}
		_, _ = conn.Write([]byte{'S'})
		_ = conn.Close()

		// Second connection: the plain fallback dial.
		conn2, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn2.Close() }()
		_, _ = conn2.Write([]byte("fallback-ok"))
	}()

	p := &PostgresAuthProxy{config: &config.ConnectionConfig{
		Name: "t",
		Host: "127.0.0.1",
		TLS:  &config.TLSConfig{Enabled: true},
	}}
	conn, err := p.dialBackend(listener.Addr().String())
	if err != nil {
		t.Fatalf("dialBackend failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, ok := conn.(*tls.Conn); ok {
		t.Fatalf("expected plain fallback connection, got TLS")
	}

	buf := make([]byte, len("fallback-ok"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read from backend: %v", err)
	}
	if string(buf) != "fallback-ok" {
		t.Fatalf("unexpected payload: %s", buf)
	}

	select {
	case err := <-errCh:
		t.Fatalf("server goroutine: %v", err)
	default:
	}
}

// TestDialBackend_ExplicitlyDisabled covers tls.enabled=false: dialBackend
// must skip the SSLRequest negotiation entirely.
func TestDialBackend_ExplicitlyDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = conn.Write([]byte("no-negotiation"))
	}()

	p := &PostgresAuthProxy{config: &config.ConnectionConfig{
		Name: "t",
		Host: "127.0.0.1",
		TLS:  &config.TLSConfig{Enabled: false},
	}}
	conn, err := p.dialBackend(listener.Addr().String())
	if err != nil {
		t.Fatalf("dialBackend failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	buf := make([]byte, len("no-negotiation"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read from backend: %v", err)
	}
	if string(buf) != "no-negotiation" {
		t.Fatalf("unexpected payload: %s", buf)
	}
}

// TestDialBackend_VerifiesAgainstCACert covers real certificate verification:
// with SkipVerify left false (the default) and a trusted CA configured via
// TLS.CACert, dialBackend must complete the handshake and hand back a
// verified *tls.Conn - not just skip verification like the other TLS tests.
func TestDialBackend_VerifiesAgainstCACert(t *testing.T) {
	leafCert, caCertPath := generateCASignedCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if err := readSSLRequest(conn); err != nil {
			errCh <- err
			return
		}
		_, _ = conn.Write([]byte{'S'})

		tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{leafCert}})
		defer func() { _ = tlsConn.Close() }()
		if err := tlsConn.Handshake(); err != nil {
			errCh <- fmt.Errorf("server handshake failed: %w", err)
			return
		}
		_, _ = tlsConn.Write([]byte("verified-ok"))
	}()

	p := &PostgresAuthProxy{config: &config.ConnectionConfig{
		Name: "t",
		Host: "127.0.0.1",
		TLS:  &config.TLSConfig{Enabled: true, CACert: caCertPath},
	}}
	conn, err := p.dialBackend(listener.Addr().String())
	if err != nil {
		t.Fatalf("dialBackend failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, ok := conn.(*tls.Conn); !ok {
		t.Fatalf("expected TLS connection, got %T", conn)
	}

	buf := make([]byte, len("verified-ok"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read from backend: %v", err)
	}
	if string(buf) != "verified-ok" {
		t.Fatalf("unexpected payload: %s", buf)
	}

	select {
	case err := <-errCh:
		t.Fatalf("server goroutine: %v", err)
	default:
	}
}
