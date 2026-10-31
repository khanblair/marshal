package integrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const resendKey = "re_integration_test_key_987"

// resendFake answers Resend's two calls. It returns the domains it is given for a read, and the
// status and body it is given for a send, and it counts the sends so a test can see that a
// connection test sent nothing.
type resendFake struct {
	mu         sync.Mutex
	domains    string
	domainCode int
	sendCode   int
	sendBody   string
	sends      []string
}

func (f *resendFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+resendKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"API key is invalid"}`)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/domains":
		w.WriteHeader(f.domainCode)
		_, _ = io.WriteString(w, f.domains)
	case r.Method == http.MethodPost && r.URL.Path == "/emails":
		raw, _ := io.ReadAll(r.Body)
		f.sends = append(f.sends, string(raw))
		w.WriteHeader(f.sendCode)
		_, _ = io.WriteString(w, f.sendBody)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *resendFake) sent() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

// resendSetup starts the fake and an integrations service that sends to owner@example.com.
func resendSetup(t *testing.T, fake *resendFake) *fixture {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return newFixture(t, func(o *integrations.Options) {
		o.ResendBaseURL = srv.URL
		o.ResendRecipient = func(context.Context) (string, error) { return "owner@example.com", nil }
	})
}

func saveResend(t *testing.T, f *fixture, from string) {
	t.Helper()
	if err := f.svc.SaveResend(context.Background(), protocol.SaveResendRequest{Key: resendKey, From: from}); err != nil {
		t.Fatalf("SaveResend: %v", err)
	}
}

// noKeyAnywhere fails the test if the key is in what a route would send back.
func noKeyAnywhere(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), resendKey) {
		t.Errorf("the key is in an answer: %s", raw)
	}
}

func TestResendTestReadsTheAccountAndNeverSends(t *testing.T) {
	fake := &resendFake{
		domains:    `{"data":[{"name":"kolaborate.co","status":"verified"}]}`,
		domainCode: http.StatusOK,
	}
	f := resendSetup(t, fake)
	saveResend(t, f, "Marshal <marshal@kolaborate.co>")

	result, err := f.svc.Test(context.Background(), integrations.ResendID)
	if err != nil || !result.OK {
		t.Fatalf("the Resend test answered %+v (%v), want a pass", result, err)
	}
	if fake.sent() != 0 {
		t.Errorf("the connection test sent %d email(s), want none", fake.sent())
	}
	noKeyAnywhere(t, result)
}

func TestResendTestFlagsASenderOnAnUnverifiedDomain(t *testing.T) {
	fake := &resendFake{
		domains:    `{"data":[{"name":"kolaborate.co","status":"pending"}]}`,
		domainCode: http.StatusOK,
	}
	f := resendSetup(t, fake)
	saveResend(t, f, "Marshal <marshal@kolaborate.co>")

	result, err := f.svc.Test(context.Background(), integrations.ResendID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK {
		t.Fatalf("the test passed for a domain that is not verified: %+v", result)
	}
	noKeyAnywhere(t, result)
}

func TestResendTestSaysWhenTheKeyIsRefused(t *testing.T) {
	fake := &resendFake{domainCode: http.StatusOK}
	f := resendSetup(t, fake)
	if err := f.svc.SaveResend(context.Background(), protocol.SaveResendRequest{Key: "re_wrong", From: "marshal@kolaborate.co"}); err != nil {
		t.Fatalf("SaveResend: %v", err)
	}

	result, err := f.svc.Test(context.Background(), integrations.ResendID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if result.OK || !strings.Contains(strings.Join(checkMessages(result), " "), "does not accept") {
		t.Errorf("result = %+v, want a refused key named as such", result)
	}
	noKeyAnywhere(t, result)
}

func TestResendSaveNeedsASenderAndKeepsTheKeyWhenLeftOut(t *testing.T) {
	fake := &resendFake{domainCode: http.StatusOK, domains: `{"data":[{"name":"kolaborate.co","status":"verified"}]}`}
	f := resendSetup(t, fake)
	ctx := context.Background()
	if err := f.svc.SaveResend(ctx, protocol.SaveResendRequest{Key: resendKey, From: "not-an-address"}); err == nil {
		t.Fatal("a sender with no @ was saved")
	}
	saveResend(t, f, "marshal@kolaborate.co")
	if err := f.svc.SaveResend(ctx, protocol.SaveResendRequest{Key: "", From: "other@kolaborate.co"}); err != nil {
		t.Fatalf("changing the sender without the key: %v", err)
	}
	result, err := f.svc.Test(ctx, integrations.ResendID)
	if err != nil || !result.OK {
		t.Errorf("after changing only the sender, the test answered %+v (%v), want the kept key to pass", result, err)
	}
}

func TestSendEmailGoesToTheProfileAddressAndSkipsWithoutOne(t *testing.T) {
	fake := &resendFake{sendCode: http.StatusOK, sendBody: `{"id":"msg_1"}`}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	address := ""
	f := newFixture(t, func(o *integrations.Options) {
		o.ResendBaseURL = srv.URL
		o.ResendRecipient = func(context.Context) (string, error) { return address, nil }
	})
	saveResend(t, f, "marshal@kolaborate.co")
	ctx := context.Background()

	if err := f.svc.SendEmail(ctx, "Brief", "Hello"); !errors.Is(err, integrations.ErrNoRecipient) {
		t.Fatalf("SendEmail with no profile address = %v, want ErrNoRecipient", err)
	}
	if fake.sent() != 0 {
		t.Errorf("an email went out with no address")
	}
	address = "owner@example.com"
	if err := f.svc.SendEmail(ctx, "Brief", "Hello"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if fake.sent() != 1 || !strings.Contains(fake.sends[0], "owner@example.com") {
		t.Errorf("sends = %v, want one to the profile address", fake.sends)
	}
}

func TestSendResendTestShowsResendsReasonAndNeverTheKey(t *testing.T) {
	fake := &resendFake{
		sendCode: http.StatusForbidden,
		sendBody: `{"message":"You can only send testing emails to your own email address. verify a domain"}`,
	}
	f := resendSetup(t, fake)
	saveResend(t, f, "onboarding@resend.dev")

	result, err := f.svc.SendResendTest(context.Background())
	if err != nil {
		t.Fatalf("SendResendTest: %v", err)
	}
	if result.OK || !strings.Contains(strings.Join(checkMessages(result), " "), "verify a domain") {
		t.Errorf("result = %+v, want the refusal shown", result)
	}
	noKeyAnywhere(t, result)
}

// checkMessages is every check's message in a result, for a test to search.
func checkMessages(result protocol.TestResult) []string {
	var out []string
	for _, check := range result.Checks {
		out = append(out, check.Message)
	}
	return out
}

// A sending-only key cannot list domains, but it is a good key: the test says so, and it is not a failure.
func TestResendTestAcceptsASendingOnlyKeyWithAWarning(t *testing.T) {
	fake := &resendFake{
		domainCode: http.StatusUnauthorized,
		domains:    `{"name":"restricted_api_key","message":"This API key is restricted to only send emails"}`,
	}
	f := resendSetup(t, fake)
	saveResend(t, f, "Marshal <marshal@kolaborate.co>")

	result, err := f.svc.Test(context.Background(), integrations.ResendID)
	if err != nil || !result.OK {
		t.Fatalf("the test answered %+v (%v), want a pass for a sending-only key", result, err)
	}
	if !strings.Contains(strings.Join(checkMessages(result), " "), "sending-only") {
		t.Errorf("result = %+v, want the sending-only key named", result)
	}
	noKeyAnywhere(t, result)
}

// A second press inside the cooldown sends nothing, so a script cannot mail the owner in a loop.
func TestTheTestEmailHasACooldown(t *testing.T) {
	fake := &resendFake{sendCode: http.StatusOK, sendBody: `{"id":"msg_1"}`}
	f := resendSetup(t, fake)
	saveResend(t, f, "marshal@kolaborate.co")
	ctx := context.Background()

	first, err := f.svc.SendResendTest(ctx)
	if err != nil || !first.OK {
		t.Fatalf("the first send answered %+v (%v), want a pass", first, err)
	}
	second, err := f.svc.SendResendTest(ctx)
	if err != nil {
		t.Fatalf("the second send: %v", err)
	}
	if second.OK || fake.sent() != 1 {
		t.Errorf("second = %+v with %d sent, want a refusal and only the first mail", second, fake.sent())
	}
}

// Anything but a refusal that names the key or the domain is shown with what Resend said.
func TestAnInvalidSenderShowsResendsReasonNotAConnectionProblem(t *testing.T) {
	fake := &resendFake{sendCode: http.StatusUnprocessableEntity, sendBody: `{"message":"Invalid from field"}`}
	f := resendSetup(t, fake)
	saveResend(t, f, "marshal@kolaborate.co")

	result, err := f.svc.SendResendTest(context.Background())
	if err != nil {
		t.Fatalf("SendResendTest: %v", err)
	}
	joined := strings.Join(checkMessages(result), " ")
	if result.OK || !strings.Contains(joined, "Invalid from field") || strings.Contains(joined, "could not reach") {
		t.Errorf("result = %+v, want Resend's reason and no claim that Resend was unreachable", result)
	}
}

// The saved sender is shown back as the connection's target, so the dialog can open with it filled in.
func TestTheSavedSenderIsTheRowsTarget(t *testing.T) {
	f := resendSetup(t, &resendFake{domainCode: http.StatusOK, domains: `{"data":[]}`})
	saveResend(t, f, "Marshal <marshal@kolaborate.co>")
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range list {
		if row.ID == integrations.ResendID {
			if row.Target != "Marshal <marshal@kolaborate.co>" {
				t.Errorf("target = %q, want the saved sender", row.Target)
			}
			noKeyAnywhere(t, row)
			return
		}
	}
	t.Fatal("the list has no Resend row")
}

// A sender left empty is Resend's own address, which the connection test then flags as reaching the
// account owner only.
func TestAnEmptySenderIsResendsOwnAddress(t *testing.T) {
	fake := &resendFake{domainCode: http.StatusOK, domains: `{"data":[]}`}
	f := resendSetup(t, fake)
	if err := f.svc.SaveResend(context.Background(), protocol.SaveResendRequest{Key: resendKey, From: "  "}); err != nil {
		t.Fatalf("SaveResend with no sender: %v", err)
	}
	list, err := f.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, row := range list {
		if row.ID == integrations.ResendID && row.Target != integrations.DefaultResendFrom {
			t.Errorf("target = %q, want Resend's own address %q", row.Target, integrations.DefaultResendFrom)
		}
	}
	result, err := f.svc.Test(context.Background(), integrations.ResendID)
	if err != nil || !result.OK {
		t.Fatalf("the test answered %+v (%v), want a pass with a warning", result, err)
	}
	if !strings.Contains(strings.Join(checkMessages(result), " "), "only reaches the account owner") {
		t.Errorf("result = %+v, want the owner-only warning for Resend's address", result)
	}
}
