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

	libraryName = "msak-client"
)

var (
	// ErrNoTargets is returned if all Locate targets have been tried.
	ErrNoTargets = errors.New("no targets available")

	libraryVersion = version.Version
)

type Locator interface {
	Nearest(ctx context.Context, service string) ([]v2.Target, error)
}

type Throughput1Client struct {
	ClientName    string
	ClientVersion string

	Dialer *websocket.Dialer

	Server     string
	ServiceURL *url.URL

	Locate Locator

	Scheme string

	NumStreams        int
	Length            time.Duration
	Delay             time.Duration
	CongestionControl string
	MeasurementID     string

	OutputPath string

	Emitter Emitter

	// targets and tIndex cache the results from the Locate API.
	targets []v2.Target
	tIndex  map[string]int
}

type Result struct {
	Goodput    float64
	Throughput float64
	Elapsed    time.Duration
	MinRTT     uint32
}

type StreamResult struct {
	Result
	StreamID int
}

// makeUserAgent creates the user agent string
func makeUserAgent(clientName, clientVersion string) string {
	return clientName + "/" + clientVersion + " " + libraryName + "/" + libraryVersion
}

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
		Scheme: "wss",
		Locate: locate.NewClient(
			makeUserAgent(clientName, clientVersion),
		),
		Emitter: &HumanReadable{Debug: false},
		tIndex:  map[string]int{},
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

	globalStartTime := time.Now()
	applicationBytes := map[int][]int64{}

	// Main client loop. Spawns one goroutine per stream requested.
	for i := 0; i < c.NumStreams; i++ {
		streamID := i
		wg.Add(1)
		measurements := make(chan model.WireMeasurement)

		go func() {
			defer wg.Done()

			// Connect to mURL.
			c.Emitter.OnStart(mURL.Host, subtest)
			conn, err := c.connect(ctx, mURL)
			if err != nil {
				c.Emitter.OnError(err)
				close(measurements)
				return
			}
			c.Emitter.OnConnect(mURL.String())

			proto := throughput1.New(conn)

			var clientCh, serverCh <-chan model.WireMeasurement
			var errCh <-chan error
			switch subtest {
			case spec.SubtestDownload:
				clientCh, serverCh, errCh = proto.ReceiverLoop(globalTimeout)
			case spec.SubtestUpload:
				clientCh, serverCh, errCh = proto.SenderLoop(globalTimeout)
			}

			for {
				select {
				case <-globalTimeout.Done():
					c.Emitter.OnComplete(streamID, mURL.Host)
					return
				case m := <-clientCh:
					if subtest != spec.SubtestDownload {
						continue
					}
					c.emitResults(streamID, m, globalStartTime, applicationBytes)
				case m := <-serverCh:
					if subtest != spec.SubtestUpload {
						continue
					}
					c.emitResults(streamID, m, globalStartTime, applicationBytes)
				case err := <-errCh:
					c.Emitter.OnError(err)
				}
			}
		}()

		time.Sleep(c.Delay)
	}

	wg.Wait()

	return nil
}

func (c *Throughput1Client) emitResults(streamID int, m model.WireMeasurement,
	globalStartTime time.Time, applicationBytes map[int][]int64) {
	c.Emitter.OnMeasurement(streamID, m)
	elapsed := time.Since(globalStartTime)
	streamResult := StreamResult{
		StreamID: streamID,
		Result: Result{
			Elapsed:    elapsed,
			Goodput:    float64(m.Application.BytesReceived) / float64(m.ElapsedTime) * 8,
			Throughput: float64(m.Network.BytesReceived) / float64(m.ElapsedTime) * 8,
			MinRTT:     m.TCPInfo.MinRTT,
		},
	}
	c.Emitter.OnStreamResult(streamResult)

	applicationBytes[streamID] = append(applicationBytes[streamID], m.Application.BytesReceived)

	var sum int64
	for _, bytes := range applicationBytes {
		sum += bytes[len(bytes)-1]
	}
	result := Result{
		Elapsed: elapsed,
		Goodput: float64(sum) / float64(elapsed.Microseconds()) * 8,
	}
	c.Emitter.OnResult(result)
}

func (c *Throughput1Client) Download(ctx context.Context) {
	err := c.start(ctx, spec.SubtestDownload)
	if err != nil {
		log.Println(err)
	}
}

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
