package model

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/m-lab/go/prometheusx"
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

// ArchivalData is the archival data format for UDP latency measurements.
type ArchivalData struct {
	// GitShortCommit is the Git commit (short form) of the running server code.
	GitShortCommit string
	// Version is the symbolic version (if any) of the running server code.
	Version string
	// ID is the unique identifier for this latency measurement.
	ID string

	// Client is the client's ip:port pair.
	Client string
	// Server is the server's ip:port pair.
	Server string

	// StartTime is the test's start time.
	StartTime time.Time

	// EndTime is the test's end time. Since there is no explicit termination
	// message in the protocol, this is set when the session expires.
	EndTime time.Time

	Results []RTT

	PacketsSent     int
	PacketsReceived int
}

type RTT struct {
	RTT  int
	Lost bool `json:",omitempty"`
}

// Session is the in-memory structure holding information about a UDP latency
// measurement session.
type Session struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time

	Client string
	Server string

	Started   bool
	StartedMu *sync.Mutex

	SendTimes   map[int]time.Time
	SendTimesMu *sync.Mutex

	Results []RTT
	LastRTT *atomic.Int64
}

// Summary is the measurement's summary.
type Summary struct {
	ID              string
	StartTime       time.Time
	Results         []RTT
	PacketsSent     int
	PacketsReceived int
}

// NewSession returns an empty Session with all the fields initialized.
func NewSession(id string) *Session {
	return &Session{
		ID:        id,
		StartTime: time.Now(),

		Started:   false,
		StartedMu: &sync.Mutex{},

		Results: make([]RTT, 0),

		LastRTT: &atomic.Int64{},

		SendTimes:   make(map[int]time.Time),
		SendTimesMu: &sync.Mutex{},
	}
}

// Archive converts this Session to ArchivalData.
func (s *Session) Archive() *ArchivalData {
	return &ArchivalData{
		ID:              s.ID,
		GitShortCommit:  prometheusx.GitShortCommit,
		Version:         "",
		Client:          s.Client,
		Server:          s.Server,
		StartTime:       s.StartTime,
		Results:         s.Results,
		PacketsSent:     len(s.SendTimes),
		PacketsReceived: len(s.Results),
	}
}

// Summarize converts this Session to a Summary.
func (s *Session) Summarize() *Summary {
	return &Summary{
		ID:              s.ID,
		StartTime:       s.StartTime,
		PacketsSent:     len(s.SendTimes),
		PacketsReceived: len(s.Results),
		Results:         s.Results,
	}
}
