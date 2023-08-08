package measurer_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"testing"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/measurer"
	"github.com/m-lab/msak/internal/netx"
)

func TestNdt8Measurer_Start(t *testing.T) {
	// Use a net.Pipe to test. This has the advantage that it works on every
	// platform, allowing to test the measurer functionality on e.g. Windows,
	// but TCPInfo/BBRInfo retrieval will never work.
	client, server := net.Pipe()
	serverConn := &netx.Conn{
		Conn: server,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mchan := measurer.Start(ctx, serverConn)
	go func() {
		_, err := serverConn.Write([]byte("test"))
		rtx.Must(err, "failed to write to pipe")
		serverConn.Close()
	}()
	_, err := ioutil.ReadAll(client)
	rtx.Must(err, "failed to read from pipe")

	select {
	case m := <-mchan:
		fmt.Println("received measurement")
		if m.Network.BytesSent != 4 {
			t.Errorf("invalid byte counter value")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("did not receive any measurement")
	}
}
