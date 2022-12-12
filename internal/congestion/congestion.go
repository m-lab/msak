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

// Enable sets the congestion control algorithm for |fp|.
func Set(fp *os.File, cc string) error {
	return set(fp, cc)
}

func Get(fp *os.File) (string, error) {
	return get(fp)
}

// GetBBRInfo obtains BBR info from |fp|.
func GetBBRInfo(fp *os.File) (inetdiag.BBRInfo, error) {
	return getMaxBandwidthAndMinRTT(fp)
}
