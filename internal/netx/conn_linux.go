package netx

import (
	"net"
	"time"
)

func fromTCPConn(tcpConn *net.TCPConn) (*Conn, error) {
	// On Linux system, this can only fail when the file duplication fails.
	fp, err := tcpConn.File()
	if err != nil {
		return nil, err
	}
	return &Conn{
		Conn:       tcpConn,
		fp:         fp,
		acceptTime: time.Now(),
	}, nil
}

func (c *Conn) close() error {
	c.fp.Close()
	return c.Conn.Close()
}
