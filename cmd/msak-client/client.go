package main

import (
	"context"
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
	flagOutput   = flag.String("output", "", "Path to write measurement results to")
)

func main() {
	flag.Parse()

	if float64(*flagStreams-1)*flagDelay.Seconds() >= flagDuration.Seconds() {
		log.Print("Invalid configuration: please check streams, delay and duration and make sure they make sense.")
		os.Exit(1)
	}

	cl := client.New("msak-client", "")
	cl.Server = *flagServer
	cl.CongestionControl = *flagCC
	cl.NumStreams = *flagStreams
	cl.Scheme = *flagScheme
	cl.MeasurementID = uuid.NewString()
	cl.Length = *flagDuration
	cl.Delay = *flagDelay

	cl.OutputPath = *flagOutput

	cl.Download(context.Background())
}
