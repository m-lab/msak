package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/locate/api/locate"
	v2 "github.com/m-lab/locate/api/v2"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/throughput1"
	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/m-lab/msak/pkg/throughput1/spec"
	"github.com/m-lab/msak/pkg/version"
)

const (
	// DefaultWebSocketHandshakeTimeout is the default timeout used by the client
	// for the WebSocket handshake.
	DefaultWebSocketHandshakeTimeout = 5 * time.Second

	// DefaultStreams is the default number of streams for a new client.
	DefaultStreams = 5

	// DefaultLength is the default test duration for a new client.
	DefaultLength = 5 * time.Second

	libraryName = "msak-client"
)

var (
	// ErrNoTargets is returned if all Locate targets have been tried.
	ErrNoTargets = errors.New("no targets available")

	libraryVersion = version.Version
)

// Locator is an interface used to get a list of available servers to test against.
type Locator interface {
	Nearest(ctx context.Context, service string) ([]v2.Target, error)
}

// Throughput1Client is a client for the throughput1 protocol.
type Throughput1Client struct {
	// ClientName is the name of the client as sent to the server as part of the user-agent.
	ClientName string
	// ClientVersion is the version of the client as sent to the server as part of the user-agent.
	ClientVersion string

	// Dialer is the websocket.Dialer used by the client.
	Dialer *websocket.Dialer

	// Server is the server to connect to. If both Server and ServiceURL are empty,
	// the server is obtained by querying the configured Locator.
	Server string

	// ServiceURL is the full URL to connect to. If both Server and ServiceURL are empty,
	// the server is obtained by querying the configured Locator.
	ServiceURL *url.URL

	// Locate is the Locator used to obtain the server to connect to.
	Locate Locator

	// Scheme is the WebSocket scheme used to connect to the server (ws or wss).
	Scheme string

	// NumStreams is the number of streams that will be spawned by this client to run a
	// download or an upload test.
	NumStreams int

	// Length is the duration of the test.
	Length time.Duration

	// Delay is the delay between each stream.
	Delay time.Duration

	// CongestionControl is the congestion control algorithm to request from the server.
	CongestionControl string

	// MeasurementID is the manually configured Measurement ID ("mid") to pass to the server.
	MeasurementID string

	// Emitter is the emitter used to emit results.
	Emitter Emitter

	// targets and tIndex cache the results from the Locate API.
	targets []v2.Target
	tIndex  map[string]int

	recvByteCounters      map[int][]int64
	recvByteCountersMutex sync.Mutex
}

// Result contains the aggregate metrics collected during the test.
type Result struct {
	Goodput    float64
	Throughput float64
	Elapsed    time.Duration
	MinRTT     uint32
}

// StreamResult contains the per-stream metrics collected during the test.
type StreamResult struct {
	Result
	StreamID int
}

// makeUserAgent creates the user agent string.
func makeUserAgent(clientName, clientVersion string) string {
	return clientName + "/" + clientVersion + " " + libraryName + "/" + libraryVersion
}

// New returns a new Throughput1Client with the provided client name and version.
func New(clientName, clientVersion string) *Throughput1Client {
	return &Throughput1Client{
		ClientName:    clientName,
		ClientVersion: clientVersion,
		Dialer: &websocket.Dialer{
			HandshakeTimeout: DefaultWebSocketHandshakeTimeout,
			NetDial: func(network, addr string) (net.Conn, error) {
				conn, err := net.Dial(network, addr)
				if err != nil {
					return nil, err
				}
				return netx.FromTCPConn(conn.(*net.TCPConn))
			},
		},
		Scheme:     "wss",
		NumStreams: DefaultStreams,
		Length:     DefaultLength,
		Locate: locate.NewClient(
			makeUserAgent(clientName, clientVersion),
		),
		Emitter: &HumanReadable{Debug: false},

		tIndex:           map[string]int{},
		recvByteCounters: map[int][]int64{},
	}
}

func (c *Throughput1Client) connect(ctx context.Context, serviceURL *url.URL) (*websocket.Conn, error) {
	q := serviceURL.Query()
	q.Set("streams", fmt.Sprint(c.NumStreams))
	q.Set("cc", c.CongestionControl)
	q.Set("duration", fmt.Sprintf("%d", c.Length.Milliseconds()))
	q.Set("client_arch", runtime.GOARCH)
	q.Set("client_library_name", libraryName)
	q.Set("client_library_version", libraryVersion)
	q.Set("client_os", runtime.GOOS)
	q.Set("client_name", c.ClientName)
	q.Set("client_version", c.ClientVersion)
	serviceURL.RawQuery = q.Encode()
	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	headers.Add("User-Agent", makeUserAgent(c.ClientName, c.ClientVersion))
	conn, _, err := c.Dialer.DialContext(ctx, serviceURL.String(), headers)
	return conn, err
}

// nextURLFromLocate returns the next URL to try from the Locate API.
// If it's the first time we're calling this function, it contacts the Locate
// API. Subsequently, it returns the next URL from the cache.
// If there are no more URLs to try, it returns an error.
func (c *Throughput1Client) nextURLFromLocate(ctx context.Context, p string) (string, error) {
	if len(c.targets) == 0 {
		targets, err := c.Locate.Nearest(ctx, "msak/throughput1")
		if err != nil {
			return "", err
		}
		// cache targets on success.
		c.targets = targets
	}
	k := c.Scheme + "://" + p
	if c.tIndex[k] < len(c.targets) {
		fmt.Println(c.targets[c.tIndex[k]].URLs)
		r := c.targets[c.tIndex[k]].URLs[k]
		c.tIndex[k]++
		return r, nil
	}
	return "", ErrNoTargets
}

func (c *Throughput1Client) start(ctx context.Context, subtest spec.SubtestKind) error {
	// Find the URL to use for this measurement.
	var mURL *url.URL
	// If the server has been provided, use it and use default paths based on
	// the subtest kind (download/upload).
	if c.Server != "" {
		c.Emitter.OnDebug(fmt.Sprintf("using server provided via flags %s", c.Server))
		path := getPathForSubtest(subtest)
		mURL = &url.URL{
			Scheme: c.Scheme,
			Host:   c.Server,
			Path:   path,
		}
		q := mURL.Query()
		q.Set("mid", c.MeasurementID)
		mURL.RawQuery = q.Encode()
	}

	// If a service URL was provided, use it as-is.
	if c.ServiceURL != nil {
		c.Emitter.OnDebug(fmt.Sprintf("using service url provided via flags %s", c.ServiceURL.String()))
		// Override scheme to match the provided service url.
		c.Scheme = c.ServiceURL.Scheme
		mURL = c.ServiceURL
	}

	// If no service URL nor server was provided, use the Locate API.
	if mURL == nil {
		log.Print("using locate")
		urlStr, err := c.nextURLFromLocate(ctx, getPathForSubtest(subtest))
		if err != nil {
			return err
		}
		mURL, err = url.Parse(urlStr)
		if err != nil {
			return err
		}
		log.Print("URL: ", mURL.String())
	}

	wg := &sync.WaitGroup{}
	globalTimeout, cancel := context.WithTimeout(ctx, c.Length)
	defer cancel()

	// Reset the counters.
	c.recvByteCounters = map[int][]int64{}
	globalStartTime := time.Now()

	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		// Print goodput every 100ms. Stop when the context is cancelled.
		for {
			select {
			case <-globalTimeout.Done():
				return
			case <-t.C:
				c.emitResult(globalStartTime)
			}
		}
	}()

	// Main client loop. Spawns one goroutine per stream.
	for i := 0; i < c.NumStreams; i++ {
		streamID := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Run a single stream.
			err := c.runStream(globalTimeout, streamID, mURL, subtest, globalStartTime)
			if err != nil {
				c.Emitter.OnError(err)
			}
		}()

		time.Sleep(c.Delay)
	}

	wg.Wait()

	return nil
}

func (c *Throughput1Client) runStream(ctx context.Context, streamID int, mURL *url.URL,
	subtest spec.SubtestKind, globalStartTime time.Time) error {

	measurements := make(chan model.WireMeasurement)

	c.Emitter.OnStart(mURL.Host, subtest)
	conn, err := c.connect(ctx, mURL)
	if err != nil {
		c.Emitter.OnError(err)
		close(measurements)
		return err
	}
	c.Emitter.OnConnect(mURL.String())

	proto := throughput1.New(conn)

	var clientCh, serverCh <-chan model.WireMeasurement
	var errCh <-chan error
	switch subtest {
	case spec.SubtestDownload:
		clientCh, serverCh, errCh = proto.ReceiverLoop(ctx)
	case spec.SubtestUpload:
		clientCh, serverCh, errCh = proto.SenderLoop(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			c.Emitter.OnComplete(streamID, mURL.Host)
			return nil
		case m := <-clientCh:
			// If subtest is download, store the client-side measurement.
			if subtest != spec.SubtestDownload {
				continue
			}
			c.Emitter.OnMeasurement(streamID, m)
			c.Emitter.OnDebug(fmt.Sprintf("Stream #%d - application r/w: %d/%d, network r/w: %d/%d",
				streamID, m.Application.BytesReceived, m.Application.BytesSent,
				m.Network.BytesReceived, m.Network.BytesSent))
			c.storeMeasurement(streamID, m)
		case m := <-serverCh:
			// If subtest is upload, store the server-side measurement.
			if subtest != spec.SubtestUpload {
				continue
			}
			c.Emitter.OnMeasurement(streamID, m)
			c.Emitter.OnDebug(fmt.Sprintf("#%d - application r/w: %d/%d, network r/w: %d/%d",
				streamID, m.Application.BytesReceived, m.Application.BytesSent,
				m.Network.BytesReceived, m.Network.BytesSent))
			c.storeMeasurement(streamID, m)
		case err := <-errCh:
			return err
		}
	}
}

func (c *Throughput1Client) storeMeasurement(streamID int, m model.WireMeasurement) {
	// Append the value of the Application.BytesReceived counter to the corresponding recvByteCounters map entry.
	c.recvByteCountersMutex.Lock()
	c.recvByteCounters[streamID] = append(c.recvByteCounters[streamID], m.Application.BytesReceived)
	c.recvByteCountersMutex.Unlock()
}

// applicationBytes returns the aggregate application-level bytes transferred by all the streams.
func (c *Throughput1Client) applicationBytes() int64 {
	var sum int64
	c.recvByteCountersMutex.Lock()
	for _, bytes := range c.recvByteCounters {
		sum += bytes[len(bytes)-1]
	}
	c.recvByteCountersMutex.Unlock()
	return sum
}

// emitResult emits the result of the current measurement via the configured Emitter.
func (c *Throughput1Client) emitResult(start time.Time) {
	applicationBytes := c.applicationBytes()
	elapsed := time.Since(start)
	goodput := float64(applicationBytes) / float64(elapsed.Seconds()) * 8 // bps
	result := Result{
		Elapsed:    elapsed,
		Goodput:    goodput,
		Throughput: 0, // TODO
	}
	c.Emitter.OnResult(result)
}

// Download runs a download test using the settings configured for this client.
func (c *Throughput1Client) Download(ctx context.Context) {
	err := c.start(ctx, spec.SubtestDownload)
	if err != nil {
		log.Println(err)
	}
}

// Upload runs an upload test using the settings configured for this client.
func (c *Throughput1Client) Upload(ctx context.Context) {
	err := c.start(ctx, spec.SubtestUpload)
	if err != nil {
		log.Println(err)
	}
}

func getPathForSubtest(subtest spec.SubtestKind) string {
	switch subtest {
	case spec.SubtestDownload:
		return spec.DownloadPath
	case spec.SubtestUpload:
		return spec.UploadPath
	default:
		return "invalid"
	}
}
