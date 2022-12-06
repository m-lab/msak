package persistence

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path"
	"time"
)

// DataFile is the file where we save measurements.
type DataFile struct {
	writer io.WriteCloser
	fp     *os.File
}

func newDataFile(datadir, datatype, subtest, uuid string) (*DataFile, error) {
	timestamp := time.Now()
	dir := path.Join(datadir, datatype, timestamp.Format("2006/01/02"))
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	filepath := path.Join(dir, datatype+"-"+subtest+"-"+
		timestamp.Format("20060102T150405.000000000Z")+"."+uuid+".json.gz")
	fp, err := os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}
	writer, err := gzip.NewWriterLevel(fp, gzip.BestSpeed)
	if err != nil {
		fp.Close()
		return nil, err
	}
	return &DataFile{
		writer: writer,
		fp:     fp,
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

// Write writes a JSON representation of result to this file.
func (df *DataFile) Write(result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = df.writer.Write(data)
	return err
}

// Close closes the gzip writer and the file.
func (df *DataFile) Close() error {
	err := df.writer.Close()
	if err != nil {
		df.fp.Close()
		return err
	}
	return df.fp.Close()
}
