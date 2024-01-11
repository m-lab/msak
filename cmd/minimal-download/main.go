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
	"log"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	clientName    = "msak-minimal-download-go"
	clientVersion = "v0.0.1"
	locateURL     = "https://locate.measurementlab.net/v2/nearest/"
)

var (
	flagCC        = flag.String("cc", "bbr", "Congestion control algorithm to use")
	flagDuration  = flag.Duration("duration", 5*time.Second, "Length of the last stream")
	flagByteLimit = flag.Int("bytes", 0, "Byte limit to request to the server")
	flagNoVerify  = flag.Bool("no-verify", false, "Skip TLS certificate verification")
	flagServerURL = flag.String("server.url", "", "URL to directly target")
	flagMID       = flag.String("server.mid", uuid.NewString(), "Measurement ID to use")
	flagScheme    = flag.String("locate.scheme", "wss", "Websocket scheme (wss or ws)")
	flagLocateURL = flag.String("locate.url", locateURL, "The base url for the Locate API")
	flagStreams   = flag.Int("streams", 1, "The number of streams to create")
)

// WireMeasurement is a wrapper for Measurement structs that contains
// information about this TCP stream that does not need to be sent every time.
// Every field except for Measurement is only expected to be non-empty once.
//
// Find the authoritative structures in:
// * github.com/m-lab/msak/pkg/throughput1/model/measurement.go
type WireMeasurement struct {
	// CC is the congestion control used by the sender of this WireMeasurement.
	CC string `json:",omitempty"`
	// UUID is the unique identifier for this TCP stream.
	UUID string `json:",omitempty"`
	// LocalAddr is the local TCP endpoint (ip:port).
	LocalAddr string `json:",omitempty"`
	// RemoteAddr is the server's TCP endpoint (ip:port).
	RemoteAddr string `json:",omitempty"`
	// Measurement is the Measurement struct wrapped by this WireMeasurement.
	Measurement
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON and sent as a textual message.
type Measurement struct {
	// Application contains the application-level BytesSent/Received pair.
	Application ByteCounters
	// Network contains the network-level BytesSent/Received pair.
	Network ByteCounters
	// ElapsedTime is the time elapsed since the start of the measurement
	// according to the party sending this Measurement.
	ElapsedTime int64 `json:",omitempty"`
	// BBRInfo is an optional struct containing BBR metrics. Only applicable
	// when the congestion control algorithm used by the party sending this
	// Measurement is BBR. WARNING: field types are approximate.
	BBRInfo map[string]int64 `json:",omitempty"`
	// TCPInfo is an optional struct containing some of the TCP_INFO kernel
	// metrics for this TCP stream. Only applicable when the party sending this
	// Measurement has access to it. WARNING: field types are approximate.
	TCPInfo map[string]int64 `json:",omitempty"`
}

type ByteCounters struct {
	// BytesSent is the number of bytes sent.
	BytesSent int64 `json:",omitempty"`
	// BytesReceived is the number of bytes received.
	BytesReceived int64 `json:",omitempty"`
}

// NearestResult is returned by the Locate API in response to query requests.
type NearestResult struct {
	// Results contains an array of Targets matching the client request.
	Results []Target `json:"results,omitempty"`
}

// Target is returned by the Locate API.
type Target struct {
	// URLs contains measurement service resource names and the complete URL for
	// running a measurement.
	URLs map[string]string `json:"urls"`
}

// localDialer allows insecure TLS for explicit servers.
var localDialer = &websocket.Dialer{
	HandshakeTimeout: 5 * time.Second,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: *flagNoVerify,
	},
}

func init() {
	// Disable all prefixing for logging.
	log.SetFlags(0)
}

// connect to the given msak server URL, returning a *websocket.Conn.
func connect(ctx context.Context, s *url.URL) (*websocket.Conn, error) {
	q := s.Query()
	q.Set("streams", fmt.Sprintf("%d", *flagStreams))
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
func formatMessage(prefix string, stream int, m WireMeasurement) {
	log.Printf("%s #%d - rate %0.2f Mbps, rtt %5.2fms, elapsed %0.4fs, application r/w: %d/%d, network r/w: %d/%d kernel* r/w: %d/%d\n",
		prefix, stream,
		8*float64(m.TCPInfo["BytesAcked"])/(float64(m.ElapsedTime)), // to mbps.
		float64(m.TCPInfo["RTT"])/1000.0,                            // to ms.
		float64(m.ElapsedTime)/1000000.0,                            // to sec.
		m.Application.BytesReceived, m.Application.BytesSent,
		m.Network.BytesReceived, m.Network.BytesSent,
		m.TCPInfo["BytesReceived"], m.TCPInfo["BytesAcked"],
	)
}

// locateGetServers contacts the Locate API for a set of healthy servers.
func locateGetServers(ctx context.Context, userAgent, locate string) ([]Target, error) {
	u, err := url.Parse(*flagLocateURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, locate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if userAgent == "" {
		// User agent is required.
		return nil, errors.New("no user agent given")
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	reply := &NearestResult{}
	err = json.Unmarshal(b, reply)
	if err != nil {
		return nil, err
	}
	return reply.Results, err
}

// getDownloadServer find a single server from given flags or Locate API.
func getDownloadServer(ctx context.Context) (*url.URL, error) {
	// Use explicit server if provided.
	if *flagServerURL != "" {
		u, err := url.Parse(*flagServerURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("mid", *flagMID)
		u.RawQuery = q.Encode()
		return u, nil
	}

	// Use Locate API to request otherwise.
	targets, err := locateGetServers(ctx, clientName+"/"+clientVersion, "msak/throughput1")
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
func (s *sharedResults) getConn(ctx context.Context, streams int) error {
	srv, err := getDownloadServer(ctx)
	if err != nil {
		return err
	}
	// Connect to server.
	wg := &sync.WaitGroup{}
	for i := 0; i < streams; i++ {
		wg.Add(1)
		go func() {
			conn, err := connect(ctx, srv)
			if err != nil {
				log.Println("skipping failed conn:", err)
			}
			s.mu.Lock() // protect conns and startTime.
			s.conns = append(s.conns, conn)
			if !s.started.Load() {
				s.started.Store(true)
				// record start time as first open connection.
				s.startTime = time.Now()
			}
			s.mu.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()
	return nil
}

type sharedResults struct {
	applicationBytesReceived atomic.Int64
	minRTT                   atomic.Int64
	mu                       sync.Mutex
	conns                    []*websocket.Conn
	started                  atomic.Bool // set true after first connection opens.
	startTime                time.Time
	stopped                  atomic.Bool // set true after first connection closes (may be different than start conn).
	stopTime                 time.Time
}

func (s *sharedResults) downloadConn(ctx context.Context, wg *sync.WaitGroup, start time.Time, streamCount int, stream int, conn *websocket.Conn) {
	defer func() {
		s.mu.Lock() // protect stopTime.
		if !s.stopped.Load() {
			// Stop after first connect close.
			s.stopped.Store(true)
			s.stopTime = time.Now()
		}
		s.mu.Unlock()
		wg.Done()
	}()
outer:
	// receive from text & binary messages from conn until the context expires or conn closes.
	for {
		select {
		case <-ctx.Done():
			break outer
		default:
			kind, reader, err := conn.NextReader()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					log.Println("error", err)
				}
				break outer
			}
			if s.stopped.Load() {
				// Stop counting after first connection closes.
				return
			}
			switch kind {
			case websocket.BinaryMessage:
				// Binary messages are discarded after reading their size.
				size, err := io.Copy(io.Discard, reader)
				if err != nil {
					log.Println("error", err)
					return
				}
				s.applicationBytesReceived.Add(size)
			case websocket.TextMessage:
				data, err := io.ReadAll(reader)
				if err != nil {
					log.Println("error", err)
					return
				}
				s.applicationBytesReceived.Add(int64(len(data)))

				var m WireMeasurement
				if err := json.Unmarshal(data, &m); err != nil {
					log.Println("error", err)
					return
				}
				s.minRTT.Store(m.TCPInfo["MinRTT"])

				if streamCount == 1 {
					// Use server metrics for single stream tests.
					formatMessage("Download server", 1, m)
				} else {
					if stream == 0 {
						// Only do this for one stream.
						log.Printf("Download client #1 - Avg %0.2f Mbps, elapsed %0.4fs, application r/w: %d/%d\n",
							8*float64(s.applicationBytesReceived.Load())/1e6/time.Since(start).Seconds(), // as mbps.
							time.Since(start).Seconds(), 0, s.applicationBytesReceived.Load())
					}
				}
			}
		}
	}
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *flagDuration*2)
	defer cancel()

	s := &sharedResults{}
	err := s.getConn(ctx, *flagStreams)
	if err != nil {
		log.Fatal(err)
	}
	defer func(c []*websocket.Conn) {
		for i := range c {
			c[i].Close()
		}
	}(s.conns)

	// Max runtime.
	deadline := time.Now().Add(*flagDuration * 2)
	for i := range s.conns {
		s.conns[i].SetWriteDeadline(deadline)
		s.conns[i].SetReadDeadline(deadline)
	}

	wg := &sync.WaitGroup{}
	for i := range s.conns {
		wg.Add(1)
		go s.downloadConn(ctx, wg, s.startTime, *flagStreams, i, s.conns[i])
	}
	wg.Wait()

	elapsed := s.stopTime.Sub(s.startTime)
	log.Printf("Download client #1 - Avg %0.2f Mbps, MinRTT %5.2fms, elapsed %0.4fs, application r/w: %d/%d\n",
		8*float64(s.applicationBytesReceived.Load())/1e6/elapsed.Seconds(), // as mbps.
		float64(s.minRTT.Load())/1000.0,                                    // as ms.
		elapsed.Seconds(), 0, s.applicationBytesReceived.Load())
}
