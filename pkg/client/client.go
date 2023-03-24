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
	"github.com/m-lab/msak/pkg/ndt8"
	"github.com/m-lab/msak/pkg/ndt8/model"
	"github.com/m-lab/msak/pkg/ndt8/spec"
)

const (
	// DefaultWebSocketHandshakeTimeout is the default timeout used by the client
	// for the WebSocket handshake.
	DefaultWebSocketHandshakeTimeout = 5 * time.Second

	// DefaultStreams is the default number of streams for a new client.
	DefaultStreams = 5

	libraryName    = "msak-client"
	libraryVersion = "0.0.1"
)

var (
	// ErrNoTargets is returned if all Locate targets have been tried.
	ErrNoTargets = errors.New("no targets available")
)

type Locator interface {
	Nearest(ctx context.Context, service string) ([]v2.Target, error)
}

type NDTMClient struct {
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

	OutputPath    string
	ResultsByUUID map[string]*model.NDT8Result

	// targets and tIndex cache the results from the Locate API.
	targets []v2.Target
	tIndex  map[string]int
}

// makeUserAgent creates the user agent string
func makeUserAgent(clientName, clientVersion string) string {
	return clientName + "/" + clientVersion + " " + libraryName + "/" + libraryVersion
}

func New(clientName, clientVersion string) *NDTMClient {
	return &NDTMClient{
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
		ResultsByUUID: make(map[string]*model.NDT8Result),
		Scheme:        "wss",
		Locate: locate.NewClient(
			makeUserAgent(clientName, clientVersion),
		),
		tIndex: map[string]int{},
	}
}

func (c *NDTMClient) connect(ctx context.Context, serviceURL *url.URL) (*websocket.Conn, error) {
	q := serviceURL.Query()
	q.Set("streams", fmt.Sprint(c.NumStreams))
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
func (c *NDTMClient) nextURLFromLocate(ctx context.Context, p string) (string, error) {
	if len(c.targets) == 0 {
		targets, err := c.Locate.Nearest(ctx, "msak/ndtm")
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

func (c *NDTMClient) start(ctx context.Context, subtest spec.SubtestKind) error {
	// Find the URL to use for this measurement.
	var mURL *url.URL
	// If the server has been provided, use it and use default paths based on
	// the subtest kind (download/upload).
	if c.Server != "" {
		log.Print("using server ", c.Server)
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
		log.Print("using service url ", c.ServiceURL.String())
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

	for i := 0; i < c.NumStreams; i++ {
		wg.Add(2)
		measurements := make(chan model.WireMeasurement)
		result := &model.NDT8Result{
			MeasurementID: c.MeasurementID,
			Direction:     string(subtest),
		}

		go func() {
			defer wg.Done()
			log.Print("connecting to ", mURL.String())
			// Connect to mURL.
			conn, err := c.connect(ctx, mURL)
			if err != nil {
				log.Print(err)
				close(measurements)
				return
			}

			netxConn := netx.ToConnInfo(conn.UnderlyingConn())

			// To store measurement results we use a map associating the
			// TCP flow's unique identifier to the corresponding results.
			uuid, err := netxConn.UUID()
			if err != nil {
				log.Print(err)
				return
			}

			result.UUID = uuid
			result.StartTime = time.Now().UTC()
			c.ResultsByUUID[uuid] = result

			defer func() {
				result.EndTime = time.Now().UTC()
			}()

			proto := ndt8.New(conn)

			var senderCh, receiverCh <-chan model.WireMeasurement
			var errCh <-chan error
			switch subtest {
			case spec.SubtestDownload:
				senderCh, receiverCh, errCh = proto.ReceiverLoop(globalTimeout)
			case spec.SubtestUpload:
				senderCh, receiverCh, errCh = proto.SenderLoop(globalTimeout)
			}

			for {
				select {
				case <-globalTimeout.Done():
					return
				case m := <-senderCh:
					log.Print(m)
				case m := <-receiverCh:
					log.Print(m)
				case err := <-errCh:
					log.Print(err)
				}
			}
		}()

		time.Sleep(c.Delay)
	}

	wg.Wait()

	return nil
}

func (c *NDTMClient) Download(ctx context.Context) {
	err := c.start(ctx, spec.SubtestDownload)
	if err != nil {
		log.Println(err)
	}
}

func (c *NDTMClient) Upload(ctx context.Context) {
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
