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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := ndt8.Upgrade(w, r)
		if err != nil {
			return
		}
	}))

	u, err := url.Parse(server.URL)
	rtx.Must(err, "cannot parse server URL")
	r.URL = u

	t.Run("upgrade-correct-protocol", func(t *testing.T) {
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		fmt.Printf("resp: %v\n", resp)

		if resp.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("upgrader did not start upgrade")
		}
	})

	t.Run("upgrade-wrong-protocol", func(t *testing.T) {
		r.Header.Set("Sec-WebSocket-Protocol", "wrong-protocol")

		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("upgrader did not return bad request on wrong protocol")
		}
	})
}

func downloadHandler(rw http.ResponseWriter, req *http.Request) {
	wsConn, err := ndt8.Upgrade(rw, req)
	rtx.Must(err, "failed to upgrade to WS")
	proto := ndt8.New(wsConn)
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	tx, rx, errCh := proto.SenderLoop(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-tx:
			fmt.Println(m)
		case m := <-rx:
			fmt.Println(m)
		case err := <-errCh:
			fmt.Printf("err: %v", err)
		}
	}
}

func TestProtocol_Download(t *testing.T) {
	tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{})
	rtx.Must(err, "failed to create listener")

	srv := &httptest.Server{
		Listener: netx.NewListener(tcpl),
		Config:   &http.Server{Handler: http.HandlerFunc(downloadHandler)},
	}
	srv.Start()

	u, err := url.Parse(srv.URL)
	u.Scheme = "ws"
	rtx.Must(err, "cannot get server URL")
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)

	d := websocket.Dialer{
		NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := net.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			return netx.FromTCPConn(conn.(*net.TCPConn))
		},
	}

	conn, _, err := d.Dial(u.String(), headers)

	rtx.Must(err, "cannot dial server")
	proto := ndt8.New(conn)
	senderCh, receiverCh, errCh := proto.ReceiverLoop(context.Background())
	start := time.Now()
	for {
		select {
		case <-context.Background().Done():
			return
		case m := <-senderCh:
			fmt.Printf("senderCh BytesReceived: %d, BytesSent: %d\n", m.BytesReceived, m.BytesSent)
			fmt.Printf("senderCh Goodput: %f Mb/s\n", float64(m.BytesReceived)/float64(time.Since(start).Microseconds())*8)
		case <-receiverCh:

		case err := <-errCh:
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				fmt.Printf("err: %v\n", err)
				return
			}
			fmt.Println("normal close")
			return
		}
	}
}
