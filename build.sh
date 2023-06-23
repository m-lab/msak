#!/bin/sh
COMMIT=$(git log -1 --format=%h)
versionflags="-X github.com/m-lab/go/prometheusx.GitShortCommit=${COMMIT}"

go build -v \
    -tags netgo \
    -ldflags "$versionflags -extldflags \"-static\"" \
    ./cmd/msak-server

go build -v ./cmd/generate-schema
