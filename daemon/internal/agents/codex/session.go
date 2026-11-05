package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const jsonRPCVersion = "2.0"
const jsonRPCField = "jsonrpc"

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type rpcWait struct {
	result json.RawMessage
	err    error
}
type session struct {
	a             *Adapter
	spec          agents.StartSpec
	id            string
	idMu          sync.RWMutex
	handleValue   agents.SessionHandle
	p             *proc.Process
	cancelLife    context.CancelFunc
	sink          *agents.EventSink
	writeMu       sync.Mutex
	mu            sync.Mutex
	waiters       map[string]chan rpcWait
	pending       map[string]pendingApproval
	next          atomic.Uint64
	turn          string
	busy          bool
	closing       bool
	done          chan struct{}
	turnReady     chan struct{}
	turnReadyOnce sync.Once
	stopMu        sync.Mutex
	items         map[string]itemInfo
}
type pendingApproval struct {
	id      json.RawMessage
	options map[string]string
}
type itemInfo struct {
	title, kind, path, command, description string
	changes                                 []agents.FileDiff
}

func newSession(ctx context.Context, a *Adapter, spec agents.StartSpec, threadSettings, configSettings map[string]any, resume string) (*session, error) {
	life, cancelLife := context.WithCancel(context.Background())
	env := append(catalog.ProgramEnv(a.cfg.Path), a.cfg.Env...)
	env = append(env, spec.Env...)
	p, err := proc.Start(life, proc.Spec{Path: a.cfg.Path, Args: catalog.CodexAppServerArgs(), Dir: spec.Cwd, Env: env, Stdin: true})
	if err != nil {
		cancelLife()
		return nil, err
	}
	id := resume
	if id == "" {
		id = fmt.Sprintf("pending-%d", time.Now().UnixNano())
	}
	s := &session{a: a, spec: spec, id: id, p: p, cancelLife: cancelLife, sink: agents.NewEventSink(128), waiters: map[string]chan rpcWait{}, pending: map[string]pendingApproval{}, items: map[string]itemInfo{}, done: make(chan struct{})}
	go func() { <-p.Done(); s.sink.Release() }()
	go s.readLoop()
	startCtx, cancelStart := context.WithTimeout(ctx, a.cfg.RequestTimeout)
	defer cancelStart()
	if _, err = s.request(startCtx, "initialize", catalog.CodexInitializeParams()); err != nil {
		_ = p.Stop(context.Background(), a.cfg.StopGrace)
		cancelLife()
		return nil, err
	}
	s.notify("initialized", map[string]any{})
	method := "thread/start"
	thread := map[string]any{"cwd": spec.Cwd, "config": configSettings}
	for key, value := range threadSettings {
		thread[key] = value
	}
	if spec.Model != "" && spec.Model != "default" {
		thread["model"] = spec.Model
	}
	if spec.Instructions != "" && resume == "" {
		thread["developerInstructions"] = spec.Instructions
	}
	if resume != "" {
		method = "thread/resume"
		thread = map[string]any{"threadId": resume, "cwd": spec.Cwd, "config": configSettings}
		for key, value := range threadSettings {
			thread[key] = value
		}
		if spec.Model != "" && spec.Model != "default" {
			thread["model"] = spec.Model
		}
	}
	raw, err := s.request(startCtx, method, thread)
	if err != nil {
		_ = p.Stop(context.Background(), a.cfg.StopGrace)
		cancelLife()
		return nil, fmt.Errorf("codex %s: %w", method, err)
	}
	var answer struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = json.Unmarshal(raw, &answer); err != nil || answer.Thread.ID == "" {
		_ = p.Stop(context.Background(), a.cfg.StopGrace)
		cancelLife()
		if resume != "" {
			return nil, fmt.Errorf("%w: Codex returned no thread id", agents.ErrCannotResume)
		}
		return nil, fmt.Errorf("codex %s returned no thread id", method)
	}
	if resume != "" && answer.Thread.ID != resume {
		_ = p.Stop(context.Background(), a.cfg.StopGrace)
		cancelLife()
		return nil, fmt.Errorf("%w: Codex resumed %q instead of %q", agents.ErrCannotResume, answer.Thread.ID, resume)
	}
	s.setThreadID(answer.Thread.ID)
	s.handleValue = agents.SessionHandle{ID: answer.Thread.ID, Label: spec.Label, Model: spec.Model, Thinking: spec.Thinking, PermissionMode: spec.PermissionMode, Applied: agents.Applied{Model: spec.Model != "", Thinking: spec.Thinking != "", PermissionMode: spec.PermissionMode != ""}, Capabilities: a.Capabilities()}
	return s, nil
}
func (s *session) handle() agents.SessionHandle { return s.handleValue }
func (s *session) threadID() string             { s.idMu.RLock(); defer s.idMu.RUnlock(); return s.id }
func (s *session) setThreadID(id string)        { s.idMu.Lock(); s.id = id; s.idMu.Unlock() }
func (s *session) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.p.Stdin == nil {
		return errors.New("codex stdin is closed")
	}
	_, err = s.p.Stdin.Write(append(b, '\n'))
	return err
}
func (s *session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := fmt.Sprintf("marshal-%d", s.next.Add(1))
	ch := make(chan rpcWait, 1)
	s.mu.Lock()
	s.waiters[id] = ch
	s.mu.Unlock()
	if err := s.write(map[string]any{jsonRPCField: jsonRPCVersion, "id": id, "method": method, "params": params}); err != nil {
		s.dropWaiter(id)
		return nil, err
	}
	select {
	case r := <-ch:
		return r.result, r.err
	case <-ctx.Done():
		s.dropWaiter(id)
		return nil, ctx.Err()
	case <-s.done:
		return nil, agents.ErrStopped
	}
}
func (s *session) dropWaiter(id string) { s.mu.Lock(); delete(s.waiters, id); s.mu.Unlock() }
func (s *session) notify(method string, params any) {
	_ = s.write(map[string]any{jsonRPCField: jsonRPCVersion, "method": method, "params": params})
}
func (s *session) readLoop() {
	defer close(s.done)
	defer func() { _ = s.p.Stdout.Close() }()
	scan := bufio.NewScanner(s.p.Stdout)
	scan.Buffer(make([]byte, 4096), 4<<20)
	for scan.Scan() {
		var m rpcMessage
		if json.Unmarshal(scan.Bytes(), &m) != nil {
			continue
		}
		if m.Method == "" {
			s.reply(m)
			continue
		}
		s.dispatch(m)
	}
	if scan.Err() != nil {
		_ = s.p.Stop(context.Background(), s.a.cfg.StopGrace)
	}
	s.finishProcess()
}
func (s *session) reply(m rpcMessage) {
	var id string
	if json.Unmarshal(m.ID, &id) != nil {
		return
	}
	s.mu.Lock()
	ch := s.waiters[id]
	delete(s.waiters, id)
	s.mu.Unlock()
	if ch == nil {
		return
	}
	r := rpcWait{result: m.Result}
	if m.Error != nil {
		r.err = fmt.Errorf("RPC %d: %s", m.Error.Code, m.Error.Message)
	}
	ch <- r
}
func (s *session) send(ctx context.Context, msg agents.UserMessage) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return agents.ErrStopped
	}
	if s.busy {
		s.mu.Unlock()
		return agents.ErrBusy
	}
	s.busy = true
	s.turnReady = make(chan struct{})
	s.turnReadyOnce = sync.Once{}
	s.mu.Unlock()
	params := map[string]any{"threadId": s.threadID(), "input": []any{map[string]any{"type": "text", "text": msg.Text}}}
	if s.spec.Thinking != "" {
		params["effort"] = s.spec.Thinking
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.a.cfg.RequestTimeout)
	result, err := s.request(requestCtx, "turn/start", params)
	cancel()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// The server may already have accepted the turn. End this process before
			// permitting another send; the saved thread remains available for resume.
			stopCtx, cancelStop := context.WithTimeout(context.Background(), s.a.cfg.StopGrace+time.Second)
			defer cancelStop()
			if stopErr := s.stop(stopCtx); stopErr != nil {
				s.cancelLife()
				s.mu.Lock()
				s.closing = true
				s.mu.Unlock()
			}
		}
		s.clearFailedStart()
		return err
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(result, &started) == nil && started.Turn.ID != "" {
		s.mu.Lock()
		if s.busy {
			s.setTurn(started.Turn.ID)
		}
		s.mu.Unlock()
	}
	return nil
}
func (s *session) interrupt(ctx context.Context) error {
	s.mu.Lock()
	if !s.busy {
		s.mu.Unlock()
		return nil
	}
	turn := s.turn
	ready := s.turnReady
	s.mu.Unlock()
	if turn == "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return agents.ErrStopped
		case <-ready:
		}
		s.mu.Lock()
		turn = s.turn
		s.mu.Unlock()
		if turn == "" {
			return nil
		}
	}
	_, err := s.request(ctx, "turn/interrupt", map[string]any{"threadId": s.threadID(), "turnId": turn})
	return err
}
func (s *session) stop(ctx context.Context) error {
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	s.mu.Lock()
	s.closing = true
	s.clearPendingLocked()
	s.mu.Unlock()
	err := s.p.Stop(ctx, s.a.cfg.StopGrace)
	if err != nil {
		s.mu.Lock()
		select {
		case <-s.p.Done():
		default:
			s.closing = false
		}
		s.mu.Unlock()
		return err
	}
	select {
	case <-s.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
func (s *session) finishProcess() {
	s.cancelLife()
	s.mu.Lock()
	stopped := s.closing
	busy := s.busy
	s.closing = true
	s.clearPendingLocked()
	s.mu.Unlock()
	if busy && !stopped {
		s.sink.Emit(agents.Failed{Message: "Codex process exited during the turn"})
		s.sink.Emit(agents.TurnEnded{Reason: agents.TurnError})
	}
	exit := s.p.Wait()
	s.sink.EmitFinal(agents.Exited{Code: exit.Code, Err: exit.Err})
	s.sink.Close()
	s.a.forget(s)
}
func (s *session) clearPendingLocked() { s.pending = map[string]pendingApproval{} }
func (s *session) endTurn(reason, detail string) {
	s.mu.Lock()
	if !s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = false
	s.turn = ""
	s.clearPendingLocked()
	if s.turnReady != nil {
		s.turnReadyOnce.Do(func() { close(s.turnReady) })
	}
	s.mu.Unlock()
	if detail != "" {
		s.sink.Emit(agents.Failed{Message: detail})
	}
	s.sink.Emit(agents.TurnEnded{Reason: reason})
}
func (s *session) setTurn(id string) {
	if id == "" {
		return
	}
	s.turn = id
	if s.turnReady != nil {
		s.turnReadyOnce.Do(func() { close(s.turnReady) })
	}
}
func (s *session) clearFailedStart() {
	s.mu.Lock()
	s.busy = false
	s.turn = ""
	s.clearPendingLocked()
	if s.turnReady != nil {
		s.turnReadyOnce.Do(func() { close(s.turnReady) })
	}
	s.mu.Unlock()
}
func (s *session) respond(ctx context.Context, r agents.ApprovalResponse) error {
	return s.respondApproval(ctx, r)
}
