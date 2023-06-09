package latency

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/m-lab/msak/pkg/latency/model"
)

func TestNewHandler(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler(tempDir, 2*time.Second)
	if h.dataDir != tempDir || h.sessions == nil {
		t.Errorf("NewHandler(): invalid handler returned")
	}
	// Add an item to the cache and check its TTL is the configured one.
	h.sessions.Set("test", &model.Session{}, ttlcache.DefaultTTL)
	ttl := h.sessions.Get("test").TTL()
	if ttl != 2*time.Second {
		t.Errorf("cached item has invalid TTL %s", ttl)
	}

	// Check that cache cleanup goroutine can be stopped. If it hasn't been
	// started, calling Stop() will block indefinitely.
	completed := make(chan bool)
	go func() {
		h.sessions.Stop()
		completed <- true
	}()

	select {
	case <-time.After(1 * time.Second):
		t.Fatalf("failed to stop cache cleanup goroutine")
	case <-completed:
		// NOTHING - happy path
	}
}

func TestOnEviction(t *testing.T) {
	// Create a cache with a very low TTL
	tempDir := t.TempDir()
	h := NewHandler(tempDir, 1*time.Millisecond)
	h.sessions.Set("test", model.NewSession("test"), ttlcache.DefaultTTL)

	// Wait for the TTL to expire.
	<-time.After(100 * time.Millisecond)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("cannot read temp data folder: %v\n", err)
	}
	if len(files) == 0 {
		t.Errorf("cache expired but no file written")
	}
}

func TestHandler_Authorize(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler(tempDir, 5*time.Second)

	rw := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/latency/v1/authorize", nil)
	if err != nil {
		t.Fatalf("cannot create request: %v", err)
	}

	// Valid authorization request.
	req.URL.RawQuery = "mid=test"
	h.Authorize(rw, req)
	if rw.Result().StatusCode != http.StatusOK {
		t.Errorf("invalid HTTP status code %d (expected 200)\n",
			rw.Result().StatusCode)
	}

	// No mid provided on the querystring.
	rw = httptest.NewRecorder()
	req.URL.RawQuery = ""
	h.Authorize(rw, req)
	if rw.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid HTTP status code %d (expected %d)\n",
			rw.Result().StatusCode, 401)
	}
}

func TestHandler_Result(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler(tempDir, 5*time.Second)
	rw := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/latency/v1/authorize", nil)
	if err != nil {
		t.Fatalf("cannot create request: %v", err)
	}
	req.URL.RawQuery = "mid=test"
	h.Authorize(rw, req)
	if rw.Result().StatusCode != http.StatusOK {
		t.Errorf("invalid HTTP status code %d (expected 200)",
			rw.Result().StatusCode)
	}

	// Verify that there is an in-memory session for mid=test.
	rw = httptest.NewRecorder()
	req, err = http.NewRequest(http.MethodGet, "/latency/v1/result", nil)
	if err != nil {
		t.Fatalf("cannot create request: %v", err)
	}
	req.URL.RawQuery = "mid=test"
	h.Result(rw, req)
	if rw.Result().StatusCode != http.StatusOK {
		t.Errorf("invalid HTTP status code %d (expected 200)",
			rw.Result().StatusCode)
	}
	// Unmarshal response body.
	b, err := io.ReadAll(rw.Body)
	if err != nil {
		t.Errorf("cannot read response body: %v", err)
	}
	var summary model.Summary
	err = json.Unmarshal(b, &summary)
	if err != nil {
		t.Errorf("cannot unmarshal response body: %v", err)
	}
	if summary.ID != "test" {
		t.Errorf("invalid ID in summary")
	}

	// Do not provide any mid.
	rw = httptest.NewRecorder()
	req.URL.RawQuery = ""
	h.Result(rw, req)
	if rw.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("invalid HTTP status code %d (expected 400)",
			rw.Result().StatusCode)
	}

	// Request the summary for a mid that does not exist in the session cache.
	rw = httptest.NewRecorder()
	req.URL.RawQuery = "mid=doesnotexist"
	h.Result(rw, req)
	if rw.Result().StatusCode != http.StatusNotFound {
		t.Errorf("invalid HTTP status code %d (expected 404)",
			rw.Result().StatusCode)
	}

}

func TestHandler_processPacket(t *testing.T) {
	serverConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("cannot create test socket")
	}

	tempDir := t.TempDir()
	h := NewHandler(tempDir, 5*time.Second)

	clientConn, err := net.Dial("udp", serverConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("cannot connect to test socket")
	}

	invalidPayload := []byte("test")
	err = h.processPacket(serverConn, clientConn.LocalAddr(),
		invalidPayload, time.Now())
	if err == nil {
		t.Errorf("expected error on invalid payload, got nil.")
	}

	invalidSession := []byte(`{"ID":"invalid"}`)
	err = h.processPacket(serverConn, clientConn.LocalAddr(),
		invalidSession, time.Now())
	if err != errorUnauthorized {
		t.Errorf("wrong error: expected %v, got %v", errorUnauthorized, err)
	}

	// Add a session to the cache
	h.sessions.Set("test", model.NewSession("test"), ttlcache.DefaultTTL)
	// Send a kickoff message
	validKickoff := []byte(`{"ID":"test","Type":"c2s"}`)
	err = h.processPacket(serverConn, clientConn.LocalAddr(), validKickoff,
		time.Now())
	if err != nil {
		t.Errorf("unexpected error with valid session: %v", err)
	}
	// Check that packets are received within 1s.
	err = clientConn.SetDeadline(time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("failed to set deadline on client conn")
	}
	buf := make([]byte, 1024)
	packetsRead := 0
	for {
		n, err := clientConn.Read(buf)
		if err != nil {
			// The error should not be != timeout
			if netErr, ok := err.(net.Error); ok && !netErr.Timeout() {
				t.Errorf("Read() returned an error: %v", netErr)
			}
			break
		}

		// Is it a valid latency measurement?
		var latencyPacket model.LatencyPacket
		fmt.Printf("packet: %s\n", buf[:n])
		err = json.Unmarshal(buf[:n], &latencyPacket)
		if err != nil {
			t.Fatalf("cannot unmarshal latency packet: %v", err)
		}
		packetsRead++
	}

	if packetsRead == 0 {
		t.Errorf("did not receive any latency packets after kickoff")
	}
}

func Test_processS2CPacket(t *testing.T) {
	serverConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("cannot create test socket")
	}

	tempDir := t.TempDir()
	h := NewHandler(tempDir, 5*time.Second)

	clientConn, err := net.Dial("udp", serverConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("cannot connect to test socket")
	}

	// Create a valid session with a fake sendTime.
	pingTime := time.Now()
	pongTime := pingTime.Add(100 * time.Millisecond)
	session := h.sessions.Set("test", model.NewSession("test"),
		ttlcache.DefaultTTL)

	// Set sendTime for Seq=0 to pingTime.
	session.Value().SendTimes[0] = pingTime
	payload := []byte(`{"Type":"s2c","ID":"test","Seq":0}`)
	err = h.processPacket(serverConn, clientConn.RemoteAddr(), payload, pongTime)
	if err != nil {
		t.Fatalf("unexpected error while processing pong packet: %v", err)
	}

	// Check that the PacketsReceived counter has been updated.
	if session.Value().PacketsReceived.Load() != 1 {
		t.Errorf("failed to update PacketsReceived (expected %d, got %d)", 1,
			session.Value().PacketsReceived.Load())
	}
	// The measurement slice should contain one measurement.
	if len(session.Value().Packets) != 1 {
		t.Errorf("wrong number of measurements (expected %d, got %d)", 1,
			len(session.Value().Packets))
	}

	// Check the computed RTT.
	rtt := session.Value().LastRTT.Load()
	expected := pongTime.Sub(pingTime).Microseconds()
	if session.Value().LastRTT.Load() != pongTime.Sub(pingTime).Microseconds() {
		t.Errorf("wrong computed RTT (expected %d, got %d)", expected, rtt)
	}

}
