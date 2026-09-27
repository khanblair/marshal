package providers

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// This file checks the one call Marshal makes on its own account: naming a chat from its first message
// (B4.7). The provider is chosen by price, the model's answer is trimmed to something a chat can be
// called, and a Marshal that cannot write a title says so rather than inventing one - the caller falls
// back to the message's own words.

// titleServiceFor builds a service whose clients are the fake, and records every model asked for.
func titleServiceFor(t *testing.T, f *fakeProvider) (*Service, *[]string) {
	t.Helper()
	asked := &[]string{}
	s := newTestService(t, security.NewMemoryKeychain())
	s.build = func(info Info, secret string) (Client, error) {
		*asked = append(*asked, info.ID)
		return openAITestClient(f, info.ID), nil
	}
	return s, asked
}

// TestCheapestPicksTheCheapestProviderThatIsSetUp: the model named by Marshal's price table with the
// lowest input and output cost, among the providers a key has been saved for.
func TestCheapestPicksTheCheapestProviderThatIsSetUp(t *testing.T) {
	s := newTestService(t, nil)
	for _, saved := range []struct{ id, key string }{
		{AnthropicID, anthropicTestKey},
		{OpenAIID, openAITestKey},
		{DeepSeekID, openAITestKey},
	} {
		if err := s.Save(saved.id, saved.key); err != nil {
			t.Fatalf("Save(%s): %v", saved.id, err)
		}
	}
	got, err := s.Cheapest()
	if err != nil {
		t.Fatalf("Cheapest: %v", err)
	}
	if got.ProviderID != DeepSeekID || got.Model != "deepseek-chat" {
		t.Errorf("Cheapest answered %s/%s, want deepseek/deepseek-chat", got.ProviderID, got.Model)
	}
}

// TestCheapestSkipsAProviderNoKeyIsSavedFor: nothing is spent on a provider the person has not set up,
// and the next cheapest one is used.
func TestCheapestSkipsAProviderNoKeyIsSavedFor(t *testing.T) {
	s := newTestService(t, nil)
	if err := s.Save(GeminiID, "AIzaSyD9x8Y7w6V5uT4s3R27Qe0"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Cheapest()
	if err != nil {
		t.Fatalf("Cheapest: %v", err)
	}
	if got.ProviderID != GeminiID {
		t.Errorf("Cheapest answered %s, want %s", got.ProviderID, GeminiID)
	}
}

// TestCheapestPrefersAPaidModelOverAFreeLocalOne is the tie rule: every free model costs zero, so a
// price Marshal can read beats a tie it cannot. A machine with Ollama running and a DeepSeek key
// saved names its chats with DeepSeek.
func TestCheapestPrefersAPaidModelOverAFreeLocalOne(t *testing.T) {
	s := newTestService(t, nil)
	if err := s.Save(OllamaID, testLocalAddress); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(DeepSeekID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Cheapest()
	if err != nil {
		t.Fatalf("Cheapest: %v", err)
	}
	if got.ProviderID != DeepSeekID {
		t.Errorf("Cheapest answered %s, want %s", got.ProviderID, DeepSeekID)
	}
}

// TestCheapestUsesALocalModelWhenItIsAllThereIs: a free model is still a model, and a person who runs
// one locally names their chats with it rather than paying anything.
func TestCheapestUsesALocalModelWhenItIsAllThereIs(t *testing.T) {
	s := newTestService(t, nil)
	if err := s.Save(OllamaID, testLocalAddress); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Cheapest()
	if err != nil {
		t.Fatalf("Cheapest: %v", err)
	}
	if got.ProviderID != OllamaID || got.Model != "qwen2.5-coder:32b" {
		t.Errorf("Cheapest answered %s/%s, want the local model", got.ProviderID, got.Model)
	}
}

// TestCheapestWithNothingSetUpSaysSo: ErrNoProvider is what the caller falls back on, so it has to be
// recognisable rather than a sentence.
func TestCheapestWithNothingSetUpSaysSo(t *testing.T) {
	s := newTestService(t, nil)
	if _, err := s.Cheapest(); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("Cheapest answered %v, want ErrNoProvider", err)
	}
}

// TestCheapestOverAKeychainThatCannotBeReadIsNotNoProvider: an unreadable keychain is the daemon's own
// failure, and reporting it as "nothing is set up" would hide it.
func TestCheapestOverAKeychainThatCannotBeReadIsNotNoProvider(t *testing.T) {
	broken := errors.New("the keychain is not answering")
	s := newTestService(t, brokenKeychain{err: broken})
	if _, err := s.Cheapest(); !errors.Is(err, broken) {
		t.Fatalf("Cheapest answered %v, want the keychain's own failure", err)
	}
}

// TestTitleIsWrittenByTheCheapestModel is the call itself: the cheapest model is asked, with the
// system words that keep a title short, and the person's message is what it is asked about.
func TestTitleIsWrittenByTheCheapestModel(t *testing.T) {
	f := newFakeProvider(t)
	s, asked := titleServiceFor(t, f)
	if err := s.Save(DeepSeekID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSON(`{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"deepseek-chat",
		"choices":[{"index":0,"message":{"role":"assistant","content":"Fix the login redirect bug"},
		"finish_reason":"stop"}],
		"usage":{"prompt_tokens":9,"completion_tokens":6,"total_tokens":15}}`)
	got, err := s.Title(t.Context(), "the login redirect is broken when the session expires")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if got != "Fix the login redirect bug" {
		t.Errorf("title = %q", got)
	}
	if len(*asked) != 1 || (*asked)[0] != DeepSeekID {
		t.Errorf("the title was written by %v, want only %s", *asked, DeepSeekID)
	}
	sent := f.sent(t)
	if sent["model"] != "deepseek-chat" {
		t.Errorf("the title asked %v, want the cheapest model", sent["model"])
	}
	if sent["max_tokens"] != float64(titleMaxTokens) {
		t.Errorf("the title asked for %v tokens, want %d", sent["max_tokens"], titleMaxTokens)
	}
}

// TestTitleWithNothingToWriteIsAnError: an empty answer is not a title, and the caller would rather
// name the chat from the message than call it nothing.
func TestTitleWithNothingToWriteIsAnError(t *testing.T) {
	f := newFakeProvider(t)
	s, _ := titleServiceFor(t, f)
	if err := s.Save(DeepSeekID, openAITestKey); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.answerJSON(`{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"deepseek-chat",
		"choices":[{"index":0,"message":{"role":"assistant","content":"   "},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`)
	if _, err := s.Title(t.Context(), "hello"); err == nil {
		t.Fatal("an empty answer was taken as a title")
	}
}

// TestTitleWithNoProviderSetUpSaysSo: the caller falls back, so the failure has to reach it.
func TestTitleWithNoProviderSetUpSaysSo(t *testing.T) {
	s := newTestService(t, nil)
	if _, err := s.Title(t.Context(), "hello"); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("Title answered %v, want ErrNoProvider", err)
	}
}

// TestAChatTitleIsTrimmedToSomethingAThingCanBeCalled covers what a model's answer is turned into: a
// title is a name, so wrapping quotes, a trailing full stop, an explanation on a second line, and a
// model that simply answers at length all still leave something a chat can be called.
func TestAChatTitleIsTrimmedToSomethingAThingCanBeCalled(t *testing.T) {
	long := strings.Repeat("word ", 40)
	cases := []struct {
		name     string
		answered string
		want     string
	}{
		{"a plain title", "Fix the login bug", "Fix the login bug"},
		{"wrapped in quotes", `"Fix the login bug"`, "Fix the login bug"},
		{"with a full stop", "Fix the login bug.", "Fix the login bug"},
		{"with space around it", "\n  Fix the login bug  \n", "Fix the login bug"},
		{"with an explanation under it", "Fix the login bug\n\nThis title captures...", "Fix the login bug"},
		{"in lower case", "fix the login bug", "Fix the login bug"},
		{"too many words", "one two three four five six seven eight nine ten",
			"One two three four five six seven eight"},
		{"nothing at all", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanTitle(tc.answered); got != tc.want {
				t.Errorf("cleanTitle(%q) = %q, want %q", tc.answered, got, tc.want)
			}
		})
	}
	t.Run("longer than a chat may be called", func(t *testing.T) {
		got := cleanTitle(long)
		if len([]rune(got)) > titleMaxChars {
			t.Errorf("the title is %d characters, want at most %d", len([]rune(got)), titleMaxChars)
		}
		if strings.HasSuffix(got, " ") {
			t.Errorf("the title ends with a space: %q", got)
		}
	})
}

// TestCheapestIsAlwaysAModelMarshalCanName keeps the choice honest: whatever Cheapest answers is a
// model of the provider it names, so a caller never hands a provider an id from another provider's
// table.
func TestCheapestIsAlwaysAModelMarshalCanName(t *testing.T) {
	s := newTestService(t, nil)
	for _, info := range known {
		if info.Local {
			if err := s.Save(info.ID, testLocalAddress); err != nil {
				t.Fatalf("Save(%s): %v", info.ID, err)
			}
			continue
		}
		if err := s.Save(info.ID, openAITestKey); err != nil {
			t.Fatalf("Save(%s): %v", info.ID, err)
		}
	}
	got, err := s.Cheapest()
	if err != nil {
		t.Fatalf("Cheapest: %v", err)
	}
	info, ok := Lookup(got.ProviderID)
	if !ok {
		t.Fatalf("Cheapest answered an unknown provider %q", got.ProviderID)
	}
	if _, named := info.Model(got.Model); !named {
		t.Errorf("%s does not name the model %q that Cheapest chose", got.ProviderID, got.Model)
	}
	if got.Client == nil || got.Client.ID() != got.ProviderID {
		t.Errorf("the client %v does not belong to %s", got.Client, got.ProviderID)
	}
	var zero protocol.AgentModel
	if zero.ID == got.Model {
		t.Error("Cheapest answered no model")
	}
}
