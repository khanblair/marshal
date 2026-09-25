package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	sessionFileExt   = ".json"
	sessionIDPrefix  = "stub-"
	maxSessionIDLen  = 128
	stateDirMode     = 0o755
	stateTempPattern = "session-*.tmp"
)

// errSessionNotFound says that no saved session has the requested id.
var errSessionNotFound = errors.New("session not found")

// turnRecord is one finished turn: what the user said and what the agent said back.
type turnRecord struct {
	User  string `json:"user"`
	Agent string `json:"agent"`
}

// sessionState is everything about a session that survives a restart. Cancelled turns count, so
// the counter and the list never disagree about how much the agent remembers.
type sessionState struct {
	ID    string       `json:"sessionId"`
	Cwd   string       `json:"cwd"`
	Turn  int          `json:"turnCounter"`
	Turns []turnRecord `json:"turns"`
}

// store keeps one JSON file per session in a folder.
type store struct {
	dir string
}

// file returns the path of a session's file. The id comes from the client, so it is checked before
// it becomes part of a path.
func (s store) file(id string) (string, error) {
	if !validSessionID(id) {
		// An id that cannot be a file name cannot name a saved session either.
		return "", fmt.Errorf("%w: %q is not a valid session id", errSessionNotFound, id)
	}
	return filepath.Join(s.dir, id+sessionFileExt), nil
}

// save writes the session through a temporary file and a rename, so a process that is killed in
// the middle never leaves half a file for the next process to read.
func (s store) save(st sessionState) error {
	target, err := s.file(st.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session %s: %w", st.ID, err)
	}
	if err := os.MkdirAll(s.dir, stateDirMode); err != nil {
		return fmt.Errorf("create state folder: %w", err)
	}
	if err := writeFileAtomic(target, data); err != nil {
		return fmt.Errorf("save session %s: %w", st.ID, err)
	}
	return nil
}

// writeFileAtomic replaces the target file with data in one step.
func writeFileAtomic(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), stateTempPattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), target)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", filepath.Base(target), err)
	}
	return nil
}

// load reads a session back. A missing file is errSessionNotFound.
func (s store) load(id string) (sessionState, error) {
	target, err := s.file(id)
	if err != nil {
		return sessionState{}, err
	}
	data, err := os.ReadFile(target)
	if errors.Is(err, fs.ErrNotExist) {
		return sessionState{}, fmt.Errorf("%w: %s", errSessionNotFound, id)
	}
	if err != nil {
		return sessionState{}, fmt.Errorf("read session %s: %w", id, err)
	}
	var st sessionState
	if err := json.Unmarshal(data, &st); err != nil {
		return sessionState{}, fmt.Errorf("decode session %s: %w", id, err)
	}
	if st.ID != id {
		return sessionState{}, fmt.Errorf("session file %s holds session %q", id, st.ID)
	}
	return st, nil
}

// newSessionID makes a random id that is safe to use as a file name.
func newSessionID() string {
	return sessionIDPrefix + strings.ToLower(rand.Text())
}

// validSessionID accepts only letters, digits, dash and underscore, so an id can never reach
// outside the state folder.
func validSessionID(id string) bool {
	if id == "" || len(id) > maxSessionIDLen {
		return false
	}
	for _, c := range id {
		if !isNameRune(c) {
			return false
		}
	}
	return true
}

// session is a session that the agent has open. Only one turn runs at a time.
type session struct {
	mu     sync.Mutex
	state  sessionState
	cancel context.CancelFunc
	// slot holds a token while a turn runs.
	slot chan struct{}
}

// newSession opens a session from its saved state.
func newSession(st sessionState) *session {
	return &session{state: st, slot: make(chan struct{}, 1)}
}

// snapshot returns a copy of the state that is safe to read while other calls change it.
func (s *session) snapshot() sessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state
	st.Turns = slices.Clone(st.Turns)
	return st
}

// moveTo saves the session with the working folder that the client named. The save happens under
// the lock, so it cannot land after a newer save from a turn that finishes at the same time.
func (s *session) moveTo(st store, cwd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.state
	next.Cwd = cwd
	if err := st.save(next); err != nil {
		return err
	}
	s.state = next
	return nil
}

// beginTurn waits for the turn slot, so a second prompt on the same session starts after the first
// one has stopped. The returned context ends when the turn is cancelled, and end must be called
// once when the turn is over.
func (s *session) beginTurn(ctx context.Context) (context.Context, func(), error) {
	select {
	case s.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("wait for the running turn: %w", ctx.Err())
	}
	turnCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	end := func() {
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		<-s.slot
	}
	return turnCtx, end, nil
}

// stop cancels the running turn, if there is one.
func (s *session) stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// record saves a finished turn. The memory changes only after the file is written, so a failed
// save leaves the counter where it was.
func (s *session) record(st store, user, agent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.state
	next.Turn++
	next.Turns = append(slices.Clone(s.state.Turns), turnRecord{User: user, Agent: agent})
	if err := st.save(next); err != nil {
		return err
	}
	s.state = next
	return nil
}
