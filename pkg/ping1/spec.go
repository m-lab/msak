package ping1

import "time"

const (
	SecWebSocketProtocol = "net.measurementlab.ping.v1"
	DefaultDuration      = 1 * time.Second
)

type PingMessage struct {
	// NS is the time (nanoseconds) when this PingMessage was created.
	NS int64
}

type ResultMessage struct {
	// RTTs is the list of collected RTTs in microseconds.
	RTTs []int64
}
