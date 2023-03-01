package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// Listener is a TCPListener. Connections accepted by this listener provide
// extra methods to interact with the connection's underlying file descriptor.
type Listener struct {
	*net.TCPListener
}

func NewListener(l *net.TCPListener) *Listener {
	return &Listener{
		TCPListener: l,
	}
}

// Accept accepts a connection and returns a netx.Conn which includes the
// connection's "accept time" and provides operations on the underlying file
// descriptor.
func (ln *Listener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	// The "accept time" is recorded immediately after AcceptTCP. This is the
	// closest thing we can get to a reference "start time" for TCPInfo metrics
	// since the TCP_INFO struct does not include time fields.
	acceptTime := time.Now()
	// Note: File() creates a copy which must be independently closed.
	fp, err := ln.File()
	if err != nil {
		tc.Close()
		return nil, err
	}

	mc := &Conn{
		Conn:       tc,
		fp:         fp,
		acceptTime: acceptTime,
	}
	return mc, nil
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
