package ndt8_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	hj "github.com/getlantern/httptest"
	"github.com/gorilla/websocket"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/ndt8"
	"github.com/m-lab/msak/pkg/ndt8/spec"
)

type HijackableResponseWriter struct {
	http.ResponseWriter

	Conn net.Conn
	in   *bufio.Reader
	out  *bufio.Writer
}

func (rw HijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return rw.Conn, bufio.NewReadWriter(rw.in, rw.out), nil
}

func TestProtocol_Upgrade(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ndt/v8/download", bytes.NewReader([]byte{}))
	r.Header.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	r.Header.Add("Sec-Websocket-Version", "13")
	r.Header.Add("Sec-WebSocket-Key", "test")
	r.Header.Add("Connection", "upgrade")
	r.Header.Add("Upgrade", "websocket")

	resp := hj.NewRecorder(nil)
	conn, err := ndt8.Upgrade(resp, r)
	if err != nil {
		t.Fatalf("Upgrade failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("Upgrade returned nil")
	}
	r.Header.Set("Sec-WebSocket-Protocol", "wrong-protocol")
	conn, err = ndt8.Upgrade(resp, r)
	if err == nil {
		t.Fatalf("Upgrade accepted a wrong subprotocol")
	}
	if conn != nil {
		t.Fatalf("Upgrade returned a websocket.Conn on error")
	}
}

func TestProtocol_Send(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ndt/v8/download", bytes.NewReader([]byte{}))
	req.Header.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	req.Header.Add("Sec-Websocket-Version", "13")
	req.Header.Add("Sec-WebSocket-Key", "test")
	req.Header.Add("Connection", "upgrade")
	req.Header.Add("Upgrade", "websocket")

	client, server := net.Pipe()
	serverConn := &netx.Conn{Conn: server}
	clientConn := &netx.Conn{Conn: client}

	rw := httptest.NewRecorder()

	hjrw := HijackableResponseWriter{
		ResponseWriter: rw,
		Conn:           serverConn,
		in:             bufio.NewReader(&bytes.Buffer{}),
		out:            bufio.NewWriter(rw.Body),
	}

	wsURL, err := url.Parse("ws://localhost/test")
	rtx.Must(err, "failed to parse WS url")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// receiver loop goroutine
	go func() {
		clientWS, _, err := websocket.NewClient(clientConn, wsURL, req.Header, 1024, 1024)
		rtx.Must(err, "cannot create websocket client conn")

		clientProto := ndt8.New(clientWS)

		_, _, errCh := clientProto.ReceiverLoop(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errCh:
				t.Errorf("ReceiverLoop error: %v", err)
				return
			}
		}
	}()

	// upgrade the request to websocket
	wsWriterConn, err := ndt8.Upgrade(hjrw, req)
	rtx.Must(err, "upgrade failed")

	proto := ndt8.New(wsWriterConn)
	txMeasurements, rxMeasurements, errCh := proto.Send(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-txMeasurements:
			fmt.Printf("tx sent: %d, tx received: %d\n", m.BytesSent, m.BytesReceived)
		case m := <-rxMeasurements:
			fmt.Println(m)
		case err := <-errCh:
			t.Errorf("received error: %v", err)
			return
		}
	}

}
