package main

import (
	"context"
	"flag"
	"log"

	"github.com/google/uuid"
	"github.com/m-lab/msak/pkg/client"
	"github.com/m-lab/msak/pkg/version"
)

const clientName = "msak-client-go"

var clientVersion = version.Version

var (
	flagServer    = flag.String("server", "", "Server address")
	flagStreams   = flag.Int("streams", client.DefaultStreams, "Number of streams")
	flagCC        = flag.String("cc", "bbr", "Congestion control algorithm to use")
	flagDelay     = flag.Duration("delay", 0, "Delay between each stream")
	flagDuration  = flag.Duration("duration", client.DefaultLength, "Length of the last stream")
	flagScheme    = flag.String("scheme", client.DefaultScheme, "Websocket scheme (wss or ws)")
	flagMID       = flag.String("mid", uuid.NewString(), "Measurement ID to use")
	flagNoVerify  = flag.Bool("no-verify", false, "Skip TLS certificate verification")
	flagDebug     = flag.Bool("debug", false, "Enable debug logging")
	flagByteLimit = flag.Int("bytes", 0, "Byte limit to request to the server")
	flagUpload    = flag.Bool("upload", true, "Whether to run upload test")
	flagDownload  = flag.Bool("download", true, "Whether to run download test")
)

func main() {
	flag.Parse()

	// For a given number of streams, there will be streams-1 delays. This makes
	// sure that all the streams can at least start with the current configuration.
	if float64(*flagStreams-1)*flagDelay.Seconds() >= flagDuration.Seconds() {
		log.Fatal("Invalid configuration: please check streams, delay and duration and make sure they make sense.")
	}

	if *flagStreams < 1 || *flagStreams > 4 {
		log.Fatal("Invalid configuration: the number of streams must be between 1 and 4.")
	}

	config := client.Config{
		Server:            *flagServer,
		Scheme:            *flagScheme,
		NumStreams:        *flagStreams,
		CongestionControl: *flagCC,
		Delay:             *flagDelay,
		Length:            *flagDuration,
		MeasurementID:     *flagMID,
		Emitter: client.HumanReadable{
			Debug: *flagDebug,
		},
		NoVerify:  *flagNoVerify,
		ByteLimit: *flagByteLimit,
	}

	cl := client.New(clientName, clientVersion, config)

	if *flagDownload {
		cl.Download(context.Background())
	}
	if *flagUpload {
		cl.Upload(context.Background())
	}

	cl.PrintSummary()
}
