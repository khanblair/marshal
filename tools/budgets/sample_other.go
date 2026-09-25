//go:build !darwin && !linux && !windows

package main

import (
	"context"
	"errors"
)

func takeSample(context.Context, int) (sample, error) {
	return sample{}, errors.New("measuring a process is not supported on this operating system")
}
