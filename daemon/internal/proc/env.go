package proc

import "strings"

// allowedName says whether a variable of the daemon's environment may reach a child. The list is
// short on purpose: the daemon's environment can hold settings and tokens that an agent has no
// business seeing. What a child gets beyond this, the caller says in Spec.Env.
//
// USER and LOGNAME are on the list because Claude Code on macOS looks its login up in the
// Keychain by the user's name, and reports "Not logged in" without it. Found by running one real
// session; a stub agent cannot show it. They name the person, and hold no secret.
//
// The Windows names in the second case are the ones that Node-based agents need to find a shell
// and the programs on PATH. They are written in capitals because on Windows the name is turned
// into capitals before it is compared, so "ProgramFiles" arrives here as "PROGRAMFILES".
func allowedName(name string) bool {
	if foldEnvNames {
		name = strings.ToUpper(name)
	}
	switch name {
	case "PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "SYSTEMROOT",
		"TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "TERM", "SHELL", "SSH_AUTH_SOCK", "USER", "LOGNAME":
		return true
	case "COMSPEC", "PATHEXT", "WINDIR", "PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA":
		return true
	}
	return false
}

// buildEnv returns the environment for a child: the allowed part of base, then the extra
// entries, which replace an entry with the same name.
func buildEnv(base, extra []string) []string {
	env := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		if name, _, ok := strings.Cut(entry, "="); ok && allowedName(name) {
			env = append(env, entry)
		}
	}
	for _, entry := range extra {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		env = withoutName(env, name)
		env = append(env, entry)
	}
	return env
}

// withoutName drops the entries that set the given variable.
func withoutName(env []string, name string) []string {
	kept := env[:0]
	for _, entry := range env {
		other, _, _ := strings.Cut(entry, "=")
		if sameName(other, name) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// sameName compares variable names the way the platform does.
func sameName(a, b string) bool {
	if foldEnvNames {
		return strings.EqualFold(a, b)
	}
	return a == b
}
