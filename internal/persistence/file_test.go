package persistence

import (
	"compress/gzip"
	"fmt"
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
		!strings.HasSuffix(df.fp.Name(), "fake-uuid.json.gz") {
		t.Errorf("invalid output filename: %s", df.fp.Name())
	}

	if _, ok := df.writer.(*gzip.Writer); !ok {
		t.Errorf("writer is not a gzip.Writer")
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
}
