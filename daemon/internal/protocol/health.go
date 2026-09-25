package protocol

// Health is the answer to GET /v1/health.
type Health struct {
	// Status is "ok" while the daemon is serving.
	Status string `json:"status"`
	// Version is the Marshal version.
	Version string `json:"version"`
	// Mode is "normal" or "dev".
	Mode string `json:"mode"`
	// ServerTime is the daemon's current time. Clients show ages and countdowns from it, so a
	// clock that is a little off never shows a wrong "4 min ago".
	ServerTime Timestamp `json:"serverTime"`
}
