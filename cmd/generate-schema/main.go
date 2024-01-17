package main

import (
	"flag"
	"os"

	"github.com/m-lab/go/cloud/bqx"
	"github.com/m-lab/go/rtx"
	latency1model "github.com/m-lab/msak/pkg/latency1/model"
	"github.com/m-lab/msak/pkg/throughput1/model"

	"cloud.google.com/go/bigquery"
)

var (
	throughput1Schema string
	latency1Schema    string
)

func init() {
	flag.StringVar(&throughput1Schema, "throughput1", "/var/spool/datatypes/throughput1.json", "filename to write throughput1 schema")
	flag.StringVar(&latency1Schema, "latency1", "/var/spool/datatypes/latency1.json", "filename to write latency1 schema")
}

func main() {
	flag.Parse()
	// Generate and save schemas for autoloading.
	// throughput1 schema.
	throughput1Result := model.Throughput1Result{}
	sch, err := bigquery.InferSchema(throughput1Result)
	rtx.Must(err, "failed to generate throughput1 schema")
	sch = bqx.RemoveRequired(sch)
	b, err := sch.ToJSONFields()
	rtx.Must(err, "failed to marshal throughput1 schema")
	err = os.WriteFile(throughput1Schema, b, 0o644)
	rtx.Must(err, "failed to write throughput1 schema")
	// latency1 schema.
	latency1Result := latency1model.ArchivalData{}
	sch, err = bigquery.InferSchema(latency1Result)
	rtx.Must(err, "failed to generate latency1 schema")
	sch = bqx.RemoveRequired(sch)
	b, err = sch.ToJSONFields()
	rtx.Must(err, "failed to marshal latency1 schema")
	err = os.WriteFile(latency1Schema, b, 0o644)
	rtx.Must(err, "failed to write latency1 schema")
}
