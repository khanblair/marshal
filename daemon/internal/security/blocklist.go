package security

import (
	"path"
	"path/filepath"
	"strings"
)

// The names the blocklist's rules are recorded under. They are stable: an audit row and a test both
// name the rule that fired, and renaming one would make an old row mean something else.
const (
	// RuleProfile is an action the permission profile does not allow at all.
	RuleProfile = "profile"
	// RuleDeploy is a deploy workflow, which needs a person in every mode but bypass.
	RuleDeploy = "deploy"
	// RuleForkBomb is a shell fork bomb.
	RuleForkBomb = "fork-bomb"
	// RuleRecursiveDelete is a recursive delete of a path that is not a build artifact.
	RuleRecursiveDelete = "recursive-delete"
	// RuleDestructiveSQL is dropping or truncating a database, table, schema, or role.
	RuleDestructiveSQL = "destructive-sql"
	// RuleRawDisk is writing straight to a disk device.
	RuleRawDisk = "raw-disk"
	// RuleFilesystemFormat is formatting a filesystem.
	RuleFilesystemFormat = "filesystem-format"
	// RuleSystemPower is shutting down or rebooting the machine.
	RuleSystemPower = "system-power"
	// RuleWidePermissions is a recursive permission or owner change over a wide path.
	RuleWidePermissions = "wide-permissions"
	// RuleSudo is running a command as another user.
	RuleSudo = "sudo"
	// RuleProductionCredentials is a command that reaches for production credentials.
	RuleProductionCredentials = "production-credentials"
	// RulePipeToShell is running a script fetched from the network in a shell.
	RulePipeToShell = "pipe-to-shell"
	// RuleUnreadable is a command line no rule could read, because it holds a substitution or a
	// variable where the command should be. It is never allowed on its own words.
	RuleUnreadable = "unreadable-command"
)

// Blocklist is the command blocklist (docs/marshal-product-scope.md section 14.4): the commands
// Marshal always refuses and the commands that always need a person, in every mode except bypass.
//
// It reads a command line, not an agent's description of it, so it holds whatever the agent claims.
// Only the first rule that fires decides, and the rules run from the most dangerous to the least, so
// a command that is more than one of them is recorded under the worst one.
type Blocklist struct {
	rules []blockRule
}

// blockRule is one entry of the list: its name, what it answers, and how it recognizes a command.
// The match returns a short phrase naming what it saw, for the detail of a refusal. A raw rule reads
// the whole line rather than one segment, because the characters it looks for are the ones a segment
// splitter treats as separators.
type blockRule struct {
	name    string
	verdict Verdict
	raw     bool
	match   func(command string, args []string) (string, bool)
}

// DefaultBlocklist is the list Marshal ships with. It is small on purpose: every rule here is one a
// person loses nothing by, and a rule that would refuse ordinary work does not belong on it.
func DefaultBlocklist() *Blocklist {
	return &Blocklist{rules: []blockRule{
		{name: RuleForkBomb, verdict: VerdictBlock, raw: true, match: matchForkBomb},
		{name: RuleRawDisk, verdict: VerdictBlock, match: matchRawDisk},
		{name: RuleFilesystemFormat, verdict: VerdictBlock, match: matchFilesystemFormat},
		{name: RuleSystemPower, verdict: VerdictBlock, match: matchSystemPower},
		{name: RuleDestructiveSQL, verdict: VerdictBlock, match: matchDestructiveSQL},
		{name: RuleRecursiveDelete, verdict: VerdictBlock, match: matchRecursiveDelete},
		{name: RuleWidePermissions, verdict: VerdictBlock, match: matchWidePermissions},
		{name: RulePipeToShell, verdict: VerdictAsk, raw: true, match: matchPipeToShell},
		{name: RuleSudo, verdict: VerdictAsk, match: matchSudo},
		{name: RuleProductionCredentials, verdict: VerdictAsk, match: matchProductionCredentials},
	}}
}

// Check answers what the list says about a command line. It reports false when no rule matches,
// which means the list has nothing to say and the mode decides.
//
// A rule with something to say about any one command in the line decides it: "git status && rm -rf /"
// is a recursive delete, not a status. The rules run from the most dangerous to the least, so a line
// that is more than one of them is recorded under the worst one.
//
// A line no rule matched but that cannot be read - it holds a command substitution, or a command
// whose own name is a variable - is handed to a person rather than allowed: the list has nothing to
// say about a command it cannot see, and "nothing to say" must not read as "fine".
func (b *Blocklist) Check(command string) (Finding, bool) {
	if b == nil {
		return Finding{}, false
	}
	segments := Segments(command)
	for _, r := range b.rules {
		if r.raw {
			if _, ok := r.match(command, Words(command)); ok {
				return Finding{Verdict: r.verdict, Rule: r.name}, true
			}
			continue
		}
		for _, segment := range segments {
			if _, ok := r.match(segment, Words(segment)); ok {
				return Finding{Verdict: r.verdict, Rule: r.name}, true
			}
		}
	}
	if Substitute(command) {
		return Finding{Verdict: VerdictAsk, Rule: RuleUnreadable}, true
	}
	for _, segment := range segments {
		if namesByVariable(Words(segment)) {
			return Finding{Verdict: VerdictAsk, Rule: RuleUnreadable}, true
		}
	}
	return Finding{}, false
}

// matchForkBomb recognizes a shell fork bomb. It reads the raw line, because the characters the bomb
// is made of are the ones Words treats as separators.
func matchForkBomb(command string, _ []string) (string, bool) {
	compact := strings.Join(strings.Fields(command), "")
	if strings.Contains(compact, ":(){") {
		return "a fork bomb", true
	}
	return "", false
}

// matchRawDisk recognizes a write straight to a disk device: dd writing to one, or a shell redirect
// into one. A command that merely names a disk device - ls, fdisk -l - is reading, and is left alone.
func matchRawDisk(command string, args []string) (string, bool) {
	args = commandTail(args)
	if len(args) > 0 && filepath.Base(args[0]) == "dd" {
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "of=/dev/") {
				return a, true
			}
		}
	}
	compact := strings.Join(strings.Fields(command), " ")
	for _, device := range diskDevices {
		if strings.Contains(compact, "> "+device) || strings.Contains(compact, ">"+device) {
			return "> " + device, true
		}
	}
	return "", false
}

// diskDevices are the device name prefixes a redirect must never write into.
var diskDevices = []string{
	"/dev/sd", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/nvme", "/dev/disk", "/dev/rdisk",
	"/dev/mapper", "/dev/loop", "/dev/mmcblk",
}

// matchFilesystemFormat recognizes mkfs and its variants, and the other tools that write a fresh
// filesystem over a device.
func matchFilesystemFormat(_ string, args []string) (string, bool) {
	args = commandTail(args)
	if len(args) == 0 {
		return "", false
	}
	name := filepath.Base(args[0])
	switch {
	case name == "mkfs" || strings.HasPrefix(name, "mkfs."):
		return name, true
	case name == "mke2fs" || strings.HasPrefix(name, "mke2fs."):
		return name, true
	case name == "newfs" || strings.HasPrefix(name, "newfs_"):
		return name, true
	}
	return "", false
}

// matchSystemPower recognizes a command that would take the machine down.
func matchSystemPower(_ string, args []string) (string, bool) {
	args = commandTail(args)
	if len(args) == 0 {
		return "", false
	}
	name := filepath.Base(args[0])
	switch name {
	case "shutdown", "reboot", "halt", "poweroff":
		return name, true
	case "init", "telinit":
		if len(args) > 1 && (args[1] == "0" || args[1] == "6") {
			return name + " " + args[1], true
		}
	case "systemctl":
		if len(args) > 1 {
			switch args[1] {
			case "reboot", "poweroff", "halt", "shutdown":
				return "systemctl " + args[1], true
			}
		}
	}
	return "", false
}

// matchDestructiveSQL recognizes dropping or truncating a database, table, schema, or role. It reads
// the argument list a shell would hand the client, so a statement passed with -c is seen the same way
// as one typed into a client's own prompt; but only the argument that is a statement is read, so a
// word that merely resembles SQL - a commit message, a string another program was handed - is
// ordinary work.
func matchDestructiveSQL(command string, _ []string) (string, bool) {
	tail := ArgvTail(command)
	if len(tail) == 0 {
		return "", false
	}
	client := filepath.Base(tail[0])
	switch client {
	case "dropdb", "dropuser", "pg_dropcluster", "dropdatabase":
		// These clients are destructive on their own name.
		return client, true
	}
	if !sqlClients[client] {
		return "", false
	}
	for _, statement := range statementArguments(client, tail[1:]) {
		if what, ok := destructiveStatement(statement); ok {
			return what, true
		}
	}
	return "", false
}

// statementArguments are the SQL statements a client invocation carries on its command line: the
// value of a flag that takes a statement, and, for a client that takes one with no flag at all, the
// bare arguments that follow. Reading only these is what keeps a word that merely resembles SQL from
// being read as a statement.
func statementArguments(client string, args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if flag, value, hasValue := strings.Cut(arg, "="); hasValue {
				if sqlValueFlags[flag] {
					out = append(out, value)
				}
				continue
			}
			if sqlValueFlags[arg] && i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
			continue
		}
		if positionalSQLClients[client] {
			out = append(out, arg)
		}
	}
	return out
}

// destructiveStatement says whether one statement drops or truncates something. Its words are taken
// as a shell hands them over, so a name inside a string literal keeps the quote that opens it and is
// not read as a statement of its own: "select 'drop table'" is a query, "drop table users" is not.
func destructiveStatement(statement string) (string, bool) {
	words := lowerWords(strings.Fields(statement))
	for i, w := range words {
		switch w {
		case "dropdb", "dropuser", "pg_dropcluster", "dropdatabase":
			return w, true
		case "drop":
			if next, ok := nextWord(words, i); ok && dropTargets[next] {
				return "drop " + next, true
			}
		case "truncate":
			// "truncate" is also a coreutils command that shortens a file, which is ordinary work;
			// only the SQL form, "truncate table", is destructive.
			if next, ok := nextWord(words, i); ok && next == "table" {
				return "truncate table", true
			}
		}
	}
	return "", false
}

// sqlClients are the tools that run a SQL statement given on the command line.
var sqlClients = map[string]bool{
	"psql": true, "mysql": true, "mariadb": true, "sqlite": true, "sqlite3": true,
	"mongo": true, "mongosh": true, "cockroach": true, "duckdb": true, "cqlsh": true,
	"clickhouse-client": true, "redis-cli": true,
}

// sqlValueFlags are the flags a SQL client uses to take a statement on the command line.
var sqlValueFlags = map[string]bool{
	"-c": true, "--command": true, "-e": true, "--execute": true,
	"-q": true, "--query": true, "--eval": true, "--sql": true,
}

// positionalSQLClients are the clients that take a statement as a bare argument rather than behind a
// flag. A client that takes a subcommand of its own first (redis-cli) is not one of them, because its
// bare arguments are that subcommand's, not SQL.
var positionalSQLClients = map[string]bool{
	"sqlite": true, "sqlite3": true, "duckdb": true, "cqlsh": true,
}

// dropTargets are the things a drop statement is destructive against.
var dropTargets = map[string]bool{
	"database": true, "table": true, "schema": true, "index": true, "role": true,
	"user": true, "tablespace": true, "extension": true, "view": true,
}

// matchRecursiveDelete recognizes a recursive delete of a path that is not a folder inside a
// project. Deleting a build folder, a cache, or a temporary folder is ordinary work and is left
// alone: what this refuses is the whole of a home folder, a root, or a disk.
func matchRecursiveDelete(_ string, args []string) (string, bool) {
	args = commandTail(args)
	if len(args) == 0 || filepath.Base(args[0]) != "rm" {
		return "", false
	}
	recursive := false
	var targets []string
	for _, a := range args[1:] {
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "--") {
			recursive = recursive || a == "--recursive"
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			recursive = recursive || strings.ContainsAny(a[1:], "rR")
			continue
		}
		targets = append(targets, a)
	}
	if !recursive || len(targets) == 0 {
		return "", false
	}
	for _, t := range targets {
		if wideDeleteTarget(t) {
			return "rm " + strings.Join(targets, " "), true
		}
	}
	return "", false
}

// wideDeleteTarget says whether a recursive delete of this path is wide: a home folder, a root, a
// disk, a working folder, or everything under one of them. A folder with anything below it is not
// wide, so "rm -rf /Users/me/project/build" is ordinary work.
//
// The path is read as a shell would read it: a trailing slash is ignored, and a variable's brace
// spelling is folded to its bare one, so "~/" is the home folder, "${HOME:?}" is the home folder,
// and "$PWD" is the folder the command runs in.
func wideDeleteTarget(target string) bool {
	trimmed := strings.TrimRight(target, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	folded := foldExpansion(trimmed)
	if wideTargets[trimmed] || wideTargets[folded] {
		return true
	}
	clean := path.Clean(folded)
	switch {
	case wideTargets[clean]:
		return true
	case path.Base(clean) == "..":
		// "rm -rf ./.." and "rm -rf $PWD/.." reach the folder above, whatever it is.
		return true
	case path.IsAbs(clean) && path.Dir(clean) == "/":
		return true
	}
	if base, ok := strings.CutSuffix(clean, "/*"); ok && wideTargets[base] {
		return true
	}
	return false
}

// foldExpansion reduces the brace spelling of a shell variable to its bare one, so "${HOME}",
// "${HOME:?}", and "$HOME" are all read the same. It does not resolve a variable: it only removes
// the spelling a person may add around one.
func foldExpansion(target string) string {
	if !strings.Contains(target, "${") {
		return target
	}
	var b strings.Builder
	for i := 0; i < len(target); {
		if strings.HasPrefix(target[i:], "${") {
			if end := strings.IndexByte(target[i:], '}'); end >= 0 {
				inner := target[i+2 : i+end]
				if cut := strings.IndexAny(inner, ":-#%/"); cut >= 0 {
					inner = inner[:cut]
				}
				b.WriteString("$" + inner)
				i += end + 1
				continue
			}
		}
		b.WriteByte(target[i])
		i++
	}
	return b.String()
}

// wideTargets are the paths a recursive delete must never reach.
var wideTargets = map[string]bool{
	"/": true, "/*": true, "~": true, "~/*": true, "$HOME": true, "${HOME}": true, "$HOME/*": true,
	"$PWD": true, "${PWD}": true, "$OLDPWD": true, "${OLDPWD}": true,
	".": true, "..": true, "./*": true, "../*": true, "*": true,
}

// matchWidePermissions recognizes a recursive permission or owner change over a wide path, and the
// classic "chmod 777" of a root.
func matchWidePermissions(_ string, args []string) (string, bool) {
	args = commandTail(args)
	if len(args) == 0 {
		return "", false
	}
	tool := filepath.Base(args[0])
	if tool != "chmod" && tool != "chown" && tool != "chgrp" {
		return "", false
	}
	recursive := false
	var targets []string
	for _, a := range args[1:] {
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "--") {
			recursive = recursive || a == "--recursive"
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			recursive = recursive || strings.ContainsAny(a[1:], "rR")
			continue
		}
		targets = append(targets, a)
	}
	if !recursive {
		return "", false
	}
	for _, t := range targets {
		if wideDeleteTarget(t) {
			return tool + " " + t, true
		}
	}
	return "", false
}

// matchPipeToShell recognizes a script fetched over the network and run in a shell in one line:
// "curl -fsSL https://example.com/install.sh | sh". The pipe is what makes it a different thing
// from downloading a file, so it is looked for in the raw line. Every pipe in the line is read, so
// a fetch chained behind another command ("true && curl ... | sh") is caught too.
func matchPipeToShell(command string, _ []string) (string, bool) {
	if !strings.Contains(command, "|") {
		return "", false
	}
	parts := splitPipes(command)
	for i := 0; i+1 < len(parts); i++ {
		switch commandProgram(parts[i]) {
		case "curl", "wget":
		default:
			continue
		}
		switch commandProgram(parts[i+1]) {
		case "sh", "bash", "zsh", "dash", "ksh":
			return "a downloaded script", true
		}
	}
	return "", false
}

// splitPipes cuts a pipeline at its pipes. A pipe inside a quote is text rather than a pipe, so a URL
// that holds one ("curl 'https://x/a|b' | sh") does not pair the wrong commands.
func splitPipes(command string) []string {
	var out []string
	var b strings.Builder
	runes := []rune(command)
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '\\':
			b.WriteRune(r)
			if i+1 < len(runes) {
				i++
				b.WriteRune(runes[i])
			}
		case '|':
			if i+1 < len(runes) && runes[i+1] == '|' {
				b.WriteRune(r)
				b.WriteRune(runes[i+1])
				i++
				continue
			}
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(out, b.String())
}

// commandProgram is the program a segment would run: the last command in it, past the wrappers.
func commandProgram(segment string) string {
	sub := splitOperators(segment)
	for i := len(sub) - 1; i >= 0; i-- {
		one := strings.TrimSpace(sub[i])
		if one == "" {
			continue
		}
		if args := commandTail(Words(one)); len(args) > 0 {
			return filepath.Base(args[0])
		}
		return ""
	}
	return ""
}

// matchSudo recognizes a command run as another user. It is not refused, because a person may well
// mean it, but it is never something an agent should do without being asked. It looks past the other
// wrappers, their options, and a leading VAR=value assignment, so "env -i sudo ls" and "nice -n 5
// sudo ls" are both seen, but it stops at the sudo rather than unwrapping it.
func matchSudo(_ string, args []string) (string, bool) {
	for len(args) > 0 {
		if isAssignment(args[0]) {
			args = args[1:]
			continue
		}
		name := filepath.Base(args[0])
		if name == "sudo" || name == "doas" {
			return name, true
		}
		if !wrappers[name] {
			return "", false
		}
		args = skipOneWrapper(name, args)
	}
	return "", false
}

// matchProductionCredentials recognizes a command that reaches for a production credential: an
// environment variable whose name says production, or a flag that asks for the production
// environment.
func matchProductionCredentials(_ string, args []string) (string, bool) {
	for _, a := range args {
		name := strings.TrimPrefix(strings.TrimPrefix(a, "$"), "{")
		name = strings.TrimSuffix(name, "}")
		if productionEnvName(name) || productionFlag(a) {
			return a, true
		}
	}
	return "", false
}

// productionEnvName says whether a word is the name of an environment variable that holds a
// production credential. It is deliberately narrow: an ordinary variable called "prod" is not
// enough, because plenty of code has one that is not a secret.
func productionEnvName(name string) bool {
	if name == "" || name != strings.ToUpper(name) || !strings.ContainsAny(name, "_") {
		return false
	}
	for _, part := range strings.Split(name, "_") {
		switch part {
		case "PROD", "PRODUCTION", "LIVE", "SECRET", "TOKEN", "PASSWORD", "CREDENTIALS":
			return true
		}
	}
	return false
}

// productionEnvFlags are the flags that name the production environment.
var productionEnvFlags = map[string]bool{
	"--production": true, "--env=production": true, "--environment=production": true,
	"--env=prod": true, "--environment=prod": true,
}

// productionFlag says whether a word asks for the production environment.
func productionFlag(word string) bool {
	return productionEnvFlags[word]
}

// nextWord is the word after the one at i, and whether there is one.
func nextWord(words []string, i int) (string, bool) {
	if i+1 >= len(words) {
		return "", false
	}
	return words[i+1], true
}

// lowerWords copies words in lower case, for a rule that compares case-insensitively.
func lowerWords(words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = strings.ToLower(w)
	}
	return out
}
