package session

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
)

func editSignalOf(path, fingerprint string, diffs []agents.FileDiff, truncated bool) harness.Signal {
	if path == "" && len(diffs) == 1 {
		path = diffs[0].Path
	}
	if path == "" && len(diffs) > 1 {
		paths := make([]string, 0, len(diffs))
		for _, diff := range diffs {
			paths = append(paths, diff.Path)
		}
		sort.Strings(paths)
		path = strings.Join(paths, ", ")
	}
	if fingerprint == "" && len(diffs) > 0 && !truncated {
		ordered := slices.Clone(diffs)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
		encoded, err := json.Marshal(ordered)
		if err == nil {
			fingerprint = agents.ToolInputFingerprint(encoded)
		}
	}
	path = strings.TrimSpace(path)
	return harness.Signal{Kind: harness.SignalEdit, Key: path, Where: path, Fingerprint: fingerprint}
}

// Count completed edits, not attempts that may fail. The metadata is bounded
// just like toolPaths, and is discarded when a tool completes or fails.
func (ls *liveSession) rememberEdit(id string, signal harness.Signal) {
	if id == "" {
		return
	}
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	if ls.toolEdits == nil || len(ls.toolEdits) >= maxRememberedToolPaths {
		ls.toolEdits = make(map[string]harness.Signal)
	}
	ls.toolEdits[id] = signal
}

func (ls *liveSession) takeEdit(id string) (harness.Signal, bool) {
	ls.turnMu.Lock()
	defer ls.turnMu.Unlock()
	signal, ok := ls.toolEdits[id]
	delete(ls.toolEdits, id)
	return signal, ok
}
