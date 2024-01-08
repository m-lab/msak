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

To build the minimal client and target a local or remote server:

```sh
$ go install github.com/m-lab/msak/cmd/minimal-download@latest
...
# Local
$ minimal-download -duration 1s -server.url ws://localhost:8080/throughput/v1/download
Download #1 - rate 37646.34 Mbps, rtt  0.03ms, elapsed 0.2005s, application r/w: 0/944766976, network r/w: 0/943727864 kernel* r/w: 538/940629967
Download #1 - rate 36690.44 Mbps, rtt  0.02ms, elapsed 0.6009s, application r/w: 0/2756707483, network r/w: 0/2755685655 kernel* r/w: 538/2752249838
Download #1 - rate 34228.14 Mbps, rtt  0.06ms, elapsed 0.8259s, application r/w: 0/3534752935, network r/w: 0/3533738535 kernel* r/w: 538/3530874047
Download #1 - rate 34016.59 Mbps, rtt  0.04ms, elapsed 0.9253s, application r/w: 0/3935309999, network r/w: 0/3934299423 kernel* r/w: 538/3931040660
Download #1 - rate 33946.60 Mbps, rtt  0.02ms, elapsed 1.0006s, application r/w: 0/4245689527, network r/w: 0/4245730501 kernel* r/w: 538/4243256029
Download #1 - Avg 33882.39 Mbps, MinRTT  0.00ms, elapsed 1.0025s, application r/w: 0/4245690561


# Remote with time limit.
$ minimal-download -duration 1s
Download #1 - rate 469.19 Mbps, rtt 11.89ms, elapsed 0.2012s, application r/w: 0/12582912, network r/w: 0/11798314 kernel* r/w: 1304/8338096
Download #1 - rate 506.90 Mbps, rtt 22.16ms, elapsed 0.4679s, application r/w: 0/30409859, network r/w: 0/29649767 kernel* r/w: 1304/26703080
Download #1 - rate 517.75 Mbps, rtt 15.57ms, elapsed 0.7783s, application r/w: 0/50333840, network r/w: 0/50372216 kernel* r/w: 1304/47179248
Download #1 - rate 514.20 Mbps, rtt 17.25ms, elapsed 0.8789s, application r/w: 0/56626332, network r/w: 0/56492908 kernel* r/w: 1304/53583752
Download #1 - rate 513.92 Mbps, rtt 16.05ms, elapsed 0.9798s, application r/w: 0/62918825, network r/w: 0/62941721 kernel* r/w: 1304/60156224
Download #1 - rate 516.71 Mbps, rtt 15.96ms, elapsed 1.0080s, application r/w: 0/65017014, network r/w: 0/65108472 kernel* r/w: 1304/61950296
Download #1 - Avg 495.42 Mbps, MinRTT  4.28ms, elapsed 1.0499s, application r/w: 0/65018054

# Remote with bytes limit.
$ minimal-download -bytes=150000
Download #1 - rate 110.47 Mbps, rtt 11.70ms, elapsed 0.0119s, application r/w: 0/150000, network r/w: 0/164976 kernel* r/w: 1309/14594
Download #1 - Avg 34.14 Mbps, MinRTT 10.05ms, elapsed 0.0387s, application r/w: 0/164974
```

> NOTE: the application, network, and kernel metrics may differ to the degree
that some data is buffered, or includes added headers, or is traversing the
physical network itself. For example, after a websocket write and before
TLS/WebSocket headers are added (application), after TLS/WebSocket headers are
added before being sent to the Linux kernel (network), or after the Linux kernel
sends over the physical network and before the remote client acknowledges the
bytes received (kernel).

The maximum difference between the application and network sent sizes should be
equal to the `spec.MaxScaledMessageSize` + WebSocket/TLS headers size, which
should typically be below 1MB, and the maximum difference between the network
and kernel sent sizes should equal the Linux kernel buffers plus the network's
bandwidth delay product. Typically values range between 64k and 4MB or more.
