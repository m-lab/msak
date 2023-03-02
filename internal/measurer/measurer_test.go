package measurer_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/m-lab/msak/internal/measurer"
	"github.com/m-lab/msak/internal/netx"
)

type mockWSConn struct {
	underlyingConn net.Conn
}

func (c *mockWSConn) UnderlyingConn() net.Conn {
	return c.underlyingConn
}

func TestNdt8Measurer_Start(t *testing.T) {
	// Use a net.Pipe to test. This has the advantage that it works on every
	// platform, allowing to test the measurer functionality on e.g. Windows,
	// but TCPInfo/BBRInfo retrieval will never work.
	client, _ := net.Pipe()

	netxConn := &netx.Conn{
		Conn: client,
	}
	mc := &mockWSConn{
		underlyingConn: netxConn,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mchan := measurer.Start(ctx, mc)
	select {
	case <-mchan:
		fmt.Println("received measurement")
	case <-time.After(1 * time.Second):
		t.Fatalf("did not receive any measurement")
	}
}
