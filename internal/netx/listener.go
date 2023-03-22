package netx

import (
	"net"
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
	return ln.accept()
}
