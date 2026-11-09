package catalog

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	loginPathTimeout  = 3 * time.Second
	loginPathLifetime = time.Minute
	maxLoginPathBytes = 64 << 10
	loginPathMarker   = "__MARSHAL_TOOL_PATH__"
)

var loginPaths struct { //nolint:gochecknoglobals // bounded, synchronized discovery cache shared by subprocess launchers
	sync.Mutex
	key, path string
	until     time.Time
}

// RuntimeEnv supplies command lookup for a GUI or service process without importing shell
// credentials. Only PATH is recovered. Explicit workflow environment entries may override it.
func RuntimeEnv(workspace, program string) []string {
	first := filepath.Dir(program)
	if !filepath.IsAbs(first) {
		first = ""
	}
	home, _ := os.UserHomeDir()
	base := joinPath("", loginPath(home), filepath.SplitList(os.Getenv("PATH")))
	if workspace != "" {
		var projectBins []string
		for _, relative := range []string{".venv/bin", "venv/bin", ".venv/Scripts", "venv/Scripts", "node_modules/.bin", "vendor/bin"} {
			dir := filepath.Join(workspace, filepath.FromSlash(relative))
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				projectBins = append(projectBins, dir)
			}
		}
		projectBins = append(projectBins, first, base)
		base, first = strings.Join(projectBins, string(os.PathListSeparator)), ""
	}
	return []string{"PATH=" + joinPath(first, base, KnownDirs())}
}

func runtimeDirs(home string) []string {
	var dirs []string
	if home != "" {
		for _, relative := range []string{".cargo/bin", "go/bin", ".pyenv/shims", ".asdf/shims", ".local/share/mise/shims", ".local/share/pnpm", "Library/pnpm"} {
			dirs = append(dirs, filepath.Join(home, filepath.FromSlash(relative)))
		}
	}
	for _, name := range []string{"BUN_INSTALL", "CARGO_HOME", "GOPATH"} {
		if root := os.Getenv(name); filepath.IsAbs(root) {
			dirs = append(dirs, filepath.Join(root, "bin"))
		}
	}
	if root := os.Getenv("PNPM_HOME"); filepath.IsAbs(root) {
		dirs = append(dirs, root)
	}
	return append(dirs, filepath.SplitList(loginPath(home))...)
}

func loginPath(home string) string {
	if runtime.GOOS == "windows" || home == "" {
		return ""
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	if !filepath.IsAbs(shell) {
		return ""
	}
	switch filepath.Base(shell) {
	case "sh", "bash", "zsh", "fish":
	default:
		return ""
	}
	key := home + "\x00" + shell + "\x00" + os.Getenv("PATH")
	loginPaths.Lock()
	defer loginPaths.Unlock()
	if loginPaths.key == key && time.Now().Before(loginPaths.until) {
		return loginPaths.path
	}
	loginPaths.key, loginPaths.path = key, readLoginPath(home, shell)
	loginPaths.until = time.Now().Add(loginPathLifetime)
	return loginPaths.path
}

func readLoginPath(home, shell string) string {
	ctx, cancel := context.WithTimeout(context.Background(), loginPathTimeout)
	defer cancel()
	command := `printf '\n` + loginPathMarker + `%s\n' "$PATH"`
	if filepath.Base(shell) == "fish" {
		command = `printf '\n` + loginPathMarker + `'; string join : $PATH`
	}
	child, err := proc.Start(ctx, proc.Spec{Path: shell, Args: []string{"-ilc", command}, Dir: home})
	if err != nil {
		return ""
	}
	output, readErr := io.ReadAll(io.LimitReader(child.Stdout, maxLoginPathBytes))
	// A chatty or stalled shell cannot hold daemon startup or a coding check indefinitely.
	exit := child.Wait()
	_ = child.Stop(context.Background(), 100*time.Millisecond)
	if readErr != nil || exit.Code != 0 {
		return ""
	}
	_, tail, found := strings.Cut(string(output), loginPathMarker)
	if !found {
		return ""
	}
	line, _, _ := strings.Cut(tail, "\n")
	var dirs []string
	for _, dir := range filepath.SplitList(strings.TrimSpace(line)) {
		if filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}
