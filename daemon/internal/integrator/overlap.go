package integrator

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// overlapTTL is how long a project's overlaps are kept before they are worked out again. They are read
// by the Integration view and at the start of every card's turn, and a file changed a second ago
// matters less than not running a Git command per card for each of those.
const overlapTTL = 3 * time.Second

// overlapEntry is a project's overlaps and when they were worked out.
type overlapEntry struct {
	at      time.Time
	overlap []protocol.CardOverlap
}

// Overlaps answers the files that more than one unfinished card of the project has changed, as of each
// card's last commit: a card's work reaches its branch when its turn ends, so a conflict is shown as
// soon as the second card's turn that touches the file is over, and well before a merge would find it.
// The list is never nil.
func (s *Service) Overlaps(ctx context.Context, projectID string) []protocol.CardOverlap {
	if s.directWorkspace {
		return []protocol.CardOverlap{}
	}
	s.overlapMu.Lock()
	if hit, ok := s.overlapCache[projectID]; ok && s.now().Sub(hit.at) < overlapTTL {
		s.overlapMu.Unlock()
		return hit.overlap
	}
	s.overlapMu.Unlock()
	found := s.workOutOverlaps(ctx, projectID)
	s.overlapMu.Lock()
	s.overlapCache[projectID] = overlapEntry{at: s.now(), overlap: found}
	s.overlapMu.Unlock()
	return found
}

// workOutOverlaps reads each started, unfinished card's changed files against the target and groups
// them by file.
func (s *Service) workOutOverlaps(ctx context.Context, projectID string) []protocol.CardOverlap {
	overlaps := []protocol.CardOverlap{}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return overlaps
	}
	rows, err := s.ledger.Branches(ctx, projectID)
	if err != nil {
		return overlaps
	}
	type holder struct{ key, id string }
	byPath := map[string][]holder{}
	for _, row := range rows {
		if strings.TrimSpace(row.Branch) == "" || row.State == protocol.CardStateDone {
			continue
		}
		out, err := s.git.Run(ctx, project.Path, "diff", "--name-only", project.Target()+"..."+row.Branch)
		if err != nil {
			continue
		}
		for _, path := range strings.Split(out, "\n") {
			if path = strings.TrimSpace(path); path != "" {
				byPath[path] = append(byPath[path], holder{key: row.Key, id: row.ID})
			}
		}
	}
	for path, holders := range byPath {
		if len(holders) < 2 {
			continue
		}
		sort.Slice(holders, func(i, j int) bool { return holders[i].key < holders[j].key })
		overlap := protocol.CardOverlap{Path: path, Cards: make([]string, 0, len(holders)), CardIDs: make([]string, 0, len(holders))}
		for _, h := range holders {
			overlap.Cards = append(overlap.Cards, h.key)
			overlap.CardIDs = append(overlap.CardIDs, h.id)
		}
		overlaps = append(overlaps, overlap)
	}
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].Path < overlaps[j].Path })
	return overlaps
}
