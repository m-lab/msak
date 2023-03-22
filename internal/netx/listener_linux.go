package netx

import (
	"net"
	"time"
)

func (ln *Listener) accept() (net.Conn, error) {
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
