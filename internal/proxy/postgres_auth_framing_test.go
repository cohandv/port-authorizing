package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func pgMsg(t byte, body string) []byte {
	b := []byte{t, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(4+len(body)))
	return append(b, body...)
}

func readPgMsg(t *testing.T, r *bufio.Reader) (byte, []byte) {
	t.Helper()
	var hdr [5]byte
	typ, body, err := readHeader(r, &hdr)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	b := make([]byte, body)
	if _, err := io.ReadFull(r, b); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return typ, b
}

func newFramingProxy(whitelist ...string) *PostgresAuthProxy {
	return NewPostgresAuthProxy(&config.ConnectionConfig{Name: "c"}, "", "u", "id", nil, &config.Config{}, whitelist, nil)
}

func expectBlocked(t *testing.T, cr *bufio.Reader, status byte) {
	t.Helper()
	if typ, _ := readPgMsg(t, cr); typ != 'E' {
		t.Fatalf("want ErrorResponse, got %q", typ)
	}
	if typ, b := readPgMsg(t, cr); typ != 'Z' || b[0] != status {
		t.Fatalf("want ReadyForQuery %q, got %q %q", status, typ, b)
	}
}

// Messages split across writes are still validated; blocked ones never reach the backend and get E+Z back.
func TestForwardClientFraming(t *testing.T) {
	p := newFramingProxy("^SELECT")
	p.txStatus.Store('T')
	client, proxyClient := net.Pipe()
	proxyBackend, backend := net.Pipe()
	go p.forwardClient(proxyClient, proxyBackend, newClientWriter(proxyClient))

	big := pgMsg('Q', "DELETE FROM t WHERE x = '"+strings.Repeat("x", 40<<10)+"'\x00")
	ok := pgMsg('Q', "SELECT 1\x00")
	go func() {
		for _, chunk := range [][]byte{big[:3], big[3:20000], big[20000:], ok} {
			_, _ = client.Write(chunk)
		}
		// extended protocol: blocked Parse drops Bind/Execute and is answered at Sync
		_, _ = client.Write(bytes.Join([][]byte{pgMsg('P', "\x00DELETE FROM t\x00\x00\x00"), pgMsg('B', "\x00\x00\x00\x00\x00\x00\x00"), pgMsg('E', "\x00\x00\x00\x00\x00"), pgMsg('S', "")}, nil))
	}()

	cr := bufio.NewReader(client)
	expectBlocked(t, cr, 'T')
	got := make([]byte, len(ok)) // the allowed SELECT reached the backend, alone
	if _, err := io.ReadFull(backend, got); err != nil || !bytes.Equal(got, ok) {
		t.Fatalf("backend got %q, %v", got, err)
	}
	expectBlocked(t, cr, 'T')
}

// An allowed query pipelined with a blocked one in the same read must still be flushed to the backend
// (it used to sit in the write buffer while the proxy waited for more client input).
func TestForwardClientFlushesBeforeBlocking(t *testing.T) {
	p := newFramingProxy("^SELECT")
	client, proxyClient := net.Pipe()
	proxyBackend, backend := net.Pipe()
	go p.forwardClient(proxyClient, proxyBackend, newClientWriter(proxyClient))

	ok := pgMsg('Q', "SELECT 1\x00")
	go func() { _, _ = client.Write(append(append([]byte{}, ok...), pgMsg('Q', "DROP TABLE t\x00")...)) }()
	go func() { _, _ = io.Copy(io.Discard, client) }()

	got := make([]byte, len(ok))
	_ = backend.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(backend, got); err != nil || !bytes.Equal(got, ok) {
		t.Fatalf("backend got %q, %v", got, err)
	}
}

// A blocked-query error injected while a large backend message is mid-flight must land after it, not inside it.
func TestBlockedErrorNeverSplitsBackendMessage(t *testing.T) {
	p := newFramingProxy("^SELECT")
	client, proxyClient := net.Pipe()
	proxyBackend, backend := net.Pipe()
	cw := newClientWriter(proxyClient)
	go p.forwardBackend(proxyBackend, cw)

	row := pgMsg('D', strings.Repeat("x", 100<<10))
	go func() {
		_, _ = backend.Write(row[:50<<10])
		go p.sendQueryBlockedError(cw, "DROP TABLE t")
		time.Sleep(20 * time.Millisecond)
		_, _ = backend.Write(row[50<<10:])
	}()

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second)) // a split message desyncs framing and would hang
	cr := bufio.NewReader(client)
	if typ, b := readPgMsg(t, cr); typ != 'D' || len(b) != len(row)-5 {
		t.Fatalf("want whole DataRow, got %q len %d", typ, len(b))
	}
	expectBlocked(t, cr, 'I')
}

// Parse carries parameter type OIDs after the query's terminator; they must not become part of the query text.
func TestParseQueryExcludesParamTypes(t *testing.T) {
	p := newFramingProxy(`^SELECT .*;$`)
	q := "SELECT oid FROM pg_type WHERE oid = ANY($1) ORDER BY oid;"
	if blocked, got := p.validateAndLogQuery(pgMsg('P', "\x00"+q+"\x00\x00\x01\x00\x00\x03\xf8")); blocked {
		t.Fatalf("blocked %q", got)
	}
}
