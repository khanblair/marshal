package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// tool is one thing the built-in agent can do. The model chooses it by name and sends its
// arguments; kind is the word agents.ToolCall.Kind and harness.Request.Kind both use, so the
// permission rules read every tool the same way.
type tool struct {
	name        string
	description string
	schema      json.RawMessage
	kind        string
	// probe reads the file and command a call names, before anything runs, so the permission
	// rules can be asked about exactly what the call would touch.
	probe func(s *session, args map[string]any) (path, command string)
	run   func(ctx context.Context, s *session, args map[string]any) toolOutcome
}

// toolOutcome is what one tool run produced: what the model is told, and the shape fields the chat
// view shows on the ToolCall event.
type toolOutcome struct {
	Text    string
	IsError bool
	Path    string
	Command string
}

// toolsFor returns the tool set, in the order the provider is offered them. It is built fresh for
// every session so nothing is shared, but the contents are the same for all of them.
func toolsFor() []tool {
	return []tool{
		{
			name: "read_file",
			description: "Read a file. Returns its contents, up to a limit, with line numbers. " +
				"Use offset and limit to read part of a long file.",
			kind:   "read",
			schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"the file to read, relative to the working folder"},"offset":{"type":"integer","description":"the first line to read, 1-based"},"limit":{"type":"integer","description":"how many lines to read"}},"required":["path"]}`),
			probe:  probePath("path"),
			run:    runReadFile,
		},
		{
			name:        "write_file",
			description: "Write a whole file, making its folders if needed. It replaces whatever the file held.",
			kind:        "edit",
			schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"the file to write, relative to the working folder"},"content":{"type":"string","description":"the whole new contents"}},"required":["path","content"]}`),
			probe:       probePath("path"),
			run:         runWriteFile,
		},
		{
			name: "edit_file",
			description: "Change a file by replacing one exact piece of text with another. The old " +
				"text must appear exactly once unless replace_all is set.",
			kind:   "edit",
			schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string","description":"the exact text to replace"},"new_string":{"type":"string","description":"what to put in its place"},"replace_all":{"type":"boolean","description":"replace every occurrence"}},"required":["path","old_string","new_string"]}`),
			probe:  probePath("path"),
			run:    runEditFile,
		},
		{
			name:        "list_files",
			description: "List files under a folder, by an optional glob pattern such as *.go. Folders are left out.",
			kind:        "search",
			schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"the folder to list, relative to the working folder"},"pattern":{"type":"string","description":"a glob to keep, such as *.go"}},"required":[]}`),
			probe:       probePath("path"),
			run:         runListFiles,
		},
		{
			name:        "search_files",
			description: "Search file contents for a regular expression, and return the matching lines with their file and line number.",
			kind:        "search",
			schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"the regular expression to find"},"path":{"type":"string","description":"the folder to search, relative to the working folder"},"glob":{"type":"string","description":"a glob to limit the files searched, such as *.go"}},"required":["pattern"]}`),
			probe:       probePath("path"),
			run:         runSearchFiles,
		},
		{
			name:        "run_command",
			description: "Run a shell command in the working folder and return what it printed.",
			kind:        "execute",
			schema:      json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"the command line to run"},"timeout_ms":{"type":"integer","description":"how long to allow, in milliseconds"}},"required":["command"]}`),
			probe:       probeCommand,
			run:         runCommand,
		},
	}
}

// probePath is a probe for a tool whose only interesting argument is a file or folder path.
func probePath(key string) func(s *session, args map[string]any) (string, string) {
	return func(s *session, args map[string]any) (string, string) {
		path, err := s.resolveOrCwd(args, key)
		if err != nil {
			return "", ""
		}
		return path, ""
	}
}

// probeCommand is a probe for the run_command tool.
func probeCommand(_ *session, args map[string]any) (string, string) {
	return "", stringArg(args, "command")
}

// runReadFile reads a file, with line numbers, bounded by offset and limit.
func runReadFile(_ context.Context, s *session, args map[string]any) toolOutcome {
	path, err := s.resolve(args, "path")
	if err != nil {
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not read %s: %s", s.shownPath(path), err), IsError: true, Path: path}
	}
	lines := strings.Split(string(data), "\n")
	offset := max(intArg(args, "offset", 1), 1)
	limit := intArg(args, "limit", len(lines))
	if offset > len(lines) {
		return toolOutcome{Text: fmt.Sprintf("%s has %d lines; line %d is past its end", s.shownPath(path), len(lines), offset), Path: path}
	}
	end := min(offset-1+limit, len(lines))
	var b strings.Builder
	for i := offset - 1; i < end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i+1, lines[i])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "(showing lines %d-%d of %d)\n", offset, end, len(lines))
	}
	return toolOutcome{Text: b.String(), Path: path}
}

// runWriteFile writes a whole file, making its folders.
func runWriteFile(_ context.Context, s *session, args map[string]any) toolOutcome {
	path, err := s.resolve(args, "path")
	if err != nil {
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	content := stringArg(args, "content")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not make the folder for %s: %s", s.shownPath(path), err), IsError: true, Path: path}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not write %s: %s", s.shownPath(path), err), IsError: true, Path: path}
	}
	return toolOutcome{Text: fmt.Sprintf("wrote %s (%d bytes)", s.shownPath(path), len(content)), Path: path}
}

// runEditFile replaces one exact piece of text with another.
func runEditFile(_ context.Context, s *session, args map[string]any) toolOutcome {
	path, err := s.resolve(args, "path")
	if err != nil {
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	old := stringArg(args, "old_string")
	if old == "" {
		return toolOutcome{Text: "old_string must not be empty", IsError: true, Path: path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not read %s: %s", s.shownPath(path), err), IsError: true, Path: path}
	}
	text := string(data)
	count := strings.Count(text, old)
	switch {
	case count == 0:
		return toolOutcome{Text: fmt.Sprintf("%s does not contain that exact text", s.shownPath(path)), IsError: true, Path: path}
	case count > 1 && !boolArg(args, "replace_all"):
		return toolOutcome{Text: fmt.Sprintf("that text appears %d times in %s; add more of it, or set replace_all", count, s.shownPath(path)), IsError: true, Path: path}
	}
	next := strings.Replace(text, old, stringArg(args, "new_string"), 1)
	replaced := 1
	if boolArg(args, "replace_all") {
		next = strings.ReplaceAll(text, old, stringArg(args, "new_string"))
		replaced = count
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not write %s: %s", s.shownPath(path), err), IsError: true, Path: path}
	}
	what := "1 place"
	if replaced != 1 {
		what = strconv.Itoa(replaced) + " places"
	}
	return toolOutcome{Text: fmt.Sprintf("edited %s (%s)", s.shownPath(path), what), Path: path}
}

// runListFiles lists files under a folder, by an optional glob.
func runListFiles(_ context.Context, s *session, args map[string]any) toolOutcome {
	root, err := s.resolveOrCwd(args, "path")
	if err != nil {
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	pattern := stringArg(args, "pattern")
	var found []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if pattern != "" {
			if ok, _ := filepath.Match(pattern, d.Name()); !ok {
				return nil
			}
		}
		found = append(found, s.shownPath(path))
		if len(found) >= maxListedFiles {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return toolOutcome{Text: fmt.Sprintf("could not list %s: %s", s.shownPath(root), err), IsError: true, Path: root}
	}
	sort.Strings(found)
	if len(found) == 0 {
		return toolOutcome{Text: "no files matched", Path: root}
	}
	text := strings.Join(found, "\n")
	if len(found) >= maxListedFiles {
		text += fmt.Sprintf("\n(showing the first %d)", maxListedFiles)
	}
	return toolOutcome{Text: text, Path: root}
}

// runSearchFiles searches file contents for a regular expression.
func runSearchFiles(_ context.Context, s *session, args map[string]any) toolOutcome {
	root, err := s.resolveOrCwd(args, "path")
	if err != nil {
		return toolOutcome{Text: err.Error(), IsError: true}
	}
	expr, err := regexp.Compile(stringArg(args, "pattern"))
	if err != nil {
		return toolOutcome{Text: fmt.Sprintf("that is not a valid regular expression: %s", err), IsError: true, Path: root}
	}
	glob := stringArg(args, "glob")
	var b strings.Builder
	matches := 0
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			if ok, _ := filepath.Match(glob, d.Name()); !ok {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if expr.MatchString(line) {
				fmt.Fprintf(&b, "%s:%d: %s\n", s.shownPath(path), i+1, strings.TrimSpace(line))
				matches++
				if matches >= maxSearchMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return toolOutcome{Text: fmt.Sprintf("could not search %s: %s", s.shownPath(root), walkErr), IsError: true, Path: root}
	}
	if matches == 0 {
		return toolOutcome{Text: "no matches", Path: root}
	}
	text := b.String()
	if matches >= maxSearchMatches {
		text += fmt.Sprintf("(showing the first %d matches)\n", maxSearchMatches)
	}
	return toolOutcome{Text: text, Path: root}
}

// runCommand runs a shell command in the working folder.
func runCommand(ctx context.Context, s *session, args map[string]any) toolOutcome {
	command := stringArg(args, "command")
	if strings.TrimSpace(command) == "" {
		return toolOutcome{Text: "command must not be empty", IsError: true}
	}
	timeout := s.cfg.CommandTimeout
	if ms := intArg(args, "timeout_ms", 0); ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = s.cwd
	cmd.Env = s.commandEnv()
	out, err := cmd.CombinedOutput()
	text := clampOutput(string(out), s.cfg.ToolOutputBytes)
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		return toolOutcome{Text: exitText(text, "the command ran for too long and was stopped"), IsError: true, Command: command}
	case err != nil:
		return toolOutcome{Text: exitText(text, "the command failed: "+err.Error()), IsError: true, Command: command}
	case strings.TrimSpace(text) == "":
		return toolOutcome{Text: "(the command printed nothing)", Command: command}
	default:
		return toolOutcome{Text: text, Command: command}
	}
}

// exitText joins what a command printed with the sentence about how it ended.
func exitText(output, tail string) string {
	if strings.TrimSpace(output) == "" {
		return tail
	}
	return output + "\n" + tail
}

// commandEnv is the environment a run_command gets: the daemon's own, with a plain locale. It is
// deliberately the daemon's environment, so the agent can reach the tools the owner has on their
// PATH, exactly as a CLI agent started by Marshal can.
func (s *session) commandEnv() []string {
	return append(os.Environ(), "LANG=en_US.UTF-8")
}

// resolve reads a path argument, checks it is there, and returns it as an absolute path inside the
// session's working folder.
func (s *session) resolve(args map[string]any, key string) (string, error) {
	raw := stringArg(args, key)
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	return s.resolveOrCwd(args, key)
}

// resolveOrCwd is resolve for a path that may be left out, in which case the working folder is
// meant.
func (s *session) resolveOrCwd(args map[string]any, key string) (string, error) {
	raw := stringArg(args, key)
	if strings.TrimSpace(raw) == "" {
		return s.cwd, nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	clean := filepath.Clean(filepath.Join(s.cwd, raw))
	return clean, nil
}

// shownPath is a path as the model and the chat view see it: relative to the working folder when it
// is inside it, and absolute when it is not.
func (s *session) shownPath(path string) string {
	rel, err := filepath.Rel(s.cwd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// clampOutput cuts a tool's own output to the configured limit, keeping the head.
func clampOutput(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := text[:limit]
	for len(cut) > 0 && !isRuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	return cut + fmt.Sprintf("\n(output cut at %d bytes)", limit)
}

// isRuneStart reports whether b can start a UTF-8 character.
func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// Limits on how much a listing or search may return, so one call cannot flood the model.
const (
	maxListedFiles   = 2000
	maxSearchMatches = 500
)

// stringArg reads a string argument, or "" when it is missing or not a string.
func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// boolArg reads a boolean argument, or false when it is missing or not a boolean.
func boolArg(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

// intArg reads an integer argument, or fallback when it is missing or not a number. JSON numbers
// arrive as float64, so a whole number is taken and anything else is the fallback.
func intArg(args map[string]any, key string, fallback int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return fallback
}

// parseArgs reads a tool call's arguments as a plain map. Anything that does not decode this way is
// treated as having no fields, so a tool that needs one reports it.
func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}
