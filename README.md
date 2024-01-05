# Measurements Swiss Army Knife

[![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/msak)](https://goreportcard.com/report/github.com/m-lab/msak)
[![Build Status](https://github.com/m-lab/msak/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/m-lab/msak/actions/workflows/test.yml)
[![Coverage Status](https://coveralls.io/repos/github/m-lab/msak/badge.svg?branch=main)](https://coveralls.io/github/m-lab/msak?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/m-lab/msak.svg)](https://pkg.go.dev/github.com/m-lab/msak)

The [MSAK design doc (view)][1] describes the throughput1 and latency1 protocols.

[1]: https://docs.google.com/document/d/1OmKXGhQe2mT1gSXI2NT_SxvnKu5OHpBGIYpoWNJwmWA/edit

* `msak-server` - is the MSAK server
* `msak-client` - is a full reference client for the throughput1 protocol

Additional reference clients are also available:

* `minimal-download` - is a minimal download-only, reference cleint for the throughput1 protocol
* `msak-latency` - is a reference client for the latency1 protocol

## Server

To build the server and run locally without TLS certificates:

```sh
$ go install github.com/m-lab/msak/cmd/msak-server@latest
...
$ msak-server
2024/01/04 17:41:01 INFO <msak-server/server.go:117> About to listen for ws tests endpoint=:8080
2024/01/04 17:41:01 INFO <latency1/latency1.go:286> Accepting UDP packets...
```

## Clients

To build the client and target the local server:

```sh
$ go install github.com/m-lab/msak/cmd/msak-client@latest
...
$ msak-client -duration=2s -streams=1 -server localhost:8080  -scheme ws
Starting download stream (server: localhost:8080)
Connected to ws://localhost:8080/throughput/v1/download?bytes=0&...
Elapsed: 0.10s, Goodput: 0.000000 Mb/s, MinRTT: 0
Elapsed: 0.20s, Goodput: 0.000000 Mb/s, MinRTT: 0
Elapsed: 0.30s, Goodput: 0.000000 Mb/s, MinRTT: 0
Elapsed: 0.40s, Goodput: 0.000000 Mb/s, MinRTT: 0
Elapsed: 0.50s, Goodput: 18941.574715 Mb/s, MinRTT: 0
Elapsed: 0.60s, Goodput: 15784.580735 Mb/s, MinRTT: 0
Elapsed: 0.70s, Goodput: 13526.483102 Mb/s, MinRTT: 0
Elapsed: 0.80s, Goodput: 11839.166779 Mb/s, MinRTT: 0
Elapsed: 0.90s, Goodput: 21180.762969 Mb/s, MinRTT: 0
Elapsed: 1.00s, Goodput: 21419.807987 Mb/s, MinRTT: 0
Stream 0 complete (server localhost:8080)
Starting upload stream (server: localhost:8080)
Connected to ws://localhost:8080/throughput/v1/upload?bytes=0&...
Elapsed: 0.10s, Goodput: 0.000000 Mb/s, MinRTT: 0
Elapsed: 0.20s, Goodput: 18007.157915 Mb/s, MinRTT: 0
Elapsed: 0.30s, Goodput: 19836.665589 Mb/s, MinRTT: 0
Elapsed: 0.40s, Goodput: 14878.343408 Mb/s, MinRTT: 0
Elapsed: 0.50s, Goodput: 11902.100544 Mb/s, MinRTT: 0
Elapsed: 0.60s, Goodput: 23205.208525 Mb/s, MinRTT: 0
Elapsed: 0.70s, Goodput: 19895.442053 Mb/s, MinRTT: 0
Elapsed: 0.80s, Goodput: 21784.415044 Mb/s, MinRTT: 0
Elapsed: 0.90s, Goodput: 19363.955232 Mb/s, MinRTT: 0
Elapsed: 1.00s, Goodput: 17431.176757 Mb/s, MinRTT: 0
Stream 0 complete (server localhost:8080)
```

To build the minimal client and target the local server:

```sh
$ go install github.com/m-lab/msak/cmd/minimal-download@latest
...
$ minimal-download -duration 1s -server.url ws://localhost:8080/throughput/v1/download
Server #1 - avg 40331.74 Mbps, elapsed 0.1096s, payload r/w: 0/553648128, network r/w: 0/552605286 kernel* r/w: 532/556927351
Server #1 - avg 37079.39 Mbps, elapsed 0.5090s, payload r/w: 0/2360345763, network r/w: 0/2359320155 kernel* r/w: 532/2371631146
Server #1 - avg 37434.47 Mbps, elapsed 0.7453s, payload r/w: 0/3488615636, network r/w: 0/3487600796 kernel* r/w: 532/3504496455
Server #1 - avg 37546.81 Mbps, elapsed 0.8448s, payload r/w: 0/3965718768, network r/w: 0/3964708482 kernel* r/w: 532/3982651869
```
