package cardpanel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/localci"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	checkKindCommand = "command"
	checkKindReview  = "review"
	maxChecksPerCard = 20
	maxCheckName     = 80
	maxCheckCommand  = 500

	messageNoWorktree = "This card has no worktree yet, so its checks cannot run. Start the card first."
)

type checkSpec struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// defaultChecks are the checks a card starts with, chosen by the project's language. A language
// Marshal has no known commands for gets only the reviewer's approval.
func defaultChecks(language string) []checkSpec {
	var list []checkSpec
	switch l := strings.ToLower(language); {
	case l == "go":
		list = []checkSpec{{"Tests pass", "go test ./..."}, {"Lint clean", "go vet ./..."}}
	case strings.Contains(l, "script"):
		list = []checkSpec{{"Tests pass", "npm test --silent"}, {"Lint clean", "npm run lint --silent"}}
	case l == "rust":
		list = []checkSpec{{"Tests pass", "cargo test"}, {"Lint clean", "cargo clippy"}}
	}
	return append(list, checkSpec{Name: "Reviewer approval"})
}

func checkOf(row db.CardCheck, card db.Card) protocol.CardCheck {
	var spec checkSpec
	_ = json.Unmarshal([]byte(row.SpecJSON), &spec)
	out := protocol.CardCheck{
		ID: row.ID, Name: spec.Name, Kind: row.Kind, Command: spec.Command,
		Status: protocol.CheckStatus(row.Status), RunRef: row.RunRef,
	}
	if row.Kind == checkKindReview {
		// The reviewer's approval is the card's own state, so it is never stored twice.
		out.Status = protocol.CheckStatusPending
		switch protocol.CardState(card.State) {
		case protocol.CardStateReady, protocol.CardStateMerging, protocol.CardStateDone:
			out.Status = protocol.CheckStatusPassed
		}
	}
	return out
}

// Checks lists a card's acceptance checks. A card that has none yet is given its defaults first.
func (s *Service) Checks(ctx context.Context, cardID string) (protocol.CardCheckList, error) {
	card, err := s.card(ctx, cardID)
	if err != nil {
		return protocol.CardCheckList{}, err
	}
	if err := s.seedChecks(ctx, card); err != nil {
		return protocol.CardCheckList{}, err
	}
	return s.checkList(ctx, card)
}

func (s *Service) checkList(ctx context.Context, card db.Card) (protocol.CardCheckList, error) {
	var rows []db.CardCheck
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		rows, err = q.ListCardChecks(ctx, card.ID)
		return err
	})
	if err != nil {
		return protocol.CardCheckList{}, fmt.Errorf("list the checks of card %s: %w", card.ID, err)
	}
	out := protocol.CardCheckList{Checks: make([]protocol.CardCheck, 0, len(rows)), ServerTime: s.serverTime()}
	for _, row := range rows {
		out.Checks = append(out.Checks, checkOf(row, card))
	}
	return out, nil
}

func (s *Service) seedChecks(ctx context.Context, card db.Card) error {
	var have int64
	var language string
	err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		if have, err = q.CountCardChecks(ctx, card.ID); err != nil || have > 0 {
			return err
		}
		project, err := q.GetProject(ctx, card.ProjectID)
		language = project.Language
		return err
	})
	if err != nil || have > 0 {
		return err
	}
	for i, spec := range defaultChecks(language) {
		if err := s.insertCheck(ctx, card.ID, spec, int64(i)); err != nil {
			return err
		}
	}
	return nil
}

// insertCheck writes one check. `offset` is added to the time so checks made together keep their order.
func (s *Service) insertCheck(ctx context.Context, cardID string, spec checkSpec, offset int64) error {
	id, err := s.newID()
	if err != nil {
		return fmt.Errorf("make a check id: %w", err)
	}
	kind := checkKindCommand
	if spec.Command == "" {
		kind = checkKindReview
	}
	body, _ := json.Marshal(spec)
	now := ms(s.now()) + offset
	return s.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateCardCheck(ctx, db.CreateCardCheckParams{
			ID: id, CardID: cardID, Kind: kind, SpecJSON: string(body),
			Status: string(protocol.CheckStatusPending), CreatedAt: now, UpdatedAt: now,
		})
	})
}

// AddCheck adds a command check to a card.
func (s *Service) AddCheck(ctx context.Context, cardID string, req protocol.AddCardCheckRequest) (protocol.CardCheckList, error) {
	card, err := s.card(ctx, cardID)
	if err != nil {
		return protocol.CardCheckList{}, err
	}
	name, command := strings.TrimSpace(req.Name), strings.TrimSpace(req.Command)
	switch {
	case name == "" || command == "":
		return protocol.CardCheckList{}, protocol.InvalidArgument("A check needs a name and a command.")
	case len(name) > maxCheckName || len(command) > maxCheckCommand:
		return protocol.CardCheckList{}, protocol.InvalidArgument("That check's name or command is too long.")
	}
	if err := s.seedChecks(ctx, card); err != nil {
		return protocol.CardCheckList{}, err
	}
	if list, err := s.checkList(ctx, card); err != nil || len(list.Checks) >= maxChecksPerCard {
		if err != nil {
			return protocol.CardCheckList{}, err
		}
		return protocol.CardCheckList{}, protocol.InvalidArgument("A card can have at most 20 checks.")
	}
	if err := s.insertCheck(ctx, cardID, checkSpec{Name: name, Command: command}, 0); err != nil {
		return protocol.CardCheckList{}, err
	}
	return s.checkList(ctx, card)
}

// RemoveCheck removes a check. A check that is not there is not an error.
func (s *Service) RemoveCheck(ctx context.Context, cardID, checkID string) (protocol.CardCheckList, error) {
	card, err := s.card(ctx, cardID)
	if err != nil {
		return protocol.CardCheckList{}, err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetCardCheck(ctx, checkID)
		if err != nil || row.CardID != cardID {
			return nil
		}
		return q.DeleteCardCheck(ctx, checkID)
	})
	if err != nil {
		return protocol.CardCheckList{}, fmt.Errorf("remove check %s: %w", checkID, err)
	}
	return s.checkList(ctx, card)
}

// RunChecks runs every command check in the card's worktree, one after another, and records what
// each found with a reference to this run. A check that could not be started counts as failed.
func (s *Service) RunChecks(ctx context.Context, cardID string) (protocol.CardCheckList, error) {
	card, err := s.card(ctx, cardID)
	if err != nil {
		return protocol.CardCheckList{}, err
	}
	if err := s.seedChecks(ctx, card); err != nil {
		return protocol.CardCheckList{}, err
	}
	dir, err := s.worktreeOf(ctx, cardID)
	if err != nil {
		return protocol.CardCheckList{}, err
	}
	var rows []db.CardCheck
	if err := s.store.Read(ctx, func(q *db.Queries) (err error) {
		rows, err = q.ListCardChecks(ctx, cardID)
		return err
	}); err != nil {
		return protocol.CardCheckList{}, fmt.Errorf("list the checks of card %s: %w", cardID, err)
	}
	ref := fmt.Sprintf("run-%d", ms(s.now()))
	for _, row := range rows {
		if row.Kind != checkKindCommand {
			continue
		}
		var spec checkSpec
		_ = json.Unmarshal([]byte(row.SpecJSON), &spec)
		status := protocol.CheckStatusPassed
		result, runErr := s.runner.Run(ctx, localci.RunRequest{Dir: dir, Command: spec.Command})
		if runErr != nil || result.Failed {
			status = protocol.CheckStatusFailed
		}
		if err := s.store.Write(ctx, func(q *db.Queries) error {
			return q.UpdateCardCheckStatus(ctx, db.UpdateCardCheckStatusParams{
				Status: string(status), RunRef: ref, UpdatedAt: ms(s.now()), ID: row.ID,
			})
		}); err != nil {
			return protocol.CardCheckList{}, fmt.Errorf("record check %s: %w", row.ID, err)
		}
	}
	s.untickFailedEvidence(ctx, cardID)
	s.publish(cardID, protocol.EventTypeChecklistUpdated)
	return s.checkList(ctx, card)
}

func (s *Service) worktreeOf(ctx context.Context, cardID string) (string, error) {
	if s.worktrees == nil {
		return "", protocol.Refused(messageNoWorktree)
	}
	dir, _, err := s.worktrees.Worktree(ctx, cardID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dir) == "" {
		return "", protocol.Refused(messageNoWorktree)
	}
	return dir, nil
}

// UnpassedChecks says how many of a card's checks have not passed. A review check counts as passed
// once the card is ready to merge, the same as the list shows.
func (s *Service) UnpassedChecks(ctx context.Context, cardID string) (int, error) {
	list, err := s.Checks(ctx, cardID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range list.Checks {
		if c.Kind == checkKindCommand && c.Status != protocol.CheckStatusPassed {
			n++
		}
	}
	return n, nil
}
