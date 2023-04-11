package handler_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/handler"
	"github.com/m-lab/msak/internal/netx"
)

func TestNew(t *testing.T) {
	h := handler.New("testdata/")
	if h == nil {
		t.Errorf("New returned nil")
	}
}

func TestHandlers(t *testing.T) {
	// Server setup.
	dlHandler := http.HandlerFunc(handler.New("testdata/").Download)
	tcpl, err := net.ListenTCP("tcp", nil)
	rtx.Must(err, "cannot listen")
	server := httptest.NewUnstartedServer(dlHandler)
	server.Listener = netx.NewListener(tcpl)
	defer server.Close()

}

func TestHandler_Validation(t *testing.T) {
	// This string exceeds the maximum metadata key length.
	longKey := strings.Repeat("longkey", 10)
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
			name:       "missing Upgrade header",
			target:     "/?mid=test&streams=2&duration=5s",
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
