package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/builtin"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/session"
)

type resumeProvider struct{}

func (resumeProvider) ID() string { return "fixture" }
func (resumeProvider) Complete(context.Context, providers.Request) (providers.Reply, error) {
	return providers.Reply{}, errors.New("streaming only")
}
func (resumeProvider) Stream(_ context.Context, req providers.Request) (providers.Stream, error) {
	seenQuestion, seenAnswer := false, false
	for _, message := range req.Messages {
		for _, part := range message.Parts {
			seenQuestion = seenQuestion || (message.Role == providers.RoleUser && strings.Contains(part.Text, "REMEMBER_FIRST_MARKER"))
			seenAnswer = seenAnswer || (message.Role == providers.RoleAssistant && part.Text == "marker stored")
		}
	}
	answer := "marker stored"
	if seenQuestion && seenAnswer && strings.Contains(req.System, "ROLE_CONTEXT") {
		answer = "REPLAY_OK"
	}
	return &resumeStream{events: []providers.Event{{Kind: providers.EventText, Text: answer}, {Kind: providers.EventDone, StopReason: providers.StopEndTurn}}}, nil
}

type resumeStream struct{ events []providers.Event }

func (s *resumeStream) Recv() (providers.Event, error) {
	if len(s.events) == 0 {
		return providers.Event{}, io.EOF
	}
	ev := s.events[0]
	s.events = s.events[1:]
	return ev, nil
}
func (*resumeStream) Close() error { return nil }

type resumeResolver struct{}

func (resumeResolver) Resolve(string) (builtin.Resolved, error) {
	return builtin.Resolved{ProviderID: "fixture", Model: "fixture", Client: resumeProvider{}}, nil
}

type resumeAttachment struct{}

func (resumeAttachment) Attach(context.Context, protocol.Card) (session.Attachment, error) {
	return session.Attachment{Instructions: "ROLE_CONTEXT"}, nil
}
func (resumeAttachment) Detach(context.Context, string) {}

func TestBuiltinConversationSurvivesHTTPStopAndDaemonRestart(t *testing.T) {
	var st *stack
	factory := func() (agents.Agent, error) {
		return builtin.New(builtin.Config{Resolver: resumeResolver{}, History: st.hist.Conversation})
	}
	st = newStack(t, withAgent(factory))
	if err := st.reg.Register(protocol.AgentKindBuiltin, factory); err != nil {
		t.Fatal(err)
	}
	st.mgr.SetAttacher(resumeAttachment{})
	project, _ := st.addProject("small-repo")
	card := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards", protocol.CreateCardRequest{Title: "Keep context", Agent: protocol.AgentKindBuiltin}).want(t, http.StatusCreated))
	base := "/v1/cards/" + card.ID
	st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "REMEMBER_FIRST_MARKER"}).want(t, http.StatusNoContent)
	awaitBuiltinReply(t, st, base, "marker stored", 0)
	for range 2 {
		st.do(http.MethodPost, base+"/stop", nil).want(t, http.StatusNoContent)
		st.restart()
		st.mgr.SetAttacher(resumeAttachment{})
		st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
		page := decode[protocol.Page[protocol.ChatMessage]](t, st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK))
		before := page.Items[0].Seq
		st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "What do you remember?"}).want(t, http.StatusNoContent)
		awaitBuiltinReply(t, st, base, "REPLAY_OK", before)
	}
}

func awaitBuiltinReply(t *testing.T, st *stack, base, text string, after int64) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		card := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
		if card.Session != nil && *card.Session == protocol.SessionStateAwake {
			page := decode[protocol.Page[protocol.ChatMessage]](t, st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK))
			for _, message := range page.Items {
				if message.Seq > after && message.Text == text {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reply %q missing after resumed turn", text)
}
