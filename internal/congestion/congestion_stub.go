//go:build !linux
// +build !linux

package congestion

import (
	"os"

	"github.com/m-lab/tcp-info/inetdiag"
)

func set(*os.File, string) error {
	return ErrNoSupport
}

func get(*os.File) (string, error) {
	return "", ErrNoSupport
}

func getMaxBandwidthAndMinRTT(*os.File) (inetdiag.BBRInfo, error) {
	return inetdiag.BBRInfo{}, ErrNoSupport
}
