package claude

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestPermissionCallbackRoundTrip(t *testing.T) {
	for _, choice := range []string{"allow", "deny", "cancel"} {
		t.Run(choice, func(t *testing.T) {
			a := fakeAdapter(t, "permission")
			h := start(t, a, startSpec(t))
			ctx := context.Background()
			if err := a.Send(ctx, h, agents.UserMessage{Text: "write a file"}); err != nil {
				t.Fatal(err)
			}
			var request agents.PermissionRequested
			select {
			case event := <-a.Events(h):
				var ok bool
				request, ok = event.(agents.PermissionRequested)
				if !ok {
					t.Fatalf("event = %T, want permission", event)
				}
			case <-time.After(eventTimeout):
				t.Fatal("no permission request")
			}
			if request.Kind != "edit" || request.Path == "" || request.ToolCallID != "write-1" {
				t.Fatalf("request = %+v", request)
			}
			if err := a.Respond(ctx, h, agents.ApprovalResponse{RequestID: request.RequestID, OptionID: "invalid"}); !errors.Is(err, agents.ErrUnknownRequest) {
				t.Fatalf("invalid choice = %v", err)
			}
			response := agents.ApprovalResponse{RequestID: request.RequestID, OptionID: choice, Cancelled: choice == "cancel"}
			if err := a.Respond(ctx, h, response); err != nil {
				t.Fatal(err)
			}
			tr := collect(t, a.Events(h))
			want := choice
			if choice == "cancel" {
				want = "deny"
			}
			if tr.text() != want || tr.ended == nil {
				t.Fatalf("response = %+v, want %s", tr, want)
			}
			if err := a.Respond(ctx, h, response); !errors.Is(err, agents.ErrUnknownRequest) {
				t.Fatalf("duplicate choice = %v", err)
			}
		})
	}
}
