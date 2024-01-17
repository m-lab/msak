package spec

import "time"

const (
	// ServiceName is the service name for the Locate V2 API.
	ServiceName = "msak/latency1"

	// AuthorizeV1 is the v1 /authorize endpoint.
	AuthorizeV1 = "/latency/v1/authorize"
	// ResultV1 is the v1 /result endpoint.
	ResultV1 = "/latency/v1/result"

	// DefaultSessionCacheTTL is the default session cache TTL.
	DefaultSessionCacheTTL = 1 * time.Minute
)
