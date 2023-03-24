package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
	"github.com/m-lab/access/controller"
	"github.com/m-lab/go/prometheusx"
	"github.com/m-lab/msak/internal/netx"
	"github.com/m-lab/msak/internal/persistence"
	"github.com/m-lab/msak/pkg/ndt8"
	"github.com/m-lab/msak/pkg/ndt8/model"
)

// knownOptions are the known ndt8 options.
var knownOptions = map[string]struct{}{
	"streams":  {},
	"duration": {},
	"delay":    {},
	"cc":       {},
}

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
	mid, err := getMIDFromRequest(req)
	if err != nil {
		log.Printf("Received request without mid from %s, %v\n",
			req.RemoteAddr, err)
		writeBadRequest(rw)
		return
	}

	// Read known protocol options from the querystring and validate them.
	query := req.URL.Query()
	requestStreams := query.Get("streams")
	if requestStreams == "" {
		log.Printf("Received request without streams from %s\n",
			req.RemoteAddr)
		writeBadRequest(rw)
		return
	}
	requestDuration := query.Get("duration")
	var duration = 5 * time.Second
	if d, err := strconv.Atoi(requestDuration); requestDuration != "" && err == nil {
		duration = time.Duration(d) * time.Second
	}
	requestCC := query.Get("cc")
	requestDelay := query.Get("delay")

	// Read metadata (i.e. everything in the querystring that's not a known
	// option).
	metadata, err := getRequestMetadata(req)
	if err != nil {
		log.Info("Error while parsing metadata", "error", err)
		writeBadRequest(rw)
		return
	}

	// Everything looks good, try upgrading the connection to WebSocket.
	// Once upgraded, the underlying TCP connection is hijacked and the ndt8
	// protocol code will take care of closing it. Note that for this reason
	// we cannot call writeBadRequest after attempting an Upgrade.
	wsConn, err := ndt8.Upgrade(rw, req)
	if err != nil {
		log.Info("Websocket upgrade failed", "error", err)
		return
	}

	// Now that the connection has been upgraded to WebSocket, we get access to
	// the underlying TCP connection. If this is not a netx.Conn, it means the
	// server was not initialized correctly and the following line will panic.
	conn := netx.ToConnInfo(wsConn.UnderlyingConn())

	// If a congestion control algorithm was requested, attempt to set it here.
	// Errors are not fatal: for example, the client might have requested a
	// congestion control algorithm that's not available on this system. In
	// this case, we should still run with the default and record the requested
	// vs/ actual CC used in the archival data.
	if requestCC != "" {
		err = conn.SetCC(requestCC)
		if err != nil {
			log.Info("Failed to set cc (ctx: %p, cc: %s): %v\n", req.Context(),
				requestCC, err)
		}
	}

	uuid, err := conn.UUID()
	if err != nil {
		// UUID() has a fallback that won't ever fail. This should not happen.
		log.Printf("Failed to read UUID (ctx: %p): %v\n", req.Context(), err)
		wsConn.Close()
		return
	}
	archivalData := model.NDT8Result{
		MeasurementID:  mid,
		UUID:           uuid,
		StartTime:      time.Now(),
		Server:         wsConn.UnderlyingConn().LocalAddr().String(),
		Client:         wsConn.UnderlyingConn().RemoteAddr().String(),
		Direction:      string(kind),
		GitShortCommit: prometheusx.GitShortCommit,
		Version:        "v0.0.1",
		ClientMetadata: metadata,
		ClientOptions: []model.NameValue{
			{Name: "streams", Value: requestStreams},
			{Name: "duration", Value: requestDuration},
			{Name: "delay", Value: requestDelay},
			{Name: "cc", Value: requestCC},
		},
	}
	defer func() {
		archivalData.EndTime = time.Now()
		h.writeResult(uuid, kind, &archivalData)
	}()

	// Set the runtime to the requested duration.
	timeout, cancel := context.WithTimeout(req.Context(),
		time.Duration(duration)*time.Millisecond)
	defer cancel()

	proto := ndt8.New(wsConn)
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
				log.Printf("Connection closed unexpectedly: %v\n", err)
				// TODO: Add Prometheus metric
			}
			return
		}
	}
}

func (h *Handler) writeResult(uuid string, kind model.TestDirection, result *model.NDT8Result) {
	_, err := persistence.WriteDataFile(
		h.archivalDataDir, "ndt8", string(kind), uuid,
		result)
	if err != nil {
		log.Printf("failed to write ndt8 result: %v\n", err)
		return
	}
}

// getMIDFromRequest extracts the measurement id ("mid") from a given HTTP
// request, if present.
//
// A measurement ID can be specified in two ways: via a "mid" querystring
// parameter (when access tokens are not required) or via the ID field
// in the JWT access token.
func getMIDFromRequest(req *http.Request) (string, error) {
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
		// This maximum length for keys and values is meant to limit abuse.
		if len(k) > 50 || len(v[0]) > 512 {
			return nil, errors.New("maximum key or value length exceeded")
		}
		// Filter known options.
		if _, ok := knownOptions[k]; !ok {
			filtered = append(filtered, model.NameValue{
				Name:  k,
				Value: v[0],
			})
		}
	}
	return filtered, nil
}
