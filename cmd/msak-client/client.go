package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"

	"github.com/google/uuid"
	"github.com/m-lab/msak/pkg/client"
	"github.com/m-lab/msak/pkg/version"
)

const clientName = "msak-client-go"

var clientVersion = version.Version

var (
	flagServer   = flag.String("server", "", "Server address")
	flagStreams  = flag.Int("streams", client.DefaultStreams, "Number of streams")
	flagCC       = flag.String("cc", "bbr", "Congestion control algorithm to use")
	flagDelay    = flag.Duration("delay", 0, "Delay between each stream")
	flagDuration = flag.Duration("duration", client.DefaultLength, "Length of the last stream")
	flagScheme   = flag.String("scheme", "ws", "Websocket scheme (wss or ws)")
	flagMID      = flag.String("mid", uuid.NewString(), "Measurement ID to use")
	flagNoVerify = flag.Bool("no-verify", false, "Skip TLS certificate verification")
	flagDebug    = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	// For a given number of streams, there will be streams-1 delays. This makes
	// sure that all the streams can at least start with the current configuration.
	if float64(*flagStreams-1)*flagDelay.Seconds() >= flagDuration.Seconds() {
		log.Fatal("Invalid configuration: please check streams, delay and duration and make sure they make sense.")
	}

	cl := client.New(clientName, clientVersion)
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

	cl.Emitter = &client.HumanReadable{
		Debug: *flagDebug,
	}

	cl.Download(context.Background())
	cl.Upload(context.Background())
}
