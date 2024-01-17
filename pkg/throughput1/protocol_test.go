package throughput1_test

import (
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
	"github.com/m-lab/msak/pkg/throughput1"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

func TestProtocol_Upgrade(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ndt/v8/download", bytes.NewReader([]byte{}))
	r.Header.Add("Sec-Websocket-Version", "13")
	r.Header.Add("Sec-WebSocket-Key", "test")
	r.Header.Add("Connection", "upgrade")
	r.Header.Add("Upgrade", "websocket")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := throughput1.Upgrade(w, r)
		if err != nil {
			return
		}
	}))

	u, err := url.Parse(server.URL)
	rtx.Must(err, "cannot parse server URL")
	r.URL = u

	t.Run("upgrade-correct-protocol", func(t *testing.T) {
		r.Header.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
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
	wsConn, err := throughput1.Upgrade(rw, req)
	rtx.Must(err, "failed to upgrade to WS")
	proto := throughput1.New(wsConn)
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
			return netx.FromTCPLikeConn(conn.(*net.TCPConn))
		},
	}

	conn, _, err := d.Dial(u.String(), headers)

	rtx.Must(err, "cannot dial server")
	proto := throughput1.New(conn)
	senderCh, receiverCh, errCh := proto.ReceiverLoop(context.Background())
	start := time.Now()
	for {
		select {
		case <-context.Background().Done():
			return
		case m := <-senderCh:
			fmt.Printf("senderCh Network.BytesReceived: %d, Network.BytesSent: %d\n",
				m.Network.BytesReceived, m.Network.BytesSent)
			fmt.Printf("senderCh Network throughput: %f Mb/s\n",
				float64(m.Network.BytesReceived)/float64(time.Since(start).Microseconds())*8)
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

func TestProtocol_ScaleMessage(t *testing.T) {
	tests := []struct {
		name      string
		byteLimit int
		msgSize   int
		bytesSent int
		want      int
	}{
		{
			name:      "no-limit",
			byteLimit: 0,
			msgSize:   10,
			bytesSent: 100,
			want:      10,
		},
		{
			name:      "under-limit",
			byteLimit: 200,
			msgSize:   10,
			bytesSent: 100,
			want:      10,
		},
		{
			name:      "at-limit",
			byteLimit: 110,
			msgSize:   10,
			bytesSent: 100,
			want:      10,
		},
		{
			name:      "over-limit",
			byteLimit: 110,
			msgSize:   20,
			bytesSent: 100,
			want:      10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &throughput1.Protocol{}
			p.SetByteLimit(tt.byteLimit)
			if got := p.ScaleMessage(tt.msgSize, tt.bytesSent); got != tt.want {
				t.Errorf("Protocol.ScaleMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}
