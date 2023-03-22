//!linux
//go:build !linux
// +build !linux

package netx

func (ln *Listener) accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}

	return fromTCPConn(tc)
}
