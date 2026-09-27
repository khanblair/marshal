package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/secrets"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// Default sizes and times, all configurable through Config.
const (
	// defaultLogSegmentBytes is how big an on-disk log segment grows before it rotates.
	defaultLogSegmentBytes int64 = 10 << 20
	// defaultRingBytes bounds the in-memory ring of recent log entries, per live session.
	defaultRingBytes = 64 << 10
	// maxQueuedMessages is the most messages Send queues for a busy session before it refuses.
	maxQueuedMessages = 50
	// resumeTimeout bounds one Resume call inside RestoreAll or Manager.Resume, so a stuck agent
	// program cannot hold up the rest of the rows being restored.
	resumeTimeout = 30 * time.Second
	// closeStopTimeout bounds one agent.Stop call inside Manager.Close.
	closeStopTimeout = 15 * time.Second
	// logFlushInterval is how often a session's on-disk log is flushed on a timer, so a crash
	// loses at most this much unflushed chat text.
	logFlushInterval = time.Second
	// sessionLogDirMode is the folder mode for a session's own log folder, matching the daemon's
	// usual 0700.
	sessionLogDirMode = 0o700
	// sessionLogFileMode is the file mode of a session log segment: only its owner may read it,
	// matching the database file (internal/store's fileMode).
	sessionLogFileMode = 0o600
)

// ResumeMode says how RestoreAll treats sessions that were live when the daemon last stopped
// (docs/architecture.md section 5.3).
type ResumeMode string

const (
	// ResumeModeAuto resumes every session that was starting, awake, or working, right away.
	ResumeModeAuto ResumeMode = "auto"
	// ResumeModeManual leaves those sessions as they are in the database and only logs how many
	// are waiting; a person resumes one later with Manager.Resume.
	ResumeModeManual ResumeMode = "manual"
)

// Config configures a Manager. DataDir is required; every other field has a default.
type Config struct {
	// DataDir is Marshal's own data folder, as a full path. Session logs go under
	// <DataDir>/logs/sessions/<session id>/.
	DataDir string
	// History stores the typed history and activity of every session this manager runs (migration
	// 0006), so a card's chat can be paged rather than replayed from a log file. Left unset, the
	// manager writes to the store it already has; set it to share the history module's own store,
	// which is what reads a card's history back.
	History HistoryRecorder
	// Plans reads and writes the plans a card waits on in plan-first mode (docs/backend-checklist.md
	// B5.2): the plan a person approves, rejects, or edits. Left unset, the manager writes to the
	// store it already has; set it to share the history module's own store, which is what pages a
	// card's plan back in its chat.
	Plans PlanStore
	// Terminals makes the agent that runs a card's CLI in a pseudo-terminal, for the terminal view
	// (docs/architecture.md 4.3), by the same kinds as the registry the manager starts chat sessions
	// through. Every agent it makes must also be an agents.Terminal, as the PTY adapter is. A kind
	// with no factory here has no terminal view, and asking for one is refused with a plain sentence
	// before anything is stopped. Left nil, no card has a terminal view.
	Terminals *agents.Registry
	// ResumeMode says how RestoreAll treats sessions left over from the last run. The default is
	// ResumeModeAuto.
	ResumeMode ResumeMode
	// LogSegmentBytes is how big an on-disk log segment grows before it rotates to a new file.
	// The default is 10 MiB.
	LogSegmentBytes int64
	// RingBytes bounds the in-memory ring of recent log entries, per live session, by a rough
	// estimate of their encoded size. The default is 64 KiB.
	RingBytes int
	// Logger receives the manager's log lines. The default is slog.Default().
	Logger *slog.Logger
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// Entropy is where an opaque id gets its random part, for the ids this manager makes itself (an
	// approval's own id). The default is crypto/rand.Reader; a test sets it to get ids it can
	// predict.
	Entropy io.Reader
	// Audit records the actions a person may need to account for: a decision on an approval, and the
	// rest of Phase 3 (docs/backend-checklist.md B3.5). Left unset, the manager makes one on the
	// store it already has.
	Audit *audit.Recorder
	// Profile says what a session's agent may do at all, in every mode except bypass
	// (docs/marshal-product-scope.md section 14.3). Left nil, the profile Marshal ships with is used,
	// which allows everything a person's own agent normally does. Set it to a profile meant to
	// refuse - security.NothingAllowed, say - to narrow what the manager's sessions may do; a
	// pointer is used so that "nothing allowed" is distinguishable from "not set".
	Profile *security.Profile
	// Secrets scans every commit an agent makes for credentials, and stops the card when it finds
	// one (docs/backend-checklist.md B3.5). Left nil, Marshal's own scanner is built from the rule
	// set gitleaks ships with; set it to a scanner a test controls to prove the block without
	// inventing a key that the real rules would not match.
	Secrets *secrets.Scanner
}

// withDefaults checks the config and fills in what was left out.
func (c Config) withDefaults() (Config, error) {
	if c.DataDir == "" || !filepath.IsAbs(c.DataDir) {
		return Config{}, errors.New("the session manager needs the data folder as a full path")
	}
	if c.ResumeMode == "" {
		c.ResumeMode = ResumeModeAuto
	}
	if c.ResumeMode != ResumeModeAuto && c.ResumeMode != ResumeModeManual {
		return Config{}, fmt.Errorf("the resume mode must be %q or %q, not %q", ResumeModeAuto, ResumeModeManual, c.ResumeMode)
	}
	if c.LogSegmentBytes <= 0 {
		c.LogSegmentBytes = defaultLogSegmentBytes
	}
	if c.RingBytes <= 0 {
		c.RingBytes = defaultRingBytes
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Entropy == nil {
		c.Entropy = rand.Reader
	}
	if c.Profile == nil {
		profile := security.DefaultProfile()
		c.Profile = &profile
	}
	if c.Secrets == nil {
		c.Secrets = secrets.New()
	}
	return c, nil
}

// sessionLogDir is where one session's on-disk log segments live.
func sessionLogDir(dataDir, sessionID string) string {
	return filepath.Join(dataDir, "logs", "sessions", sessionID)
}
