package netx_test

import (
	"net"
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
	at := c.GetAcceptTime()
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
	if cc, err := c.GetCC(); err != nil || cc != "cubic" {
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
	if _, err := c.GetUUID(); err != nil {
		t.Errorf("GetUUID failed: %v", err)
	}
	if _, _, err = c.GetInfo(); err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
}
