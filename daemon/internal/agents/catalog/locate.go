package catalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// KnownDirs lists the folders where the agents' installers put their programs, for this machine.
// The daemon looks here after its own PATH, because a daemon that the system starts as a user
// service often has a PATH that misses all of them.
func KnownDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return knownDirs(runtime.GOOS, home, os.Getenv)
}

// knownDirs builds the list for an operating system, a home folder, and an environment lookup.
// Folders that need a variable or a home folder that is not there are left out.
func knownDirs(goos, home string, getenv func(string) string) []string {
	var dirs []string
	add := func(elem ...string) {
		if elem[0] != "" {
			dirs = append(dirs, filepath.Join(elem...))
		}
	}
	add(home, ".local", "bin")
	add(home, ".claude", "local")
	add(home, ".npm-global", "bin")
	if prefix := getenv("NPM_CONFIG_PREFIX"); prefix != "" {
		if goos == "windows" {
			add(prefix)
		} else {
			add(prefix, "bin")
		}
	}
	if goos == "windows" {
		add(getenv("APPDATA"), "npm")
		add(getenv("LOCALAPPDATA"), "Programs")
		return dirs
	}
	// Homebrew's folder differs between Apple silicon and Intel Macs, and both are cheap to check.
	dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
	return dirs
}

// ProgramEnv returns the environment entries that a program of an agent needs beyond the short
// list that internal/proc passes: a PATH that starts with the folder of the program itself. Agents
// written in Node.js start through "env node", and node usually sits beside them, but a daemon
// started as a user service may have no node on its own PATH. The rest of the PATH follows, and
// then the well-known folders. Pass the result in the Env of a proc.Spec or an adapter Config.
func ProgramEnv(programPath string) []string {
	return []string{"PATH=" + joinPath(filepath.Dir(programPath), os.Getenv("PATH"), KnownDirs())}
}

// joinPath makes a PATH from a folder that goes first, an existing PATH, and more folders that go
// last. A folder that is already in it is not repeated.
func joinPath(first, existing string, more []string) string {
	seen := map[string]bool{}
	var out []string
	add := func(dir string) {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	add(first)
	for _, dir := range filepath.SplitList(existing) {
		add(dir)
	}
	for _, dir := range more {
		add(dir)
	}
	return strings.Join(out, string(os.PathListSeparator))
}
