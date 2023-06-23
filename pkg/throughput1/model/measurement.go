package model

import (
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

// WireMeasurement is a wrapper for Measurement structs that contains
// information about this TCP stream that does not need to be sent every time.
// Every field except for Measurement is only expected to be non-empty once.
type WireMeasurement struct {
	// CC is the congestion control used by the sender of this WireMeasurement.
	CC string `json:",omitempty"`
	// UUID is the unique identifier for this TCP stream.
	UUID string `json:",omitempty"`
	// LocalAddr is the local TCP endpoint (ip:port).
	LocalAddr string `json:",omitempty"`
	// RemoteAddr is the server's TCP endpoint (ip:port).
	RemoteAddr string `json:",omitempty"`
	// Measurement is the Measurement struct wrapped by this WireMeasurement.
	Measurement
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON and sent as a textual message.
type Measurement struct {
	// BytesSent is the number of bytes sent at the application level by the
	// party sending this Measurement.
	BytesSent int64 `json:",omitempty"`

	// BytesReceived is the number of bytes received at the application level
	// by the party sending this Measurement.
	BytesReceived int64 `json:",omitempty"`

	// ElapsedTime is the time elapsed since the start of the measurement
	// according to the party sending this Measurement.
	ElapsedTime int64 `json:",omitempty"`

	// BBRInfo is an optional struct containing BBR metrics. Only applicable
	// when the congestion control algorithm used by the party sending this
	// Measurement is BBR.
	BBRInfo *inetdiag.BBRInfo `json:",omitempty"`

	// TCPInfo is an optional struct containing some of the TCP_INFO kernel
	// metrics for this TCP stream. Only applicable when the party sending this
	// Measurement has access to it.
	TCPInfo *TCPInfo `json:",omitempty"`
}

// TCPInfo is an extension to Linux's TCPInfo struct that includes the time
// elapsed since the connection was accepted.
type TCPInfo struct {
	tcp.LinuxTCPInfo
	ElapsedTime int64
}
