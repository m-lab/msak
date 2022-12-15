package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
)

// GetFile returns the socket's file descriptor for the given net.Conn.
// It currently supports net.TCPConn and tls.Conn only, and will return an
// error if you call it with something else.
func GetFile(conn net.Conn) (*os.File, error) {
	switch t := conn.(type) {
	case *net.TCPConn:
		return t.File()
	case *tls.Conn:
		return t.NetConn().(*net.TCPConn).File()
	default:
		return nil, fmt.Errorf("unsupported connection type: %T", t)
	}
}
