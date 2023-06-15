package persistence

import (
	"encoding/json"
	"os"
	"path"
	"time"
)

// DataFile is the file where we save measurements.
type DataFile struct {
	// The path prefix.
	Prefix string
	// Datatype component of the path.
	Datatype string
	// Subtest component of the path.
	Subtest string
	// UUID of this measurement file.
	UUID string
	// The size of this data file on disk, in bytes.
	Size int

	// The relative file path, generated according to the provided prefix,
	// datatype, subtest, uuid and the timestamp at generation time.
	Path string
}

// WriteDataFile creates a new JSON output file containing the representation
// of the data struct.
//
// The path is determined by the provided prefix, datatype, subtest and uuid.
func WriteDataFile(prefix, datatype, subtest, uuid string,
	data interface{}) (*DataFile, error) {
	timestamp := time.Now()
	dir := path.Join(prefix, datatype, timestamp.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	var filename string
	if subtest != "" {
		filename = datatype + "-" + subtest + "-" +
			timestamp.Format("20060102T150405.000000000Z") + "." + uuid + ".json"
	} else {
		filename = datatype + "-" +
			timestamp.Format("20060102T150405.000000000Z") + "." + uuid + ".json"
	}
	filepath := path.Join(dir, filename)
	fp, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	// Marshal data struct.
	jsonResult, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	n, err := fp.Write(jsonResult)
	if err != nil {
		return nil, err
	}
	return &DataFile{
		Prefix:   prefix,
		Datatype: datatype,
		Subtest:  subtest,
		UUID:     uuid,
		Path:     filepath,
		Size:     n,
	}, nil
}
