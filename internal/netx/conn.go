package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
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

// Conn is an extended net.Conn that stores its accept time and a copy of the
// underlying socket's file descriptor.
type Conn struct {
	net.Conn

	fp         *os.File
	acceptTime time.Time
}

// Close closes the underlying net.Conn and the duplicate file descriptor.
func (c *Conn) Close() error {
	c.fp.Close()
	return c.Conn.Close()
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

func (c *Conn) Info() (inetdiag.BBRInfo, tcp.LinuxTCPInfo, error) {
	// This is expected to fail if this connection isn't set to use BBR.
	bbrInfo, _ := congestion.GetBBRInfo(c.fp)
	// If TCP_INFO isn't available on this platform, this may return
	// ErrNoSupport.
	tcpInfo, err := tcpinfox.GetTCPInfo(c.fp)
	return bbrInfo, *tcpInfo, err
}

func (c *Conn) AcceptTime() time.Time {
	return c.acceptTime
}

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
