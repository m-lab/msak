package handler_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/handler"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/ndt8"
	"github.com/m-lab/msak/pkg/ndt8/model"
	"github.com/m-lab/msak/pkg/ndt8/spec"
)

func TestNew(t *testing.T) {
	h := handler.New("testdata/")
	if h == nil {
		t.Errorf("New returned nil")
	}
}

func setupTestServer(datadir string, handler http.Handler) *httptest.Server {
	tcpl, err := net.ListenTCP("tcp", nil)
	rtx.Must(err, "cannot listen")
	server := httptest.NewUnstartedServer(handler)
	server.Listener = netx.NewListener(tcpl)
	return server
}

func setupTestWSDialer(u *url.URL) *websocket.Dialer {
	return &websocket.Dialer{
		NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := net.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			return netx.FromTCPConn(conn.(*net.TCPConn))
		},
	}
}

func TestHandler_Upload(t *testing.T) {
	tempDir := t.TempDir()
	h := handler.New(tempDir)

	server := setupTestServer(tempDir, http.HandlerFunc(h.Upload))
	server.Start()
	defer server.Close()

	u, err := url.Parse(server.URL)
	rtx.Must(err, "cannot get server URL")
	u.Scheme = "ws"
	q := u.Query()
	q.Add("mid", "test-mid")
	q.Add("streams", "1")
	q.Add("duration", "500")
	u.RawQuery = q.Encode()

	dialer := setupTestWSDialer(u)

	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)

	conn, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("websocket dial returned nil")
	}
	proto := ndt8.New(conn)
	timeout, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	senderCh, receiverCh, errCh := proto.SenderLoop(timeout)
	drain(t, timeout, senderCh, receiverCh, errCh)

	// Check that the output JSON file has been created.
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("reading output folder failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("invalid number of files in output folder")
	}
}

func TestHandler_Download(t *testing.T) {
	// Server setup.
	tempDir := t.TempDir()
	h := handler.New(tempDir)

	server := setupTestServer(tempDir, http.HandlerFunc(h.Download))
	server.Start()
	defer server.Close()

	u, err := url.Parse(server.URL)
	rtx.Must(err, "cannot get server URL")
	u.Scheme = "ws"
	q := u.Query()
	q.Add("mid", "test-mid")
	q.Add("streams", "1")
	q.Add("duration", "500")
	u.RawQuery = q.Encode()

	dialer := setupTestWSDialer(u)

	headers := http.Header{}
	headers.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)

	conn, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("websocket dial returned nil")
	}

	proto := ndt8.New(conn)
	timeout, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	senderCh, receiverCh, errCh := proto.ReceiverLoop(timeout)
	drain(t, timeout, senderCh, receiverCh, errCh)

	// Check that the output JSON file has been created.
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("reading output folder failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("invalid number of files in output folder")
	}
}

// Utility function to drain sender/receiver channels in tests.
func drain(t *testing.T, timeout context.Context, senderCh,
	receiverCh <-chan model.WireMeasurement, errCh <-chan error) {
	for {
		select {
		case <-timeout.Done():
			return
		case <-senderCh:
			// nothing
		case <-receiverCh:
			// nothing
		case err := <-errCh:
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("unexpected close: %v", err)
			}
			return
		}
	}
}

func TestHandler_Validation(t *testing.T) {
	// This string exceeds the maximum metadata key length.
	longKey := strings.Repeat("longkey", 10)
	longValue := strings.Repeat("longvalue", 100)
	h := handler.New("testdata/")
	tests := []struct {
		name       string
		target     string
		headers    http.Header
		statusCode int
	}{
		{
			name:       "missing mid",
			target:     "/",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "missing streams",
			target:     "/?mid=test",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "invalid duration",
			target:     "/?mid=test&streams=2&duration=invalid",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "metadata key too long",
			target:     "/?mid=test&streams=2&" + longKey,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "metadata value too long",
			target:     "/?mid=test&ŝtreams=2&key=" + longValue,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "missing Upgrade header",
			target:     "/?mid=test&streams=2&duration=5000",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "unsupported CC",
			target:     "/?mid=test&streams=2&cc=invalid",
			statusCode: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test download handler.
			res := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.target, nil)
			req.Header = tt.headers
			h.Download(res, req)
			if res.Result().StatusCode != tt.statusCode {
				t.Errorf("unexpected status code %d", res.Result().StatusCode)
			}

			// Repeat the test for the upload handler.
			res = httptest.NewRecorder()
			h.Upload(res, req)
			if res.Result().StatusCode != tt.statusCode {
				t.Errorf("unexpected status code %d", res.Result().StatusCode)
			}
		})
	}
}
