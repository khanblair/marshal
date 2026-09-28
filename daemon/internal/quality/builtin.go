package quality

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Marshal's own built-in smell checks (docs/architecture.md section 17.2, product scope 15.4,
// build-plan 5.20). They are the second layer of the three, after a project's own linters and before
// the model review, and they run on the card's own new code only: every finding names a line the
// card added, so a smell that was already there is never blamed on the card.
//
// They are line- and structure-based on purpose. The codebase map that would let a check compare a
// card's code with the rest of its repository is Phase 7's; until then duplication is judged within
// the card's own new code, and function and nesting shapes are read from the file itself. Each check
// says what it does and does not do in its own comment, so a later phase can deepen one without the
// others having to change.
//
// Nothing here runs a program, reads the network, or touches the store: the checks take the text of
// the card's changed files and answer findings. That is what makes them fast, testable, and safe to
// run in the card's worktree.

// rawFinding is one thing a check found, before the profile gives it a family and a severity. A
// built-in check names itself in Check and leaves Family, Smell, and Severity empty, so the profile
// decides how much it matters and the check's family is the one it always is; a project linter's
// finding carries its own family, rule name, and severity instead.
type rawFinding struct {
	Check      protocol.SmellCheck
	Family     protocol.SmellFamily
	Smell      string
	Severity   protocol.SmellSeverity
	File       string
	Line       int
	Message    string
	Suggestion string
}

// family is the smell family a finding belongs to: the one it was given, or the one its check always
// has.
func (f rawFinding) family() protocol.SmellFamily {
	if f.Family != "" {
		return f.Family
	}
	return familyOf(f.Check)
}

// smellName is the rule's own name, which is the check's name for a built-in finding.
func (f rawFinding) smellName() string {
	if f.Smell != "" {
		return f.Smell
	}
	return string(f.Check)
}

// sourceFile is one file of the card's diff as the checks see it: the language it is, whether the
// card added it, what the target branch has (empty when the card added it), what the worktree has
// (empty when the card removed it), and which lines of the worktree version the card added.
type sourceFile struct {
	Path     string
	Language string
	New      bool
	Base     string
	Now      string
	Added    map[int]bool
}

// checkBuiltins runs every built-in check the profile has turned on, over the card's changed files,
// and answers what they found. A check that is off produces nothing; a file in a language a check
// does not fit is skipped by that check alone, so one profile serves a project of several languages.
func checkBuiltins(profile protocol.SmellProfile, files []sourceFile) []rawFinding {
	var found []rawFinding
	for _, file := range files {
		if generatedFile(file.Path) {
			// A lock file, a minified bundle, or a generated source file is not code a person
			// wrote, and flagging it teaches the checks to be ignored.
			continue
		}
		code := file.Language != ""
		if on, severity := settingFor(profile, protocol.SmellCheckLargeFile); on && severity != "" {
			if one, ok := largeFile(profile, file); ok {
				found = append(found, one)
			}
		}
		if !code {
			continue
		}
		if on, _ := settingFor(profile, protocol.SmellCheckLongLine); on {
			found = append(found, longLines(profile, file)...)
		}
		if on, _ := settingFor(profile, protocol.SmellCheckMagicNumber); on {
			found = append(found, magicNumbers(file)...)
		}
		if on, _ := settingFor(profile, protocol.SmellCheckDeepNesting); on {
			found = append(found, deepNesting(profile, file)...)
		}
		if on, _ := settingFor(profile, protocol.SmellCheckLongFunction); on {
			found = append(found, longFunctions(profile, file)...)
		}
		if on, _ := settingFor(profile, protocol.SmellCheckLongParameterList); on {
			found = append(found, longParameterLists(profile, file)...)
		}
	}
	if on, _ := settingFor(profile, protocol.SmellCheckDuplicateBlock); on {
		found = append(found, duplicateBlocks(profile, files)...)
	}
	sort.SliceStable(found, func(a, b int) bool {
		if found[a].File != found[b].File {
			return found[a].File < found[b].File
		}
		return found[a].Line < found[b].Line
	})
	return found
}

// settingFor reads a check's switch and severity from a resolved profile.
func settingFor(profile protocol.SmellProfile, check protocol.SmellCheck) (on bool, severity protocol.SmellSeverity) {
	setting := settingOf(profile, check)
	return setting.Enabled, setting.Severity
}

// largeFile flags a file the card pushed over the length limit. A file that was already over the
// limit before the card is not flagged, and a new file is flagged as soon as it is too long: this is
// the "a smell the card made worse is kept, one that predates it is not" rule for a whole file.
func largeFile(profile protocol.SmellProfile, file sourceFile) (rawFinding, bool) {
	if file.Now == "" {
		return rawFinding{}, false
	}
	nowLines := countLines(file.Now)
	if nowLines <= profile.MaxFileLines {
		return rawFinding{}, false
	}
	if !file.New && file.Base != "" && countLines(file.Base) > profile.MaxFileLines {
		return rawFinding{}, false
	}
	return rawFinding{
		Check:      protocol.SmellCheckLargeFile,
		File:       file.Path,
		Line:       0,
		Message:    fmt.Sprintf("This file is %d lines long, over the %d-line limit.", nowLines, profile.MaxFileLines),
		Suggestion: "Split it into smaller files, each holding one kind of thing.",
	}, true
}

// longLines flags added lines that are longer than the profile allows. A line with no spaces is
// skipped: a minified blob, a base64 payload, or a long URL is long but not a smell a person can fix
// by wrapping it.
func longLines(profile protocol.SmellProfile, file sourceFile) []rawFinding {
	var found []rawFinding
	for _, line := range addedLines(file) {
		text := line.Text
		if len(text) <= profile.MaxLineLength || !strings.ContainsRune(text, ' ') {
			continue
		}
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckLongLine,
			File:       file.Path,
			Line:       line.Number,
			Message:    fmt.Sprintf("This line is %d characters long, over the %d-character limit.", len(text), profile.MaxLineLength),
			Suggestion: "Break it into several lines, or name the long value in a constant.",
		})
	}
	return capPerFile(found)
}

// magicNumberRe matches a number that is probably a magic number rather than a small counting value:
// three or more digits, or a decimal fraction. It does not match a number that is part of a longer
// word, and a caller drops the matches inside comments.
var magicNumberRe = regexp.MustCompile(`(^|[^A-Za-z0-9_.$])(\d{3,}|\d+\.\d+)([^A-Za-z0-9_]|$)`)

// magicNumbers flags added lines that carry a bare number that should be named. The first number on
// a line is reported once; a line of nothing but numbers is left alone, since a table of numbers is
// not a magic number.
func magicNumbers(file sourceFile) []rawFinding {
	var found []rawFinding
	for _, line := range addedLines(file) {
		text := strings.TrimSpace(line.Text)
		if text == "" || commentLine(file.Language, text) {
			continue
		}
		match := magicNumberRe.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		if allNumbers(text) {
			continue
		}
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckMagicNumber,
			File:       file.Path,
			Line:       line.Number,
			Message:    fmt.Sprintf("The number %s has no name, so a reader cannot tell what it means.", match[2]),
			Suggestion: "Give it a name in a constant, so the number and its meaning stay together.",
		})
	}
	return capPerFile(found)
}

// deepNesting flags added lines that sit deeper than the profile allows.
func deepNesting(profile protocol.SmellProfile, file sourceFile) []rawFinding {
	depths := nestingDepths(file.Language, file.Now)
	var found []rawFinding
	reported := map[int]bool{}
	for _, line := range addedLines(file) {
		depth := depths[line.Number-1]
		if depth <= profile.MaxNesting || reported[line.Number] {
			continue
		}
		reported[line.Number] = true
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckDeepNesting,
			File:       file.Path,
			Line:       line.Number,
			Message:    fmt.Sprintf("This line nests %d levels deep, over the %d-level limit.", depth, profile.MaxNesting),
			Suggestion: "Return early, or move the inner block into a function of its own.",
		})
	}
	return capPerFile(found)
}

// longFunctions flags functions the card wrote or grew past the length limit. A function is reported
// only when one of its lines is a line the card added, so a long function the card never touched is
// not blamed on it.
func longFunctions(profile protocol.SmellProfile, file sourceFile) []rawFinding {
	var found []rawFinding
	for _, fn := range scanFunctions(file.Language, file.Now) {
		length := fn.EndLine - fn.StartLine + 1
		if length <= profile.MaxFunctionLines {
			continue
		}
		if !touchesAdded(file, fn.StartLine, fn.EndLine) {
			continue
		}
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckLongFunction,
			File:       file.Path,
			Line:       fn.StartLine,
			Message:    fmt.Sprintf("%s is %d lines long, over the %d-line limit.", fn.describe(), length, profile.MaxFunctionLines),
			Suggestion: "Split it into smaller functions, each doing one thing that its name says.",
		})
	}
	return capPerFile(found)
}

// longParameterLists flags functions with more parameters than the profile allows, again only when
// the card's own change put the function there or changed its signature.
func longParameterLists(profile protocol.SmellProfile, file sourceFile) []rawFinding {
	var found []rawFinding
	for _, fn := range scanFunctions(file.Language, file.Now) {
		if fn.Params <= profile.MaxParameters {
			continue
		}
		if !file.Added[fn.StartLine] && !file.New {
			continue
		}
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckLongParameterList,
			File:       file.Path,
			Line:       fn.StartLine,
			Message:    fmt.Sprintf("%s takes %d parameters, over the %d-parameter limit.", fn.describe(), fn.Params, profile.MaxParameters),
			Suggestion: "Pass a small struct of the related values, or take the object that holds them.",
		})
	}
	return capPerFile(found)
}

// duplicateBlocks flags a run of added lines that appears more than once in the card's own new code.
// The codebase map that would compare a card's code with the rest of its repository is Phase 7's, so
// this compares the card with itself, which is exactly the copy-paste a card's own agent makes.
func duplicateBlocks(profile protocol.SmellProfile, files []sourceFile) []rawFinding {
	width := profile.DuplicateBlockLines
	if width < 1 {
		return nil
	}
	type addedLine struct {
		file  string
		line  int
		block []string
	}
	// Every meaningful added line, with the next `width` lines as its block, in file order.
	var runs []addedLine
	for _, file := range files {
		lines := significantAddedLines(file)
		for i := 0; i+width <= len(lines); i++ {
			block := make([]string, 0, width)
			for j := 0; j < width; j++ {
				block = append(block, strings.Join(strings.Fields(lines[i+j].Text), " "))
			}
			runs = append(runs, addedLine{file: file.Path, line: lines[i].Number, block: block})
		}
	}
	seen := map[string]int{}
	var found []rawFinding
	for _, run := range runs {
		key := strings.Join(run.block, "\n")
		seen[key]++
		if seen[key] != 2 {
			// The first copy is the original; a third and later copy is not reported again, so one
			// block produces one finding however many times it was pasted.
			continue
		}
		found = append(found, rawFinding{
			Check:      protocol.SmellCheckDuplicateBlock,
			File:       run.file,
			Line:       run.line,
			Message:    fmt.Sprintf("These %d lines are a copy of a block elsewhere in this card's new code.", width),
			Suggestion: "Move the shared block into one function and call it from both places.",
		})
	}
	return found
}

// addedLine is one line of the worktree version of a file, with its number.
type addedLine struct {
	Number int
	Text   string
}

// addedLines returns the lines of a file's worktree version that the card added, in order. A file
// the card added is all its own, so every line of it counts as added. A trailing newline ends the
// last line rather than starting an empty one, the same way countLines reads a file.
func addedLines(file sourceFile) []addedLine {
	if file.Now == "" {
		return nil
	}
	raw := strings.Split(strings.TrimSuffix(file.Now, "\n"), "\n")
	out := make([]addedLine, 0, len(raw))
	for number := 1; number <= len(raw); number++ {
		if file.New || file.Added[number] {
			out = append(out, addedLine{Number: number, Text: strings.TrimRight(raw[number-1], "\r")})
		}
	}
	return out
}

// significantAddedLines returns a file's added lines that carry code worth comparing: a blank line,
// a lone brace, and a line of only punctuation are skipped, so a block of `}`s is not called a
// duplicate.
func significantAddedLines(file sourceFile) []addedLine {
	var out []addedLine
	for _, line := range addedLines(file) {
		text := strings.TrimSpace(line.Text)
		if len(text) < 8 || strings.Trim(text, "{}(),;[]=") == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// touchesAdded reports whether any line number in [start, end] is a line the card added. A file the
// card added is entirely its own.
func touchesAdded(file sourceFile, start, end int) bool {
	if file.New {
		return true
	}
	for number := start; number <= end; number++ {
		if file.Added[number] {
			return true
		}
	}
	return false
}

// countLines counts the lines of a file's text, counting a trailing newline as ending the last line
// rather than starting an empty one.
func countLines(text string) int {
	if text == "" {
		return 0
	}
	trimmed := strings.TrimSuffix(text, "\n")
	return strings.Count(trimmed, "\n") + 1
}

// maxFindingsPerFile is the most findings of one check that one file reports. A file that trips a
// check on every line would otherwise bury the card's other findings.
const maxFindingsPerFile = 50

// capPerFile keeps at most maxFindingsPerFile findings, in the order they were found.
func capPerFile(found []rawFinding) []rawFinding {
	if len(found) <= maxFindingsPerFile {
		return found
	}
	return found[:maxFindingsPerFile]
}

// generatedFile reports whether a path is one a person did not write: a lock file, a minified
// bundle, or a generated source file. A finding in one of these teaches people to ignore the checks.
func generatedFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "go.sum",
		"cargo.lock", "composer.lock", "gemfile.lock", "poetry.lock", "bun.lockb", "flake.lock":
		return true
	}
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") ||
		strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, "_gen.go") ||
		strings.HasSuffix(base, "_generated.go") || strings.HasSuffix(base, ".generated.ts") {
		return true
	}
	// A folder named generated, vendor, or node_modules holds only code that was not written here.
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		switch strings.ToLower(part) {
		case "node_modules", "vendor", "generated", "dist", "build":
			return true
		}
	}
	return false
}

// languageOf names the language of a file from its extension, or the empty string for a file the
// checks do not know how to read. A file with no language is only checked for being too large.
func languageOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return langPython
	case ".java":
		return "java"
	case ".rb":
		return langRuby
	case ".rs":
		return "rust"
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".cs", ".kt", ".swift", ".scala":
		return "brace"
	default:
		return ""
	}
}

// commentLine reports whether a trimmed line is a comment in the file's language, so a number or a
// long line inside a comment is not flagged.
func commentLine(language, text string) bool {
	switch language {
	case "go", "javascript", "typescript", "java", "rust", "brace":
		return strings.HasPrefix(text, "//") || strings.HasPrefix(text, "/*") || strings.HasPrefix(text, "*")
	case langPython, langRuby:
		return strings.HasPrefix(text, "#")
	default:
		return false
	}
}

// allNumbers reports whether a line holds nothing but numbers and separators, which is a table of
// numbers rather than a magic number.
var numberOnlyRe = regexp.MustCompile(`^[\s\d.,:;|+*/%()\[\]{}=<>_xX-]+$`)

func allNumbers(text string) bool { return numberOnlyRe.MatchString(text) }
