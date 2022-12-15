// Package congestion contains code required to set the congestion control
// algorithm and read BBR variables of a net.Conn. This code currently only
// works on Linux systems, as BBR is only available there.
package congestion

import (
	"errors"
	"os"

	"github.com/m-lab/tcp-info/inetdiag"
)

// ErrNoSupport indicates that this system does not support BBR.
var ErrNoSupport = errors.New("TCP_CC_INFO not supported")

// Set sets the congestion control algorithm for the given socket to a
// string value. It can fail if the requested cc algorithm is not available.
func Set(fp *os.File, cc string) error {
	return set(fp, cc)
}

// Get returns the congestion control algorithm set for the given socket's
// file descriptor.
func Get(fp *os.File) (string, error) {
	return get(fp)
}

// GetBBRInfo obtains BBR info from fp.
func GetBBRInfo(fp *os.File) (inetdiag.BBRInfo, error) {
	return getMaxBandwidthAndMinRTT(fp)
}
