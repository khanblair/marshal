package session_test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// fakeTerminal is a scriptable stand-in for the PTY adapter, for unit tests that must control
// exactly what a terminal prints and when, and that never start a process. It is an agents.Agent
// that resumes any session id it is given (a real terminal program picks a session up by the id in
// its arguments, and the fake only records it), and an agents.Terminal that records what it is
// asked to write and resize.
type fakeTerminal struct {
	caps agents.Capabilities

	mu       sync.Mutex
	sessions map[string]*fakeTerminalSession
	specs    []agents.StartSpec
	resumed  []string
	// resumeErr, when set, is what Resume returns.
	resumeErr error
	// resumeHold, when set, makes Resume wait for a receive on it, so a test can hold a view switch
	// half done.
	resumeHold chan struct{}
	// resumeEntered, when set, gets a value when Resume begins, so a test knows the switch is under
	// way.
	resumeEntered chan struct{}
	// writeHold, when set, makes WriteRaw wait for a receive on it, like a program that does not
	// read its input.
	writeHold chan struct{}
	written   [][]byte
	sizes     [][2]int
	cols      int
	rows      int
	// writeErr, resizeErr, and sizeErr, when set, are what WriteRaw, Resize, and Size return.
	writeErr, resizeErr, sizeErr error
}

// fakeTerminalSession is one running fake terminal.
type fakeTerminalSession struct {
	id   string
	sink *agents.EventSink
}

func newFakeTerminal() *fakeTerminal {
	return &fakeTerminal{
		caps:     agents.Capabilities{Resume: true},
		sessions: make(map[string]*fakeTerminalSession),
		cols:     120, rows: 32,
	}
}

var (
	_ agents.Agent    = (*fakeTerminal)(nil)
	_ agents.Terminal = (*fakeTerminal)(nil)
)

func (t *fakeTerminal) Start(context.Context, agents.StartSpec) (agents.SessionHandle, error) {
	return agents.SessionHandle{}, errors.New("the fake terminal only resumes")
}

func (t *fakeTerminal) Resume(_ context.Context, sessionID string, spec agents.StartSpec) (agents.SessionHandle, error) {
	t.mu.Lock()
	hold, entered := t.resumeHold, t.resumeEntered
	t.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if hold != nil {
		<-hold
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.specs = append(t.specs, spec)
	t.resumed = append(t.resumed, sessionID)
	if t.resumeErr != nil {
		return agents.SessionHandle{}, t.resumeErr
	}
	t.sessions[sessionID] = &fakeTerminalSession{id: sessionID, sink: agents.NewEventSink(4096)}
	return agents.SessionHandle{ID: sessionID, Label: spec.Label, Capabilities: t.caps}, nil
}

func (t *fakeTerminal) Send(_ context.Context, h agents.SessionHandle, msg agents.UserMessage) error {
	return t.WriteRaw(context.Background(), h, []byte(msg.Text+"\n"))
}

func (t *fakeTerminal) Interrupt(context.Context, agents.SessionHandle) error { return nil }

func (t *fakeTerminal) Events(h agents.SessionHandle) <-chan agents.AgentEvent {
	s, err := t.find(h.ID)
	if err != nil {
		closed := make(chan agents.AgentEvent)
		close(closed)
		return closed
	}
	return s.sink.C()
}

func (*fakeTerminal) Respond(context.Context, agents.SessionHandle, agents.ApprovalResponse) error {
	return agents.ErrUnknownRequest
}

func (t *fakeTerminal) Stop(_ context.Context, h agents.SessionHandle) error {
	s, err := t.find(h.ID)
	if err != nil {
		return nil
	}
	t.mu.Lock()
	delete(t.sessions, h.ID)
	t.mu.Unlock()
	s.sink.EmitFinal(agents.Exited{Code: 0})
	s.sink.Release()
	s.sink.Close()
	return nil
}

func (t *fakeTerminal) Capabilities() agents.Capabilities {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caps
}

func (t *fakeTerminal) WriteRaw(_ context.Context, h agents.SessionHandle, data []byte) error {
	if _, err := t.find(h.ID); err != nil {
		return agents.ErrStopped
	}
	t.mu.Lock()
	hold, writeErr := t.writeHold, t.writeErr
	t.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if writeErr != nil {
		return writeErr
	}
	t.mu.Lock()
	t.written = append(t.written, append([]byte(nil), data...))
	t.mu.Unlock()
	return nil
}

func (t *fakeTerminal) Resize(_ context.Context, h agents.SessionHandle, cols, rows int) error {
	if _, err := t.find(h.ID); err != nil {
		return agents.ErrStopped
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.resizeErr != nil {
		return t.resizeErr
	}
	t.sizes = append(t.sizes, [2]int{cols, rows})
	t.cols, t.rows = cols, rows
	return nil
}

func (t *fakeTerminal) Size(h agents.SessionHandle) (cols, rows int, err error) {
	if _, err := t.find(h.ID); err != nil {
		return 0, 0, agents.ErrUnknownSession
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sizeErr != nil {
		return 0, 0, t.sizeErr
	}
	return t.cols, t.rows, nil
}

func (t *fakeTerminal) find(id string) (*fakeTerminalSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.sessions[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", agents.ErrUnknownSession, id)
	}
	return s, nil
}

// print makes the terminal of a session print bytes.
func (t *fakeTerminal) print(id string, data []byte) error {
	s, err := t.find(id)
	if err != nil {
		return err
	}
	s.sink.Emit(agents.TerminalOutput{Data: data})
	return nil
}

// crash ends a session the way a program that exited on its own would: Exited, with no Stop asked
// for.
func (t *fakeTerminal) crash(id string) {
	s, err := t.find(id)
	if err != nil {
		return
	}
	t.mu.Lock()
	delete(t.sessions, id)
	t.mu.Unlock()
	s.sink.EmitFinal(agents.Exited{Code: 1, Err: errors.New("exit status 1")})
	s.sink.Release()
	s.sink.Close()
}

// resumedIDs copies the session ids the fake was asked to resume, in order.
func (t *fakeTerminal) resumedIDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.resumed...)
}

// typed copies what was written to the terminals, in order.
func (t *fakeTerminal) typed() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([][]byte(nil), t.written...)
}

// resizes copies the sizes the terminals were given, in order.
func (t *fakeTerminal) resizes() [][2]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([][2]int(nil), t.sizes...)
}

// startSpecs copies the specs the fake was asked to resume a process with.
func (t *fakeTerminal) startSpecs() []agents.StartSpec {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]agents.StartSpec(nil), t.specs...)
}

func (t *fakeTerminal) setResumeErr(err error) {
	t.mu.Lock()
	t.resumeErr = err
	t.mu.Unlock()
}

func (t *fakeTerminal) setWriteHold(hold chan struct{}) {
	t.mu.Lock()
	t.writeHold = hold
	t.mu.Unlock()
}

// setErrs changes what the calls that reach the terminal return.
func (t *fakeTerminal) setErrs(write, resize, size error) {
	t.mu.Lock()
	t.writeErr, t.resizeErr, t.sizeErr = write, resize, size
	t.mu.Unlock()
}
