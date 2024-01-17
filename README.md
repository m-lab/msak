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
minimal-download -duration 1s -server.url ws://localhost:8080/throughput/v1/download
Download server #1 - rate 35260.80 Mbps, rtt  0.03ms, elapsed 0.2076s, application r/w: 0/918552576, network r/w: 0/917513214 kernel* r/w: 538/914903145
Download server #1 - rate 34191.58 Mbps, rtt  0.03ms, elapsed 0.5168s, application r/w: 0/2213545117, network r/w: 0/2212518109 kernel* r/w: 538/2208912708
Download server #1 - rate 33703.03 Mbps, rtt  0.05ms, elapsed 0.9170s, application r/w: 0/3868199075, network r/w: 0/3867187851 kernel* r/w: 538/3863293895
Download server #1 - rate 33591.42 Mbps, rtt  0.03ms, elapsed 1.0005s, application r/w: 0/4203744426, network r/w: 0/4203784992 kernel* r/w: 538/4200858760
Download client #1 - Avg 33552.56 Mbps, MinRTT  0.00ms, elapsed 1.0023s, application r/w: 0/4203745461

# Remote with time limit.
$ minimal-download -duration 1s
Download server #1 - rate 239.68 Mbps, rtt 13.96ms, elapsed 0.1014s, application r/w: 0/6815744, network r/w: 0/6400516 kernel* r/w: 1304/3039466
Download server #1 - rate 375.56 Mbps, rtt 15.13ms, elapsed 0.2024s, application r/w: 0/13632647, network r/w: 0/13112011 kernel* r/w: 1304/9503338
Download server #1 - rate 429.03 Mbps, rtt 19.15ms, elapsed 0.3034s, application r/w: 0/19925135, network r/w: 0/19298323 kernel* r/w: 1304/16271290
Download server #1 - rate 473.54 Mbps, rtt 15.88ms, elapsed 0.5237s, application r/w: 0/35654810, network r/w: 0/34737910 kernel* r/w: 1304/30997450
Download server #1 - rate 487.79 Mbps, rtt 15.34ms, elapsed 0.6464s, application r/w: 0/42995877, network r/w: 0/42564857 kernel* r/w: 1304/39414674
Download server #1 - rate 499.34 Mbps, rtt 17.36ms, elapsed 1.0154s, application r/w: 0/66065584, network r/w: 0/66158482 kernel* r/w: 1304/63380522
Download client #1 - Avg 502.43 Mbps, MinRTT  4.11ms, elapsed 1.0520s, application r/w: 0/66066624

# Remote with bytes limit.
$ minimal-download -bytes=150000
Download server #1 - rate 8.24 Mbps, rtt 12.17ms, elapsed 0.0128s, application r/w: 0/150000, network r/w: 0/164976 kernel* r/w: 1309/13146
Download client #1 - Avg 30.51 Mbps, MinRTT 10.99ms, elapsed 0.0433s, application r/w: 0/151008
```

Every TCP connection has performance metrics accessible from the server end and
the client end. The `minimal-download` client reports metrics generated by the
server side and concludes with client side average performance. Client side
performance is comparable to what a user (or user application) would see.

## Measurements

The application, network, and kernel metrics may differ to the degree
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
