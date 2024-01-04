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
	"sync/atomic"
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

// Measurer is an interface for collecting connection metrics.
type Measurer interface {
	Start(context.Context, net.Conn) <-chan model.Measurement
	Measure(ctx context.Context) model.Measurement
}

// Protocol is the implementation of the throughput1 protocol.
type Protocol struct {
	conn     *websocket.Conn
	connInfo netx.ConnInfo
	rnd      *rand.Rand
	measurer Measurer
	once     sync.Once

	applicationBytesReceived atomic.Int64
	applicationBytesSent     atomic.Int64

	byteLimit int
}

// New returns a new Protocol with the specified connection and every other
// option set to default.
func New(conn *websocket.Conn) *Protocol {
	return &Protocol{
		conn:     conn,
		connInfo: netx.ToConnInfo(conn.UnderlyingConn()),
		// Seed randomness source with the current time.
		rnd:      rand.New(rand.NewSource(time.Now().UnixMilli())),
		measurer: measurer.New(),
	}
}

// SetByteLimit sets the number of bytes sent after which a test (either download or upload) will stop.
// Set the value to zero to disable the byte limit.
func (p *Protocol) SetByteLimit(value int) {
	p.byteLimit = value
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
// the measurements received over the provided channel and updates the sent and
// received byte counters as needed.
func (p *Protocol) receiver(ctx context.Context,
	results chan<- model.WireMeasurement, errCh chan<- error) {
	for {
		kind, reader, err := p.conn.NextReader()
		if err != nil {
			errCh <- err
			return
		}
		if kind == websocket.BinaryMessage {
			// Binary messages are discarded after reading their size.
			size, err := io.Copy(io.Discard, reader)
			if err != nil {
				errCh <- err
				return
			}
			p.applicationBytesReceived.Add(size)
		}
		if kind == websocket.TextMessage {
			data, err := io.ReadAll(reader)
			if err != nil {
				errCh <- err
				return
			}
			p.applicationBytesReceived.Add(int64(len(data)))
			var m model.WireMeasurement
			if err := json.Unmarshal(data, &m); err != nil {
				errCh <- err
				return
			}
			results <- m
		}
	}
}

func (p *Protocol) sendWireMeasurement(ctx context.Context, m model.Measurement) (*model.WireMeasurement, error) {
	wm := model.WireMeasurement{}
	p.once.Do(func() {
		wm = p.createWireMeasurement(ctx)
	})
	wm.Measurement = m
	wm.Application = model.ByteCounters{
		BytesSent:     p.applicationBytesSent.Load(),
		BytesReceived: p.applicationBytesReceived.Load(),
	}
	// Encode as JSON separately so we can read the message size before
	// sending.
	jsonwm, err := json.Marshal(wm)
	if err != nil {
		log.Printf("failed to encode measurement (ctx: %p, err: %v)", ctx, err)
		return nil, err
	}
	err = p.conn.WriteMessage(websocket.TextMessage, jsonwm)
	if err != nil {
		log.Printf("failed to write measurement JSON (ctx: %p, err: %v)", ctx, err)
		return nil, err
	}
	p.applicationBytesSent.Add(int64(len(jsonwm)))
	return &wm, nil
}

func (p *Protocol) sendCounterflow(ctx context.Context,
	measurerCh <-chan model.Measurement, results chan<- model.WireMeasurement,
	errCh chan<- error) {
	byteLimit := int64(p.byteLimit)
	for {
		select {
		case <-ctx.Done():
			// Attempt to send final write message before close. Ignore errors.
			p.sendWireMeasurement(ctx, p.measurer.Measure(ctx))
			p.close(ctx)
			return
		case m := <-measurerCh:
			wm, err := p.sendWireMeasurement(ctx, m)
			if err != nil {
				errCh <- err
				return
			}
			// This send is non-blocking in case there is no one to read the
			// Measurement message and the channel's buffer is full.
			select {
			case results <- *wm:
			default:
			}

			// End the test once enough bytes have been received.
			if byteLimit > 0 && m.TCPInfo != nil && m.TCPInfo.BytesReceived >= byteLimit {
				// WireMessage was just sent above, so we do not need to send another.
				p.close(ctx)
				return
			}
		}
	}
}

func (p *Protocol) sender(ctx context.Context, measurerCh <-chan model.Measurement,
	results chan<- model.WireMeasurement, errCh chan<- error) {
	size := p.ScaleMessage(spec.MinMessageSize, 0)
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
			// Attempt to send final write message before close. Ignore errors.
			p.sendWireMeasurement(ctx, p.measurer.Measure(ctx))
			p.close(ctx)
			return
		case m := <-measurerCh:
			wm, err := p.sendWireMeasurement(ctx, m)
			if err != nil {
				errCh <- err
				return
			}

			// This send is non-blocking in case there is no one to read the
			// Measurement message and the channel's buffer is full.
			select {
			case results <- *wm:
			default:
			}
		default:
			err = p.conn.WritePreparedMessage(message)
			if err != nil {
				log.Printf("failed to write prepared message (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}
			p.applicationBytesSent.Add(int64(size))

			bytesSent := int(p.applicationBytesSent.Load())
			if p.byteLimit > 0 && bytesSent >= p.byteLimit {
				_, err := p.sendWireMeasurement(ctx, p.measurer.Measure(ctx))
				if err != nil {
					errCh <- err
					return
				}
				p.close(ctx)
				return
			}

			// Determine whether it's time to scale the message size.
			if size >= spec.MaxScaledMessageSize || size > bytesSent/spec.ScalingFraction {
				size = p.ScaleMessage(size, bytesSent)
				continue
			}

			size = p.ScaleMessage(size*2, bytesSent)
			message, err = p.makePreparedMessage(size)
			if err != nil {
				log.Printf("failed to make prepared message (ctx: %p, err: %v)", ctx, err)
				errCh <- err
				return
			}
		}
	}
}

// ScaleMessage sets the binary message size taking into consideration byte limits.
func (p *Protocol) ScaleMessage(msgSize int, bytesSent int) int {
	// Check if the next payload size will push the total number of bytes over the limit.
	excess := bytesSent + msgSize - p.byteLimit
	if p.byteLimit > 0 && excess > 0 {
		msgSize -= excess
	}
	return msgSize
}

func (p *Protocol) close(ctx context.Context) {
	msg := websocket.FormatCloseMessage(
		websocket.CloseNormalClosure, "Done sending")

	err := p.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	if err != nil {
		log.Printf("WriteControl failed (ctx: %p, err: %v)", ctx, err)
		return
	}
	// The closing message is part of the measurement and added to bytesSent.
	p.applicationBytesSent.Add(int64(len(msg)))

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
	uuid := p.connInfo.UUID()
	wm.CC = cc
	wm.UUID = uuid
	return wm
}
