package projects

import (
	"context"
	"path/filepath"
	"sync"
)

// repoLocks lets one Git write at a time run in each repository. Two sparse worktree adds at the
// same moment can collide on Git's config lock, and one of them would roll back, so writes to a
// repository take turns. A wait ends when the caller's context does.
type repoLocks struct {
	mu   sync.Mutex
	sems map[string]chan struct{}
}

// acquire waits for the repository's turn and returns the function that gives it back.
func (l *repoLocks) acquire(ctx context.Context, repo string) (release func(), err error) {
	sem := l.semaphore(filepath.Clean(repo))
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// semaphore returns the repository's lock, making it the first time. There is one small entry per
// project, so the map is bounded by the number of projects.
func (l *repoLocks) semaphore(repo string) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.sems == nil {
		l.sems = make(map[string]chan struct{})
	}
	sem, ok := l.sems[repo]
	if !ok {
		sem = make(chan struct{}, 1)
		l.sems[repo] = sem
	}
	return sem
}
