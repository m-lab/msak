package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
)

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
