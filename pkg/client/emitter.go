package client

import (
	"fmt"

	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

type Emitter interface {
	OnStart(server string, kind spec.SubtestKind)
	OnConnect(server string)
	OnMeasurement(id int, m model.WireMeasurement)
	OnResult(Result)
	OnStreamResult(StreamResult)
	OnError(err error)
	OnComplete(streamID int, server string)
	OnDebug(msg string)
}

type HumanReadable struct {
	Debug bool
}

// OnResult prints the aggregate result.
func (*HumanReadable) OnResult(r Result) {
	fmt.Printf("Elapsed: %.2fs, Goodput: %f, MinRTT: %d\n", r.Elapsed.Seconds(),
		r.Goodput, r.MinRTT)
}

// OnStreamResult prints the per-stream result.
func (*HumanReadable) OnStreamResult(sr StreamResult) {
	fmt.Printf("\tStream #%d - gp %f, tp: %f, minrtt: %d\n", sr.StreamID,
		sr.Goodput, sr.Throughput, sr.MinRTT)
}

// OnStart is called when the stream starts.
func (e *HumanReadable) OnStart(server string, kind spec.SubtestKind) {
	fmt.Printf("Starting %s stream (server: %s)\n", kind, server)
}

// OnConnect is called when the connection to the server is established.
func (e *HumanReadable) OnConnect(server string) {
	fmt.Printf("Connected to %s\n", server)
}

// OnMeasurement is called on received Measurement objects.
func (e *HumanReadable) OnMeasurement(id int, m model.WireMeasurement) {
	// NOTHING - don't print individual measurement objects in this Emitter.
}

// OnError is called on errors.
func (e *HumanReadable) OnError(err error) {
	fmt.Println(err)
}

// OnComplete is called after a stream completes.
func (e *HumanReadable) OnComplete(streamID int, server string) {
	fmt.Printf("Stream %d complete (server %s)\n", streamID, server)
}

// OnDebug is called to print debug information.
func (e *HumanReadable) OnDebug(msg string) {
	if e.Debug {
		fmt.Printf("DEBUG: %s\n", msg)
	}
}

var _ Emitter = &HumanReadable{}
