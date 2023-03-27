package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/m-lab/msak/pkg/client"
)

var (
	flagServer   = flag.String("server", "", "Server address")
	flagStreams  = flag.Int("streams", 2, "Number of streams")
	flagCC       = flag.String("cc", "bbr", "Congestion control algorithm to use")
	flagDelay    = flag.Duration("delay", 2*time.Second, "Delay between each stream")
	flagDuration = flag.Duration("duration", 5*time.Second, "Length of the last stream")
	flagScheme   = flag.String("scheme", "ws", "Websocket scheme (wss or ws)")
	flagMID      = flag.String("mid", uuid.NewString(), "Measurement ID to use")
	flagNoVerify = flag.Bool("no-verify", false, "Skip TLS certificate verification")
)

func main() {
	flag.Parse()

	if float64(*flagStreams-1)*flagDelay.Seconds() >= flagDuration.Seconds() {
		log.Print("Invalid configuration: please check streams, delay and duration and make sure they make sense.")
		os.Exit(1)
	}

	cl := client.New("msak-client", "")
	cl.Dialer.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: *flagNoVerify,
	}
	cl.Server = *flagServer
	cl.CongestionControl = *flagCC
	cl.NumStreams = *flagStreams
	cl.Scheme = *flagScheme
	cl.MeasurementID = *flagMID
	cl.Length = *flagDuration
	cl.Delay = *flagDelay

	cl.Download(context.Background())
}
