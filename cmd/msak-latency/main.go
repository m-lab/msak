package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/m-lab/locate/api/locate"
	"github.com/m-lab/msak/pkg/latency/model"
)

var (
	flagServer    = flag.String("server", "", "MSAK server hostname and port")
	flagSessionID = flag.String("mid", "", "Measurement ID to use")
)

func startSession() (*url.URL, string) {
	cl := locate.NewClient("msak-latency")
	targets, err := cl.Nearest(context.Background(), "msak/latency")
	rtx.Must(err, "cannot get targets from locate")
	if len(targets) == 0 {
		fmt.Println("empty targets list from locate")
		return nil, ""
	}

	requestURL, err := url.Parse(targets[0].URLs["http:///latency/v1/authorize"])
	rtx.Must(err, "cannot parse URL returned by locate")

	resp, err := http.Get(requestURL.String())
	rtx.Must(err, "cannot authorize session")
	mid, err := ioutil.ReadAll(resp.Body)
	rtx.Must(err, "cannot read response body")
	return requestURL, string(mid)
}

func main() {
	flag.Parse()

	var (
		serverURL *url.URL
		host      string
		mid       string
		err       error
	)

	if *flagServer != "" {
		serverURL, err = url.Parse(*flagServer)
		rtx.Must(err, "cannot parse provided server URL")

		serverURL := &url.URL{
			Scheme:   "http",
			Host:     *flagServer,
			Path:     "/latency/v1/authorize",
			RawQuery: "mid=" + *flagSessionID,
		}

		fmt.Printf("GET %s\n", serverURL.String())
		resp, err := http.Get(serverURL.String())
		rtx.Must(err, "cannot authorize session")
		body, err := ioutil.ReadAll(resp.Body)
		rtx.Must(err, "cannot read response body")
		mid = string(body)
	} else {
		serverURL, mid = startSession()
	}

	host, _, err = net.SplitHostPort(serverURL.Host)
	if err != nil {
		host = serverURL.Host
	}

	fmt.Printf("mid: %s\n", mid)
	fmt.Printf("connecting to %s...\n", host)

	addr, err := net.ResolveUDPAddr("udp", host+":1053")
	rtx.Must(err, "invalid address")

	conn, err := net.DialUDP("udp", nil, addr)
	rtx.Must(err, "connection failed")
	defer conn.Close()

	fmt.Printf("connected to %s.\n", conn.RemoteAddr())

	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("sending kickoff message...")
	m := &model.LatencyPacket{
		Type: "c2s",
		ID:   mid,
	}
	b, err := json.Marshal(m)
	rtx.Must(err, "cannot marshal kickoff message")

	n, err := conn.Write(b)
	rtx.Must(err, "write error")
	if n != len(b) {
		fmt.Printf("partial write")
		return
	}

	buf := make([]byte, 1024)
	for timeout.Err() == nil {
		// Read incoming packets
		nRead, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}

		var m model.LatencyPacket
		err = json.Unmarshal(buf[:nRead], &m)
		if err != nil {
			fmt.Printf("unmarshal error: %v\n", err)
			return
		}

		fmt.Printf("seq %d lastrtt %d us\n", m.Seq, m.LastRTT)

		// Send packet back as-is.
		nWrote, err := conn.Write(buf[:nRead])
		if err != nil {
			fmt.Printf("write error: %v\n", err)
			return
		}
		if nWrote != nRead {
			fmt.Printf("partial write (read %d, wrote %d)\n", nRead, nWrote)
			return
		}
	}

	<-timeout.Done()
}
