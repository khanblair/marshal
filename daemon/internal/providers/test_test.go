package providers

import (
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// This file checks one provider's connection test: what Marshal tells a person whose key works, a
// person whose key the provider refused, a person whose provider cannot be reached, and a person who
// has saved nothing. Every call goes to a fake HTTP server on the loopback interface; no test here
// reaches a provider, and no test uses a real key.

// answerAWholeOpenAIAnswer is what a working provider answers: one word, and the usage OpenAI sends.
const answerAWholeOpenAIAnswer = `{"id":"chatcmpl_1","object":"chat.completion","created":1,
	"model":"gpt-5-mini",
	"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
	"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`

// testServiceFor builds a service whose whole family is the fake, so a test drives the real adapters
// over the loopback interface.
func testServiceFor(t *testing.T, f *fakeProvider) *Service {
	t.Helper()
	s := newTestService(t, nil)
	s.build = func(info Info, secret string) (Client, error) {
		return openAITestClient(f, info.ID), nil
	}
	return s
}

// TestAWorkingKeyPassesAndItsRateLimitsAreRead is the whole of a good test: the key is accepted, the
// provider's own rate-limit headers come back as a number, and the result is OK.
func TestAWorkingKeyPassesAndItsRateLimitsAreRead(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenAIID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSONWith(map[string]string{
		openAILimitHeader:     "500",
		openAIRemainingHeader: "499",
	}, answerAWholeOpenAIAnswer)
	got, err := s.Test(t.Context(), OpenAIID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !got.OK {
		t.Errorf("the test was not OK: %+v", got.Checks)
	}
	if got.ConnectionID != OpenAIID {
		t.Errorf("connection = %q, want %q", got.ConnectionID, OpenAIID)
	}
	if got.RanAt.Time().IsZero() {
		t.Error("the result was not stamped with the time it ran")
	}
	if len(got.Checks) != 2 {
		t.Fatalf("checks = %d, want the key and the rate limits", len(got.Checks))
	}
	if got.Checks[0].Name != CheckAPIKey || got.Checks[0].State != protocol.CheckStatePassed {
		t.Errorf("the key check = %+v, want passed", got.Checks[0])
	}
	if got.Checks[1].Name != CheckRateLimits || got.Checks[1].State != protocol.CheckStatePassed {
		t.Errorf("the rate-limit check = %+v, want passed", got.Checks[1])
	}
	if got.Checks[1].Message != "499 of 500 requests left this window." {
		t.Errorf("the rate-limit sentence = %q", got.Checks[1].Message)
	}
	if _, ok := got.FirstFailed(); ok {
		t.Error("a working key reported a failed check")
	}
}

// TestTheTinyRequestAsksForAlmostNothing is the cost rule: the test asks the provider's cheapest model
// for a handful of tokens, not for a real answer.
func TestTheTinyRequestAsksForAlmostNothing(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenAIID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSON(answerAWholeOpenAIAnswer)
	if _, err := s.Test(t.Context(), OpenAIID); err != nil {
		t.Fatalf("Test: %v", err)
	}
	sent := f.sent(t)
	if sent["model"] != "gpt-5-mini" {
		t.Errorf("the test asked for %v, want the provider's cheapest model", sent["model"])
	}
	if sent["max_tokens"] != float64(testMaxTokens) {
		t.Errorf("the test asked for %v tokens, want %d", sent["max_tokens"], testMaxTokens)
	}
}

// TestAProviderThatSaysNothingAboutRateLimitsOnlyWarns is the second state a screen shows: the key
// works, so the test is OK, and Marshal says it has no numbers rather than showing a zero.
func TestAProviderThatSaysNothingAboutRateLimitsOnlyWarns(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenAIID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSON(answerAWholeOpenAIAnswer)
	got, err := s.Test(t.Context(), OpenAIID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !got.OK {
		t.Error("a provider that reported no rate limits failed the test, want a warning only")
	}
	if got.Checks[1].State != protocol.CheckStateWarning {
		t.Errorf("the rate-limit check = %+v, want a warning", got.Checks[1])
	}
}

// TestARefusedKeyIsAnsweredWithWhatToDo is the failure a person most often sees, and the whole point
// of the test: the row says the provider refused the key and what to do about it.
func TestARefusedKeyIsAnsweredWithWhatToDo(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenAIID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerStatus(401, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`)
	got, err := s.Test(t.Context(), OpenAIID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got.OK {
		t.Error("a refused key passed the test")
	}
	failed, ok := got.FirstFailed()
	if !ok {
		t.Fatal("a refused key reported no failed check")
	}
	if failed.Name != CheckAPIKey {
		t.Errorf("the failed check = %q, want the key", failed.Name)
	}
	if failed.Message != "OpenAI refused this key." {
		t.Errorf("message = %q", failed.Message)
	}
	if failed.Fix == "" {
		t.Error("the failed check gave no fix, which is the whole point of it")
	}
}

// TestAProviderThatCannotBeReachedSaysSo: the fix differs from a refused key's, because the person
// has nothing to check about the key.
func TestAProviderThatCannotBeReachedSaysSo(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenAIID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerStatus(503, `{"error":{"message":"the service is down"}}`)
	got, err := s.Test(t.Context(), OpenAIID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	failed, ok := got.FirstFailed()
	if !ok {
		t.Fatal("an unreachable provider reported no failed check")
	}
	if failed.Message != "Marshal could not reach OpenAI." {
		t.Errorf("message = %q", failed.Message)
	}
	if failed.Fix == "" {
		t.Error("the failed check gave no fix")
	}
}

// TestAProviderWithNoKeyIsToldSo: pressing Test before saving anything is not an error and not a call
// - Marshal says what is missing, and reaches nothing.
func TestAProviderWithNoKeyIsToldSo(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	got, err := s.Test(t.Context(), OpenAIID)
	if err != nil {
		t.Fatalf("Test with nothing saved: %v", err)
	}
	if got.OK {
		t.Error("a provider with no key passed the test")
	}
	failed, ok := got.FirstFailed()
	if !ok {
		t.Fatal("a provider with no key reported no failed check")
	}
	if failed.Message != "No OpenAI key is stored, so Marshal cannot use OpenAI." {
		t.Errorf("message = %q", failed.Message)
	}
	if !f.sentNothing() {
		t.Error("a provider with no key was still called")
	}
}

// TestAProviderMarshalCannotNameAModelForOnlyWarns: OpenRouter proxies other providers' models and LM
// Studio runs whatever is loaded, so neither has a model in Marshal's table. Marshal says that rather
// than sending a request that would fail for Marshal's own reason.
func TestAProviderMarshalCannotNameAModelForOnlyWarns(t *testing.T) {
	f := newFakeProvider(t)
	s := testServiceFor(t, f)
	if err := s.Save(OpenRouterID, "sk-or-v1-not-real-0b33"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Test(t.Context(), OpenRouterID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !got.OK {
		t.Error("a provider Marshal cannot name a model for failed the test")
	}
	if len(got.Checks) != 1 || got.Checks[0].State != protocol.CheckStateWarning {
		t.Errorf("checks = %+v, want one warning", got.Checks)
	}
	if got.Checks[0].Fix == "" {
		t.Error("the warning gave no fix")
	}
}

// TestTestingAProviderMarshalDoesNotKnowIsNotFound: an id from a client's own imagination is not a
// provider, and it is refused before anything is read or called.
func TestTestingAProviderMarshalDoesNotKnowIsNotFound(t *testing.T) {
	s := newTestService(t, nil)
	_, err := s.Test(t.Context(), "no-such-provider")
	if errorCode(err) != protocol.ErrorCodeNotFound {
		t.Fatalf("Test answered %v, want not found", err)
	}
}

// TestAKeychainThatCannotBeReadIsTheDaemonsOwnFailure: a broken keychain is not "no key is stored",
// which would tell the person their key was gone when it is only unreadable.
func TestAKeychainThatCannotBeReadIsTheDaemonsOwnFailure(t *testing.T) {
	broken := errors.New("the keychain is not answering")
	s := newTestService(t, brokenKeychain{err: broken})
	_, err := s.Test(t.Context(), OpenAIID)
	if !errors.Is(err, broken) {
		t.Fatalf("Test answered %v, want the keychain's own failure", err)
	}
}

// TestALocalProviderIsTestedTheSameWay: Ollama has a model in Marshal's table, so its test is the
// same tiny request over the address the person saved rather than a key.
func TestALocalProviderIsTestedTheSameWay(t *testing.T) {
	f := newFakeProvider(t)
	s := newTestService(t, security.NewMemoryKeychain())
	s.build = func(info Info, secret string) (Client, error) {
		if secret != testLocalAddress {
			t.Errorf("the local provider was built with %q, want the saved address", secret)
		}
		return openAITestClient(f, info.ID), nil
	}
	if err := s.Save(OllamaID, testLocalAddress); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSON(answerAWholeOpenAIAnswer)
	got, err := s.Test(t.Context(), OllamaID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !got.OK {
		t.Errorf("a working local provider failed the test: %+v", got.Checks)
	}
}

// errorCode is a protocol error's own code, or empty when err is not one. It is spelled out here
// because the providers package asks for a code rather than importing the wire's error shape.
func errorCode(err error) protocol.ErrorCode {
	var perr *protocol.Error
	if errors.As(err, &perr) {
		return perr.Code
	}
	return ""
}
