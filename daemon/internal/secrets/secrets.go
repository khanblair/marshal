// Package secrets scans the text an agent committed for credentials
// (docs/backend-checklist.md B3.5, docs/marshal-product-scope.md section 14.5, docs/library-docs.md:
// gitleaks is the approved library).
//
// It wraps gitleaks' own detector and never keeps a secret. A finding carries the rule that fired,
// the file, and the line, and never the matched text: the audit log, the system note, and the
// response to a client all carry what was found without carrying what it was.
//
// Building the detector reads gitleaks' shipped rule set, which is a moment's work and a fair block
// of memory, so it is built on the first scan rather than at daemon start: a daemon that never has
// a commit to scan never pays for the rules.
package secrets

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/fatih/semgroup"
	"github.com/rs/zerolog"
	"github.com/zricethezav/gitleaks/v8/detect"
)

// Finding is one credential a scan found, without the credential.
type Finding struct {
	// Rule is the gitleaks rule that matched, such as "aws-access-token". It is the stable name a
	// refusal, an audit row, and a test compare against.
	Rule string
	// Description is gitleaks' own sentence for the rule, for a person to read.
	Description string
	// File is the file the finding is in.
	File string
	// Line is the line the finding is on.
	Line int
}

// Scanner scans text for credentials. It is safe for use by many goroutines, and its detector is
// built once and shared.
type Scanner struct {
	once     sync.Once
	detector *detect.Detector
	err      error
}

// New makes a Scanner. Nothing is built until the first Scan, so a daemon that never scans pays
// nothing for the rule set.
func New() *Scanner { return &Scanner{} }

// Scan reads one piece of text as if it were a file at path, and returns what it found. The text
// is scanned as it stands: a caller that wants a commit's change to be scanned hands it the patch
// or the file's content.
func (s *Scanner) Scan(path, content string) ([]Finding, error) {
	d, err := s.detectorFor()
	if err != nil {
		return nil, err
	}
	raw := d.Detect(detect.Fragment{Raw: content, FilePath: path})
	findings := make([]Finding, 0, len(raw))
	for _, f := range raw {
		// The matched text is deliberately dropped here. What is kept is what a person needs in
		// order to act, and never the credential itself.
		findings = append(findings, Finding{
			Rule: f.RuleID, Description: f.Description, File: path, Line: f.StartLine,
		})
	}
	return findings, nil
}

// detectorFor builds the detector the first time it is asked and returns the same one after.
func (s *Scanner) detectorFor() (*detect.Detector, error) {
	s.once.Do(func() {
		// gitleaks narrates every rule it considered through zerolog, at trace level, and the
		// library never sets a level of its own. Left alone it would write a line per rule per scan
		// to the daemon's standard error. Marshal logs through slog and uses zerolog for nothing
		// else, so the level is turned off here, once, before the detector is built.
		zerolog.SetGlobalLevel(zerolog.Disabled)
		d, err := detect.NewDetectorDefaultConfig()
		if err != nil {
			s.err = fmt.Errorf("build the secret scanner: %w", err)
			return
		}
		// Fully redact any secret the detector holds, so a finding can never be logged with the
		// credential in it, even by a caller that reads the field this package does not.
		d.Redact = 100
		if d.Sema == nil {
			d.Sema = semgroup.NewGroup(context.Background(), int64(runtime.NumCPU()))
		}
		s.detector = d
	})
	if s.err != nil {
		return nil, s.err
	}
	return s.detector, nil
}
