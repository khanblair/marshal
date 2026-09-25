package main

import "time"

// sample is what one look at a running process shows.
type sample struct {
	// rssBytes is the memory the process holds in RAM.
	rssBytes uint64
	// cpu is the total processor time the process has used so far.
	cpu time.Duration
}
