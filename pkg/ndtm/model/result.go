package model

import (
	"time"
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
	ClientOptions []NameValue

	// ClientMetadata is a name/value pair containing every non-standard
	// querystring parameter sent by the client.
	ClientMetadata []NameValue
}
