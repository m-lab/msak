package ping1

import "time"

const (
	SecWebSocketProtocol = "net.measurementlab.ping.v1"
	DefaultDuration      = 1 * time.Second
)

type PingMessage struct {
	ns int64
}
