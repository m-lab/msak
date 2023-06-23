package latency1

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jellydator/ttlcache/v3"
	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/handler"
	"github.com/m-lab/msak/internal/persistence"
	"github.com/m-lab/msak/pkg/latency1/model"
)

const sendDuration = 5 * time.Second

var (
	errorUnauthorized = errors.New("unauthorized")
	errorInvalidSeqN  = errors.New("invalid sequence number")
)

// Handler is the handler for latency tests.
type Handler struct {
	dataDir    string
	sessions   *ttlcache.Cache[string, *model.Session]
	sessionsMu sync.Mutex
}

// NewHandler returns a new handler for the UDP latency test.
// It sets up a cache for sessions that writes the results to disk on item
// eviction.
func NewHandler(dir string, cacheTTL time.Duration) *Handler {

	cache := ttlcache.New(
		ttlcache.WithTTL[string, *model.Session](cacheTTL),
		ttlcache.WithDisableTouchOnHit[string, *model.Session](),
	)
	cache.OnEviction(func(ctx context.Context,
		er ttlcache.EvictionReason,
		i *ttlcache.Item[string, *model.Session]) {
		log.Debug("Session expired", "id", i.Value().ID, "reason", er)

		// Save data to disk when the session expires.
		archive := i.Value().Archive()
		archive.EndTime = time.Now()
		_, err := persistence.WriteDataFile(dir, "latency1", "application", archive.ID, archive)
		if err != nil {
			log.Error("failed to write latency result", "mid", archive.ID, "error", err)
			return
		}
	})

	go cache.Start()
	return &Handler{
		dataDir:  dir,
		sessions: cache,
	}
}

// Authorize verifies that the request includes a valid JWT, extracts its jti
// and adds a new empty session to the sessions cache.
// It returns a valid kickoff LatencyPacket for this new session in the
// response body.
func (h *Handler) Authorize(rw http.ResponseWriter, req *http.Request) {
	mid, err := handler.GetMIDFromRequest(req)
	if err != nil {
		log.Info("Received request without mid", "source", req.RemoteAddr,
			"error", err)
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Header().Set("Connection", "Close")
		return
	}

	// Create a new session for this mid.
	session := model.NewSession(mid)
	h.sessionsMu.Lock()
	h.sessions.Set(mid, session, ttlcache.DefaultTTL)
	h.sessionsMu.Unlock()

	log.Debug("session created", "id", mid)

	// Create a valid kickoff packet for this session and send it in the
	// response body.
	kickoff := &model.LatencyPacket{
		Type: "c2s",
		ID:   mid,
		Seq:  0,
	}

	b, err := json.Marshal(kickoff)
	// This should never happen.
	rtx.Must(err, "cannot marshal LatencyPacket")

	_, err = rw.Write(b)
	if err != nil {
		// TODO: add Prometheus metric for write errors.
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Header().Set("Connection", "Close")
		return
	}

}

// Result returns a result for a given measurement id. Possible status codes
// are:
// - 400 if the request does not contain a mid
// - 404 if the mid is not found in the sessions cache
// - 500 if the session JSON cannot be marshalled
func (h *Handler) Result(rw http.ResponseWriter, req *http.Request) {
	mid, err := handler.GetMIDFromRequest(req)
	if err != nil {
		log.Info("Received request without mid", "source", req.RemoteAddr,
			"error", err)
		rw.WriteHeader(http.StatusBadRequest)
		rw.Header().Set("Connection", "Close")
		return
	}

	h.sessionsMu.Lock()
	cachedResult := h.sessions.Get(mid)
	h.sessionsMu.Unlock()
	if cachedResult == nil {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	session := cachedResult.Value()
	b, err := json.Marshal(session.Summarize())
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = rw.Write(b)
	if err != nil {
		// TODO: add Prometheus metric for write errors.
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Remove this session from the cache.
	h.sessions.Delete(mid)
}

// sendLoop sends UDP pings with progressive sequence numbers until the context
// expires or is canceled.
func (h *Handler) sendLoop(ctx context.Context, conn net.PacketConn,
	remoteAddr net.Addr, session *model.Session, duration time.Duration) error {
	seq := 0
	var err error

	timeout, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	memoryless.Run(timeout, func() {
		b, marshalErr := json.Marshal(&model.LatencyPacket{
			ID:      session.ID,
			Type:    "s2c",
			Seq:     seq,
			LastRTT: int(session.LastRTT.Load()),
		})

		// This should never happen, since we should always be able to marshal
		// a LatencyPacket struct.
		rtx.Must(marshalErr, "cannot marshal LatencyPacket")

		// Call time.Now() just before writing to the socket. The RTT will
		// include the ping packet's write time. This is intentional.
		sendTime := time.Now()
		// As the kernel's socket buffers are usually much larger than the
		// packets we send here, calling conn.WriteTo is expected to take a
		// negligible time.
		n, writeErr := conn.WriteTo(b, remoteAddr)
		if writeErr != nil {
			err = writeErr
			cancel()
			return
		}
		if n != len(b) {
			err = errors.New("partial write")
			cancel()
			return
		}

		// Update the SendTimes map after a successful write.
		session.SendTimesMu.Lock()
		session.SendTimes = append(session.SendTimes, sendTime)
		session.SendTimesMu.Unlock()

		// Add this packet to the Results slice. Results are "lost" until a
		// reply is received from the server.
		session.RoundTrips = append(session.RoundTrips, model.RoundTrip{
			Lost: true,
		})

		seq++

		log.Debug("packet sent", "len", n, "id", session.ID, "seq", seq)

	}, memoryless.Config{
		// Using randomized intervals allows to detect cyclic network
		// behaviors where a fixed interval could align to the cycle.
		Expected: 25 * time.Millisecond,
		Min:      10 * time.Millisecond,
		Max:      40 * time.Millisecond,
	})
	return err
}

// processPacket processes a single UDP latency packet.
func (h *Handler) processPacket(conn net.PacketConn, remoteAddr net.Addr,
	packet []byte, recvTime time.Time) error {
	// Attempt to unmarshal the packet.
	var m model.LatencyPacket
	err := json.Unmarshal(packet, &m)
	if err != nil {
		return err
	}

	// Check if this is a known session.
	h.sessionsMu.Lock()
	cachedResult := h.sessions.Get(m.ID)
	h.sessionsMu.Unlock()
	if cachedResult == nil {
		return errorUnauthorized
	}

	session := cachedResult.Value()

	// If this message's type is s2c, it was a server ping echoed back by the
	// client. Store it in the session's result and compute the RTT.
	if m.Type == "s2c" {
		session.SendTimesMu.Lock()
		defer session.SendTimesMu.Unlock()
		if m.Seq >= len(session.SendTimes) {
			// TODO: Add Prometheus metric.
			log.Info("received packet with valid mid and invalid seq number",
				"mid", m.ID,
				"seq", m.Seq,
				"addr", remoteAddr.String())
			return errorInvalidSeqN
		}

		rtt := recvTime.Sub(session.SendTimes[m.Seq]).Microseconds()
		session.LastRTT.Store(rtt)
		session.RoundTrips[m.Seq].RTT = int(rtt)
		session.RoundTrips[m.Seq].Lost = false

		log.Debug("received pong, updating result", "mid", session.ID,
			"result", session.RoundTrips[m.Seq])
		// TODO: prometheus metric
		return nil
	}

	// If this message's type is c2s, it's a kickoff packet. Record
	// local/remote addresses and trigger the send loop.
	if m.Type == "c2s" {
		session.StartedMu.Lock()
		defer session.StartedMu.Unlock()
		if !session.Started {
			session.Started = true
			session.Client = remoteAddr.String()
			session.Server = conn.LocalAddr().String()
			go h.sendLoop(context.Background(), conn, remoteAddr, session,
				sendDuration)
		}
	}

	return nil
}

// ProcessPacketLoop is the main packet processing loop. For each incoming
// packet, it records its timestamp and acts depending on the packet type.
func (h *Handler) ProcessPacketLoop(conn net.PacketConn) {
	log.Info("Accepting UDP packets...")
	buf := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Error("error while reading UDP packet", "err", err)
			continue
		}
		// The receive time should be recorded as soon as possible after
		// reading the packet, to improve accuracy.
		recvTime := time.Now()
		log.Debug("received UDP packet", "addr", addr, "n", n, "data", string(buf[:n]))
		err = h.processPacket(conn, addr, buf[:n], recvTime)
		if err != nil {
			log.Debug("failed to process packet",
				"err", err,
				"addr", addr.String())
		}
	}
}
