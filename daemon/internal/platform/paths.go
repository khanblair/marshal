// Package platform holds the small pieces that differ between operating systems and between
// a normal install and dev mode: where data lives, the dev token, and the dev reset guard.
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Mode says whether this daemon is a normal install or a separate dev daemon.
type Mode string

// The two modes. Dev mode keeps its data, port, and credentials apart from a normal install.
const (
	ModeNormal Mode = "normal"
	ModeDev    Mode = "dev"
)

const (
	appFolder = "Marshal"
	devSuffix = "-dev"
)

// Env is what path resolution reads from the machine. Tests supply their own.
type Env struct {
	GOOS   string
	Home   string
	Getenv func(string) string
}

// CurrentEnv reads the real machine.
func CurrentEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, fmt.Errorf("find the home folder: %w", err)
	}
	return Env{GOOS: runtime.GOOS, Home: home, Getenv: os.Getenv}, nil
}

// DataDir returns the folder where Marshal keeps its database, logs, and worktrees. The dev
// folder has its own name, so a dev daemon never touches a normal install.
func DataDir(env Env, mode Mode) (string, error) {
	if env.Home == "" {
		return "", errors.New("the home folder is not known")
	}
	name := appFolder
	if mode == ModeDev {
		name += devSuffix
	}
	switch env.GOOS {
	case "darwin":
		return filepath.Join(env.Home, "Library", "Application Support", name), nil
	case "windows":
		base := env.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(env.Home, "AppData", "Roaming")
		}
		return filepath.Join(base, name), nil
	default:
		base := env.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(env.Home, ".local", "share")
		}
		return filepath.Join(base, strings.ToLower(name)), nil
	}
}
