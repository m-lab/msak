package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/testingx"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

func TestNew(t *testing.T) {
	t.Run("new clients have the expected name and version", func(t *testing.T) {
		c := New("test", "v1.0.0", Config{})
		if c.ClientName != "test" || c.ClientVersion != "v1.0.0" {
			t.Errorf("client.New() returned client with wrong name/version")
		}
	})
}

func Test_makeUserAgent(t *testing.T) {
	t.Run("generate requested user agent", func(t *testing.T) {
		got := makeUserAgent("clientname", "clientversion")
		expected := fmt.Sprintf("%s/%s %s/%s", "clientname", "clientversion",
			libraryName, libraryVersion)
		if got != expected {
			t.Errorf("makeUserAgent() = %s, want %s", got, expected)
		}
	})
}

func setupTestServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestNDT8Client_connect(t *testing.T) {

	c := New("test", "version", Config{
		NumStreams:        3,
		CongestionControl: "cubic",
		Length:            5 * time.Second,
	})

	t.Run("connect sends qs parameters and headers", func(t *testing.T) {
		upgrader := websocket.Upgrader{}

		// Set up a test server with a handler that verifies querystring parameters
		// and headers.
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer wsConn.Close()

			// Check querystring parameters.
			expected := map[string]string{
				"streams":                "3",
				"cc":                     "cubic",
				"duration":               fmt.Sprintf("%d", c.config.Length.Milliseconds()),
				"client_arch":            runtime.GOARCH,
				"client_library_name":    libraryName,
				"client_library_version": libraryVersion,
				"client_os":              runtime.GOOS,
				"client_name":            c.ClientName,
				"client_version":         c.ClientVersion,
			}
			for k, v := range expected {
				if got := r.URL.Query().Get(k); got != v {
					t.Errorf("expected qs parameter %s = %s, got %s", k, v, got)
				}
			}

			// Check headers
			expected = map[string]string{
				"Sec-WebSocket-Protocol": spec.SecWebSocketProtocol,
				"User-Agent":             makeUserAgent(c.ClientName, c.ClientVersion),
			}
			for k, v := range expected {
				if got := r.Header.Get(k); got != v {
					t.Errorf("expected header %s = %s, got %s", k, v, got)
				}
			}
		})

		s := setupTestServer(handler)
		defer s.Close()

		urlStr := "ws" + strings.TrimPrefix(s.URL, "http")

		u, err := url.Parse(urlStr)
		testingx.Must(t, err, "cannot parse server URL")

		_, err = c.connect(context.Background(), u)
		if err != nil {
			t.Errorf("NDT8Client.connect() error: %v", err)
			return
		}
	})
}
