package netx

import (
	"net"
	"time"
)

// Listener is a TCPListener. Connections accepted by this listener provide
// extra methods to interact with the connection's underlying file descriptor.
type Listener struct {
	*net.TCPListener
}

// NewListener returns a netx.Listener.
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
	// Note: File() duplicates the underlying file descriptor. This duplicate
	// must be independently closed.
	fp, err := tc.File()
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
