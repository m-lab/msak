package client

import (
	"context"
	"crypto/tls"
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
	DefaultStreams = 3

	// DefaultLength is the default test duration for a new client.
	DefaultLength = 5 * time.Second

	// DefaultScheme is the default WebSocket scheme for a new Client.
	DefaultScheme = "wss"

	libraryName = "msak-client"
)

var (
	// ErrNoTargets is returned if all Locate targets have been tried.
	ErrNoTargets = errors.New("no targets available")

	libraryVersion = version.Version
)

// defaultDialer is the default websocket.Dialer used by the client.
// Its NetDial function wraps the net.Conn with a netx.Conn.
var defaultDialer = &websocket.Dialer{
	HandshakeTimeout: DefaultWebSocketHandshakeTimeout,
	NetDial: func(network, addr string) (net.Conn, error) {
		conn, err := net.Dial(network, addr)
		if err != nil {
			return nil, err
		}
		return netx.FromTCPLikeConn(conn.(*net.TCPConn))
	},
	TLSClientConfig: &tls.Config{},
}

// Locator is an interface used to get a list of available servers to test against.
type Locator interface {
	Nearest(ctx context.Context, service string) ([]v2.Target, error)
}

// Throughput1Client is a client for the throughput1 protocol.
type Throughput1Client struct {
	// ClientName is the name of the client sent to the server as part of the user-agent.
	ClientName string
	// ClientVersion is the version of the client sent to the server as part of the user-agent.
	ClientVersion string

	config Config

	dialer  *websocket.Dialer
	locator Locator

	// targets and tIndex cache the results from the Locate API.
	targets []v2.Target
	tIndex  map[string]int

	// recvByteCounters is a map of stream IDs to number of bytes, used to compute the goodput.
	// A new byte count is appended every time the client sees a receiver-side Measurement.
	recvByteCounters      map[int][]int64
	recvByteCountersMutex sync.Mutex
}

// Result contains the aggregate metrics collected during the test.
type Result struct {
	// Goodput is the average number of application-level bits per second that
	// have been transferred so far across all the streams.
	Goodput float64
	// Throughput is the average number of network-level bits per second that
	// have been transferred so far across all the streams.
	Throughput float64
	// Elapsed is the total time elapsed since the test started.
	Elapsed time.Duration
	// MinRTT is the minimum of MinRTT values observed across all the streams.
	MinRTT uint32
}

// makeUserAgent creates the user agent string.
func makeUserAgent(clientName, clientVersion string) string {
	return clientName + "/" + clientVersion + " " + libraryName + "/" + libraryVersion
}

// New returns a new Throughput1Client with the provided client name, version and config.
// It panics if clientName or clientVersion are empty.
func New(clientName, clientVersion string, config Config) *Throughput1Client {
	if clientName == "" || clientVersion == "" {
		panic("client name and version must be non-empty")
	}
	defaultDialer.TLSClientConfig.InsecureSkipVerify = config.NoVerify
	return &Throughput1Client{
		ClientName:    clientName,
		ClientVersion: clientVersion,

		config: config,
		dialer: defaultDialer,

		locator: locate.NewClient(makeUserAgent(clientName, clientVersion)),

		tIndex:           map[string]int{},
		recvByteCounters: map[int][]int64{},
	}
}

func (c *Throughput1Client) connect(ctx context.Context, serviceURL *url.URL) (*websocket.Conn, error) {
	q := serviceURL.Query()
	q.Set("streams", fmt.Sprint(c.config.NumStreams))
	q.Set("cc", c.config.CongestionControl)
	q.Set("duration", fmt.Sprintf("%d", c.config.Length.Milliseconds()))
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
	conn, _, err := c.dialer.DialContext(ctx, serviceURL.String(), headers)
	return conn, err
}

// nextURLFromLocate returns the next URL to try from the Locate API.
// If it's the first time we're calling this function, it contacts the Locate
// API. Subsequently, it returns the next URL from the cache.
// If there are no more URLs to try, it returns an error.
func (c *Throughput1Client) nextURLFromLocate(ctx context.Context, p string) (string, error) {
	if len(c.targets) == 0 {
		targets, err := c.locator.Nearest(ctx, "msak/throughput1")
		if err != nil {
			return "", err
		}
		// cache targets on success.
		c.targets = targets
	}
	// Returns the next URL from the cache.
	// The index to access the next URL (tIndex[k]) is per-path rather than global.
	k := c.config.Scheme + "://" + p
	if c.tIndex[k] < len(c.targets) {
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
	if c.config.Server != "" {
		c.config.Emitter.OnDebug(fmt.Sprintf("using server provided via flags %s", c.config.Server))
		path := getPathForSubtest(subtest)
		mURL = &url.URL{
			Scheme: c.config.Scheme,
			Host:   c.config.Server,
			Path:   path,
		}
		q := mURL.Query()
		q.Set("mid", c.config.MeasurementID)
		mURL.RawQuery = q.Encode()
	}

	// If no server has been provided, use the Locate API.
	if mURL == nil {
		c.config.Emitter.OnDebug("using locate")
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
	globalTimeout, cancel := context.WithTimeout(ctx, c.config.Length)
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
	for i := 0; i < c.config.NumStreams; i++ {
		streamID := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Run a single stream.
			err := c.runStream(globalTimeout, streamID, mURL, subtest, globalStartTime)
			if err != nil {
				c.config.Emitter.OnError(err)
			}
		}()

		time.Sleep(c.config.Delay)
	}

	wg.Wait()

	return nil
}

func (c *Throughput1Client) runStream(ctx context.Context, streamID int, mURL *url.URL,
	subtest spec.SubtestKind, globalStartTime time.Time) error {

	measurements := make(chan model.WireMeasurement)

	c.config.Emitter.OnStart(mURL.Host, subtest)
	conn, err := c.connect(ctx, mURL)
	if err != nil {
		c.config.Emitter.OnError(err)
		close(measurements)
		return err
	}
	c.config.Emitter.OnConnect(mURL.String())

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
			c.config.Emitter.OnComplete(streamID, mURL.Host)
			return nil
		case m := <-clientCh:
			// If subtest is download, store the client-side measurement.
			if subtest != spec.SubtestDownload {
				continue
			}
			c.config.Emitter.OnMeasurement(streamID, m)
			c.config.Emitter.OnDebug(fmt.Sprintf("Stream #%d - application r/w: %d/%d, network r/w: %d/%d",
				streamID, m.Application.BytesReceived, m.Application.BytesSent,
				m.Network.BytesReceived, m.Network.BytesSent))
			c.storeMeasurement(streamID, m)
		case m := <-serverCh:
			// If subtest is upload, store the server-side measurement.
			if subtest != spec.SubtestUpload {
				continue
			}
			c.config.Emitter.OnMeasurement(streamID, m)
			c.config.Emitter.OnDebug(fmt.Sprintf("#%d - application r/w: %d/%d, network r/w: %d/%d",
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
	c.config.Emitter.OnResult(result)
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
		panic(fmt.Sprintf("invalid subtest: %s", subtest))
	}
}
