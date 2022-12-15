package persistence_test

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/m-lab/msak/internal/persistence"
)

// A struct that can be marshalled to JSON.
type Marshallable struct {
	Test string
}

// A struct that cannot be marshalled to JSON.
type Unmarshallable struct {
	Invalid chan byte
}

func TestWriteDataFile(t *testing.T) {
	testdata := Marshallable{Test: "foo"}
	df, err := persistence.WriteDataFile("testdata", "type", "subtest", "fake-uuid", testdata)
	if err != nil {
		t.Fatalf("cannot create test datafile: %v", err)
	}

	if df.Prefix != "testdata" || df.Datatype != "type" ||
		df.Subtest != "subtest" || df.UUID != "fake-uuid" {
		t.Fatalf("invalid field values in DataFile")
	}

	// Check the generated path.
	prefix := fmt.Sprintf("testdata/type/%s/type-subtest-", time.Now().Format("2006/01/02"))
	if !strings.HasPrefix(df.Path, prefix) ||
		!strings.HasSuffix(df.Path, "fake-uuid.json") {
		t.Errorf("invalid output path: %s", df.Path)
	}
	// Check the file contents.
	content, err := ioutil.ReadFile(df.Path)
	if err != nil {
		t.Errorf("error while reading file content: %v", err)
	}
	if string(content) != `{"Test":"foo"}` {
		t.Errorf("unexpected file content: %s", string(content))
	}
	if df.Size != len(content) {
		t.Errorf("invalid Size: %d (should be %d)", df.Size, len(content))
	}

	invaliddata := Unmarshallable{Invalid: make(chan byte)}
	_, err = persistence.WriteDataFile("testdata", "type", "subtest", "fake-uuid", invaliddata)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
