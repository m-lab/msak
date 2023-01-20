package results

import (
	"time"

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
	// Direction is the test direction (download or upload)
	Direction string
	// MeasurementID is the unique identifier for multiple TCP flows belonging
	// to the same measurement.
	MeasurementID string
	// UUID is the unique ID for this TCP flow.
	UUID string
	// Server is the server's TCP endpoint (ip:port).
	Server string
	// Client is the client's TCP endpoint (ip:port).
	Client string
	// CCAlgorithm is the Congestion control algorithm used by the sender in
	// this flow.
	CCAlgorithm string
	// StartTime is the time when the flow started. It does not include the
	// connection setup time.
	StartTime time.Time
	// EndTime is the time when the flow ended.
	EndTime time.Time

	// ServerMeasurements is a list of measurements taken by the server.
	ServerMeasurements []Measurement
	// ClientMeasurements is a list of measurements taken by the client.
	ClientMeasurements []Measurement

	// ClientOptions is a map containing the standard querystring parameters
	// sent by the client and recognized by the server as options.
	ClientOptions map[string]string

	// ClientMetadata is a map containing every non-standard querystring
	// parameter sent by the client.
	ClientMetadata map[string]string
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
type Measurement struct {
	// BytesRecv is the number of bytes received at the application level
	// by the party sending this Measurement.
	BytesRecv int64 `json:",omitempty"`

	// ElapsedTime is the time elapsed since the start of the measurement
	// according to the party sending this Measurement.
	ElapsedTime int64 `json:",omitempty"`

	// Origin is the origin of this Measurement ("sender" or "receiver")
	Origin string `json:",omitempty"`

	// BBRInfo is an optional struct containing BBR metrics. Only applicable
	// when the congestion control algorithm used by the party sending this
	// Measurement is BBR.
	BBRInfo *inetdiag.BBRInfo `json:",omitempty"`

	// TCPInfo is an optional struct containing some of the TCP_INFO kernel
	// metrics for this TCP flow. Only applicable when the party sending this
	// Measurement has access to it.
	TCPInfo *tcp.LinuxTCPInfo `json:",omitempty"`
}

// WireMeasurement is a wrapper for Measurement structs that contains
// information about this TCP flow that does not need to be sent every time.
// Every field except for Measurement is only expected to be filled once.
type WireMeasurement struct {
	UUID string `json:",omitempty"`

	Client string `json:",omitempty"`
	Server string `json:",omitempty"`

	Measurement Measurement
}
