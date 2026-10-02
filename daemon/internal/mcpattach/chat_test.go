package mcpattach

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/integrator"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// A project chat is attached the way a card is: its server is hosted, and the agent is told the
// command that reaches it and the instructions its kind starts from.

const testChatID = "chat-1"

// namedRoles answers the role asked for with instructions that name it, so a test can see which role
// a chat started from.
type namedRoles struct{ asked []string }

func (r *namedRoles) Role(_ context.Context, name, _ string) (protocol.Role, error) {
	r.asked = append(r.asked, name)
	return protocol.Role{Name: name, Spec: protocol.RoleSpec{Instr: "Instructions of the " + name + " role."}}, nil
}

func newChatAttacher(t *testing.T, host *mcpserver.Host, roles Roles, merge func() integrator.MergeTools) *ChatAttacher {
	t.Helper()
	base := newAttacher(t, host, roles)
	c, err := NewChatAttacher(base, ChatDeps{
		Harness: func(string) (harness.Config, bool) {
			return harness.Config{Mode: protocol.PermissionModePlan, Profile: security.DefaultProfile()}, true
		},
		MergeTools: merge,
	})
	if err != nil {
		t.Fatalf("build the chat attacher: %v", err)
	}
	return c
}

func chatOf(target protocol.ChatTarget, system string) protocol.Chat {
	return protocol.Chat{ID: testChatID, ProjectID: testProjectID, Target: target, System: system}
}

func TestAChatIsGivenItsServerUnderItsOwnIdAndTheCommandThatReachesIt(t *testing.T) {
	host := mcpserver.NewHost()
	c := newChatAttacher(t, host, &namedRoles{}, nil)
	got, err := c.AttachChat(context.Background(), chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator}, ""))
	if err != nil {
		t.Fatalf("AttachChat: %v", err)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("the chat was given %d servers, want one", len(got.Servers))
	}
	server := got.Servers[0]
	wantArgs := []string{"mcp", "--address", "127.0.0.1:47800", "--card", testChatID}
	if server.Name != "marshal" || strings.Join(server.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("server = %+v, want the command that reaches the chat's own server", server)
	}
	if len(server.Env) != 1 || !strings.HasPrefix(server.Env[0], TokenEnv+"=") || strings.Contains(strings.Join(server.Args, " "), "=") {
		t.Errorf("env = %q and args = %q, want the secret in the environment alone", server.Env, server.Args)
	}
	if !host.Has(testChatID) {
		t.Fatal("the chat's server was not hosted")
	}
	c.DetachChat(context.Background(), testChatID)
	if host.Has(testChatID) {
		t.Error("the chat is still hosted after DetachChat")
	}
	c.DetachChat(context.Background(), "never-attached")
}

func TestEachKindOfChatStartsFromItsRoleAndItsPreface(t *testing.T) {
	tests := []struct {
		name      string
		chat      protocol.Chat
		role      string
		mentions  []string
		doesNot   []string
		wantTools []string
	}{
		{
			name: "orchestrator", chat: chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator}, ""),
			role: "Orchestrator", mentions: []string{"Orchestrator chat", "create_card", "cannot change its files"},
			doesNot: []string{"merge_context"}, wantTools: []string{"board_status", "create_card"},
		},
		{
			name: "the Orchestrator as a role", chat: chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "orchestrator"}, ""),
			role: "Orchestrator", mentions: []string{"Orchestrator chat"}, wantTools: []string{"create_card"},
		},
		{
			name: "integrator", chat: chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Integrator"}, protocol.ChatSystemIntegrator),
			role: "Integrator", mentions: []string{"merge_context", "merge_report", "own workspace", "never in the owner's folder"},
			doesNot: []string{"create_card"}, wantTools: []string{"merge_context", "merge_report", "ask_owner"},
		},
		{
			name: "a role chat", chat: chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Tester"}, ""),
			role: "Tester", mentions: []string{"chat with the owner", "board_status"}, doesNot: []string{"create_card", "merge_context"},
			wantTools: []string{"board_status"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roles := &namedRoles{}
			c := newChatAttacher(t, mcpserver.NewHost(), roles, nil)
			got, err := c.AttachChat(context.Background(), tc.chat)
			if err != nil {
				t.Fatalf("AttachChat: %v", err)
			}
			if len(roles.asked) != 1 || roles.asked[0] != tc.role {
				t.Errorf("the role asked for was %v, want %s", roles.asked, tc.role)
			}
			text := got.Instructions
			if !strings.HasPrefix(text, "Instructions of the ") {
				t.Errorf("the instructions do not start with the role's own:\n%s", text)
			}
			for _, want := range append(tc.mentions, tc.wantTools...) {
				if !strings.Contains(text, want) {
					t.Errorf("the instructions do not mention %q:\n%s", want, text)
				}
			}
			for _, not := range tc.doesNot {
				if strings.Contains(text, not) {
					t.Errorf("the instructions mention %q, which this chat does not have:\n%s", not, text)
				}
			}
		})
	}
}

func TestAChatThatTalksToACardRunsAsNoRole(t *testing.T) {
	roles := &namedRoles{}
	c := newChatAttacher(t, mcpserver.NewHost(), roles, nil)
	got, err := c.AttachChat(context.Background(), chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindCard, ID: "card-1"}, ""))
	if err != nil {
		t.Fatalf("AttachChat: %v", err)
	}
	if len(roles.asked) != 0 || strings.Contains(got.Instructions, "Instructions of the") {
		t.Errorf("roles asked = %v, instructions = %q, want no role at all", roles.asked, got.Instructions)
	}
}

func TestAChatOnADaemonThatCannotGiveAServerStillGetsItsInstructions(t *testing.T) {
	base, err := New(Deps{
		Host: mcpserver.NewHost(), Cards: &fakeCards{}, Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{},
		Roles:   &namedRoles{},
		Harness: func(string) (harness.Config, bool) { return harness.Config{}, false },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c, err := NewChatAttacher(base, ChatDeps{Harness: func(string) (harness.Config, bool) { return harness.Config{}, false }})
	if err != nil {
		t.Fatalf("NewChatAttacher: %v", err)
	}
	got, err := c.AttachChat(context.Background(), chatOf(protocol.ChatTarget{Kind: protocol.ChatTargetKindOrchestrator}, ""))
	if err != nil || len(got.Servers) != 0 || !strings.Contains(got.Instructions, "Orchestrator") {
		t.Errorf("AttachChat = %+v, %v, want the instructions and no server", got, err)
	}
}

func TestANewChatAttacherNeedsItsParts(t *testing.T) {
	if _, err := NewChatAttacher(nil, ChatDeps{Harness: func(string) (harness.Config, bool) { return harness.Config{}, false }}); err == nil {
		t.Error("a chat attacher with no attacher to build on was built")
	}
	if _, err := NewChatAttacher(newAttacher(t, mcpserver.NewHost(), nil), ChatDeps{}); err == nil {
		t.Error("a chat attacher with no permission rules was built")
	}
}

func TestTheSessionManagersChatAttacherIsSatisfied(t *testing.T) {
	var _ session.ChatAttacher = (*ChatAttacher)(nil)
}
