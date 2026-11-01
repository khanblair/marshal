package integrations

// Resend is the email connection: Marshal sends its briefs by email through Resend, to the address
// the person gave in their profile (docs/architecture.md section 18). It only sends. A connection
// test reads the account and sends nothing; the one live send is a separate action the person asks
// for.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/resend"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// checkSender names the check of the sender address in a Resend test.
const checkSender = "Sender"

// ResendID is the Resend connection's own id, and the id its row, its keychain entry, and its test
// result are filed under.
const ResendID = "resend"

// KindResend is the kind the Resend connection's row and its test are filed under.
const KindResend = "resend"

// DefaultResendFrom is the sender used when none is given: Resend's own test address. It works on any
// account, but it only delivers to the account owner's address until a domain is verified.
const DefaultResendFrom = "Marshal <onboarding@resend.dev>"

// resendTestDomain is Resend's own sender. It works on any account, but it only delivers to the
// account owner's address until a domain is verified.
const resendTestDomain = "resend.dev"

// resendConfig is the connection's settings in its row. The key is not here.
type resendConfig struct {
	// From is the sender address mail is sent from.
	From string `json:"from"`
}

// resendSecrets is what the keychain holds for the connection.
type resendSecrets struct {
	// Key is the Resend API key.
	Key string `json:"key"`
}

// ErrNoRecipient is what a send answers when the person has no email address in their profile. It
// is a reason to skip the email, not a failure of the connection.
var ErrNoRecipient = errors.New("there is no email address in your profile")

// SaveResend stores the Resend connection, replacing whatever was there. A key left out keeps the
// one already saved. The key goes into the keychain before the settings go into the row, for the
// same reason the chat connections do: a key with no sender is inert, and a sender with no key is a
// connection whose every send fails.
func (s *Service) SaveResend(ctx context.Context, req protocol.SaveResendRequest) error {
	from := strings.TrimSpace(req.From)
	if from == "" {
		from = DefaultResendFrom
	}
	if !strings.Contains(from, "@") {
		return protocol.InvalidArgument("Enter the address Marshal should send email from, such as marshal@your-domain.com.").With("field", "from")
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		_, stored, err := s.readResend(ctx)
		if err != nil {
			return err
		}
		key = stored.Key
	}
	if key == "" {
		return protocol.InvalidArgument("Paste the Resend API key.").With("field", "key")
	}
	secrets, err := json.Marshal(resendSecrets{Key: key})
	if err != nil {
		return fmt.Errorf("write the Resend connection's secrets: %w", err)
	}
	if err := s.keys.Set(ResendID, string(secrets)); err != nil {
		return fmt.Errorf("save the Resend connection's key in the keychain: %w", err)
	}
	config, err := json.Marshal(resendConfig{From: from})
	if err != nil {
		return fmt.Errorf("write the Resend connection's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: ResendID, Kind: KindResend, ConfigJSON: string(config), KeychainRef: ResendID,
		})
	})
	if err != nil {
		return fmt.Errorf("save the Resend connection's settings: %w", err)
	}
	return nil
}

// readResend reads the Resend settings and key. Nothing saved answers empty values, not an error.
func (s *Service) readResend(ctx context.Context) (resendConfig, resendSecrets, error) {
	return readConnection[resendConfig, resendSecrets](ctx, s, ResendID, "Resend")
}

// resendClient builds the client from what is saved. It answers ErrNotConnected when no key or no
// sender is saved, which is the ordinary state of a daemon nobody has connected Resend to.
func (s *Service) resendClient(ctx context.Context) (*resend.Client, error) {
	config, secrets, err := s.readResend(ctx)
	if err != nil {
		return nil, err
	}
	if config.From == "" || secrets.Key == "" {
		return nil, ErrNotConnected
	}
	client, err := resend.New(resend.Config{
		BaseURL: s.resendBase, Key: secrets.Key, From: config.From, HTTPClient: s.resendHTTP,
	})
	if err != nil {
		return nil, fmt.Errorf("build the Resend client: %w", err)
	}
	return client, nil
}

// SendEmail sends one email to the address in the person's profile. It is what a brief uses for
// the email channel. A profile with no address is ErrNoRecipient, which the brief reports as a skip.
func (s *Service) SendEmail(ctx context.Context, subject, text string) error {
	client, err := s.resendClient(ctx)
	if err != nil {
		return err
	}
	to, err := s.recipient(ctx)
	if err != nil {
		return err
	}
	if to == "" {
		return ErrNoRecipient
	}
	_, err = client.Send(ctx, to, subject, text)
	return err
}

// SendResendTest sends one test email to the profile address and answers what happened. It is the
// only live send in the Resend connection, and it runs only when the person presses Send test email.
func (s *Service) SendResendTest(ctx context.Context) (protocol.TestResult, error) {
	const name = "Send test email"
	if wait := s.resendCooldown(); wait > 0 {
		return protocol.NewTestResult(ResendID, []protocol.TestCheck{{
			Name: name, State: protocol.CheckStateFailed,
			Message: "A test email was just sent.",
			Fix:     fmt.Sprintf("Wait %d seconds before sending another.", int(wait.Seconds())+1),
		}}, s.now()), nil
	}
	client, err := s.resendClient(ctx)
	if errors.Is(err, ErrNotConnected) {
		return protocol.NewTestResult(ResendID, []protocol.TestCheck{{
			Name: name, State: protocol.CheckStateFailed,
			Message: "Resend is not connected, so there is nothing to send with.",
			Fix:     "Save the Resend key and the sender address first.",
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}
	to, err := s.recipient(ctx)
	if err != nil {
		return protocol.TestResult{}, err
	}
	if to == "" {
		return protocol.NewTestResult(ResendID, []protocol.TestCheck{{
			Name: name, State: protocol.CheckStateFailed,
			Message: "Your profile has no email address, so there is nowhere to send to.",
			Fix:     "Add your email in Settings under Profile, then send again.",
		}}, s.now()), nil
	}
	_, sendErr := client.Send(ctx, to, "Marshal test email", "This is a test from Marshal. If you read it, email from Marshal reaches this address.")
	if sendErr != nil {
		message, fix := resendFailure(sendErr)
		return protocol.NewTestResult(ResendID, []protocol.TestCheck{{
			Name: name, State: protocol.CheckStateFailed, Message: message, Fix: fix,
		}}, s.now()), nil
	}
	return protocol.NewTestResult(ResendID, []protocol.TestCheck{{
		Name: name, State: protocol.CheckStatePassed, Message: "Sent to " + to + ".",
	}}, s.now()), nil
}

// resendCooldown is how long a test email must wait after the last one. It stops a script or a
// second device from mailing the person in a loop, and it counts the attempt, not the success.
const resendCooldown = 10 * time.Second

// resendCooldown claims the send slot. It answers how long the caller still has to wait, and zero
// when the slot was free, in which case the time is recorded.
func (s *Service) resendCooldown() time.Duration {
	s.resendMu.Lock()
	defer s.resendMu.Unlock()
	now := s.now()
	if wait := s.lastResendTest.Add(resendCooldown).Sub(now); wait > 0 {
		return wait
	}
	s.lastResendTest = now
	return 0
}

// testResend is the connection test. It reads the account and checks the sender's domain. It never
// sends mail: a test that sends on every press would mail the person each time they check.
func (s *Service) testResend(ctx context.Context, info Info) (protocol.TestResult, error) {
	config, secrets, err := s.readResend(ctx)
	if err != nil {
		return protocol.TestResult{}, err
	}
	if config.From == "" || secrets.Key == "" {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed,
			Message: "Marshal has no Resend key or sender address saved, so it cannot send email.",
			Fix:     "Add the Resend key and the sender address under Integrations.",
		}}, s.now()), nil
	}
	client, err := s.resendClient(ctx)
	if err != nil {
		return protocol.TestResult{}, err
	}
	domains, checkErr := client.Check(ctx)
	if resend.Restricted(checkErr) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{
			{Name: "API key", State: protocol.CheckStatePassed,
				Message: "Resend accepts the key. It is a sending-only key, so it can send mail but cannot list your domains."},
			{Name: checkSender, State: protocol.CheckStateWarning,
				Message: "Marshal cannot check the sender's domain with a sending-only key.",
				Fix:     "Press Send test email to check that mail leaves from " + config.From + "."},
		}, s.now()), nil
	}
	if checkErr != nil {
		message, fix := resendFailure(checkErr)
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name: CheckSummary, State: protocol.CheckStateFailed, Message: message, Fix: fix,
		}}, s.now()), nil
	}
	checks := []protocol.TestCheck{{
		Name: "API key", State: protocol.CheckStatePassed,
		Message: fmt.Sprintf("Resend accepts the key. The account has %d sending %s.", len(domains), plural(len(domains), "domain", "domains")),
	}}
	return protocol.NewTestResult(info.ID, append(checks, senderCheck(config.From, domains)), s.now()), nil
}

// senderCheck says whether mail from the sender address can leave: its domain is verified on the
// account, or it is Resend's own test sender.
func senderCheck(from string, domains []resend.Domain) protocol.TestCheck {
	address := from
	if i := strings.LastIndex(from, "<"); i >= 0 {
		address = strings.TrimSuffix(from[i+1:], ">")
	}
	domain := address[strings.LastIndex(address, "@")+1:]
	if domain == resendTestDomain {
		return protocol.TestCheck{Name: checkSender, State: protocol.CheckStateWarning,
			Message: "Mail is sent from Resend's test address, which only reaches the account owner until a domain is verified.",
			Fix:     "Verify your own domain at resend.com/domains, then send from an address on it."}
	}
	for _, d := range domains {
		if strings.EqualFold(d.Name, domain) && d.Status == "verified" {
			return protocol.TestCheck{Name: checkSender, State: protocol.CheckStatePassed,
				Message: "Mail can be sent from " + domain + "."}
		}
	}
	return protocol.TestCheck{Name: checkSender, State: protocol.CheckStateFailed,
		Message: domain + " is not a verified domain on this Resend account, so Resend refuses mail from it.",
		Fix:     "Verify " + domain + " at resend.com/domains, then try again."}
}

// resendFailure turns a Resend refusal into a sentence and a fix. The reason Resend gave is kept,
// because it names what to do, and a bare network error says only that Resend could not be reached.
func resendFailure(err error) (message, fix string) {
	switch {
	case resend.Refused(err, http.StatusUnauthorized):
		return "Resend does not accept this API key.", "Copy the key again from resend.com/api-keys and save it."
	case resend.Refused(err, http.StatusForbidden):
		return "Resend refused the email: " + refusalText(err), "Verify a domain at resend.com/domains and send from an address on it."
	case resend.Refused(err, http.StatusTooManyRequests):
		return "Resend is limiting requests right now.", "Wait a minute, then try again."
	default:
		var apiErr *resend.APIError
		if errors.As(err, &apiErr) {
			return "Resend refused the request: " + apiErr.Message, "Check the sender address and the key, then try again."
		}
		return "Marshal could not reach Resend: " + refusalText(err), "Check the internet connection, then try again."
	}
}

// refusalText is what Resend said, without the status prefix.
func refusalText(err error) string {
	var apiErr *resend.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return err.Error()
}

// recipient is the address in the owner's profile, read at send time so a change of address takes
// effect at once. It is read through the function the daemon hands in, because the owner is found in
// the accounts store and not by the integrations package.
func (s *Service) recipient(ctx context.Context) (string, error) {
	if s.resendRecipient == nil {
		return "", nil
	}
	return s.resendRecipient(ctx)
}
