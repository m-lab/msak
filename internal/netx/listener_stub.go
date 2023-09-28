//go:build !linux
// +build !linux

package netx

import "net"

func (ln *Listener) accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}

	return fromTCPLikeConn(tc)
}
