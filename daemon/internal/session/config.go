package session

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
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
	return c, nil
}

// sessionLogDir is where one session's on-disk log segments live.
func sessionLogDir(dataDir, sessionID string) string {
	return filepath.Join(dataDir, "logs", "sessions", sessionID)
}
