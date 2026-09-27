package providers

// This file owns the ceilings on what Marshal may spend and how many cards it may keep awake at
// once: the `limits` table of docs/architecture.md section 10, inventory B4.5, build-plan task 4.8.
// A limit is a setting a person edits on Settings > Limits (S26b) and a ceiling the daemon will act
// on in Phase 5 (B5.3 pauses cards at a cost limit, B5.6 at the awake limit and the sleep reminder):
// this file only reads and writes the numbers. Nothing here pauses a card, and no notices table
// exists yet, so nothing here makes a notice either.
//
// There are three ceilings per scope - today's cost, this month's cost, and the awake card count -
// because that is what the form has a field for (apps/web/src/views/settings/limit-rows.ts,
// LIMIT_SPECS). Money is in micro-dollars, the unit the rest of Marshal counts money in (pricing.go);
// the awake limit is a count of cards and not a length of time (protocol.LimitKindAwake).
//
// It holds the store rather than the keychain, because a ceiling is not a secret: this is the half of
// the providers package a daemon could run with no key stored at all.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Limits reads and writes the ceilings in the `limits` table. One is built when the daemon starts
// and it is safe for concurrent use: the settings screen saves a limit while the usage writer is
// filing calls.
type Limits struct {
	store *store.Store
}

// NewLimits returns the limits store over st. The store is required: a limits service with nowhere
// to read would report every scope as unlimited, which is the answer a person gets when nothing is
// set and so the one answer that must never be given by mistake.
func NewLimits(st *store.Store) *Limits {
	return &Limits{store: st}
}

// List returns every ceiling that is set, the global ones first and, within a scope, the three kinds
// in the order the settings form shows them. A scope with no ceiling has no entry, so this is empty
// on a fresh install - which is the shipped state: nothing is limited until a person sets a limit.
//
// The list is ordered here and not by the query, because the order is a screen's, not a database's:
// the query orders by (scope, kind) so a reader's answer is stable, and this puts "global" ahead of
// the project ids (which sort before it alphabetically: they start with digits) and the kinds in
// LIMIT_SPECS' order rather than alphabetically.
func (l *Limits) List(ctx context.Context) (protocol.LimitList, error) {
	if err := l.ready(); err != nil {
		return protocol.LimitList{}, err
	}
	rows, err := l.store.Queries().ListLimits(ctx)
	if err != nil {
		return protocol.LimitList{}, fmt.Errorf("read the limits: %w", err)
	}
	out := make([]protocol.Limit, 0, len(rows))
	for _, row := range rows {
		out = append(out, protocol.Limit{
			Scope: row.Scope,
			Kind:  protocol.LimitKind(row.Kind),
			Value: row.Value,
		})
	}
	sortLimits(out)
	return protocol.NewLimitList(out), nil
}

// Set stores one ceiling, replacing whatever that scope had of that kind, and answers with the whole
// list so a screen redraws its form from one answer. Saving twice leaves one ceiling, which is what
// editing a number does.
//
// The value is checked first, so a number the screens will not accept is refused with the sentence
// the form itself shows rather than stored and read back on the next open.
func (l *Limits) Set(ctx context.Context, scope string, kind protocol.LimitKind, value int64) (protocol.LimitList, error) {
	if err := l.ready(); err != nil {
		return protocol.LimitList{}, err
	}
	if err := checkLimit(scope, kind, value); err != nil {
		return protocol.LimitList{}, err
	}
	err := l.store.Write(ctx, func(q *db.Queries) error {
		return q.SetLimit(ctx, db.SetLimitParams{Scope: scope, Kind: string(kind), Value: value})
	})
	if err != nil {
		return protocol.LimitList{}, fmt.Errorf("save the %s limit for %q: %w", kind, scope, err)
	}
	return l.List(ctx)
}

// Delete removes one ceiling and answers with the whole list. A scope that had no ceiling of that
// kind is not an error: there was nothing to remove and the answer is the same list either way,
// which is how clearing a field twice behaves on the screen.
func (l *Limits) Delete(ctx context.Context, scope string, kind protocol.LimitKind) (protocol.LimitList, error) {
	if err := l.ready(); err != nil {
		return protocol.LimitList{}, err
	}
	if err := checkLimitKey(scope, kind); err != nil {
		return protocol.LimitList{}, err
	}
	err := l.store.Write(ctx, func(q *db.Queries) error {
		return q.DeleteLimit(ctx, db.DeleteLimitParams{Scope: scope, Kind: string(kind)})
	})
	if err != nil {
		return protocol.LimitList{}, fmt.Errorf("remove the %s limit for %q: %w", kind, scope, err)
	}
	return l.List(ctx)
}

// ready reports whether the service can reach its store, so a daemon wired without one fails loudly
// instead of reporting that nothing is limited.
func (l *Limits) ready() error {
	if l == nil || l.store == nil {
		return errors.New("providers: a limits service needs a store")
	}
	return nil
}

// AwakeLimit answers how many cards a project may keep awake at once, and false when no ceiling
// says so. A project's own ceiling wins over the whole install's, which is the rule for every
// limit; a project that sets none follows the global one, and an install that sets neither keeps as
// many cards awake as it likes. It is what the session manager asks when a card must wake and the
// limit is full (docs/architecture.md section 5.1, B5.6).
func (l *Limits) AwakeLimit(ctx context.Context, projectID string) (int, bool, error) {
	if err := l.ready(); err != nil {
		return 0, false, err
	}
	for _, scope := range []string{projectID, protocol.LimitScopeGlobal} {
		if scope == "" {
			continue
		}
		value, err := l.store.Queries().GetLimit(ctx, db.GetLimitParams{Scope: scope, Kind: string(protocol.LimitKindAwake)})
		switch {
		case err == nil:
			return int(value), true, nil
		case store.IsNotFound(err):
			continue
		default:
			return 0, false, fmt.Errorf("read the awake limit for %q: %w", scope, err)
		}
	}
	return 0, false, nil
}

// sortLimits puts the global scope first, then the project scopes by id, and within a scope the
// kinds in the order LimitKindValues gives them - the same order the settings form has.
func sortLimits(limits []protocol.Limit) {
	sort.SliceStable(limits, func(i, j int) bool {
		a, b := limits[i], limits[j]
		if ar, br := scopeRank(a.Scope), scopeRank(b.Scope); ar != br {
			return ar < br
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		return kindRank(a.Kind) < kindRank(b.Kind)
	})
}

// scopeRank is 0 for the whole install and 1 for one project, so the global ceilings come first.
func scopeRank(scope string) int {
	if scope == protocol.LimitScopeGlobal {
		return 0
	}
	return 1
}

// kindRank is a kind's place in LimitKindValues, or the end of the list for a kind the constants do
// not name. A stored kind is always one of the constants (Set refuses any other), so the fallback is
// only for a row written by an older or newer build; it keeps such a row last rather than dropping
// it from a screen.
func kindRank(kind protocol.LimitKind) int {
	for i, known := range protocol.LimitKindValues() {
		if known == kind {
			return i
		}
	}
	return len(protocol.LimitKindValues())
}

// checkLimit refuses a ceiling a screen would not be allowed to save.
func checkLimit(scope string, kind protocol.LimitKind, value int64) error {
	if err := checkLimitKey(scope, kind); err != nil {
		return err
	}
	if value <= 0 {
		// The form's own words (apps/web/src/views/settings/limit-rows.ts): zero is not a way to
		// say "no limit", it is a number the person has not finished typing.
		return protocol.InvalidArgument("Enter a limit above zero.")
	}
	return nil
}

// checkLimitKey refuses a scope or a kind Marshal has no meaning for. The scope is checked only for
// being present: a project's id is its own slug ("web-dashboard"), not an opaque id, and a ceiling
// set for a project that is not here yet is harmless, so its shape is not a thing to refuse.
func checkLimitKey(scope string, kind protocol.LimitKind) error {
	if scope == "" {
		return protocol.InvalidArgument("Say which scope the limit is for.")
	}
	if !kind.Valid() {
		return protocol.InvalidArgument(fmt.Sprintf(
			"There is no %q limit. A limit is a daily cost, a monthly cost, or the number of awake cards.",
			kind))
	}
	return nil
}
