package main

import (
	"context"
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/m-lab/access/controller"
	"github.com/m-lab/access/token"
	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/msak/internal/handler"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/pkg/throughput1/spec"
)

var (
	flagCertFile          = flag.String("cert", "", "The file with server certificates in PEM format.")
	flagKeyFile           = flag.String("key", "", "The file with server key in PEM format.")
	flagEndpoint          = flag.String("wss_addr", ":4443", "Listen address/port for TLS connections")
	flagEndpointCleartext = flag.String("ws_addr", ":8080", "Listen address/port for cleartext connections")
	flagDataDir           = flag.String("datadir", "./data", "Directory to store data in")
	tokenVerifyKey        = flagx.FileBytesArray{}
	tokenVerify           bool
	tokenMachine          string

	// Context for the whole program.
	ctx, cancel = context.WithCancel(context.Background())
)

func init() {
	flag.Var(&tokenVerifyKey, "token.verify-key", "Public key for verifying access tokens")
	flag.BoolVar(&tokenVerify, "token.verify", false, "Verify access tokens")
	flag.StringVar(&tokenMachine, "token.machine", "", "Use given machine name to verify token claims")
}

// httpServer creates a new *http.Server with explicit Read and Write
// timeouts, the provided address and handler, and an empty TLS configuration.
//
// This server can only be used with a net.Listener that returns netx.ConnInfo
// after accepting a new connection.
func httpServer(addr string, handler http.Handler) *http.Server {
	tlsconf := &tls.Config{}
	return &http.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsconf,
		// NOTE: set absolute read and write timeouts for server connections.
		// This prevents clients, or middleboxes, from opening a connection and
		// holding it open indefinitely. This applies equally to TLS and non-TLS
		// servers.
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return netx.ToConnInfo(c).SaveUUID(ctx)
		},
	}
}

func main() {
	flag.Parse()

	// Initialize logging and metrics.
	log.SetReportCaller(true)
	log.SetReportTimestamp(true)
	log.SetLevel(log.DebugLevel)

	promSrv := prometheusx.MustServeMetrics()
	defer promSrv.Close()

	v, err := token.NewVerifier(tokenVerifyKey.Get()...)
	if (tokenVerify) && err != nil {
		rtx.Must(err, "Failed to load verifier")
	}
	// Enforce tokens on uploads and downloads.
	throughput1TxPaths := controller.Paths{
		spec.DownloadPath: true,
		spec.UploadPath:   true,
	}
	throughput1TokenPaths := controller.Paths{
		spec.DownloadPath: true,
		spec.UploadPath:   true,
	}
	acm, _ := controller.Setup(ctx, v, tokenVerify, tokenMachine,
		throughput1TxPaths, throughput1TokenPaths)

	throughput1Mux := http.NewServeMux()
	throughput1Handler := handler.New(*flagDataDir)
	throughput1Mux.Handle(spec.DownloadPath, http.HandlerFunc(throughput1Handler.Download))
	throughput1Mux.Handle(spec.UploadPath, http.HandlerFunc(throughput1Handler.Upload))
	throughput1ServerCleartext := httpServer(
		*flagEndpointCleartext,
		acm.Then(throughput1Mux))

	log.Info("About to listen for ws tests", "endpoint", *flagEndpointCleartext)

	tcpl, err := net.Listen("tcp", throughput1ServerCleartext.Addr)
	rtx.Must(err, "failed to create listener")
	l := netx.NewListener(tcpl.(*net.TCPListener))
	defer l.Close()

	go func() {
		err := throughput1ServerCleartext.Serve(l)
		rtx.Must(err, "Could not start cleartext server")
		defer throughput1ServerCleartext.Close()
	}()

	// Only start TLS-based services if certs and keys are provided
	if *flagCertFile != "" && *flagKeyFile != "" {
		throughput1Server := httpServer(
			*flagEndpoint,
			acm.Then(throughput1Mux))
		log.Info("About to listen for wss tests", "endpoint", *flagEndpoint)

		tcpl, err := net.Listen("tcp", throughput1Server.Addr)
		rtx.Must(err, "failed to create listener")
		l := netx.NewListener(tcpl.(*net.TCPListener))
		defer l.Close()

		go func() {
			err := throughput1Server.ServeTLS(l, *flagCertFile, *flagKeyFile)
			rtx.Must(err, "Could not start cleartext server")
			defer throughput1Server.Close()
		}()
	}

	<-ctx.Done()
	cancel()
}
