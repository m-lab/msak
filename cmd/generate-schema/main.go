package main

import (
	"flag"
	"io/ioutil"

	"github.com/m-lab/go/cloud/bqx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/pkg/throughput1/model"

	"cloud.google.com/go/bigquery"
)

var (
	throughput1Schema string
)

func init() {
	flag.StringVar(&throughput1Schema, "throughput1", "/var/spool/datatypes/throughput1.json", "filename to write throughput1 schema")
}

func main() {
	flag.Parse()
	// Generate and save ndt7 schema for autoloading.
	throughput1Result := model.Throughput1Result{}
	sch, err := bigquery.InferSchema(throughput1Result)
	rtx.Must(err, "failed to generate throughput1 schema")
	sch = bqx.RemoveRequired(sch)
	b, err := sch.ToJSONFields()
	rtx.Must(err, "failed to marshal schema")
	ioutil.WriteFile(throughput1Schema, b, 0o644)
}
