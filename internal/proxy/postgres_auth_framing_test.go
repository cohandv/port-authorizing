package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/davidcohan/port-authorizing/internal/config"
)

func pgMsg(t byte, body string) []byte {
	b := []byte{t, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(4+len(body)))
	return append(b, body...)
}

// Messages split across writes are still validated; blocked ones never reach the backend and get E+Z back.
func TestForwardClientFraming(t *testing.T) {
	p := NewPostgresAuthProxy(&config.ConnectionConfig{Name: "c"}, "", "u", "id", &config.Config{}, []string{"^SELECT"}, nil)
	p.txStatus.Store('T')
	client, proxyClient := net.Pipe()
	proxyBackend, backend := net.Pipe()
	go p.forwardClient(proxyClient, proxyBackend)

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
	for i := 0; i < 2; i++ { // one E+Z per blocked query
		if typ, _, err := readMessage(cr, maxClientMessage); err != nil || typ != 'E' {
			t.Fatalf("want ErrorResponse, got %q %v", typ, err)
		}
		if typ, msg, err := readMessage(cr, maxClientMessage); err != nil || typ != 'Z' || msg[5] != 'T' {
			t.Fatalf("want ReadyForQuery T, got %q %q %v", typ, msg, err)
		}
		if i == 0 { // the allowed SELECT reached the backend, alone
			got := make([]byte, len(ok))
			if _, err := io.ReadFull(backend, got); err != nil || !bytes.Equal(got, ok) {
				t.Fatalf("backend got %q, %v", got, err)
			}
		}
	}
}
