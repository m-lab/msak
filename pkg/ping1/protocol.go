package ping1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/rtx"
)

type Protocol struct {
	conn *websocket.Conn
}

func New(conn *websocket.Conn) *Protocol {
	return &Protocol{
		conn: conn,
	}
}

// Upgrade takes a HTTP request and upgrades the connection to WebSocket.
// Returns a websocket Conn if the upgrade succeeded, and an error otherwise.
func Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// We expect WebSocket's subprotocol to be throughput1's. The same subprotocol is
	// added as a header on the response.
	if r.Header.Get("Sec-WebSocket-Protocol") != SecWebSocketProtocol {
		w.WriteHeader(http.StatusBadRequest)
		return nil, errors.New("missing Sec-WebSocket-Protocol header")
	}
	h := http.Header{}
	h.Add("Sec-WebSocket-Protocol", SecWebSocketProtocol)
	u := websocket.Upgrader{
		// Allow cross-origin resource sharing.
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	return u.Upgrade(w, r, h)
}

func (p *Protocol) Start(ctx context.Context) {
	deadline, _ := ctx.Deadline()

	t, err := memoryless.NewTicker(ctx, memoryless.Config{
		Expected: 100 * time.Millisecond,
		Min:      30 * time.Millisecond,
		Max:      300 * time.Millisecond,
	})

	rtx.Must(err, "invalid configuration for memoryless.Ticker")

	// Test start time. All time differences are based on this value.
	start := time.Now()

	p.conn.SetPongHandler(func(appData string) error {
		_, rtt, err := ParseTicks(appData, start)
		if err != nil {
			log.Error("failed to parse PONG message: %s", err)
			return err
		}
		log.Info("RTT: %d", rtt)

		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.sendTicks(start, deadline)
		}
	}
}

func (p *Protocol) receiver(ctx context.Context) {
	for {
		_, _, err := p.conn.NextReader()
		if err != nil {
			return
		}
	}
}

func (p *Protocol) sendTicks(start time.Time, deadline time.Time) error {
	msg := PingMessage{
		ns: time.Since(start).Nanoseconds(),
	}

	data, err := json.Marshal(msg)
	if err == nil {
		err = p.conn.WriteControl(websocket.PingMessage, data, deadline)
	}

	return err
}

func ParseTicks(s string, start time.Time) (elapsed time.Duration, d time.Duration, err error) {
	elapsed = time.Since(start)
	var msg PingMessage
	err = json.Unmarshal([]byte(s), &msg)
	if err != nil {
		return
	}
	prev := msg.ns
	if 0 <= prev && prev <= elapsed.Nanoseconds() {
		d = time.Duration(elapsed.Nanoseconds() - prev)
	} else {
		err = errors.New("RTT is negative")
	}
	return
}
