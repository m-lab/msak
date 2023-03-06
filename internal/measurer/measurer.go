// The measurer package provides functions to periodically read kernel metrics
// for a given network connection and return them over a channel wrapped in an
// ndt8 Measurement object.
package measurer

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/ndt8/model"
	"github.com/m-lab/msak/pkg/ndt8/spec"
)

type Connection interface {
	UnderlyingConn() net.Conn
}

type ndt8Measurer struct {
	connInfo  netx.ConnInfo
	startTime time.Time

	dstChan chan model.Measurement
}

// Start starts a measurer goroutine that periodically reads the tcp_info and
// bbr_info kernel structs for the connection, if available, and sends them
// wrapped in a Measurement over the returned channel.
//
// The context determines the measurer goroutine's lifetime.
func Start(ctx context.Context, conn Connection) <-chan model.Measurement {
	// Implementation note: this channel must be buffered to account for slow
	// readers. The "typical" reader is an ndt8 send or receive loop, which
	// might be busy with data r/w. The buffer size corresponds to at least 10
	// seconds:
	//
	// 10000ms / 100 ms/snapshot = 100 snapshots
	dst := make(chan model.Measurement, 100)

	connInfo := netx.ToConnInfo(conn.UnderlyingConn())
	m := &ndt8Measurer{
		connInfo:  connInfo,
		dstChan:   dst,
		startTime: time.Now(),
	}
	go m.loop(ctx)
	return dst
}

func (m *ndt8Measurer) loop(ctx context.Context) {
	log.Printf("measurer: start (context %p)", ctx)
	defer log.Printf("measurer: stop (context %p)", ctx)
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

func (m *ndt8Measurer) measure(ctx context.Context) {
	// On non-Linux systems, collecting kernel metrics WILL fail. In that case,
	// we still want to return a (empty) Measurement.

	bbrInfo, tcpInfo, err := m.connInfo.Info()
	if err != nil {
		log.Printf("GetInfo() failed for context %p: %v", ctx, err)
	}

	select {
	case <-ctx.Done():
		// NOTHING
	case m.dstChan <- model.Measurement{
		ElapsedTime: time.Since(m.startTime).Microseconds(),
		BBRInfo:     &bbrInfo,
		TCPInfo: &model.TCPInfo{
			LinuxTCPInfo: tcpInfo,
			ElapsedTime:  time.Since(m.connInfo.AcceptTime()).Microseconds(),
		},
	}:
	}
}
