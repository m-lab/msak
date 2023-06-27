package throughput1

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/m-lab/msak/internal/measurer"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

type senderFunc func(ctx context.Context,
	measurerCh <-chan model.Measurement, results chan<- model.WireMeasurement,
	errCh chan<- error)

type Measurer interface {
	Start(context.Context, net.Conn) <-chan model.Measurement
}

// DefaultMeasurer is the default throughput1 measurer that wraps the measurer
// package's Start function.
type DefaultMeasurer struct{}

func (*DefaultMeasurer) Start(ctx context.Context,
	c net.Conn) <-chan model.Measurement {
	return measurer.Start(ctx, c)
}

// Protocol is the implementation of the throughput1 protocol.
type Protocol struct {
	conn     *websocket.Conn
	connInfo netx.ConnInfo
	rnd      *rand.Rand
	measurer Measurer
	once     *sync.Once
}

// New returns a new Protocol with the specified connection and every other
// option set to default.
func New(conn *websocket.Conn) *Protocol {
	return &Protocol{
		conn:     conn,
		connInfo: netx.ToConnInfo(conn.UnderlyingConn()),
		// Seed randomness source with the current time.
		rnd:      rand.New(rand.NewSource(time.Now().UnixMilli())),
		measurer: &DefaultMeasurer{},
		once:     &sync.Once{},
	}
}

// Upgrade takes a HTTP request and upgrades the connection to WebSocket.
// Returns a websocket Conn if the upgrade succeeded, and an error otherwise.
func Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// We expect WebSocket's subprotocol to be throughput1's. The same subprotocol is
	// added as a header on the response.
	if r.Header.Get("Sec-WebSocket-Protocol") != spec.SecWebSocketProtocol {
		w.WriteHeader(http.StatusBadRequest)
		return nil, errors.New("missing Sec-WebSocket-Protocol header")
	}
	h := http.Header{}
	h.Add("Sec-WebSocket-Protocol", spec.SecWebSocketProtocol)
	u := websocket.Upgrader{
		// Allow cross-origin resource sharing.
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		// Set r/w buffers to the maximum expected message size.
		ReadBufferSize:  spec.MaxScaledMessageSize,
		WriteBufferSize: spec.MaxScaledMessageSize,
	}
	return u.Upgrade(w, r, h)
}

// makePreparedMessage returns a websocket.PreparedMessage of the requested
// size filled with random bytes read from the Protocol's randomness source.
func (p *Protocol) makePreparedMessage(size int) (*websocket.PreparedMessage, error) {
	data := make([]byte, size)
	// Each Protocol has its own instance of Rand, so simultaneous calls to
	// Read() should never happen.
	p.rnd.Read(data)
	return websocket.NewPreparedMessage(websocket.BinaryMessage, data)
}

// SenderLoop starts the send loop of the throughput1 protocol. The context's lifetime
// determines how long to run for. It returns one channel for sender-side
// measurements, one channel for receiver-side measurements and one channel for
// errors. While the measurements channels could be ignored, the errors channel
// MUST be drained by the caller.
func (p *Protocol) SenderLoop(ctx context.Context) (<-chan model.WireMeasurement,
	<-chan model.WireMeasurement, <-chan error) {
	return p.senderReceiverLoop(ctx, p.sender)
}

// ReceiverLoop starts the receiver loop of the throughput1 protocol. The context's
// lifetime determines how long to run for. It returns one channel for
// sender-side measurements, one channel for receiver-side measurements and one
// channel for errors. While the measurements channels could be ignored, the
// errors channel MUST be drained by the caller.
func (p *Protocol) ReceiverLoop(ctx context.Context) (<-chan model.WireMeasurement,
	<-chan model.WireMeasurement, <-chan error) {
	return p.senderReceiverLoop(ctx, p.sendCounterflow)
}

func (p *Protocol) senderReceiverLoop(ctx context.Context,
	send senderFunc) (<-chan model.WireMeasurement,
	<-chan model.WireMeasurement, <-chan error) {
	// In no case this method will send for longer than spec.MaxRuntime.
	// Context cancelation will normally happen sooner than that.
	deadline := time.Now().Add(spec.MaxRuntime)
	p.conn.SetWriteDeadline(deadline)
	p.conn.SetReadDeadline(deadline)

	// Start a measurer that will periodically send measurements over
	// measurerCh. These measurements are passed to the sender or the
	// sendCounterflow goroutines so they can be sent to the other party.
	measurerCh := p.measurer.Start(ctx, p.conn.UnderlyingConn())

	// Separate sender and receiver channels are used for the sender and
	// receiver goroutines. This allows the caller to know where the
	// WireMeasurement came from.
	senderCh := make(chan model.WireMeasurement, 100)
	receiverCh := make(chan model.WireMeasurement, 100)
	errCh := make(chan error, 2)

	go p.receiver(ctx, receiverCh, errCh)
	go send(ctx, measurerCh, senderCh, errCh)
	return senderCh, receiverCh, errCh
}

// receiver reads from the connection until NextReader fails. It returns
// the measurements received over the provided channel.
func (p *Protocol) receiver(ctx context.Context,
	results chan<- model.WireMeasurement, errCh chan<- error) {
	for {
		kind, reader, err := p.conn.NextReader()
		if err != nil {
			errCh <- err
			return
		}
		if kind == websocket.TextMessage {
			data, err := io.ReadAll(reader)
			if err != nil {
				errCh <- err
				return
			}
			var m model.WireMeasurement
			if err := json.Unmarshal(data, &m); err != nil {
				errCh <- err
				return
			}
			results <- m
		}
	}
}

func (p *Protocol) sendCounterflow(ctx context.Context,
	measurerCh <-chan model.Measurement, results chan<- model.WireMeasurement,
	errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			p.close(ctx)
			return
		case m := <-measurerCh:
			wm := model.WireMeasurement{}
			p.once.Do(func() {
				wm = p.createWireMeasurement(ctx)
			})
			wm.Measurement = m
			err := p.conn.WriteJSON(wm)
			if err != nil {
				log.Printf("failed to write measurement JSON (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}
			// This send is non-blocking in case there is no one to read the
			// Measurement message and the channel's buffer is full.
			select {
			case results <- wm:
			default:
			}
		}
	}
}

func (p *Protocol) sender(ctx context.Context, measurerCh <-chan model.Measurement,
	results chan<- model.WireMeasurement, errCh chan<- error) {
	ci := netx.ToConnInfo(p.conn.UnderlyingConn())
	size := spec.MinMessageSize
	message, err := p.makePreparedMessage(size)
	if err != nil {
		log.Printf("makePreparedMessage failed (ctx: %p)", ctx)
		errCh <- err
		return
	}

	// Prepared (binary) messages and Measurement messages are written to the
	// same socket. This means the speed at which we can send measurements is
	// limited by how long it takes to send a prepared message, since they
	// can't be written simultaneously.
	for {
		select {
		case <-ctx.Done():
			p.close(ctx)
			return
		case m := <-measurerCh:
			wm := model.WireMeasurement{}
			p.once.Do(func() {
				wm = p.createWireMeasurement(ctx)
			})
			wm.Measurement = m
			err = p.conn.WriteJSON(wm)
			if err != nil {
				log.Printf("failed to write measurement JSON (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}
			// This send is non-blocking in case there is no one to read the
			// Measurement message and the channel's buffer is full.
			select {
			case results <- wm:
			default:
			}
		default:
			err = p.conn.WritePreparedMessage(message)
			if err != nil {
				log.Printf("failed to write prepared message (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}

			// Determine whether it's time to scale the message size.
			if size >= spec.MaxScaledMessageSize {
				continue
			}

			_, w := ci.ByteCounters()
			if uint64(size) > w/spec.ScalingFraction {
				continue
			}

			size *= 2
			message, err = p.makePreparedMessage(size)
			if err != nil {
				log.Printf("failed to make prepared message (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}

		}
	}
}

func (p *Protocol) close(ctx context.Context) {
	msg := websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, "Done sending")
	err := p.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	if err != nil {
		log.Printf("WriteControl failed (ctx: %p, err: %v)", ctx, err)
		return
	}
	log.Printf("Close message sent (ctx: %p)", ctx)
}

// createWireMeasurement returns an WireMeasurement populated with this
// protocol's connection's information.
func (p *Protocol) createWireMeasurement(ctx context.Context) model.WireMeasurement {
	wm := model.WireMeasurement{
		LocalAddr:  p.conn.LocalAddr().String(),
		RemoteAddr: p.conn.RemoteAddr().String(),
	}
	// When GetCC fails it returns an empty string. This failure is expected on
	// Windows systems and should not be considered fatal.
	cc, err := p.connInfo.GetCC()
	if err != nil {
		log.Printf("failed to read cc (ctx %p): %v\n",
			ctx, err)
	}
	uuid, err := p.connInfo.UUID()
	if err != nil {
		log.Printf("failed to get UUID (ctx %p): %v\n", ctx, err)
	}
	wm.CC = cc
	wm.UUID = uuid
	return wm
}