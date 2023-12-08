package client

import (
	"time"
)

// Config is the configuration for a Client.
type Config struct {
	// Server is the server to connect to. If empty, the server is obtained by
	// querying the configured Locator.
	Server string

	// Scheme is the WebSocket scheme used to connect to the server (ws or wss).
	Scheme string

	// NumStreams is the number of streams that will be spawned by this client to run a
	// download or an upload test.
	NumStreams int

	// Length is the duration of the test.
	Length time.Duration

	// Delay is the delay between each stream.
	Delay time.Duration

	// CongestionControl is the congestion control algorithm to request from the server.
	CongestionControl string

	// MeasurementID is the manually configured Measurement ID ("mid") to pass to the server.
	MeasurementID string

	// Emitter is the interface used to emit the results of the test. It can be overridden
	// to provide a custom output.
	Emitter Emitter

	// NoVerify disables the TLS certificate verification.
	NoVerify bool

	// BytesLimit is the maximum number of bytes to download or upload. If set to 0, the
	// limit is disabled.
	BytesLimit int
}
