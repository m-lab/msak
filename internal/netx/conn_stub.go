//go:build !linux
// +build !linux

package netx

func fromTCPConn(tcpConn *net.TCPConn) (*Conn, error) {
	// On non-Linux systems, TCPInfo/BBRInfo aren't supported, the file pointer
	// is not needed.
	return &Conn{
		Conn:       tcpConn,
		acceptTime: time.Now(),
	}, nil
}

func (c *Conn) close() error {
	return c.Conn.Close()
}
