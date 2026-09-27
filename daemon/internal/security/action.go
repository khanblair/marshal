// Package security holds Marshal's own rules about what an agent may do: the command blocklist,
// the permission profile, and the deploy rule (docs/backend-checklist.md B3.3 and B3.7,
// docs/marshal-product-scope.md sections 14.3, 14.4, and 14.8).
//
// Everything here reads something the daemon already knows about a request - the kind of tool call,
// the file it names, the command line it would run - and never takes an agent's word for what it is
// about to do. That is what makes these rules independent of an agent's cooperation, unlike a
// permission mode, which the agent's own implementation carries out. The harness (internal/harness)
// puts these rules together with the session's mode; this package only answers "does the profile
// allow this action", "is this command on the list", and "is this a deploy".
package security

import (
	"path/filepath"
	"strings"
)

// Action is what a tool call asks to do, as far as the daemon can read it. It is coarser than the
// agent's own kind word, because a profile speaks in categories, not in tool names.
type Action string

const (
	// ActionRead is reading or searching. Nothing changes.
	ActionRead Action = "read"
	// ActionEdit is writing, moving, or deleting a file.
	ActionEdit Action = "edit"
	// ActionCommand is running a command that is not one of the narrower commands below.
	ActionCommand Action = "command"
	// ActionPush is pushing to a remote.
	ActionPush Action = "push"
	// ActionInstall is installing or updating a dependency.
	ActionInstall Action = "install"
	// ActionDeploy is running a deploy.
	ActionDeploy Action = "deploy"
	// ActionNetwork is reaching out to a network address.
	ActionNetwork Action = "network"
	// ActionOther is anything this package cannot read: it is never treated as harmless, so a mode
	// or a profile with nothing to say about it makes the request the person's.
	ActionOther Action = "other"
)

// Verdict is what a rule says about an action.
type Verdict string

const (
	// VerdictAllow is a rule that permits the action.
	VerdictAllow Verdict = "allow"
	// VerdictAsk is a rule that permits the action only after a person says yes.
	VerdictAsk Verdict = "ask"
	// VerdictBlock is a rule that refuses the action outright.
	VerdictBlock Verdict = "block"
)

// Finding is one rule's answer: its verdict and its stable name. The name is what a refusal is
// recorded under in the audit log and what a test says it expected, so it must not change once it
// has been written down.
type Finding struct {
	Verdict Verdict
	Rule    string
}

// Classify reads a tool call's kind and command and says what the request would do. The kind word
// comes from the agent (an ACP tool call's Kind, for example), and the command is the text of the
// command line it wants to run, if there is one.
//
// An unknown kind with no command is ActionOther, not ActionCommand: the caller must not read
// "unknown" as "harmless".
func Classify(kind, command string) Action {
	k := strings.ToLower(strings.TrimSpace(kind))
	if cmd := strings.TrimSpace(command); cmd != "" {
		// A word about a request is not the request: a tool call declared "read" that carries a
		// command line is read for the command, not for the kind (docs/architecture.md section 13).
		// The kind only says what to do with a call that names no command at all.
		return classifyCommand(cmd)
	}
	switch k {
	case "read", "search", "think":
		return ActionRead
	case "edit", "delete", "move", "write":
		return ActionEdit
	case "fetch", "http", "network":
		return ActionNetwork
	}
	if k == "execute" || k == "run" || k == "bash" || k == "shell" || k == "terminal" {
		return ActionCommand
	}
	return ActionOther
}

// classifyCommand reads a whole command line to say what it would do. It reads every command in the
// line, so a deploy or a push chained behind another command ("cd infra && terraform apply") is
// still read as one.
func classifyCommand(command string) Action {
	for _, segment := range Segments(command) {
		if action := commandAction(Words(segment)); action != ActionCommand {
			return action
		}
	}
	return ActionCommand
}

// commandAction reads a command line to say what it would do.
func commandAction(args []string) Action {
	args = commandTail(args)
	if len(args) == 0 {
		return ActionCommand
	}
	tool := filepath.Base(args[0])
	rest := args[1:]
	switch tool {
	case "git":
		if gitSubcommand(rest) == "push" {
			return ActionPush
		}
	case "curl", "wget", "nc", "ncat", "ssh", "scp", "sftp", "rsync", "telnet", "ftp":
		return ActionNetwork
	}
	if isInstall(tool, rest) {
		return ActionInstall
	}
	if DeployWorkflow(strings.Join(args, " ")) {
		return ActionDeploy
	}
	return ActionCommand
}

// isInstall says whether a package manager is being asked to install or update a dependency. The
// tools it does not know are left alone, so a new package manager is a command, not a mistake.
func isInstall(tool string, rest []string) bool {
	verbs, known := installVerbs[tool]
	if !known {
		return false
	}
	for _, arg := range rest {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return verbs[arg]
	}
	return false
}

// installVerbs are the subcommands that install or update a dependency, by tool.
var installVerbs = map[string]map[string]bool{
	"npm":      {"install": true, "i": true, "ci": true, "add": true},
	"pnpm":     {"install": true, "i": true, "add": true},
	"yarn":     {"install": true, "add": true},
	"bun":      {"install": true, "add": true},
	"pip":      {"install": true},
	"pip3":     {"install": true},
	"gem":      {"install": true},
	"go":       {"install": true, "get": true},
	"cargo":    {"install": true},
	"brew":     {"install": true, "upgrade": true},
	"apt":      {"install": true},
	"apt-get":  {"install": true},
	"bundle":   {"install": true},
	"composer": {"install": true, "require": true},
	"poetry":   {"install": true, "add": true},
}

// Words splits a command line into the words a rule can compare, dropping the characters a shell
// would treat as punctuation. It is deliberately blunt: "psql -c 'DROP TABLE t'" and
// psql -c DROP TABLE t are meant to read the same way, so a quote or a semicolon cannot hide a
// command from the blocklist.
//
// A quote is removed rather than treated as a separator, because a shell joins what an empty quote
// separates: "r”m" is the word "rm" to a shell, and a rule that read it as two words could be
// dodged by typing one extra character.
func Words(command string) []string {
	return strings.FieldsFunc(stripQuotes(command), shellSeparator)
}

// stripQuotes removes the quote characters a shell would remove, without separating the text either
// side of them.
func stripQuotes(command string) string {
	if !strings.ContainsAny(command, "'\"`") {
		return command
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '\'', '"', '`':
			return -1
		}
		return r
	}, command)
}

// Argv splits a command line into the arguments a shell would pass to the program it runs: quotes
// group characters into one argument and are removed, a backslash escapes the next character, and
// everything outside a quote is split on whitespace. Unlike Words it keeps a quoted argument whole,
// which is what a rule needs when the shape of an argument matters - a commit message is one
// argument however many words it holds.
//
// A backtick is not a quote here. A shell would run what a backtick brackets and substitute its
// output, so the text of the argument is not knowable from the line; leaving the backtick in the
// argument is what lets Readable see that the line cannot be read.
func Argv(command string) []string {
	var (
		out     []string
		b       strings.Builder
		started bool
		quote   rune
	)
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == quote {
				quote = 0
				started = true
				continue
			}
			if r == '\\' && quote == '"' && i+1 < len(runes) {
				i++
				r = runes[i]
			}
			b.WriteRune(r)
			started = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case '\\':
			if i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
				started = true
			}
		case ' ', '\t', '\n', '\r':
			if started {
				out = append(out, b.String())
				b.Reset()
				started = false
			}
		default:
			b.WriteRune(r)
			started = true
		}
	}
	if started {
		out = append(out, b.String())
	}
	return out
}

// CommandWords is the words of one command with the wrappers removed, so its first word is the
// command that would really run. A rule compares against this, not against the raw line.
func CommandWords(command string) []string {
	return commandTail(Words(command))
}

// ArgvTail is the arguments of one command with the wrappers removed: the reading a rule uses when
// an argument's own shape matters, such as a Git command's argument list.
func ArgvTail(command string) []string {
	return commandTail(Argv(command))
}

// Segments splits a shell line into the separate commands a shell would run, so a rule sees every
// command in "git status && rm -rf /", not only the first. A wrapper that runs its argument as a
// script ("bash -c 'rm -rf /'") is expanded in place, and the expansion is bounded so a script that
// builds another script cannot loop forever.
func Segments(command string) []string {
	return expandSegments(command, 0)
}

// maxSegmentDepth bounds how many layers of "sh -c 'sh -c ...'" are followed.
const maxSegmentDepth = 4

func expandSegments(command string, depth int) []string {
	var out []string
	for _, raw := range splitOperators(command) {
		segment := strings.TrimSpace(raw)
		if segment == "" {
			continue
		}
		if depth < maxSegmentDepth {
			if prefix, script, ok := shellScript(segment); ok {
				// A wrapper in front of the shell ("sudo bash -c ...") is a command of its own to a
				// rule - running as another user is worth asking about - so it is kept, and the
				// script the shell would run is read like any other command.
				if prefix != "" {
					out = append(out, prefix)
				}
				out = append(out, expandSegments(script, depth+1)...)
				continue
			}
			if script, ok := envScript(segment); ok {
				out = append(out, expandSegments(script, depth+1)...)
				continue
			}
		}
		out = append(out, segment)
	}
	return out
}

// splitOperators cuts a line at the operators that separate one command from the next. A doubled
// operator ("&&", "||") counts once. A single '&' also separates, because a command run in the
// background is still a command that runs. A parenthesis and a backtick separate too, so a command
// inside a substitution ("x=$(rm -rf ~)", "x=`rm -rf ~`") is read like any other; the character
// itself is kept at the end of the command before it, so a rule that reads a command to decide what
// it does still sees that a substitution is there and can refuse to guess (Readable).
//
// An operator inside a quote is text, not an operator: "git commit -m 'a; b'" is one command. A
// backtick inside double quotes still substitutes, so it separates there too; inside single quotes
// nothing substitutes, so it does not.
func splitOperators(command string) []string {
	var out []string
	var b strings.Builder
	runes := []rune(command)
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == '\'' {
			b.WriteRune(r)
			if r == '\'' {
				quote = 0
			}
			continue
		}
		if quote == '"' {
			switch {
			case r == '\\' && i+1 < len(runes):
				b.WriteRune(r)
				i++
				b.WriteRune(runes[i])
			case r == '"':
				quote = 0
				b.WriteRune(r)
			case r == '`':
				b.WriteRune(r)
				out = append(out, b.String())
				b.Reset()
			default:
				b.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'':
			quote = '\''
			b.WriteRune(r)
		case '"':
			quote = '"'
			b.WriteRune(r)
		case '\\':
			b.WriteRune(r)
			if i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
			}
		case '`', '(', ')':
			b.WriteRune(r)
			out = append(out, b.String())
			b.Reset()
			if i+1 < len(runes) && runes[i+1] == r {
				i++
			}
		case '\n', ';', '&', '|':
			out = append(out, b.String())
			b.Reset()
			if i+1 < len(runes) && runes[i+1] == r {
				i++
			}
		default:
			b.WriteRune(r)
		}
	}
	return append(out, b.String())
}

// shells are the programs that run a script given after "-c".
var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "ash": true, "fish": true,
}

// shellScript returns the script a "sh -c SCRIPT" segment would run, so its words can be checked
// like any other command, and the part of the segment before the script, so a wrapper in front of
// the shell ("sudo bash -c ...", "busybox sh -c ...") is still read as the command it is. It
// reports false for a shell invocation that runs no script this way.
func shellScript(segment string) (prefix, script string, ok bool) {
	words := Words(segment)
	i := 0
	for i < len(words) {
		if isAssignment(words[i]) {
			i++
			continue
		}
		name := filepath.Base(words[i])
		if !wrappers[name] {
			break
		}
		i += len(words[i:]) - len(skipOneWrapper(name, words[i:]))
	}
	if i >= len(words) || !shells[filepath.Base(words[i])] {
		return "", "", false
	}
	shell := i
	for j := shell + 1; j < len(words); j++ {
		arg := words[j]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.ContainsRune(arg[1:], 'c') {
			body := strings.TrimSpace(strings.Join(words[j+1:], " "))
			if body == "" {
				return "", "", false
			}
			if shell == 0 {
				return "", body, true
			}
			return strings.Join(words[:shell+1], " "), body, true
		}
	}
	return "", "", false
}

// envScript returns the command "env -S STRING" runs. "-S"/"--split-string" makes env split the
// string into a command and its arguments and run them, so the string is read like a command line of
// its own: otherwise a whole command hides inside what looks like an option's value.
func envScript(segment string) (string, bool) {
	words := Words(segment)
	if len(words) == 0 || filepath.Base(words[0]) != "env" {
		return "", false
	}
	for i := 1; i < len(words); i++ {
		arg := words[i]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			return "", false
		}
		switch {
		case arg == "-S" || arg == "--split-string":
			if i+1 < len(words) {
				return strings.Join(words[i+1:], " "), true
			}
			return "", false
		case strings.HasPrefix(arg, "--split-string="):
			return strings.TrimPrefix(arg, "--split-string="), true
		case strings.HasPrefix(arg, "-S"):
			return strings.TrimPrefix(arg, "-S"), true
		}
	}
	return "", false
}

// wrappers are commands that run another command. A rule looks past them, because "sudo rm -rf /" is
// still a recursive delete of a root, and a rule that could be dodged by typing one extra word would
// be worth nothing.
var wrappers = map[string]bool{
	"sudo": true, "doas": true, "nohup": true, "time": true, "nice": true, "env": true,
	"command": true, "exec": true, "timeout": true, "setsid": true, "ionice": true, "chrt": true,
	"stdbuf": true, "flock": true, "xargs": true, "busybox": true,
}

// commandTail drops the wrappers from the front of a command line, so what is left starts with the
// command that would really run. A leading "NAME=value" is dropped too: a shell sets it in the
// environment of the command that follows rather than running it, so "FOO=1 rm -rf /" is a recursive
// delete. Each wrapper's own options are skipped as well, so "sudo -u root git push" and "nice -n 5
// git push" are both read as a push.
func commandTail(args []string) []string {
	for len(args) > 0 {
		if isAssignment(args[0]) {
			args = args[1:]
			continue
		}
		name := filepath.Base(args[0])
		if !wrappers[name] {
			return args
		}
		args = skipOneWrapper(name, args)
	}
	return args
}

// skipOneWrapper drops one wrapper's name, its options, and the words that belong to the wrapper
// rather than to the command it runs: the VAR=value words that follow "env", and the duration
// "timeout" still has to wait out.
func skipOneWrapper(name string, args []string) []string {
	args = skipWrapperOptions(name, args[1:])
	switch name {
	case "env":
		for len(args) > 0 && isAssignment(args[0]) {
			args = args[1:]
		}
	case "timeout":
		if len(args) > 0 && looksLikeDuration(args[0]) {
			args = args[1:]
		}
	}
	return args
}

// isAssignment says whether a word is the "NAME=value" a shell sets in the environment of the
// command that follows rather than a command of its own. Only the shell's own name form counts, so
// "-n=1" and "a-b=c" are ordinary words.
func isAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := word[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// Substitute reports whether a command line holds a command substitution or another expansion that
// runs or rewrites text the line does not show: "$(...)", a backtick, or an ANSI-C quote ("$'...'",
// "$"..."") whose escapes a shell resolves into a different word. A rule cannot read through one, so
// a line that holds one is never allowed on a rule's word alone.
func Substitute(command string) bool {
	for i := 0; i < len(command); i++ {
		if command[i] == '\\' {
			i++
			continue
		}
		switch command[i] {
		case '`':
			return true
		case '$':
			if i+1 < len(command) {
				switch command[i+1] {
				case '(', '\'', '"':
					return true
				}
			}
		}
	}
	return false
}

// Readable says whether a command line can be read for what it would do. It is false for anything
// Substitute is false for, and also for a variable or brace reference ("$NAME", "${NAME}"), because
// the command a word names - or the branch a ref names - is then not knowable from the line.
//
// A line that is not readable is read by a rule only to refuse it or to hand it to a person, never
// to allow it. That is the conservative direction the whole package takes: where a fact is missing,
// the person decides.
func Readable(command string) bool {
	for i := 0; i < len(command); i++ {
		if command[i] == '\\' {
			i++
			continue
		}
		switch command[i] {
		case '`':
			return false
		case '$':
			if i+1 >= len(command) {
				continue
			}
			switch next := command[i+1]; {
			case next == '(', next == '{', next == '\'', next == '"', next == '$':
				return false
			case next == '_', next >= 'a' && next <= 'z', next >= 'A' && next <= 'Z':
				return false
			}
		}
	}
	return true
}

// namesByVariable says whether a command's own program is a variable ("$R -rf /"), so what it would
// run is not on the line at all.
func namesByVariable(args []string) bool {
	tail := commandTail(args)
	if len(tail) == 0 {
		return false
	}
	name := tail[0]
	return strings.HasPrefix(name, "$") || strings.HasPrefix(name, "`")
}

// wrapperTakesValue names the options of each wrapper that are followed by a value, so the value is
// never mistaken for the command the wrapper will run. An option spelled "-x=value" or "-xvalue"
// carries its value with it and needs no entry.
var wrapperTakesValue = map[string]map[string]bool{
	"sudo": {
		"-u": true, "-g": true, "-p": true, "-C": true, "-h": true, "-r": true, "-t": true,
		"-U": true, "--user": true, "--group": true, "--prompt": true, "--close-from": true,
		"--host": true, "--role": true, "--type": true, "--other-user": true,
	},
	"doas": {"-u": true, "-C": true},
	"env":  {"-u": true, "--unset": true, "-C": true, "--chdir": true, "-S": true, "--split-string": true},
	"nice": {"-n": true, "--adjustment": true},
	"timeout": {
		"-k": true, "--kill-after": true, "-s": true, "--signal": true,
	},
	"time": {"-o": true, "--output": true, "-f": true, "--format": true},
	"exec": {"-a": true},
}

// skipWrapperOptions drops the options of a wrapper from the front of its argument list, so what is
// left starts with the command the wrapper will run.
func skipWrapperOptions(name string, args []string) []string {
	takes := wrapperTakesValue[name]
	for len(args) > 0 {
		arg := args[0]
		if arg == "--" {
			return args[1:]
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			return args
		}
		_, _, hasValue := strings.Cut(arg, "=")
		args = args[1:]
		if takes[arg] && !hasValue {
			if len(args) > 0 {
				args = args[1:]
			}
		}
	}
	return args
}

// looksLikeDuration says whether a word is the interval "timeout" takes, such as "5" or "2.5s".
func looksLikeDuration(word string) bool {
	i := 0
	for i < len(word) && (word[i] >= '0' && word[i] <= '9' || word[i] == '.') {
		i++
	}
	if i == 0 {
		return false
	}
	switch word[i:] {
	case "", "s", "m", "h", "d", "ms":
		return true
	}
	return false
}

// gitSubcommand is the subcommand of a git argument list, skipping the global options that come
// before it, so "git -C repo push" is read as a push. The option list mirrors gitx's own reading of
// the same arguments (internal/gitx splitGitArgs).
func gitSubcommand(rest []string) string {
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if arg == "--" {
			return ""
		}
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
		if _, _, hasValue := strings.Cut(arg, "="); hasValue {
			continue
		}
		if gitGlobalTakesValue[arg] {
			i++
		}
	}
	return ""
}

// gitGlobalTakesValue names the git options that are followed by a value before the subcommand.
var gitGlobalTakesValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--exec-path": true, "--config-env": true,
}

// shellSeparator says whether a rune separates the words of a command line. The quote characters
// are not here: Words removes them first, because a shell joins what they bracket rather than
// splitting on it.
func shellSeparator(r rune) bool {
	switch r {
	case ';', '&', '|', '(', ')', '<', '>', '[', ']', '\n', '\r', '\t', ' ':
		return true
	}
	return false
}
