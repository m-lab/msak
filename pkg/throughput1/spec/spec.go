// Package spec contains constants for the throughput1 protocol.
package spec

import "time"

const (
	// MinMessagesize is the initial size of a Websocket binary message during
	// an throughput1 test.
	MinMessageSize = 1 << 10

	// MaxScaledMessageSize is the maximum value of a scaled binary WebSocket
	// message size. This should be <= of MaxScaledMessageSize. The 1<<20 value is
	// a good compromise between Go and JavaScript as seen in cloud based tests.
	MaxScaledMessageSize = 1 << 20

	// MinMeasureInterval is the minimum interval between subsequent measurements.
	MinMeasureInterval = 100 * time.Millisecond

	// AvgMeasureInterval is the average interval between subsequent measurements.
	AvgMeasureInterval = 250 * time.Millisecond

	// MaxMeasureInterval is the maximum interval between subsequent measurements.
	MaxMeasureInterval = 400 * time.Millisecond

	// ScalingFraction sets the threshold for scaling binary messages. When
	// the current binary message size is <= than 1/scalingFactor of the
	// amount of bytes sent so far, we scale the message.
	ScalingFraction = 16

	// DownloadPath selects the download subtest.
	DownloadPath = "/msak/v1/download"
	// UploadPath selects the upload subtest.
	UploadPath = "/mask/v1/upload"

	// MaxRuntime is the maximum runtime of a subtest.
	MaxRuntime = 15 * time.Second

	// SecWebSocketProtocol is the value of the Sec-WebSocket-Protocol header.
	SecWebSocketProtocol = "net.measurementlab.msak.v1"
)

// SubtestKind indicates the subtest kind
type SubtestKind string

const (
	// SubtestDownload is a download subtest
	SubtestDownload = SubtestKind("download")

	// SubtestUpload is a upload subtest
	SubtestUpload = SubtestKind("upload")
)
