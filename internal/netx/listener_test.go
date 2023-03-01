package netx_test

import (
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/netx"
)

func dialAsync(t *testing.T, addr string) {
	go func() {
		// Because the socket already exists, Dial will block until Accept is
		// called below.
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Errorf("unexpected failure to dial local conn: %v", err)
			return
		}
		// Wait until primary test routine closes conn and returns.
		buf := make([]byte, 1)
		c.Read(buf)
		c.Close()
	}()
}

func TestListener_Accept(t *testing.T) {
	tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{})
	rtx.Must(err, "failed to create listener")
	l := netx.NewListener(tcpl)
	defer l.Close()
	dialAsync(t, tcpl.Addr().String())

	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Listener.Accept() unexpected error = %v", err)
	}

	var c netx.ConnInfo
	var ok bool
	if c, ok = got.(netx.ConnInfo); !ok {
		t.Fatalf("Listener.Accept() wrong Conn type = %T, want netx.Conn", got)
	}
	// Check that the AcceptTime is in the past minute (i.e. that it has been
	// initialized).
	at := c.AcceptTime()
	if time.Since(at) > 1*time.Minute {
		t.Fatalf("invalid accept time")
	}

	// Accept error due to closed listener.
	tcpl, err = net.ListenTCP("tcp", &net.TCPAddr{})
	rtx.Must(err, "failed to create listener")
	l = netx.NewListener(tcpl)
	defer l.Close()

	tcpl.Close()
	_, err = l.Accept()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestConn_Congestion(t *testing.T) {
	tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{})
	rtx.Must(err, "failed to create listener")
	l := netx.NewListener(tcpl)
	defer l.Close()
	dialAsync(t, tcpl.Addr().String())
	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Listener.Accept() unexpected error = %v", err)
	}
	defer got.Close()

	var c netx.ConnInfo
	var ok bool
	if c, ok = got.(netx.ConnInfo); !ok {
		t.Fatalf("Listener.Accept() wrong Conn type = %T, want netx.Conn", got)
	}

	if err = c.SetCC("cubic"); err != nil {
		t.Errorf("SetCC failed: %v", err)
	}
	if cc, err := c.CC(); err != nil || cc != "cubic" {
		t.Errorf("GetCC failed or unexpected cc: %v", err)
	}
}

func TestConn_GetInfoAndUUID(t *testing.T) {
	tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{})
	rtx.Must(err, "failed to create listener")
	l := netx.NewListener(tcpl)
	defer l.Close()
	dialAsync(t, tcpl.Addr().String())
	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Listener.Accept() unexpected error = %v", err)
	}
	defer got.Close()

	var c netx.ConnInfo
	var ok bool
	if c, ok = got.(netx.ConnInfo); !ok {
		t.Fatalf("Listener.Accept() wrong Conn type = %T, want netx.Conn", got)
	}
	if _, err := c.UUID(); err != nil {
		t.Errorf("GetUUID failed: %v", err)
	}
	if _, _, err = c.Info(); err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
}

func TestToConnInfo(t *testing.T) {
	// NOTE: because we cannot synthetically create a tls.Conn that wraps a
	// netx.Conn, we must setup an httptest server with TLS enabled. While we
	// do that, we use it to validate the regular HTTP server netx.Conn as
	// well.

	fakeHTTPReply := "HTTP/1.0 200 OK\n\ntest"
	tests := []struct {
		name    string
		conn    net.Conn
		withTLS bool
	}{
		{
			name:    "success-Conn",
			withTLS: false,
		},
		{
			name:    "success-tls.Conn",
			withTLS: true,
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		hj, ok := rw.(http.Hijacker)
		if !ok {
			t.Fatalf("httptest Server does not support Hijacker interface")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("failed to hijack responsewriter")
		}
		defer conn.Close()
		// Write a fake reply for the client.
		conn.Write([]byte(fakeHTTPReply))

		// Extract the ConnInfo from the hijacked conn.
		got := netx.ToConnInfo(conn)
		if got == nil {
			t.Errorf("ToConnInfo() failed to return ConnInfo from conn")
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewUnstartedServer(mux)
			// Setup local listener using our Listener rather than the default.
			laddr := &net.TCPAddr{
				IP: net.ParseIP("127.0.0.1"),
			}
			tcpl, err := net.ListenTCP("tcp", laddr)
			rtx.Must(err, "failed to listen during unit test")
			// Use our listener in the httptest Server.
			s.Listener = netx.NewListener(tcpl)
			// Start a plain or tls server.
			if tt.withTLS {
				s.StartTLS()
			} else {
				s.Start()
			}
			defer s.Close()

			// Use the server-provided client for TLS settings.
			c := s.Client()
			req, err := http.NewRequest(http.MethodGet, s.URL, nil)
			rtx.Must(err, "Failed to create request to %s", s.URL)
			// Run request to run conn test in handler.
			resp, err := c.Do(req)
			rtx.Must(err, "failed to GET %s", s.URL)
			b, err := ioutil.ReadAll(resp.Body)
			rtx.Must(err, "failed to read reply from %s", s.URL)

			if string(b) != "test" {
				t.Errorf("failed to receive reply from server")
			}
		})
	}
}

func TestToConnInfoPanic(t *testing.T) {
	// Verify that unsupported net.Conn types cause a panic.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("ToConnInfo did not panic on an unsupported type.")
		}
	}()

	netx.ToConnInfo(&net.UDPConn{})
}
