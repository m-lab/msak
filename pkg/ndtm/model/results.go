package model

import (
	"time"

	"github.com/m-lab/ndt-server/metadata"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

// NDTMResult is the struct that is serialized as JSON to disk as the archival
// record of an ndt-m test.
type NDTMResult struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string
	// Version is the symbolic version (if any) of the running server code.
	Version string
	// Direction is the test direction (download or upload).
	Direction string
	// MeasurementID is the unique identifier for multiple TCP streams belonging
	// to the same measurement.
	MeasurementID string
	// UUID is the unique identifier for this TCP stream.
	UUID string
	// Server is the server's TCP endpoint (ip:port).
	Server string
	// Client is the client's TCP endpoint (ip:port).
	Client string
	// CCAlgorithm is the Congestion control algorithm used by the sender in
	// this stream.
	CCAlgorithm string
	// StartTime is the time when the stream started. It does not include the
	// connection setup time.
	StartTime time.Time
	// EndTime is the time when the stream ended.
	EndTime time.Time

	// ServerMeasurements is a list of measurements taken by the server.
	ServerMeasurements []Measurement
	// ClientMeasurements is a list of measurements taken by the client.
	ClientMeasurements []Measurement

	// ClientOptions is a name/value pair containing the standard querystring
	// parameters sent by the client and recognized by the server as options.
	ClientOptions []metadata.NameValue

	// ClientMetadata is a name/value pair containing every non-standard
	// querystring parameter sent by the client.
	ClientMetadata []metadata.NameValue
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
type Measurement struct {
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
	TCPInfo *tcp.LinuxTCPInfo `json:",omitempty"`
}

// WireMeasurement is a wrapper for Measurement structs that contains
// information about this TCP stream that does not need to be sent every time.
// Every field except for Measurement is only expected to be non-empty once.
type WireMeasurement struct {
	// CC is the congestion control used by the sender of this WireMeasurement.
	CC string `json:",omitempty"`
	// UUID is the unique identifier for this TCP stream.
	UUID string `json:",omitempty"`
	// Client is the client's TCP endpoint (ip:port).
	Client string `json:",omitempty"`
	// Server is the server's TCP endpoint (ip:port).
	Server string `json:",omitempty"`
	// Measurement is the Measurement struct wrapped by this WireMeasurement.
	Measurement Measurement
}
