package results

import (
	"time"

	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
)

// NDTMResult is the struct that is serialized as JSON to disk as the archival
// record of an NDT-M test.
type NDTMResult struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string
	// Version is the symbolic version (if any) of the running server code.
	Version string

	// MeasurementID identifies multiple flows belonging to the same
	// measurement.
	MeasurementID string
	// UUID is the unique ID for this TCP flow.
	UUID string
	// StartTime is the time when the flow started. It does not include the
	// connection setup time.
	StartTime time.Time
	// EndTime is the time when the flow ended.
	EndTime time.Time
	// CongestionControl is the congestion control algorithm used by the flow.
	CongestionControl string
	// SubTest is the subtest of the measurement (download or upload)
	SubTest string
	// ServerMeasurements is a list of measurements taken by the server.
	ServerMeasurements []Measurement
	// ClientMeasurements is a list of measurements taken by the client.
	ClientMeasurements []Measurement
}

// The Measurement struct contains measurement results. This structure is
// meant to be serialised as JSON as sent as a textual message. This
// structure is specified in the ndt7 specification.
type Measurement struct {
	AppInfo        *AppInfo        `json:",omitempty"`
	ConnectionInfo *ConnectionInfo `json:",omitempty" bigquery:"-"`
	BBRInfo        *BBRInfo        `json:",omitempty"`
	TCPInfo        *TCPInfo        `json:",omitempty"`
	Origin         string          `json:",omitempty"`
}

// AppInfo contains an application-level measurement. This structure is
// described in the ndt7 specification.
type AppInfo struct {
	NumBytes    int64
	ElapsedTime int64
}

// ConnectionInfo contains connection info. This structure is described
// in the ndt7 specification.
type ConnectionInfo struct {
	Client string
	Server string
	UUID   string `json:",omitempty"`
	// CC is the congestion algorithm used by the sender of this struct.
	CC string
}

// The BBRInfo struct contains information measured using BBR. This structure is
// an extension to the ndt7 specification. Variables here have the same
// measurement unit that is used by the Linux kernel.
type BBRInfo struct {
	inetdiag.BBRInfo
	ElapsedTime int64
}

// The TCPInfo struct contains information measured using TCP_INFO. This
// structure is described in the ndt7 specification.
type TCPInfo struct {
	tcp.LinuxTCPInfo
	ElapsedTime int64
}
