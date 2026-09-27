package mcpserver

import (
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The kind words a tool hands the harness. security.Classify reads them into the coarser action a
// profile and a mode speak in (internal/security/action.go), which is what makes a tool call subject
// to the same rules a permission request is.
const (
	// kindRead is a tool that only reads: the board, a note, the memory index, the codebase map.
	kindRead = "read"

	// kindWrite is a tool that changes what Marshal holds: a note, a claim, a "doing now" line, a
	// question put to another card. It is an edit of Marshal's own record and not of a file in the
	// card's worktree, which is why no path is named with it.
	kindWrite = "edit"

	// kindCreate is create_card, and it is deliberately neither of the two above. "create" is not
	// one of the words security.Classify knows, so the request reads as security.ActionOther, which
	// no mode treats as harmless: full auto and bypass allow it, and every careful mode leaves a new
	// card to the card's owner, because a card is new work for the board and not this agent's own
	// business. That is what section 11.4's "propose a new card, subject to permissions" means here.
	kindCreate = "create"
)

// allow is the gate every tool call passes before it does anything (docs/architecture.md section 13:
// "permissions are checked by the harness before every tool call").
//
// The request it decides carries no path and no command, on purpose: these tools change Marshal's
// record and run nothing, so the worktree rule and the command rules have nothing to say about them.
// The mode is what decides, and a call it does not allow is refused here rather than quietly.
//
// A refusal is a plain error and not a protocol error, so the SDK answers it as a tool error the
// model reads and can act on (see the note on ToolHandlerFor in the MCP SDK's tool.go), which is what
// section 11.4's "a refused call comes back explained, not silently dropped" asks for.
func (s *Server) allow(tool, kind string) error {
	cfg, ok := s.deps.Harness()
	if !ok {
		// The session could not tell what mode this card is in. The session makes the same request a
		// person's in that case, and so does this gate: nothing is done, and the answer says why.
		s.logRefusal(tool, harness.Outcome{Decision: harness.DecisionAsk})
		return errors.New(cannotReadTheMode(tool))
	}
	out := cfg.Decide(harness.Request{Kind: kind})
	if out.Decision == harness.DecisionAllow {
		return nil
	}
	s.logRefusal(tool, out)
	return errors.New(refusal(cfg.Mode, tool, out))
}

// logRefusal notes a call that was not made, with the card, the decision, and the rule that made it,
// so a person reading the daemon's log can see what an agent asked for and why it was told no.
func (s *Server) logRefusal(tool string, out harness.Outcome) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("an MCP tool call was refused",
		"tool", tool, "card_id", s.identity.CardID,
		"decision", string(out.Decision), "rule", out.Rule)
}

// refusal is the sentence a model reads when a tool call was not made. It names the mode the card is
// in and what may be done about it, because a refusal an agent cannot act on is one it will simply
// repeat.
//
// There are three rules a call like this can meet: the mode's own (the ordinary case), the profile,
// and - for create_card - the mode again through security.ActionOther. Nothing here names a rule by
// its internal name alone: "the profile rule" is not something an agent can do anything with.
func refusal(mode protocol.PermissionMode, tool string, out harness.Outcome) string {
	switch {
	case out.Rule == harness.RuleMode && out.Decision == harness.DecisionDeny:
		return fmt.Sprintf(
			"The card's permission mode is %s, which only reads, so %s was not done. "+
				"Carry on without it; the card's owner can turn the mode up if it should be allowed.",
			mode, tool)
	case out.Rule == harness.RuleMode:
		return fmt.Sprintf(
			"The card's permission mode is %s, which leaves this to the card's owner, so %s was not done. "+
				"Say what you want to do and ask them for it; it can be done once the mode allows it.",
			mode, tool)
	case out.Rule == security.RuleProfile:
		return fmt.Sprintf(
			"Marshal's permission profile does not allow this at all, so %s was not done. "+
				"The card's owner can change the profile if it should be.", tool)
	default:
		return fmt.Sprintf("Marshal's %s rule refused %s, so it was not done.", out.Rule, tool)
	}
}

// cannotReadTheMode is the sentence for a session whose permission rules could not be read at all,
// which is the same state a session's own permission requests treat as the person's.
func cannotReadTheMode(tool string) string {
	return fmt.Sprintf(
		"Marshal could not tell what permission mode this card is in, so %s was not done. "+
			"Ask the card's owner to allow it.", tool)
}
