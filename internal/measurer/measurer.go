// The measurer package provides functions to periodically read kernel metrics
// for a given network connection and return them over a channel wrapped in an
// throughput1 Measurement object.
package measurer

import (
	"context"
	"net"
	"time"

	"github.com/charmbracelet/log"
	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

// Throughput1Measurer tracks state for collecting connection measurements.
type Throughput1Measurer struct {
	connInfo            netx.ConnInfo
	startTime           time.Time
	bytesReadAtStart    int64
	bytesWrittenAtStart int64

	dstChan chan model.Measurement

	// ReadChan is a readable channel for measurements created by the measurer.
	ReadChan <-chan model.Measurement
}

// New creates an empty Throughput1Measurer. The measurer must be started with Start.
func New() *Throughput1Measurer {
	return &Throughput1Measurer{}
}

// Start starts a measurer goroutine that periodically reads the tcp_info and
// bbr_info kernel structs for the connection, if available, and sends them
// wrapped in a Measurement over the returned channel.
//
// The context determines the measurer goroutine's lifetime.
// If passed a connection that is not a netx.Conn, this function will panic.
func (m *Throughput1Measurer) Start(ctx context.Context, conn net.Conn) <-chan model.Measurement {
	// Implementation note: this channel must be buffered to account for slow
	// readers. The "typical" reader is an throughput1 send or receive loop, which
	// might be busy with data r/w. The buffer size corresponds to at least 10
	// seconds:
	//
	// 10000ms / 100 ms/snapshot = 100 snapshots
	dst := make(chan model.Measurement, 100)

	connInfo := netx.ToConnInfo(conn)
	read, written := connInfo.ByteCounters()
	*m = Throughput1Measurer{
		connInfo:  connInfo,
		dstChan:   dst,
		ReadChan:  dst,
		startTime: time.Now(),
		// Byte counters are offset by their initial value, so that the
		// BytesSent/BytesReceived fields represent "application-level bytes
		// sent/received over the connection since the beginning of the
		// measurement" as precisely as possible. Note that this includes the
		// WebSocket framing overhead.
		bytesReadAtStart:    int64(read),
		bytesWrittenAtStart: int64(written),
	}
	go m.loop(ctx)
	return m.ReadChan
}

func (m *Throughput1Measurer) loop(ctx context.Context) {
	log.Debug("Measurer started", "context", ctx)
	defer log.Debug("Measurer stopped", "context", ctx)
	t, err := memoryless.NewTicker(ctx, memoryless.Config{
		Min:      spec.MinMeasureInterval,
		Expected: spec.AvgMeasureInterval,
		Max:      spec.MaxMeasureInterval,
	})
	// This can only error if min/expected/max above are set to invalid
	// values. Since they are constants, we panic here.
	rtx.PanicOnError(err, "ticker creation failed (this should never happen)")
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.measure(ctx)
		}
	}
}

func (m *Throughput1Measurer) measure(ctx context.Context) {
	select {
	case <-ctx.Done():
		// NOTHING
	case m.dstChan <- m.Measure(ctx):
	}
}

// Measure collects metrics about the life of the connection.
func (m *Throughput1Measurer) Measure(ctx context.Context) model.Measurement {
	// On non-Linux systems, collecting kernel metrics WILL fail. In that case,
	// we still want to return a (empty) Measurement.
	bbrInfo, tcpInfo, err := m.connInfo.Info()
	if err != nil {
		log.Warn("GetInfo() failed for context %p: %v", ctx, err)
	}

	// Read current bytes counters.
	totalRead, totalWritten := m.connInfo.ByteCounters()

	return model.Measurement{
		ElapsedTime: time.Since(m.startTime).Microseconds(),
		Network: model.ByteCounters{
			BytesSent:     int64(totalWritten) - m.bytesWrittenAtStart,
			BytesReceived: int64(totalRead) - m.bytesReadAtStart,
		},
		BBRInfo: &bbrInfo,
		TCPInfo: &model.TCPInfo{
			LinuxTCPInfo: tcpInfo,
			ElapsedTime:  time.Since(m.connInfo.AcceptTime()).Microseconds(),
		},
	}
}
