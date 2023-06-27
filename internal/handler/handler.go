package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/internal/persistence"
	"github.com/m-lab/msak/pkg/throughput1"
	"github.com/m-lab/msak/pkg/throughput1/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// knownOptions are the known throughput1 options.
var knownOptions = map[string]struct{}{
	"streams":      {},
	"duration":     {},
	"delay":        {},
	"cc":           {},
	"access_token": {},
	"mid":          {},
}

// validCCAlgorithms are the allowed congestion control algorithms.
var validCCAlgorithms = map[string]struct{}{
	"reno":  {},
	"cubic": {},
	"bbr":   {},
}

var (
	ClientConnections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "msak",
			Subsystem: "throughput1",
			Name:      "client_connections_total",
		},
		[]string{"direction", "status"},
	)
)

type Handler struct {
	archivalDataDir string
}

func New(archivalDataDir string) *Handler {
	return &Handler{
		archivalDataDir: archivalDataDir,
	}
}

func (h *Handler) Download(rw http.ResponseWriter, req *http.Request) {
	h.upgradeAndRunMeasurement(model.DirectionDownload, rw, req)
}

func (h *Handler) Upload(rw http.ResponseWriter, req *http.Request) {
	h.upgradeAndRunMeasurement(model.DirectionUpload, rw, req)
}

func (h *Handler) upgradeAndRunMeasurement(kind model.TestDirection, rw http.ResponseWriter,
	req *http.Request) {
	mid, err := GetMIDFromRequest(req)
	if err != nil {
		ClientConnections.WithLabelValues(string(kind), "missing-mid").Inc()
		log.Info("Received request without mid", "source", req.RemoteAddr,
			"error", err)
		writeBadRequest(rw)
		return
	}

	// Read known protocol options from the querystring and validate them.
	clientOptions := []model.NameValue{}
	query := req.URL.Query()
	requestStreams := query.Get("streams")
	if requestStreams == "" {
		ClientConnections.WithLabelValues(string(kind),
			"missing-streams").Inc()
		log.Info("Received request without streams", "source", req.RemoteAddr)
		writeBadRequest(rw)
		return
	}
	clientOptions = append(clientOptions,
		model.NameValue{Name: "streams", Value: requestStreams})

	requestDuration := query.Get("duration")
	var duration = 5 * time.Second
	if requestDuration != "" {
		if d, err := strconv.Atoi(requestDuration); err == nil {
			// Note: the provided duration must be milliseconds.
			duration = time.Duration(d) * time.Millisecond
			clientOptions = append(clientOptions,
				model.NameValue{Name: "duration", Value: requestDuration})
		} else {
			ClientConnections.WithLabelValues(string(kind),
				"invalid-duration").Inc()
			log.Info("Received request with an invalid duration",
				"source", req.RemoteAddr, "duration", requestDuration)
			writeBadRequest(rw)
			return
		}
	}

	requestCC := query.Get("cc")
	// Check that the requested CC algorithm is allowed. Note that we cannot
	// set it here since we don't have a net.Conn yet.
	if requestCC != "" {
		if _, ok := validCCAlgorithms[requestCC]; !ok {
			log.Info("Requested CC algorithm is not allowed",
				"source", req.RemoteAddr, "cc", requestCC)
			writeBadRequest(rw)
			return
		}
		clientOptions = append(clientOptions,
			model.NameValue{Name: "cc", Value: requestCC})
	}

	requestDelay := query.Get("delay")
	if requestDelay != "" {
		clientOptions = append(clientOptions,
			model.NameValue{Name: "delay", Value: requestDelay})
	}

	// Read metadata (i.e. everything in the querystring that's not a known
	// option).
	metadata, err := getRequestMetadata(req)
	if err != nil {
		ClientConnections.WithLabelValues(string(kind),
			"metadata-parse-error").Inc()
		log.Info("Error while parsing metadata", "source", req.RemoteAddr,
			"error", err)
		writeBadRequest(rw)
		return
	}

	// Everything looks good, try upgrading the connection to WebSocket.
	// Once upgraded, the underlying TCP connection is hijacked and the throughput1
	// protocol code will take care of closing it. Note that for this reason
	// we cannot call writeBadRequest after attempting an Upgrade.
	wsConn, err := throughput1.Upgrade(rw, req)
	if err != nil {
		ClientConnections.WithLabelValues(string(kind),
			"websocket-upgrade-failed").Inc()
		log.Info("Websocket upgrade failed",
			"ctx", fmt.Sprintf("%p", req.Context()), "error", err)
		return
	}

	// Now that the connection has been upgraded to WebSocket, we get access to
	// the underlying TCP connection. If this is not a netx.Conn, it means the
	// server was not initialized correctly and the following line will panic.
	conn := netx.ToConnInfo(wsConn.UnderlyingConn())

	// If a congestion control algorithm was requested, attempt to set it here.
	// This can only be done after upgrading the connection.
	// Errors are not fatal: for example, the client might have requested a
	// congestion control algorithm that's not available on this system. In
	// this case, we should still run with the default and record the requested
	// vs/ actual CC used in the archival data.
	if requestCC != "" {
		err = conn.SetCC(requestCC)
		if err != nil {
			log.Info("Failed to set cc", "ctx", fmt.Sprintf("%p", req.Context()),
				"source", wsConn.RemoteAddr(),
				"cc", requestCC, "error", err)
		}
	}

	uuid := conn.UUID()
	archivalData := model.Throughput1Result{
		MeasurementID:  mid,
		UUID:           uuid,
		StartTime:      time.Now(),
		Server:         wsConn.UnderlyingConn().LocalAddr().String(),
		Client:         wsConn.UnderlyingConn().RemoteAddr().String(),
		Direction:      string(kind),
		GitShortCommit: prometheusx.GitShortCommit,
		Version:        "v0.0.1",
		ClientMetadata: metadata,
		ClientOptions:  clientOptions,
	}
	defer func() {
		archivalData.EndTime = time.Now()
		h.writeResult(uuid, kind, &archivalData)
	}()

	// Set the runtime to the requested duration.
	timeout, cancel := context.WithTimeout(req.Context(), duration)
	defer cancel()

	proto := throughput1.New(wsConn)
	var senderCh, receiverCh <-chan model.WireMeasurement
	var errCh <-chan error
	if kind == model.DirectionDownload {
		senderCh, receiverCh, errCh = proto.SenderLoop(timeout)
	} else {
		senderCh, receiverCh, errCh = proto.ReceiverLoop(timeout)
	}

	for {
		select {
		case <-timeout.Done():
			return
		case m := <-senderCh:
			// If this is a download test we are the sender, so we can populate
			// CCAlgorithm as soon as it's sent out at least once.
			if kind == model.DirectionDownload && m.CC != "" {
				archivalData.CCAlgorithm = m.CC
			}
			archivalData.ServerMeasurements = append(
				archivalData.ServerMeasurements, m.Measurement)
		case m := <-receiverCh:
			// Same for upload tests, but in this case the sender is the
			// client. If the client ever sends the CC it's using, save it.
			if kind == model.DirectionUpload && m.CC != "" {
				archivalData.CCAlgorithm = m.CC
			}
			archivalData.ClientMeasurements = append(archivalData.ClientMeasurements,
				m.Measurement)
		case err := <-errCh:
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
				log.Info("Connection closed unexpectedly", "context",
					fmt.Sprintf("%p", timeout), "error", err)
				// TODO: Add Prometheus metric
			}
			return
		}
	}
}

func (h *Handler) writeResult(uuid string, kind model.TestDirection, result *model.Throughput1Result) {
	_, err := persistence.WriteDataFile(
		h.archivalDataDir, "throughput1", string(kind), uuid,
		result)
	if err != nil {
		log.Error("failed to write throughput1 result", "uuid", uuid, "error", err)
		return
	}
}

// GetMIDFromRequest extracts the measurement id ("mid") from a given HTTP
// request, if present.
//
// A measurement ID can be specified in two ways: via a "mid" querystring
// parameter (when access tokens are not required) or via the ID field
// in the JWT access token.
func GetMIDFromRequest(req *http.Request) (string, error) {
	// If the request includes a valid JWT token, the claim and the ID are in
	// the request's context already.
	claims := controller.GetClaim(req.Context())
	if claims != nil {
		return claims.ID, nil
	}

	// Otherwise, try getting the "mid" querystring parameter.
	if mid := req.URL.Query().Get("mid"); mid != "" {
		return mid, nil
	}

	return "", errors.New("no valid token nor mid found in the request")
}

// writeBadRequest sends a Bad Request response to the client using writer.
func writeBadRequest(writer http.ResponseWriter) {
	writer.WriteHeader(http.StatusBadRequest)
	writer.Header().Set("Connection", "Close")
}

func getRequestMetadata(req *http.Request) ([]model.NameValue, error) {
	// "metadata" in this context refers to any querystring parameter that is
	// not recognized as option.
	query := req.URL.Query()
	filtered := []model.NameValue{}
	for k, v := range query {
		// Ignore known options.
		if _, ok := knownOptions[k]; !ok {
			// This maximum length for keys and values is meant to limit abuse.
			if len(k) > 50 || len(v[0]) > 512 {
				return nil, errors.New("maximum key or value length exceeded")
			}
			filtered = append(filtered, model.NameValue{
				Name:  k,
				Value: v[0],
			})
		}
	}
	return filtered, nil
}
