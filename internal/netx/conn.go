package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	guuid "github.com/google/uuid"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/congestion"
	"github.com/m-lab/ndt-server/tcpinfox"
	"github.com/m-lab/tcp-info/inetdiag"
	"github.com/m-lab/tcp-info/tcp"
	"github.com/m-lab/uuid"
)

// ConnInfo provides operations on a net.Conn's underlying file descriptor.
type ConnInfo interface {
	ByteCounters() (uint64, uint64)
	Info() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error)
	AcceptTime() time.Time
	UUID() (string, error)
	GetCC() (string, error)
	SetCC(string) error
}

// ToConnInfo is a helper function to convert a net.Conn into a netx.ConnInfo.
// It panics if netConn does not contain a type supporting ConnInfo.
func ToConnInfo(netConn net.Conn) ConnInfo {
	switch t := netConn.(type) {
	case *Conn:
		return t
	case *tls.Conn:
		return t.NetConn().(*Conn)
	default:
		panic(fmt.Sprintf("unsupported connection type: %T", t))
	}
}

// Conn is an extended net.Conn that stores its accept time, a copy of the
// underlying socket's file descriptor, and counters for read/written bytes.
type Conn struct {
	net.Conn

	fp           *os.File
	acceptTime   time.Time
	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
}

func FromTCPConn(tcpConn *net.TCPConn) (*Conn, error) {
	return fromTCPConn(tcpConn)
}

// Read reads from the underlying net.Conn and updates the read bytes counter.
func (c *Conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.bytesRead.Add(uint64(n))
	return n, err
}

// Write writes to the underlying net.Conn and updates the written bytes counter.
func (c *Conn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.bytesWritten.Add(uint64(n))
	return n, err
}

// ByteCounters returns the read and written byte counters, in this order.
func (c *Conn) ByteCounters() (uint64, uint64) {
	return c.bytesRead.Load(), c.bytesWritten.Load()
}

// Close closes the underlying net.Conn and the duplicate file descriptor.
func (c *Conn) Close() error {
	return c.close()
}

// SetCC sets the congestion control algorithm on the underlying file
// descriptor.
func (c *Conn) SetCC(cc string) error {
	return congestion.Set(c.fp, cc)
}

// GetCC gets the current congestion control algorithm from the underlying
// file descriptor.
func (c *Conn) GetCC() (string, error) {
	return congestion.Get(c.fp)
}

// Info returns the BBRInfo and TCPInfo structs associated with the underlying
// socket. It returns an error if TCPInfo cannot be read.
func (c *Conn) Info() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error) {
	// This is expected to fail if this connection isn't set to use BBR.
	bbrInfo, _ := congestion.GetBBRInfo(c.fp)
	// If TCP_INFO isn't available on this platform, this may return
	// ErrNoSupport.
	tcpInfo, err := tcpinfox.GetTCPInfo(c.fp)
	return bbrInfo, *tcpInfo, err
}

// AcceptTime returns this connection's accept time.
func (c *Conn) AcceptTime() time.Time {
	return c.acceptTime
}

// UUID returns an M-Lab UUID. On platforms not supporting SO_COOKIE, it
// returns a google/uuid as a fallback. If the fallback fails, it panics.
func (c *Conn) UUID() (string, error) {
	uuid, err := uuid.FromFile(c.fp)
	if err != nil {
		// fallback: use google/uuid if the platform does not support SO_COOKIE.
		gid, err := guuid.NewUUID()
		// NOTE: this could only fail when guuid.GetTime() fails.
		rtx.Must(err, "unable to fallback to uuid")
		uuid = gid.String()
	}
	return uuid, nil
}
