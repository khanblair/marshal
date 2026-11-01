package protocol

// RetryDoingNowPrefix starts the "doing now" line of a card whose agent is waiting for the network,
// such as "Waiting for network, retry 3 of 10". A screen shows a line that starts with it as a badge.
const RetryDoingNowPrefix = "Waiting for network"

// NetworkStatus is whether this computer can reach the internet. The daemon checks only after a call
// failed, so Online stays true until something reports a failure the check can repeat.
type NetworkStatus struct {
	// Online says the internet is reachable.
	Online bool `json:"online"`
	// Since is when Online last changed.
	Since Timestamp `json:"since"`
}
