package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/memory"
)

// claim_files and release_files: what a card declares it is working on, and what it gives back
// (docs/architecture.md section 11.4; docs/backend-checklist.md B7.2; build-plan task 7.3).
//
// A claim is what makes an overlap visible before it becomes a merge conflict, so the answer to a
// claim names the other cards already on the same paths. That is the whole point of the tool: a card
// that claims and is told "nobody else is on this" has learned something, and one that is told a
// card has held the file since this morning can talk to it - with ask_agent - before editing it.
//
// The warning is written here and not in the memory module because naming another card well means
// reading it through the projects service, which is this module's business.

// claimFilesInput is claim_files's arguments.
type claimFilesInput struct {
	// Paths are the files or packages this card is working on, relative to the project's repository
	// root. A package's name is as good as a path when a monorepo package is what the work is in.
	Paths []string `json:"paths"`
}

// claimFilesOut is claim_files's answer.
type claimFilesOut struct {
	// Claims are the paths this card now holds among the ones it named, in the order it named them.
	Claims []string `json:"claims"`
	// Warnings are the overlaps that were already there: one sentence per other card holding one of
	// these paths. Empty is the ordinary answer, and it is worth saying: it means the card is alone
	// on this work.
	Warnings []string `json:"warnings,omitempty"`
}

// claim_files records that this card is working on the paths it names and answers who else is.
func (s *Server) claimFiles(ctx context.Context, _ *mcp.CallToolRequest, in claimFilesInput) (*mcp.CallToolResult, claimFilesOut, error) {
	if err := s.allow("claim_files", kindWrite); err != nil {
		return nil, claimFilesOut{}, err
	}
	result, err := s.deps.Claims.Claim(ctx, s.identity.CardID, in.Paths)
	if err != nil {
		return nil, claimFilesOut{}, fmt.Errorf("claim the files: %w", err)
	}
	out := claimFilesOut{Claims: make([]string, 0, len(result.Claims))}
	for _, claim := range result.Claims {
		out.Claims = append(out.Claims, claim.PathOrPackage)
	}
	out.Warnings = s.conflictWarnings(ctx, result.Conflicts)
	return nil, out, nil
}

// releaseFilesInput is release_files's arguments.
type releaseFilesInput struct {
	// Paths are the ones to give back. A path this card does not hold is not an error: releasing
	// something twice, or releasing what a finished task left, is ordinary.
	Paths []string `json:"paths"`
}

// releaseFilesOut is release_files's answer.
type releaseFilesOut struct {
	// Released are the paths the call was asked to give back.
	Released []string `json:"released"`
	// Remaining are the paths the card still holds, so the answer says what is left rather than
	// only what went.
	Remaining []string `json:"remaining"`
}

// release_files gives back claims this card no longer needs.
func (s *Server) releaseFiles(ctx context.Context, _ *mcp.CallToolRequest, in releaseFilesInput) (*mcp.CallToolResult, releaseFilesOut, error) {
	if err := s.allow("release_files", kindWrite); err != nil {
		return nil, releaseFilesOut{}, err
	}
	if err := s.deps.Claims.Release(ctx, s.identity.CardID, in.Paths); err != nil {
		return nil, releaseFilesOut{}, fmt.Errorf("release the files: %w", err)
	}
	claims, err := s.deps.Claims.Claims(ctx, s.identity.CardID)
	if err != nil {
		return nil, releaseFilesOut{}, fmt.Errorf("read the files still held: %w", err)
	}
	out := releaseFilesOut{Released: in.Paths, Remaining: make([]string, 0, len(claims))}
	for _, claim := range claims {
		out.Remaining = append(out.Remaining, claim.PathOrPackage)
	}
	return nil, out, nil
}

// conflictWarnings writes one sentence per overlap. The other card is named by its key and its title,
// because a bare id tells an agent nothing it can act on. A card that cannot be read is skipped
// rather than failing the claim: the claim is written either way, and a name is not worth losing it
// over.
func (s *Server) conflictWarnings(ctx context.Context, conflicts []memory.Conflict) []string {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		name := conflict.CardID
		if other, err := s.deps.Cards.Card(ctx, conflict.CardID); err == nil {
			name = fmt.Sprintf("card %s (%q)", other.Key, other.Title)
		}
		out = append(out, fmt.Sprintf(
			"%s has held %s since %s. Claiming is not a lock and your claim is recorded; ask that card what it is doing there before you edit it.",
			name, conflict.PathOrPackage, conflict.ClaimedAt.UTC().Format(time.RFC3339)))
	}
	return out
}
