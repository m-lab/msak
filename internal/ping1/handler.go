package ping1

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/log"

	"github.com/m-lab/msak/pkg/ping1"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) HandlePing(rw http.ResponseWriter,
	req *http.Request) {
	wsConn, err := ping1.Upgrade(rw, req)
	if err != nil {
		log.Info("Websocket upgrade failed",
			"ctx", fmt.Sprintf("%p", req.Context()), "error", err)
		return
	}

	duration := ping1.DefaultDuration

	// If the duration is specified in the query string, use that instead.
	durationStr := req.URL.Query().Get("duration")
	if durationStr != "" {
		d, err := time.ParseDuration(durationStr)
		if err == nil {
			duration = d
		} else {
			log.Info("Invalid duration", "duration", d)
		}
	}

	// Set the runtime to the expected duration.
	timeout, cancel := context.WithTimeout(req.Context(), duration)
	defer cancel()

	proto := ping1.New(wsConn)
	proto.Start(timeout)
}
