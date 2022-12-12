package persistence

import (
	"encoding/json"
	"os"
	"path"
	"time"
)

// DataFile is the file where we save measurements.
type DataFile struct {
	fp *os.File
}

func newDataFile(datadir, datatype, subtest, uuid string) (*DataFile, error) {
	timestamp := time.Now()
	dir := path.Join(datadir, datatype, timestamp.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	filepath := path.Join(dir, datatype+"-"+subtest+"-"+
		timestamp.Format("20060102T150405.000000000Z")+"."+uuid+".json")
	fp, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}

	return &DataFile{
		fp: fp,
	}, nil
}

// New creates a DataFile for saving results in datadir.
func New(datadir, datatype, subtest, uuid string) (*DataFile, error) {
	file, err := newDataFile(datadir, datatype, subtest, uuid)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Write writes a JSON representation of the result to this file.
func (df *DataFile) Write(result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = df.fp.Write(data)
	return err
}

// Close closes the file.
func (df *DataFile) Close() error {
	return df.fp.Close()
}
