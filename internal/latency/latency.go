package latency

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jellydator/ttlcache/v3"
	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/msak/internal/handler"
	"github.com/m-lab/msak/internal/persistence"
	"github.com/m-lab/msak/pkg/latency/model"
)

const sendDuration = 5 * time.Second

var errorUnauthorized = errors.New("unauthorized")

// Handler is the handler for latency tests.
type Handler struct {
	dataDir  string
	sessions *ttlcache.Cache[string, *model.Session]
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
		_, err := persistence.WriteDataFile(dir, "latency", "", archive.ID, archive)
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
func (h *Handler) Authorize(rw http.ResponseWriter, req *http.Request) {
	mid, err := handler.GetMIDFromRequest(req)
	if err != nil {
		log.Info("Received request without mid", "source", req.RemoteAddr,
			"error", err)
		rw.WriteHeader(http.StatusUnauthorized)
		rw.Header().Set("Connection", "Close")
		return
	}

	h.sessions.Set(mid, model.NewSession(mid), ttlcache.DefaultTTL)

	log.Debug("session created", "id", mid)
	rw.Write([]byte(mid))
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
	// TODO: mfence?
	cachedResult := h.sessions.Get(mid)
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
	rw.Write(b)
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

		if err != nil {
			err = marshalErr
			return
		}
		n, writeErr := conn.WriteTo(b, remoteAddr)
		if err != nil {
			err = writeErr
			return
		}
		if n != len(b) {
			err = errors.New("partial write")
			return
		}
		session.SendTimesMu.Lock()
		defer session.SendTimesMu.Unlock()
		session.SendTimes[seq] = time.Now()
		session.PacketsSent.Add(1)
		seq++

		log.Debug("packet sent", "len", n, "id", session.ID, "seq", seq)

	}, memoryless.Config{
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
	cachedResult := h.sessions.Get(m.ID)
	if cachedResult == nil {
		return errorUnauthorized
	}

	session := cachedResult.Value()

	// If this message's type is s2c, it was a server ping echoed back by the
	// client. Store it in the session's result and compute the RTT.
	if m.Type == "s2c" {
		session.SendTimesMu.Lock()
		defer session.SendTimesMu.Unlock()
		if sendTime, ok := session.SendTimes[m.Seq]; ok {
			session.LastRTT.Store(int64(recvTime.Sub(sendTime).Microseconds()))
			log.Debug("updating lastrtt", "seq", m.Seq, "rtt", session.LastRTT)
		}
		// TODO: prometheus metric?
		session.PacketsReceived.Add(1)
		session.Packets = append(
			session.Packets, m)
		return nil
	}

	// If this message's type is c2s, trigger the send loop.
	if m.Type == "c2s" {
		session.StartedMu.Lock()
		defer session.StartedMu.Unlock()
		if !session.Started {
			session.Started = true
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
		log.Debug("received UDP packet", "addr", addr, "n", n, "data", string(buf[:n]))
		// The receive time should be recorded as soon as possible after
		// reading the packet, to improve accuracy.
		recvTime := time.Now()
		err = h.processPacket(conn, addr, buf[:n], recvTime)
		if err != nil {
			log.Debug("failed to process packet", "err", err)
		}
	}
}
