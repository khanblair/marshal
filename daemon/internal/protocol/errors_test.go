package protocol_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

func TestErrorGolden(t *testing.T) {
	err := protocol.NotFound("card").With("id", "01M3C107JB041061050R3GG28A").WithCause(errors.New("secret detail"))
	testutil.Golden(t, "error", protocol.ErrorResponse{Error: *err})
}

func TestErrorShape(t *testing.T) {
	tests := []struct {
		name string
		err  *protocol.Error
		want string
	}{
		{"no details", protocol.Unauthorized(), `{"error":{"code":"unauthorized","message":"Marshal does not recognize this device. Pair it again from the desktop app."}}`},
		{"methods that work", protocol.MethodNotAllowed("GET", "POST"), `{"error":{"code":"method_not_allowed","message":"That address does not accept this kind of request. Use one of these instead: GET, POST.","details":{"allow":"GET, POST"}}}`},
		{"with details", protocol.Refused("A card in Done cannot move back to Working.").With("from", "done"), `{"error":{"code":"refused","message":"A card in Done cannot move back to Working.","details":{"from":"done"}}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(protocol.ErrorResponse{Error: *tc.err})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestErrorNeverSendsItsCause(t *testing.T) {
	err := protocol.Internal().WithCause(errors.New("open /Users/me/secret.db: permission denied"))
	got, encodeErr := json.Marshal(protocol.ErrorResponse{Error: *err})
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if strings.Contains(string(got), "secret") || strings.Contains(string(got), "permission") {
		t.Errorf("the cause leaked into %s", got)
	}
}

func TestErrorWorksWithTheErrorsPackage(t *testing.T) {
	cause := errors.New("disk is full")
	wrapped := fmt.Errorf("save the card: %w", protocol.Unavailable("Marshal cannot save right now. Try again in a moment.").WithCause(cause))
	var perr *protocol.Error
	if !errors.As(wrapped, &perr) || perr.Code != protocol.ErrorCodeUnavailable {
		t.Fatalf("errors.As did not find the unavailable error in %v", wrapped)
	}
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is did not reach the cause")
	}
	want := "unavailable: Marshal cannot save right now. Try again in a moment.: disk is full"
	if got := perr.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := protocol.NotFound("card").Error(); got != "not_found: Marshal cannot find that card. It may have been removed." {
		t.Errorf("Error() without a cause = %q", got)
	}
}

func TestErrorMessagesFollowTheCopyRules(t *testing.T) {
	errs := []*protocol.Error{
		protocol.Unauthorized(), protocol.NotFound("project"), protocol.Internal(),
		protocol.InvalidArgument("The project name is empty. Enter a name."),
		protocol.Forbidden("This device cannot remove projects. Ask the owner."),
		protocol.Conflict("A project with that folder already exists. Open it from the sidebar."),
		protocol.Refused("A card in Done cannot move back to Working. Create a new card instead."),
		protocol.Unsupported("Merging is not available on Windows yet."),
		protocol.Unavailable("Marshal is still starting. Try again in a moment."),
		protocol.MethodNotAllowed("GET", "POST"),
	}
	sentence := regexp.MustCompile(`^[A-Z][^\n]*[.]$`)
	for _, err := range errs {
		if !sentence.MatchString(err.Message) {
			t.Errorf("%s: %q is not a plain sentence that starts with a capital and ends with a period", err.Code, err.Message)
		}
		lower := strings.ToLower(err.Message)
		for _, banned := range []string{"failed to", "oops", "sorry", "exception", "stack", "panic", "nil", "!"} {
			if strings.Contains(lower, banned) {
				t.Errorf("%s: %q contains %q", err.Code, err.Message, banned)
			}
		}
	}
}

func TestEveryConstructorUsesItsCode(t *testing.T) {
	tests := map[protocol.ErrorCode]*protocol.Error{
		protocol.ErrorCodeInvalidArgument:  protocol.InvalidArgument("x."),
		protocol.ErrorCodeUnauthorized:     protocol.Unauthorized(),
		protocol.ErrorCodeForbidden:        protocol.Forbidden("x."),
		protocol.ErrorCodeNotFound:         protocol.NotFound("x"),
		protocol.ErrorCodeMethodNotAllowed: protocol.MethodNotAllowed("GET"),
		protocol.ErrorCodeConflict:         protocol.Conflict("x."),
		protocol.ErrorCodeRefused:          protocol.Refused("x."),
		protocol.ErrorCodeUnsupported:      protocol.Unsupported("x."),
		protocol.ErrorCodeUnavailable:      protocol.Unavailable("x."),
		protocol.ErrorCodeInternal:         protocol.Internal(),
	}
	for _, code := range protocol.ErrorCodeValues() {
		err, ok := tests[code]
		if !ok || err.Code != code {
			t.Errorf("no constructor test for %q, or its constructor uses another code", code)
		}
	}
}
