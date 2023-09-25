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
	OnComplete(server string)
	OnDebug(msg string)
}

type HumanReadable struct {
	debug bool
}

// OnResult implements Emitter.
func (*HumanReadable) OnResult(r Result) {
	fmt.Printf("Elapsed: %.2fs, Goodput: %f, MinRTT: %d\n", r.Elapsed.Seconds(),
		r.Goodput, r.MinRTT)
}

// OnStreamResult implements Emitter.
func (*HumanReadable) OnStreamResult(sr StreamResult) {
	fmt.Printf("\tStream #%d - gp %f, tp: %f, minrtt: %d\n", sr.StreamID,
		sr.Goodput, sr.Throughput, sr.MinRTT)
}

func (e *HumanReadable) OnStart(server string, kind spec.SubtestKind) {
	fmt.Printf("Starting %s stream (server: %s)\n", kind, server)
}

func (e *HumanReadable) OnConnect(server string) {
	fmt.Printf("Connected to %s\n", server)
}

func (e *HumanReadable) OnMeasurement(id int, m model.WireMeasurement) {
	// NOTHING - don't print individual measurement objects.
}

func (e *HumanReadable) OnError(err error) {
	fmt.Println(err)
}

func (e *HumanReadable) OnComplete(server string) {
	fmt.Printf("Stream complete (server %s)\n", server)
}

func (e *HumanReadable) OnDebug(msg string) {
	if e.debug {
		fmt.Printf("DEBUG: %s\n", msg)
	}
}

var _ Emitter = &HumanReadable{}
