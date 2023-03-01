package measurer

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/ndt8/model"
)

type mockWSConn struct {
	underlyingConn net.Conn
}

func (c *mockWSConn) UnderlyingConn() net.Conn {
	return c.underlyingConn
}

func TestNdt8Measurer_Start(t *testing.T) {
	// Create a TCP socket to test.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	rtx.Must(err, "cannot create test socket")
	fp := os.NewFile(uintptr(fd), "test-socket")
	conn, err := net.FileConn(fp)
	rtx.Must(err, "cannot create net.Conn")

	netxConn := &netx.Conn{
		Conn: conn,
	}
	mc := &mockWSConn{
		underlyingConn: netxConn,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mchan, err := Start(ctx, mc)
	if err != nil {
		t.Fatalf("Start returned an error")
	}
	select {
	case <-mchan:
		fmt.Println("received measurement")
	case <-time.After(1 * time.Second):
		t.Fatalf("did not receive any measurement")
	}
}

func TestNdt8Measurer_measure(t *testing.T) {
	dst := make(chan model.Measurement)
	measurer := &ndt8Measurer{
		dstChan: dst,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		measurer.measure(ctx)
	}()
	m := <-dst
	cancel()

	if m.TCPInfo == nil {
		t.Fatalf("missing data from Measurement")
	}
}
