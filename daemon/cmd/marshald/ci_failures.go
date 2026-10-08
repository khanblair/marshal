package main

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/dashboard"
)

// ciFailures hands the CI module's newly failed runs to Home's daily numbers, which count them.
type ciFailures struct{ home *dashboard.Subscriber }

func (c ciFailures) CIFailed(ctx context.Context, failure ci.Failure) {
	c.home.CIFailed(ctx, dashboard.CIFailure{
		ProjectID: failure.ProjectID, CardID: failure.CardID, CardKey: failure.CardKey,
		CardTitle: failure.CardTitle, Branch: failure.Branch, Workflow: failure.Workflow,
	})
}
