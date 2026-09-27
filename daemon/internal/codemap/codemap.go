// Package codemap is the codebase map: a light index of the files and symbols of each project, so an
// agent can ask where a name is written and go straight to the file instead of reading the tree
// (docs/architecture.md section 3; build-plan task 7.9).
//
// # Why it runs ctags as a program
//
// Symbols are read by universal ctags, run as an external binary. docs/library-docs.md section 2.10
// records the decision: the daemon is pure Go with no cgo, and wherever it can it runs an external
// program rather than linking a C library - gitx runs the real git, agents run as CLI processes, and
// internal/proc manages children. ctags is the option that adds no runtime and no build step.
//
// A machine with no universal ctags still gets a map: it answers from file names alone and says so
// (Notice, and the notice on the search_codebase answer). That is deliberately not a failure - "that
// name is not in this project" and "this machine cannot read symbols" are different answers and an
// agent acts differently on each.
//
// # Kept true by the file, not by the tree
//
// The first search of a project builds that project's index once. After that the map is told what
// changed - Update for a file that was written, Remove for one that went - and never walks the tree
// again while the daemon runs. A long-lived daemon therefore reads a repository once, not once per
// question. Nothing calls Update or Remove yet: the watcher that will live beside the vault's
// (slice 5) is the caller, and the seam is here for it.
package codemap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// fileKind is the kind of a match the map found by file name rather than by symbol. It is how an
	// answer says "this is the file, not the name in it".
	fileKind = "file"
	// defaultLimit is how many matches a search answers with when the caller names no limit.
	defaultLimit = 20
	// maxLimit caps a search, so a caller cannot ask for a whole repository.
	maxLimit = 200
)

// Match is one place a name is written: the answer to "where is X".
type Match struct {
	// Path is the file, relative to the project's repository root, with forward slashes.
	Path string `json:"path"`
	// Line is the one-based line the name is on. It is 0 for a match found by file name.
	Line int `json:"line"`
	// Kind is what the name is - "function", "type", "method", and so on, as ctags names it, or
	// "file" for a match found by file name.
	Kind string `json:"kind"`
	// Name is the name itself.
	Name string `json:"name"`
	// Parent is the type or scope the name belongs to, when ctags named one.
	Parent string `json:"parent,omitempty"`
}

// Runner reads the symbols of the files it is given. The daemon's runner runs universal ctags; a
// test gives its own.
type Runner interface {
	// Symbols answers the symbols the named files declare. Paths are relative to root, which is the
	// folder to run in. It answers no symbols and no error when the runner cannot read symbols at
	// all, and Notice then says why.
	Symbols(ctx context.Context, root string, paths []string) ([]Match, error)
	// Notice is the sentence to give an agent when the runner cannot read symbols - universal ctags
	// not being installed - and the map is answering from file names alone. It is empty when the
	// runner reads symbols.
	Notice() string
}

// Roots answers the folder a project's repository is in. The map indexes a project the first time it
// is searched, and cannot do that without being told where to look.
type Roots func(ctx context.Context, projectID string) (string, error)

// Deps is what a map is built from.
type Deps struct {
	// Roots finds a project's repository folder. Required.
	Roots Roots
	// Runner reads symbols. The default is universal ctags, run as a program.
	Runner Runner
	// Logger receives the lines about an index that could not be built. The default logs nothing.
	Logger *slog.Logger
	// Now is the clock an index is stamped with. The default is time.Now.
	Now func() time.Time
}

// Map is every project's index, keyed by project id. It is safe to use from several goroutines: two
// searches of the same project wait for one build rather than making two.
type Map struct {
	roots  Roots
	runner Runner
	logger *slog.Logger
	now    func() time.Time

	mu      sync.Mutex
	indexes map[string]*index
}

// index is one project's map. Its lock is taken for the length of a build, so a second search of the
// same project waits rather than walking the tree again; the map's own lock is never held while it
// is.
type index struct {
	projectID string
	root      string
	built     bool
	builtAt   time.Time

	mu      sync.RWMutex
	files   []string           // every indexed path, in name order
	symbols map[string][]Match // the symbols of a file, by path
	notice  string             // what to tell an agent when symbols could not be read
}

// Option changes how a map is built.
type Option func(*Map)

// WithLogger sends the lines about an index that could not be built to l.
func WithLogger(l *slog.Logger) Option {
	return func(m *Map) {
		if l != nil {
			m.logger = l
		}
	}
}

// WithNow sets the clock an index is stamped with. Tests use it.
func WithNow(now func() time.Time) Option {
	return func(m *Map) {
		if now != nil {
			m.now = now
		}
	}
}

// New builds the map.
func New(deps Deps, opts ...Option) (*Map, error) {
	if deps.Roots == nil {
		return nil, errors.New("codemap: a way to find a project's folder is required")
	}
	if deps.Runner == nil {
		deps.Runner = NewRunner()
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m := &Map{
		roots: deps.Roots, runner: deps.Runner, logger: deps.Logger, now: deps.Now,
		indexes: map[string]*index{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m, nil
}

// Notice is what to tell an agent when this machine cannot read symbols and the map only knows file
// names. It is empty when symbols are read.
func (m *Map) Notice() string { return m.runner.Notice() }

// Search answers where a name is written in a project, most relevant first. An exact name comes
// before a name that starts with the query, which comes before one that contains it; ties are in
// file order.
//
// When no symbol matches - including on a machine that cannot read symbols - the answer falls back
// to the files whose name contains the query, each as a match of kind "file".
func (m *Map) Search(ctx context.Context, projectID, query string, limit int) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)

	ix, err := m.load(ctx, projectID)
	if err != nil {
		return nil, err
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.find(query, limit), nil
}

// Index builds a project's whole index now, rather than leaving it to the first search. A search
// that finds no index builds it anyway, so this is for the caller that would rather pay for it at a
// moment of its choosing.
func (m *Map) Index(ctx context.Context, projectID string) error {
	_, err := m.load(ctx, projectID)
	return err
}

// Update re-reads one file. It is how a running daemon keeps the map true without re-reading the
// tree: the caller says which file was written. A file the index does not have is added; a project
// with no index yet is left alone, since the next search builds the whole thing.
func (m *Map) Update(ctx context.Context, projectID, path string) error {
	ix := m.lookup(projectID)
	if ix == nil {
		return nil
	}
	rel, ok := ix.relative(path)
	if !ok {
		// Not this project's business: a path outside the repository is another project's, or no
		// project's.
		return nil
	}
	matches, err := m.runner.Symbols(ctx, ix.root, []string{rel})
	if err != nil {
		return err
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if !ix.built {
		return nil
	}
	ix.setFile(rel, matches)
	if !slices.Contains(ix.files, rel) {
		ix.files = append(ix.files, rel)
		slices.Sort(ix.files)
	}
	return nil
}

// Remove forgets a file that has gone from the repository. Like Update it leaves an unindexed
// project alone.
func (m *Map) Remove(projectID, path string) {
	ix := m.lookup(projectID)
	if ix == nil {
		return
	}
	rel, ok := ix.relative(path)
	if !ok {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	delete(ix.symbols, rel)
	if at := slices.Index(ix.files, rel); at >= 0 {
		ix.files = slices.Delete(ix.files, at, at+1)
	}
}

// Forget drops a project's whole index, which is what a project that was removed or moved needs.
func (m *Map) Forget(projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.indexes, projectID)
}

// load answers a project's index, building it if this is the first time. Two searches of the same
// project wait for one build; searches of other projects are not held up by it.
func (m *Map) load(ctx context.Context, projectID string) (*index, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, errors.New("codemap: a project is required")
	}
	ix := m.lookup(projectID)
	if ix == nil {
		m.mu.Lock()
		ix = m.indexes[projectID]
		if ix == nil {
			ix = &index{projectID: projectID, symbols: map[string][]Match{}}
			m.indexes[projectID] = ix
		}
		m.mu.Unlock()
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.built {
		return ix, nil
	}
	root, err := m.roots(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("find the folder of project %s: %w", projectID, err)
	}
	files, err := sourceFiles(root)
	if err != nil {
		return nil, fmt.Errorf("read the files of project %s: %w", projectID, err)
	}
	matches, err := m.runner.Symbols(ctx, root, files)
	if err != nil {
		return nil, fmt.Errorf("read the symbols of project %s: %w", projectID, err)
	}
	symbols := map[string][]Match{}
	for _, match := range matches {
		if match.Path == "" {
			continue
		}
		symbols[match.Path] = append(symbols[match.Path], match)
	}
	ix.root = root
	ix.files = files
	ix.symbols = symbols
	ix.notice = m.runner.Notice()
	ix.built = true
	ix.builtAt = m.now()
	m.logger.Debug("indexed a project", "project", projectID, "files", len(files),
		"symbols", len(matches), "notice", ix.notice)
	return ix, nil
}

// lookup answers a project's index if it has one, without building it.
func (m *Map) lookup(projectID string) *index {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.indexes[projectID]
}

// relative answers a path as the index writes it, and whether it is the project's at all.
func (ix *index) relative(path string) (string, bool) {
	if ix.root == "" || path == "" {
		return "", false
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(ix.root, path)
	}
	rel, err := filepath.Rel(ix.root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// setFile replaces one file's symbols, keeping them in line order.
func (ix *index) setFile(rel string, matches []Match) {
	if len(matches) == 0 {
		delete(ix.symbols, rel)
		return
	}
	kept := make([]Match, 0, len(matches))
	for _, match := range matches {
		match.Path = rel
		kept = append(kept, match)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Line != kept[j].Line {
			return kept[i].Line < kept[j].Line
		}
		return kept[i].Name < kept[j].Name
	})
	ix.symbols[rel] = kept
}

// find answers the matches for a query, which the caller has already trimmed.
func (ix *index) find(query string, limit int) []Match {
	type hit struct {
		match Match
		rank  int
	}
	var hits []hit
	for _, matches := range ix.symbols {
		for _, match := range matches {
			if rank := rankOf(match, query); rank >= 0 {
				hits = append(hits, hit{match: match, rank: rank})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		if hits[i].match.Path != hits[j].match.Path {
			return hits[i].match.Path < hits[j].match.Path
		}
		if hits[i].match.Line != hits[j].match.Line {
			return hits[i].match.Line < hits[j].match.Line
		}
		return hits[i].match.Name < hits[j].match.Name
	})

	if len(hits) > 0 {
		out := make([]Match, 0, min(limit, len(hits)))
		for _, item := range hits {
			if len(out) == limit {
				break
			}
			out = append(out, item.match)
		}
		return out
	}
	return ix.findFiles(query, limit)
}

// findFiles answers the files whose name contains the query. It is what a machine that cannot read
// symbols can still do, and what a search for a file name gets when no symbol matched.
func (ix *index) findFiles(query string, limit int) []Match {
	lower := strings.ToLower(query)
	out := make([]Match, 0, limit)
	for _, path := range ix.files {
		base := path[strings.LastIndexByte(path, '/')+1:]
		if !strings.Contains(strings.ToLower(base), lower) {
			continue
		}
		out = append(out, Match{Path: path, Kind: fileKind, Name: base})
		if len(out) == limit {
			break
		}
	}
	return out
}

// rankOf scores one symbol against the query: 0 for the name itself, 1 for a name that starts with
// it, 2 for one that contains it, and -1 for no match. A query with a dot is also matched against
// the qualified name, so "Manager.Send" finds the method.
func rankOf(match Match, query string) int {
	best := -1
	labels := []string{match.Name}
	if match.Parent != "" {
		labels = append(labels, match.Parent+"."+match.Name)
	}
	for _, label := range labels {
		if label == "" {
			continue
		}
		rank := rankOfName(label, query)
		if rank >= 0 && (best < 0 || rank < best) {
			best = rank
		}
	}
	return best
}

func rankOfName(name, query string) int {
	lowerName, lowerQuery := strings.ToLower(name), strings.ToLower(query)
	switch {
	case lowerName == lowerQuery:
		return 0
	case strings.HasPrefix(lowerName, lowerQuery):
		return 1
	case strings.Contains(lowerName, lowerQuery):
		return 2
	default:
		return -1
	}
}
