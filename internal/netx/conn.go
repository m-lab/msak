package netx

import (
	"context"
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

type contextKey string

const uuidCtxKey = "netx-uuid"

// ConnInfo provides operations on a net.Conn's underlying file descriptor.
type ConnInfo interface {
	ByteCounters() (uint64, uint64)
	Info() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error)
	AcceptTime() time.Time
	UUID() string
	GetCC() (string, error)
	SetCC(string) error
	SaveUUID(context.Context) context.Context
}

// TCPLikeConn is a net.Conn with a File() method. This is useful for creating a
// netx.Conn based on a custom TCPConn-like type - e.g. for testing.
type TCPLikeConn interface {
	net.Conn
	File() (*os.File, error)
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

// FromTCPLikeConn creates a netx.Conn from a TCPLikeConn.
func FromTCPLikeConn(tcpConn TCPLikeConn) (*Conn, error) {
	return fromTCPLikeConn(tcpConn)
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
func (c *Conn) UUID() string {
	uuid, err := uuid.FromFile(c.fp)
	if err != nil {
		// fallback: use google/uuid if the platform does not support SO_COOKIE.
		gid, err := guuid.NewUUID()
		// NOTE: this could only fail when guuid.GetTime() fails.
		rtx.Must(err, "unable to fallback to uuid")
		uuid = gid.String()
	}
	return uuid
}

// SaveUUID saves this connection's UUID in a context.Context using a globally
// unique key. LoadUUID should be used to retrieve the uuid from the context.
func (c *Conn) SaveUUID(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey(uuidCtxKey), c.UUID())
}

// LoadUUID reads a connection UUID from a context.Context using a globally
// unique key. Returns an empty string if the UUID is not found in the context.
func LoadUUID(ctx context.Context) string {
	uuid, ok := ctx.Value(contextKey(uuidCtxKey)).(string)
	if !ok {
		return ""
	}
	return uuid
}
