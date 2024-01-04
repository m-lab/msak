// Package main implements a bare-bones minimal MSAK throughput1 client.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/locate/api/locate"
	"github.com/m-lab/msak/pkg/throughput1/model"
)

const clientName = "msak-minimal-client-go"

var clientVersion = "v0.0.1"

var (
	flagCC        = flag.String("cc", "bbr", "Congestion control algorithm to use")
	flagDuration  = flag.Duration("duration", 5*time.Second, "Length of the last stream")
	flagScheme    = flag.String("scheme", "wss", "Websocket scheme (wss or ws)")
	flagNoVerify  = flag.Bool("no-verify", false, "Skip TLS certificate verification")
	flagByteLimit = flag.Int("bytes", 0, "Byte limit to request to the server")
	flagServer    = flagx.URL{}
	flagMID       = flag.String("mid", uuid.NewString(), "Measurement ID to use")
)

func init() {
	flag.Var(&flagServer, "server.url", "URL to directly target")
}

// localDialer allows insecure TLS for explicit servers.
var localDialer = &websocket.Dialer{
	HandshakeTimeout: 5 * time.Second,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: *flagNoVerify,
	},
}

// connect to the given msak server URL, returning a *websocket.Conn.
func connect(ctx context.Context, s *url.URL) (*websocket.Conn, error) {
	q := s.Query()
	q.Set("streams", fmt.Sprintf("%d", 1))
	q.Set("cc", *flagCC)
	q.Set("bytes", fmt.Sprintf("%d", *flagByteLimit))
	q.Set("duration", fmt.Sprintf("%d", (*flagDuration).Milliseconds()))
	q.Set("client_arch", runtime.GOARCH)
	q.Set("client_library_name", clientName+"-adhoc")
	q.Set("client_library_version", clientVersion+"-adhoc")
	q.Set("client_os", runtime.GOOS)
	q.Set("client_name", clientName)
	q.Set("client_version", clientVersion)
	s.RawQuery = q.Encode()
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", "net.measurementlab.throughput.v1")
	headers.Add("User-Agent", clientName+"/"+clientVersion)
	conn, _, err := localDialer.DialContext(ctx, s.String(), headers)
	return conn, err
}

// formatMessage reports a WireMeasurement in a human readable format.
func formatMessage(prefix string, stream int, m model.WireMeasurement) {
	fmt.Printf("%s #%d - avg %0.2f Mbps, elapsed %0.4fs, payload r/w: %d/%d, network r/w: %d/%d kernel* r/w: %d/%d\n",
		prefix, stream,
		8*float64(m.Network.BytesSent)/(float64(m.ElapsedTime)),
		float64(m.ElapsedTime)/1000000.0,
		m.Application.BytesReceived, m.Application.BytesSent,
		m.Network.BytesReceived, m.Network.BytesSent,
		m.TCPInfo.BytesReceived, m.TCPInfo.BytesSent,
	)
}

// getDownloadServer find a single server from given flags or Locate API.
func getDownloadServer(ctx context.Context) (*url.URL, error) {
	// Use explicit server if provided.
	if flagServer.URL != nil {
		q := flagServer.URL.Query()
		q.Set("mid", *flagMID)
		flagServer.URL.RawQuery = q.Encode()
		return flagServer.URL, nil
	}

	// Use Locate API to request otherwise.
	lc := locate.NewClient(clientName + "/" + clientVersion)
	targets, err := lc.Nearest(ctx, "msak/throughput1")
	if err != nil {
		return nil, err
	}
	// Just use the first result.
	for i := range targets {
		srvurl := targets[i].URLs[*flagScheme+":///throughput/v1/download"]
		// Get server url.
		return url.Parse(srvurl)
	}
	return nil, errors.New("no server")
}

// getConn connects to a download server, returning the *websocket.Conn.
func getConn(ctx context.Context) (*websocket.Conn, error) {
	srv, err := getDownloadServer(ctx)
	if err != nil {
		return nil, err
	}
	// Connect to server.
	return connect(ctx, srv)
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *flagDuration)
	defer cancel()

	conn, err := getConn(ctx)
	rtx.Must(err, "failed to get websocket.Conn to server")
	defer conn.Close()

	// Max runtime.
	deadline := time.Now().Add(2 * *flagDuration)
	conn.SetWriteDeadline(deadline)
	conn.SetReadDeadline(deadline)

	// receive from text & binary messages from conn until the context expires or conn closes.
	var applicationBytesReceived int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
			kind, reader, err := conn.NextReader()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					fmt.Println("error", err)
				}
				return
			}
			switch kind {
			case websocket.BinaryMessage:
				// Binary messages are discarded after reading their size.
				size, err := io.Copy(io.Discard, reader)
				if err != nil {
					fmt.Println("error", err)
					return
				}
				applicationBytesReceived += size
			case websocket.TextMessage:
				data, err := io.ReadAll(reader)
				if err != nil {
					fmt.Println("error", err)
					return
				}
				applicationBytesReceived += int64(len(data))

				var m model.WireMeasurement
				if err := json.Unmarshal(data, &m); err != nil {
					fmt.Println("error", err)
					return
				}
				formatMessage("Server", 1, m)
			}
		}
	}
}
