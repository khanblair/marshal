package providers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/security"
)

// stubClient is a Client that reaches nothing. Every resolver test builds one of these instead of a
// real adapter, so no test can call a provider.
type stubClient struct{ id string }

func (c stubClient) ID() string { return c.id }

func (c stubClient) Complete(context.Context, Request) (Reply, error) {
	return Reply{}, errors.New("the stub client does not answer")
}

func (c stubClient) Stream(context.Context, Request) (Stream, error) {
	return nil, errors.New("the stub client does not answer")
}

// built records what the resolver asked its factory for, so a test can prove the key reached the
// client, that a client is built once and kept, and that saving a key throws the kept one away.
type built struct {
	order   []string
	secrets map[string]string
}

// serviceFor returns a service whose clients are stubs, plus the record of what was built.
func serviceFor(t *testing.T, keys security.Keychain) (*Service, *built) {
	t.Helper()
	s := newTestService(t, keys)
	rec := &built{secrets: map[string]string{}}
	s.build = func(info Info, secret string) (Client, error) {
		rec.order = append(rec.order, info.ID)
		rec.secrets[info.ID] = secret
		return stubClient{id: info.ID}, nil
	}
	return s, rec
}

func TestResolveFindsTheProviderThatNamesTheModel(t *testing.T) {
	s, rec := serviceFor(t, nil)
	key := "sk-ant-api03-Ab12Cd34Ef564f2a"
	if err := s.Save(AnthropicID, key); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderID != AnthropicID {
		t.Errorf("the model resolved to %s, want %s", got.ProviderID, AnthropicID)
	}
	if got.Model != "claude-sonnet-4-5" {
		t.Errorf("Resolve answered the model %q, want the id it was asked for", got.Model)
	}
	if got.Client == nil || got.Client.ID() != AnthropicID {
		t.Errorf("Resolve answered the client %v, want the provider's own", got.Client)
	}
	if rec.secrets[AnthropicID] != key {
		t.Error("the stored key did not reach the client that was built")
	}
}

func TestResolveRefusesAModelNothingCanRun(t *testing.T) {
	s, _ := serviceFor(t, nil)
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef564f2a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, model := range []string{"gpt-5-mini", "gemini-2.5-flash", "no-such-model"} {
		if _, err := s.Resolve(model); !errors.Is(err, ErrNoSuchModel) {
			t.Errorf("Resolve(%q) answered %v, want ErrNoSuchModel", model, err)
		} else if err.Error() == "" {
			t.Errorf("Resolve(%q) refused without a sentence to show", model)
		}
	}
}

func TestResolveSkipsProvidersThatAreNotSetUp(t *testing.T) {
	s, rec := serviceFor(t, nil)
	if err := s.Save(DeepSeekID, "sk-deepseek-not-real"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Resolve("deepseek-chat")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderID != DeepSeekID {
		t.Errorf("the model resolved to %s, want %s", got.ProviderID, DeepSeekID)
	}
	if len(rec.order) != 1 {
		t.Errorf("the resolver built %v, want one client for the provider that has a key", rec.order)
	}
}

func TestResolveTreatsAnEmptyModelAsTheDefault(t *testing.T) {
	s, _ := serviceFor(t, nil)
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef564f2a"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Model != DefaultModel {
		t.Errorf("an empty model resolved to %q, want the default %q", got.Model, DefaultModel)
	}
}

// TestResolvePrefersTheProviderThatNamesTheModel is the ordering rule: a model two providers could
// run goes to the one that names it, in the screens' order, and only falls to a catch-all when no
// naming provider is set up.
func TestResolvePrefersTheProviderThatNamesTheModel(t *testing.T) {
	s, rec := serviceFor(t, nil)
	if err := s.Save(OpenRouterID, "sk-or-v1-not-real-0b33"); err != nil {
		t.Fatalf("Save OpenRouter: %v", err)
	}
	got, err := s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderID != OpenRouterID {
		t.Errorf("with only a catch-all set up the model resolved to %s, want %s",
			got.ProviderID, OpenRouterID)
	}

	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef564f2a"); err != nil {
		t.Fatalf("Save Anthropic: %v", err)
	}
	got, err = s.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderID != AnthropicID {
		t.Errorf("with both set up the model resolved to %s, want the one that names it, %s",
			got.ProviderID, AnthropicID)
	}
	if len(rec.order) != 2 {
		t.Errorf("built %v, want a client for each provider that answered", rec.order)
	}
}

// TestResolveSendsAnUnknownModelToALocalCatchAll is the offline case: a model Marshal has no table
// for runs on the local server a person set up.
func TestResolveSendsAnUnknownModelToALocalCatchAll(t *testing.T) {
	s, rec := serviceFor(t, nil)
	if err := s.Save(LMStudioID, testLocalAddress); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Resolve("some-local-model-7b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ProviderID != LMStudioID {
		t.Errorf("resolved to %s, want %s", got.ProviderID, LMStudioID)
	}
	if got.Model != "some-local-model-7b" {
		t.Errorf("Resolve answered %q, want the model it was asked for", got.Model)
	}
	if rec.secrets[LMStudioID] != testLocalAddress {
		t.Error("the local server's address did not reach the client that was built")
	}
}

func TestResolveBuildsAClientOnceAndKeepsIt(t *testing.T) {
	s, rec := serviceFor(t, nil)
	if err := s.Save(OpenAIID, "sk-proj-not-real-91cd"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for range 3 {
		if _, err := s.Resolve("gpt-5-mini"); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	}
	if len(rec.order) != 1 {
		t.Errorf("the resolver built %v, want one client kept for the provider", rec.order)
	}
}

func TestSavingOrRemovingAKeyThrowsTheKeptClientAway(t *testing.T) {
	s, rec := serviceFor(t, nil)
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef561111"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Resolve("claude-sonnet-4-5"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := s.Save(AnthropicID, "sk-ant-api03-Ab12Cd34Ef562222"); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if _, err := s.Resolve("claude-sonnet-4-5"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(rec.order) != 2 {
		t.Fatalf("built %v, want a fresh client after the key was replaced", rec.order)
	}
	if rec.secrets[AnthropicID] != "sk-ant-api03-Ab12Cd34Ef562222" {
		t.Error("the kept client was built over the key that was replaced")
	}

	if err := s.Remove(AnthropicID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Resolve("claude-sonnet-4-5"); !errors.Is(err, ErrNoSuchModel) {
		t.Errorf("after removing the key Resolve answered %v, want ErrNoSuchModel", err)
	}
}

func TestResolveReportsAProviderThatCannotBeSetUp(t *testing.T) {
	s, _ := serviceFor(t, nil)
	failure := errors.New("the key was refused")
	s.build = func(Info, string) (Client, error) { return nil, failure }
	if err := s.Save(GeminiID, "AIzaSyD9x8Y7w6V5uT4s3R27Qe0"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Resolve("gemini-2.5-flash"); !errors.Is(err, failure) {
		t.Errorf("Resolve answered %v, want the failure that stopped the client being built", err)
	}
}

func TestModelsListsOnlyWhatIsSetUp(t *testing.T) {
	s, _ := serviceFor(t, nil)
	if got := s.Models(); len(got) != 0 {
		t.Fatalf("a fresh install offers %v, want nothing until a key is saved", got)
	}

	order := []string{AnthropicID, OpenAIID, GeminiID, DeepSeekID, OllamaID}
	keys := map[string]string{
		AnthropicID: "sk-ant-api03-Ab12Cd34Ef564f2a",
		OpenAIID:    "sk-proj-not-real-91cd",
		GeminiID:    "AIzaSyD9x8Y7w6V5uT4s3R27Qe0",
		DeepSeekID:  "sk-deepseek-not-real",
		OllamaID:    testLocalAddress,
	}
	for _, id := range order {
		if err := s.Save(id, keys[id]); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	got := s.Models()
	if len(got) != len(designBuiltinModels) {
		t.Fatalf("with every naming provider set up the picker offers %v, want the design's %d models",
			got, len(designBuiltinModels))
	}
	for i, want := range designBuiltinModels {
		if got[i].ID != want.id {
			t.Errorf("model %d is %s, want %s", i, got[i].ID, want.id)
		}
		if got[i].Thinking != want.thinking {
			t.Errorf("%s: thinking is %v, want %v", got[i].ID, got[i].Thinking, want.thinking)
		}
	}

	// A provider with no key contributes nothing, and a catch-all contributes nothing of its own
	// however many keys are saved, because Marshal cannot list what it serves.
	if err := s.Save(OpenRouterID, "sk-or-v1-not-real-0b33"); err != nil {
		t.Fatalf("Save OpenRouter: %v", err)
	}
	if err := s.Save(LMStudioID, testLocalAddress); err != nil {
		t.Fatalf("Save LM Studio: %v", err)
	}
	if after := s.Models(); len(after) != len(got) {
		t.Errorf("setting up the catch-all providers changed the picker to %v", after)
	}
	if err := s.Remove(OllamaID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if after := s.Models(); len(after) != len(got)-1 {
		t.Errorf("removing a key left %v, want the local model gone", after)
	}
}

// TestModelsHasTheShapeTheCatalogNeeds is the agreement with catalog.BuiltinModels: no context, no
// error, and an empty list rather than nil, so a picker with nothing set up encodes as [] and not
// as null.
func TestModelsHasTheShapeTheCatalogNeeds(t *testing.T) {
	s, _ := serviceFor(t, nil)
	builtinModels := s.Models
	got := builtinModels()
	if got == nil {
		t.Fatal("Models returned nil, which a client would read as null")
	}
	if len(got) != 0 {
		t.Errorf("a fresh install offers %v, want nothing until a key is saved", got)
	}
	if encoded, err := json.Marshal(got); err != nil || string(encoded) != "[]" {
		t.Errorf("an empty model list encoded as %s (%v), want []", encoded, err)
	}
}
