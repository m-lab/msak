package model

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/msak/pkg/version"
)

// LatencyPacket is the payload of a latency measurement UDP packet.
type LatencyPacket struct {
	// Type is the message type. Possible values are "s2c" and "c2s".
	Type string

	// ID is this latency measurement's unique ID.
	ID string

	// Seq is the progressive sequence number for this measurement.
	Seq int

	// LastRTT is the previous RTT (if any) measured by the party sending this
	// message. When there is no previous RTT, this will be zero.
	LastRTT int `json:",omitempty"`
}

// ArchivalData is the archival data format for latency1 measurements.
type ArchivalData struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string
	// Version is the symbolic version (if any) of the running server code.
	Version string
	// ID is the unique identifier for this latency measurement.
	ID string

	// UUID is the unique identifier of the TCP connection that started this
	// latency measurement.
	UUID string

	// Client is the client's ip:port pair.
	Client string
	// Server is the server's ip:port pair.
	Server string

	// StartTime is the test's start time.
	StartTime time.Time

	// EndTime is the test's end time. Since there is no explicit termination
	// message in the protocol, this is set when the session expires.
	EndTime time.Time

	// RoundTrips is a list of roundtrips.
	RoundTrips []RoundTrip

	// PacketSent is the number of packets sent during this measurement.
	PacketsSent int
	// PacketsReceived is the number of packets received during this
	// measurement.
	PacketsReceived int
}

// RoundTrip is a roundtrip. If the reply was lost, Lost will be true.
// If a reply was received, RTT will be populated with the round-trip time.
type RoundTrip struct {
	// RTT is the round-trip time (microseconds).
	RTT int
	// Lost says if the packet was lost.
	Lost bool `json:",omitempty"`
}

// Session is the in-memory structure holding information about a UDP latency
// measurement session.
type Session struct {
	// UUID is the unique identifier of the TCP connection that started
	// this latency measurement.
	UUID string

	// StartTime is the test's start time.
	StartTime time.Time
	// EndTime is the test's end time.
	EndTime time.Time

	// Client is the client's ip:port pair.
	Client string
	// Server is the server's ip:port pair.
	Server string

	// Started is true if this session's send loop has been started already.
	Started bool
	// StartedMu is the mutex associated to Started.
	StartedMu sync.Mutex

	// SendTimes is a slice of send times. The slice's index is the packet's
	// sequence number.
	SendTimes []time.Time
	// SendTimesMu is a mutex to synchronize access to SendTimes.
	SendTimesMu sync.Mutex

	// RoundTrips is a list of roundtrips.
	RoundTrips []RoundTrip

	// LastRTT contains the last observed RTT.
	LastRTT *atomic.Int64
}

// PacketsReceived returns the number of received packets for this session.
func (s *Session) PacketsReceived() int {
	recv := 0
	for _, v := range s.RoundTrips {
		if !v.Lost {
			recv++
		}
	}
	return recv
}

// Summary is the measurement's summary.
type Summary struct {
	// ID is the unique identifier for this latency measurement.
	ID string
	// StartTime is the test's start time.
	StartTime time.Time
	// RoundTrips is a list of roundtrips.
	RoundTrips []RoundTrip

	// PacketSent is the number of packets sent during this measurement.
	PacketsSent int
	// PacketsReceived is the number of packets received during this
	// measurement.
	PacketsReceived int
}

// NewSession returns an empty Session with all the fields initialized.
func NewSession(uuid string) *Session {
	return &Session{
		UUID:      uuid,
		StartTime: time.Now(),

		Started: false,

		RoundTrips: make([]RoundTrip, 0),

		LastRTT: &atomic.Int64{},

		SendTimes: []time.Time{},
	}
}

// Archive converts this Session to ArchivalData.
func (s *Session) Archive() *ArchivalData {
	return &ArchivalData{
		ID:              s.UUID,
		GitShortCommit:  prometheusx.GitShortCommit,
		Version:         version.Version,
		Client:          s.Client,
		Server:          s.Server,
		StartTime:       s.StartTime,
		RoundTrips:      s.RoundTrips,
		PacketsSent:     len(s.SendTimes),
		PacketsReceived: s.PacketsReceived(),
	}
}

// Summarize converts this Session to a Summary.
func (s *Session) Summarize() *Summary {
	return &Summary{
		ID:              s.UUID,
		StartTime:       s.StartTime,
		PacketsSent:     len(s.SendTimes),
		PacketsReceived: s.PacketsReceived(),
		RoundTrips:      s.RoundTrips,
	}
}
