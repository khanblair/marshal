package runctx_test

import (
	"context"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/runctx"
)

func TestARunKnowsWhenItWasDue(t *testing.T) {
	if _, late := runctx.Late(context.Background()); late {
		t.Fatal("a plain run was marked late")
	}
	due := time.Date(2026, time.October, 8, 8, 0, 0, 0, time.UTC)
	got, late := runctx.Late(runctx.WithLate(context.Background(), due))
	if !late || !got.Equal(due) {
		t.Fatalf("Late = %v, %v; want %v, true", got, late, due)
	}
}
