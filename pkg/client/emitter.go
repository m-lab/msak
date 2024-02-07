package client

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

// Emitter is an interface for emitting results.
type Emitter interface {
	// OnStart is called when a stream starts.
	OnStart(server string, kind spec.SubtestKind)
	// OnConnect is called when the WebSocket connection is established.
	OnConnect(server string)
	// OnMeasurement is called on received Measurement objects.
	OnMeasurement(id int, m model.WireMeasurement)
	// OnResult is called when the aggregate result is ready.
	OnResult(Result)
	// OnError is called on errors.
	OnError(err error)
	// OnStreamComplete is called after a stream completes.
	OnStreamComplete(streamID int, server string)
	// OnDebug is called to print debug information.
	OnDebug(msg string)
	// OnSummary is called to print summary information.
	OnSummary(results map[spec.SubtestKind]Result)
}

// HumanReadable prints human-readable output to stdout.
// It can be configured to include debug output, too.
type HumanReadable struct {
	Debug bool
}

// OnResult prints the aggregate result.
func (HumanReadable) OnResult(r Result) {
	fmt.Printf("%s rate: %f Mb/s, rtt: %.2f, minrtt: %.2f\n",
		r.Subtest, r.Goodput/1e6, float32(r.RTT)/1000, float32(r.MinRTT)/1000)
}

// OnStart is called when the stream starts and prints the subtest and server hostname.
func (HumanReadable) OnStart(server string, kind spec.SubtestKind) {
	fmt.Printf("Starting %s stream (server: %s)\n", kind, server)
}

// OnConnect is called when the connection to the server is established.
func (HumanReadable) OnConnect(server string) {
	fmt.Printf("Connected to %s\n", server)
}

// OnMeasurement is called on received Measurement objects.
func (HumanReadable) OnMeasurement(id int, m model.WireMeasurement) {
	// NOTHING - don't print individual measurement objects in this Emitter.
}

// OnError is called on errors.
func (HumanReadable) OnError(err error) {
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		fmt.Println(err)
	}
}

// OnStreamComplete is called after a stream completes.
func (HumanReadable) OnStreamComplete(streamID int, server string) {
	fmt.Printf("Stream %d complete (server %s)\n", streamID, server)
}

func (HumanReadable) OnSummary(results map[spec.SubtestKind]Result) {
	fmt.Println()
	fmt.Printf("Test results:\n")
	for kind, result := range results {
		fmt.Printf("  %s rate: %.2f Mb/s, rtt: %.2fms, minrtt: %.2fms\n",
			kind, result.Goodput/1e6, float32(result.RTT)/1000, float32(result.MinRTT)/1000)
		fmt.Printf("    streams: %d, duration: %.2fs, cc algo: %s, byte limit: %d bytes\n",
			result.Streams, result.Length.Seconds(), result.CongestionControl, result.ByteLimit)
	}
}

// OnDebug is called to print debug information.
func (e HumanReadable) OnDebug(msg string) {
	if e.Debug {
		fmt.Printf("DEBUG: %s\n", msg)
	}
}

// Checks that HumanReadable implements Emitter.
var _ Emitter = &HumanReadable{}
