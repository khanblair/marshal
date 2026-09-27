package quality

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Reading the card's diff and turning findings into wire shapes. Everything the checks compare comes
// from here: the files the card changed, which lines of each file the card added, and what the target
// branch has for a file the card changed in place. The comparison is against the merge base of the
// target branch and the worktree, the same base internal/gitx compares a diff from, so work that
// landed on the target branch after the card started is not counted as the card's.

// changedFiles reads the card's diff and answers each changed file as the checks see it: its
// language, whether the card added it, its current text, the lines the card added, and what the
// target branch has. A deleted file and a binary file are skipped, and a file that cannot be read
// from the worktree is skipped with what the diff said about it.
func (s *Service) changedFiles(ctx context.Context, dir, base string) ([]sourceFile, error) {
	if strings.TrimSpace(base) == "" {
		return nil, nil
	}
	listed, _, err := s.git.DiffFiles(ctx, dir, base, s.limits.MaxBytes)
	if err != nil {
		return nil, fmt.Errorf("read the diff of the card's worktree: %w", err)
	}
	baseSHA := s.mergeBase(ctx, dir, base)
	files := make([]sourceFile, 0, len(listed))
	for _, item := range listed {
		if len(files) >= s.limits.MaxFiles {
			break
		}
		if item.Binary || item.Status == gitx.DiffStatusDeleted {
			continue
		}
		file := sourceFile{
			Path: item.Path, Language: languageOf(item.Path),
			New: item.Status == gitx.DiffStatusAdded,
		}
		file.Now = readWorktreeFile(dir, item.Path)
		if file.Now == "" {
			continue
		}
		if !file.New {
			file.Added = s.addedLineNumbers(ctx, dir, base, item.Path)
			if baseSHA != "" {
				// A file the target branch does not have reads as empty, which is what a file the
				// card renamed onto a new path looks like.
				file.Base, _ = s.git.FileAtCommit(ctx, dir, baseSHA, item.Path)
			}
		}
		files = append(files, file)
	}
	return files, nil
}

// addedLineNumbers reads one file's hunks and collects the line numbers of the worktree side that
// the card added. A file whose hunks cannot be read has no added lines, which means its line-by-line
// checks see nothing rather than blaming every line on the card.
func (s *Service) addedLineNumbers(ctx context.Context, dir, base, path string) map[int]bool {
	hunks, found, err := s.git.DiffHunks(ctx, dir, base, path, s.limits.MaxLines, s.limits.MaxBytes)
	if err != nil || !found {
		if err != nil {
			s.log.Warn("could not read a card's file hunks", "file", path, "error", err)
		}
		return nil
	}
	added := map[int]bool{}
	for _, hunk := range hunks.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind == gitx.DiffLineAdded && line.NewLine > 0 {
				added[line.NewLine] = true
			}
		}
	}
	return added
}

// mergeBase answers the commit the target branch and the worktree last shared, which is what the
// target branch's copy of a file is read from. A repository without one answers the empty string and
// the base copy is then simply not read.
func (s *Service) mergeBase(ctx context.Context, dir, base string) string {
	out, err := s.git.Run(ctx, dir, "merge-base", "HEAD", base)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// commonLanguage answers the one language every changed file shares, or the empty string when they
// differ or none is known, so a linter that fits one language is not run over another.
func commonLanguage(files []sourceFile) string {
	language := ""
	for _, file := range files {
		if file.Language == "" {
			continue
		}
		if language == "" {
			language = file.Language
			continue
		}
		if language != file.Language {
			return ""
		}
	}
	return language
}

// toWire turns one stored finding into its wire shape.
func toWire(row db.SmellFinding) protocol.SmellFinding {
	return protocol.SmellFinding{
		ID: row.ID, CardID: row.CardID, Commit: row.CommitSha,
		Family: protocol.SmellFamily(row.Family), Smell: row.Smell,
		File: row.File, Line: int(row.Line),
		Severity: protocol.SmellSeverity(row.Severity),
		Message:  row.Message, Suggestion: row.Suggestion,
		Status: protocol.SmellStatus(row.Status), DismissReason: row.DismissReason,
	}
}

// blockingText is the message that goes back to the agent when findings block a card. It lists each
// blocking finding and what to do, and is bounded so one enormous message cannot be sent.
func blockingText(findings []protocol.SmellFinding) string {
	var b strings.Builder
	b.WriteString("The code-quality checks found problems in your changes that must be fixed before this card can be reviewed:\n")
	for _, finding := range findings {
		if finding.Status != protocol.SmellStatusOpen || finding.Severity != protocol.SmellSeverityBlocking {
			continue
		}
		fmt.Fprintf(&b, "\n- %s", findingLine(finding))
		if finding.Message != "" {
			fmt.Fprintf(&b, ": %s", finding.Message)
		}
		if finding.Suggestion != "" {
			fmt.Fprintf(&b, " Suggested fix: %s", finding.Suggestion)
		}
		if b.Len() > maxFixTextChars {
			break
		}
	}
	text := b.String()
	if len(text) > maxFixTextChars {
		text = text[:maxFixTextChars]
	}
	return text
}

// fixText is the message that asks the agent to fix one finding.
func fixText(finding protocol.SmellFinding) string {
	text := fmt.Sprintf("Please fix this code-quality finding:\n\n%s", findingLine(finding))
	if finding.Message != "" {
		text += "\n" + finding.Message
	}
	if finding.Suggestion != "" {
		text += "\n" + finding.Suggestion
	}
	if len(text) > maxFixTextChars {
		text = text[:maxFixTextChars]
	}
	return text
}

// findingLine names a finding's place and sort, which both messages open with.
func findingLine(finding protocol.SmellFinding) string {
	where := finding.File
	if finding.Line > 0 {
		where = fmt.Sprintf("%s:%d", finding.File, finding.Line)
	}
	return fmt.Sprintf("%s (%s, %s)", where, finding.Smell, finding.Family)
}
