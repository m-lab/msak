package measurer

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"time"

	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/congestion"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/ndt8/model"
	"github.com/m-lab/msak/pkg/ndt8/spec"
	"github.com/m-lab/ndt-server/tcpinfox"
)

type Connection interface {
	UnderlyingConn() net.Conn
}

type ndt8measurer struct {
	fp        *os.File
	ticker    *memoryless.Ticker
	startTime time.Time

	dstChan chan model.Measurement
}

// Start starts a measurer goroutine that periodically reads the tcp_info and
// bbr_info kernel structs for the connection, if available, and sends them
// wrapped in a Measurement over the returned channel.
//
// It returns an error if the file pointer associated with the connection
// cannot be obtained.
//
// The context determines the measurer goroutine's lifetime.
func Start(ctx context.Context, conn Connection) (<-chan model.Measurement, error) {
	// Implementation note: this channel must be buffered to account for slow
	// readers. The "typical" reader is an ndt8 send or receive loop, which
	// might be busy with data r/w. The buffer size corresponds to at least 10
	// seconds:
	//
	// 10000ms / 100 ms/snapshot = 100 snapshots
	dst := make(chan model.Measurement, 100)

	t, err := memoryless.NewTicker(ctx, memoryless.Config{
		Min:      spec.MinMeasureInterval,
		Expected: spec.AvgMeasureInterval,
		Max:      spec.MaxMeasureInterval,
	})
	// This can only error if min/expected/max above are set to invalid
	// values. Since they are constants, we panic here.
	rtx.PanicOnError(err, "ticker creation failed (this should never happen)")

	fp, err := netx.GetFile(conn.UnderlyingConn())
	if err != nil {
		return nil, err
	}
	m := &ndt8measurer{
		fp:      fp,
		ticker:  t,
		dstChan: dst,
	}

	go func() {
		m.startTime = time.Now()
		m.loop(ctx)
	}()
	return dst, nil
}

func (m *ndt8measurer) stop() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	for range m.dstChan {
		// NOTHING - just drain the channel if needed.
	}
}

func (m *ndt8measurer) loop(ctx context.Context) {
	log.Println("measurer: start")
	defer log.Println("measurer: stop")
	for {
		select {
		case <-ctx.Done():
			m.stop()
			return
		case <-m.ticker.C:
			m.measure(ctx)
		}
	}
}

func (m *ndt8measurer) measure(ctx context.Context) {
	// Note: this is expected to fail if the TCP flow does not use BBR.
	bbrInfo, _ := congestion.GetBBRInfo(m.fp)
	tcpInfo, err := tcpinfox.GetTCPInfo(m.fp)
	if err != nil && !errors.Is(err, tcpinfox.ErrNoSupport) {
		log.Printf("cannot get tcpInfo for fp %v\n", m.fp)
	}

	select {
	case <-ctx.Done():
		// NOTHING
	case m.dstChan <- model.Measurement{
		ElapsedTime: time.Since(m.startTime).Microseconds(),
		BBRInfo:     &bbrInfo,
		TCPInfo:     tcpInfo,
	}:

	}
}
