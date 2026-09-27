package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The files and packages a card is working on (docs/architecture.md section 10's `file_claims` row
// and section 11.4's `claim_files` and `release_files`; docs/backend-checklist.md B7.2, build-plan
// task 7.3).
//
// A claim is a claim on a thing and not a lock on it. Two cards may hold the same path; what the
// daemon does about that is warn both of them early, which is why claiming answers with the other
// cards already on the paths claimed rather than refusing the second card. Refusing would make an
// agent's first useful act - saying what it is about to touch - the thing that blocks it.
//
// Nothing is enforced from these rows yet beyond that warning. What a claim should do to the merge
// queue and to two cards editing one file is a decision for the slices that build those (B7.2's
// warning sentence, and the merge queue of Phase 5), and this file deliberately does no more than
// record who holds what.

const (
	// maxClaimsPerCall caps how many paths one call may claim. An agent naming its files names a
	// handful; a call naming hundreds is a mistake or an abuse, and either way the answer is to say
	// so rather than to write the rows.
	maxClaimsPerCall = 50
	// maxClaimLength caps one path or package name. It is longer than any real path in a repository
	// and short enough that a whole call's worth of them stays a reasonable row set.
	maxClaimLength = 512
)

// Claim is one path or package a card holds.
type Claim struct {
	// CardID is the card holding it.
	CardID string
	// ProjectID is the card's project, stored beside the card so a project's claims are read
	// without joining the projects module's `cards` table.
	ProjectID string
	// PathOrPackage is what is held: a path inside the repository, or the name of a package in a
	// monorepo (section 11.4 calls the tool's argument by the same name).
	PathOrPackage string
	// ClaimedAt is when the card first claimed it. Claiming something again does not move it: the
	// age of a claim is how long the card has had it, not how many times it said so.
	ClaimedAt time.Time
}

// Conflict is another card already holding a path this card has just claimed. It is what the early
// warning is made of. The card is named by its id and not by its key or title: what to call it is
// the app's business, and whoever writes the warning sentence reads the card through the projects
// service to name it.
type Conflict struct {
	// PathOrPackage is the path both cards hold.
	PathOrPackage string
	// CardID is the other card.
	CardID string
	// ClaimedAt is when the other card claimed it.
	ClaimedAt time.Time
}

// ClaimResult is what a claim answers with: the claims the card now holds among the ones it named,
// and the other cards already on them. An empty Conflicts is the ordinary answer.
type ClaimResult struct {
	Claims    []Claim
	Conflicts []Conflict
}

// Claim records that a card is working on the paths it names, and answers with the other cards
// already on them. A path the card already holds is left alone, so claiming it again is not an
// error and does not move its time.
func (s *Service) Claim(ctx context.Context, cardID string, paths []string) (ClaimResult, error) {
	wanted, err := cleanClaimPaths(paths)
	if err != nil {
		return ClaimResult{}, err
	}
	card, _, err := s.cardAndProject(ctx, cardID)
	if err != nil {
		return ClaimResult{}, err
	}
	now := s.now().UnixMilli()
	err = s.store.Write(ctx, func(q *db.Queries) error {
		for _, path := range wanted {
			err := q.InsertFileClaim(ctx, db.InsertFileClaimParams{
				CardID: card.ID, ProjectID: card.ProjectID, PathOrPackage: path, ClaimedAt: now,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, fmt.Errorf("claim files for card %s: %w", card.ID, err)
	}
	return s.claimResult(ctx, card.ProjectID, card.ID, wanted)
}

// claimResult reads back what the card holds among the paths it just named, and who else holds them.
// Both reads run in one read transaction, so the answer is one moment of the table rather than two.
func (s *Service) claimResult(ctx context.Context, projectID, cardID string, paths []string) (ClaimResult, error) {
	out := ClaimResult{Claims: []Claim{}, Conflicts: []Conflict{}}
	err := s.store.Read(ctx, func(q *db.Queries) error {
		mine, err := q.ListFileClaimsOfCard(ctx, cardID)
		if err != nil {
			return err
		}
		held := make(map[string]bool, len(paths))
		for _, path := range paths {
			held[path] = true
		}
		for _, row := range mine {
			if held[row.PathOrPackage] {
				out.Claims = append(out.Claims, claimOf(row))
			}
		}
		for _, path := range paths {
			others, err := q.ListFileClaimsOnPath(ctx, db.ListFileClaimsOnPathParams{
				ProjectID: projectID, PathOrPackage: path, CardID: cardID,
			})
			if err != nil {
				return err
			}
			for _, row := range others {
				out.Conflicts = append(out.Conflicts, Conflict{
					PathOrPackage: row.PathOrPackage, CardID: row.CardID,
					ClaimedAt: time.UnixMilli(row.ClaimedAt).UTC(),
				})
			}
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, fmt.Errorf("read back the claims of card %s: %w", cardID, err)
	}
	return out, nil
}

// Release gives up the paths a card named. Naming no paths at all releases everything the card
// holds, which is what the end of a card's work does; an empty list is not a way to release nothing,
// because a call that does nothing and a call that releases everything are the same call and only
// one of them is useful.
//
// Releasing something the card does not hold is not an error: the answer is the same list either
// way, so an agent that releases a path twice is not punished for it.
func (s *Service) Release(ctx context.Context, cardID string, paths []string) error {
	card, _, err := s.cardAndProject(ctx, cardID)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		if err := s.store.Write(ctx, func(q *db.Queries) error {
			return q.DeleteFileClaimsOfCard(ctx, card.ID)
		}); err != nil {
			return fmt.Errorf("release every claim of card %s: %w", card.ID, err)
		}
		return nil
	}
	wanted, err := cleanClaimPaths(paths)
	if err != nil {
		return err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		for _, path := range wanted {
			err := q.DeleteFileClaim(ctx, db.DeleteFileClaimParams{
				CardID: card.ID, PathOrPackage: path,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("release claims of card %s: %w", card.ID, err)
	}
	return nil
}

// Claims returns what one card holds, oldest first.
func (s *Service) Claims(ctx context.Context, cardID string) ([]Claim, error) {
	rows, err := s.store.Queries().ListFileClaimsOfCard(ctx, cardID)
	if err != nil {
		return nil, fmt.Errorf("read the claims of card %s: %w", cardID, err)
	}
	return claimsOf(rows), nil
}

// ProjectClaims returns what every card of one project holds, oldest first. It is what the
// board-awareness summary is built from (B7.2, task 7.2) and what a project's claim list shows.
func (s *Service) ProjectClaims(ctx context.Context, projectID string) ([]Claim, error) {
	rows, err := s.store.Queries().ListFileClaimsOfProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the claims of project %s: %w", projectID, err)
	}
	return claimsOf(rows), nil
}

// cleanClaimPaths checks and tidies the paths one call names: no empty ones, no absolute ones, none
// that climb out of the repository, no duplicates, and not more than a call may name. It answers
// with them in the order they were named and with the duplicates dropped, so a call naming the same
// path twice claims it once.
func cleanClaimPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, protocol.InvalidArgument("Name at least one file or package to claim.")
	}
	if len(paths) > maxClaimsPerCall {
		return nil, protocol.InvalidArgument(fmt.Sprintf("Claim at most %d files or packages at once.", maxClaimsPerCall)).
			With("count", fmt.Sprintf("%d", len(paths)))
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		switch {
		case path == "":
			return nil, protocol.InvalidArgument("A claimed file or package cannot be blank.")
		case len(path) > maxClaimLength:
			return nil, protocol.InvalidArgument("That path is too long to claim.").
				With("length", fmt.Sprintf("%d", len(path)))
		case strings.HasPrefix(path, "/"), strings.HasPrefix(path, `\`):
			return nil, protocol.InvalidArgument("Claim a path inside the repository, not an absolute one.").
				With("path", path)
		case path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") ||
			strings.HasSuffix(path, "/.."):
			return nil, protocol.InvalidArgument("Claim a path inside the repository, not one above it.").
				With("path", path)
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out, nil
}

func claimsOf(rows []db.FileClaim) []Claim {
	out := make([]Claim, 0, len(rows))
	for _, row := range rows {
		out = append(out, claimOf(row))
	}
	return out
}

func claimOf(row db.FileClaim) Claim {
	return Claim{
		CardID: row.CardID, ProjectID: row.ProjectID, PathOrPackage: row.PathOrPackage,
		ClaimedAt: time.UnixMilli(row.ClaimedAt).UTC(),
	}
}
