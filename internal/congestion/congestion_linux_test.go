package congestion

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestGet(t *testing.T) {
	// Create a TCP socket to test.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("cannot create socket: %v", err)
	}
	fp := os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd))
	defer fp.Close()

	cc, err := Get(fp)
	if err != nil {
		t.Errorf("cannot get the socket's cc: %v", err)
	}
	fmt.Println(cc)
}

func TestSet(t *testing.T) {
	// Get a list of the available cc algorithms in the environment.
	content, err := ioutil.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		t.Skip("cannot read list of available cc algorithm, skipping test")
	}

	// Create a TCP socket to test.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("cannot create socket: %v", err)
	}
	fp := os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd))
	defer fp.Close()

	ccListStr := strings.TrimSpace(string(content))
	ccList := strings.Split(ccListStr, " ")
	for _, cc := range ccList {
		t.Logf("testing cc %s", cc)
		err = Set(fp, cc)
		if err != nil {
			t.Fatalf("cannot set the socket's cc: %v", err)
		}
		actual, err := Get(fp)
		if err != nil {
			t.Fatalf("cannot get the socket's cc: %v", err)
		}
		if actual != cc {
			t.Errorf("the cc hasn't been set (found: %s, expected: %s)", actual, cc)
		}
	}
}
