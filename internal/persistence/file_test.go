package persistence

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	df, err := New("testdata", "type", "subtest", "fake-uuid")
	if err != nil {
		t.Fatalf("cannot create test datafile: %v", err)
	}

	prefix := fmt.Sprintf("testdata/type/%s/type-subtest-", time.Now().Format("2006/01/02"))
	if !strings.HasPrefix(df.fp.Name(), prefix) ||
		!strings.HasSuffix(df.fp.Name(), "fake-uuid.json") {
		t.Errorf("invalid output filename: %s", df.fp.Name())
	}
}

func TestDataFile_Write(t *testing.T) {
	df, err := New("testdata", "type", "subtest", "fake-uuid")
	if err != nil {
		t.Fatalf("cannot create test datafile: %v", err)
	}

	err = df.Write([]string{"foo"})
	if err != nil {
		t.Errorf("unexpected write error: %v", err)
	}

	err = df.Close()
	if err != nil {
		t.Errorf("unexpected close error: %v", err)
	}

	// Check the file content.
	content, err := ioutil.ReadFile(df.fp.Name())
	if err != nil {
		t.Errorf("error while reading file content: %v", err)
	}
	if string(content) != "[\"foo\"]" {
		t.Errorf("unexpected file content: %s", string(content))
	}
}