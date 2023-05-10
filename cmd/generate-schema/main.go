package main

import (
	"flag"
	"io/ioutil"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/pkg/ndt8/model"

	"cloud.google.com/go/bigquery"
)

var (
	ndt8Schema string
)

func init() {
	flag.StringVar(&ndt8Schema, "ndt8", "/var/spool/datatypes/ndt8.json", "filename to write ndt8 schema")
}

func main() {
	flag.Parse()
	// Generate and save ndt7 schema for autoloading.
	ndt8Result := model.NDT8Result{}
	sch, err := bigquery.InferSchema(ndt8Result)
	rtx.Must(err, "failed to generate ndt8 schema")
	b, err := sch.ToJSONFields()
	rtx.Must(err, "failed to marshal schema")
	ioutil.WriteFile(ndt8Schema, b, 0o644)
}
