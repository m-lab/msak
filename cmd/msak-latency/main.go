package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/locate/api/locate"
	v2 "github.com/m-lab/locate/api/v2"
	"github.com/m-lab/msak/pkg/latency1/model"
	"github.com/m-lab/msak/pkg/latency1/spec"
)

var (
	flagServer = flag.String("server", "", "Server address")
	flagScheme = flag.String("scheme", "http", "Server scheme (http|https)")
	flagMID    = flag.String("mid", "", "MID to use")
)

func getTargetsFromLocate() []v2.Target {
	locateV2 := locate.NewClient("msak-latency")
	targets, err := locateV2.Nearest(context.Background(), spec.ServiceName)
	rtx.Must(err, "cannot get server list from locate")
	return targets
}

func tryConnect(authorizeURL *url.URL) ([]byte, error) {
	resp, err := http.Get(authorizeURL.String())
	if err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

func stats(result model.Summary) (int, float64, int, float64) {
	if len(result.RoundTrips) == 0 {
		return 0, 0, 0, 0
	}
	var min, max, sum int
	min = result.RoundTrips[0].RTT
	for _, v := range result.RoundTrips {
		if v.RTT < min {
			min = v.RTT
		}
		if v.RTT > max {
			max = v.RTT
		}
		sum += v.RTT
	}
	return min, float64(sum) / float64(len(result.RoundTrips)),
		max, 1 - float64(result.PacketsReceived)/float64(result.PacketsSent)
}

func runMeasurement(authorizeURL, resultURL *url.URL, kickoff []byte) {
	udpServer, err := net.ResolveUDPAddr("udp", authorizeURL.Hostname()+":1053")
	rtx.Must(err, "ResolveUDPAddr failed")

	conn, err := net.DialUDP("udp", nil, udpServer)
	rtx.Must(err, "DialUDP failed")
	defer conn.Close()

	// Set a time limit of 6s for the test.
	conn.SetDeadline(time.Now().Add(6 * time.Second))

	_, err = conn.Write(kickoff)
	rtx.Must(err, "failed to send kickoff message")

	recvBuf := make([]byte, 512)
	for {
		n, err := conn.Read(recvBuf)
		if err != nil {
			fmt.Printf("read error: %v\n", err)
			break
		}
		_, err = conn.Write(recvBuf[:n])
		if err != nil {
			fmt.Printf("write error: %v\n", err)
			break
		}
		fmt.Printf(".")
	}
	fmt.Println()

	// Get results.
	resp, err := http.Get(resultURL.String())
	if err != nil {
		fmt.Printf("failed to read test results: %v\n", err)
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("failed to read response body: %v\n", err)
		return
	}

	var result model.Summary
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Printf("error parsing result as JSON: %v\n", err)
		return
	}
	min, avg, max, loss := stats(result)
	fmt.Printf("rtt min/avg/max: %.3f/%.3f/%.3f ms, loss: %.1f\n",
		float64(min)/1000, avg/1000, float64(max)/1000, loss)
}

func main() {
	flag.Parse()
	flagx.ArgsFromEnv(flag.CommandLine)

	var (
		authorizeURL *url.URL
		resultURL    *url.URL

		kickoffMsg []byte
	)

	if *flagServer != "" {
		// If a server was provided, use it.
		var err error
		authorizeURL = &url.URL{
			Scheme:   *flagScheme,
			Host:     *flagServer,
			Path:     spec.AuthorizeV1,
			RawQuery: "mid=" + *flagMID,
		}
		resultURL = &url.URL{
			Scheme:   *flagScheme,
			Host:     *flagServer,
			Path:     spec.ResultV1,
			RawQuery: "mid=" + *flagMID,
		}
		fmt.Printf("Attempting to connect to: %s\n", authorizeURL)
		kickoffMsg, err = tryConnect(authorizeURL)
		rtx.Must(err, "connection failed")
	} else {
		targets := getTargetsFromLocate()

		for _, t := range targets {
			var err error
			authorizeURL, err = url.Parse(t.URLs[*flagScheme+"://"+spec.AuthorizeV1])
			rtx.Must(err, "Locate returned an invalid authorization URL")

			resultURL, err = url.Parse(t.URLs[*flagScheme+"://"+spec.ResultV1])
			rtx.Must(err, "Locate returned an invalid result URL")

			fmt.Printf("Attempting to connect to: %s\n", authorizeURL)
			kickoffMsg, err = tryConnect(authorizeURL)
			if err == nil {
				break
			}
			fmt.Printf("failed to connect to %s\n", authorizeURL)
		}

		if len(kickoffMsg) == 0 {
			fmt.Printf("no server found")
			os.Exit(1)
		}
	}

	// Now we have a server and a kickoff message, start the measurement.
	runMeasurement(authorizeURL, resultURL, kickoffMsg)

}
