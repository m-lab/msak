//go:build !linux
// +build !linux

package congestion

import (
	"os"
	"testing"
)

func Test_Set(t *testing.T) {
	// This is unsupported on non-Linux systems.
	err := Set(&os.File{}, "")
	if err != ErrNoSupport {
		t.Errorf("expected ErrNoSupport, got: %v", err)
	}
}

func Test_Get(t *testing.T) {
	// This is unsupported on non-Linux systems.
	cc, err := Get(&os.File{})
	if cc != "" {
		t.Errorf("unexpected value")
	}
	if err == ErrNoSupport {
		t.Errorf("expected ErrNoSupport, got: %v", err)
	}
}

func Test_getMaxBandwidthAndMinRTT(t *testing.T) {
	// This is unsupported on non-Linux systems.
	_, err := GetBBRInfo(&os.File{})
	if err == ErrNoSupport {
		t.Errorf("expected ErrNoSupport, got: %v", err)
	}
}
