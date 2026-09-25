package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/platform"
)

func envWith(goos, home string, vars map[string]string) platform.Env {
	return platform.Env{GOOS: goos, Home: home, Getenv: func(key string) string { return vars[key] }}
}

func TestDataDir(t *testing.T) {
	home := filepath.Join("home", "sam")
	tests := []struct {
		name string
		env  platform.Env
		mode platform.Mode
		want string
	}{
		{"macOS normal", envWith("darwin", home, nil), platform.ModeNormal, filepath.Join(home, "Library", "Application Support", "Marshal")},
		{"macOS dev", envWith("darwin", home, nil), platform.ModeDev, filepath.Join(home, "Library", "Application Support", "Marshal-dev")},
		{"Linux default", envWith("linux", home, nil), platform.ModeNormal, filepath.Join(home, ".local", "share", "marshal")},
		{"Linux XDG dev", envWith("linux", home, map[string]string{"XDG_DATA_HOME": filepath.Join("data", "x")}), platform.ModeDev, filepath.Join("data", "x", "marshal-dev")},
		{"Windows APPDATA", envWith("windows", home, map[string]string{"APPDATA": filepath.Join("roaming")}), platform.ModeNormal, filepath.Join("roaming", "Marshal")},
		{"Windows fallback dev", envWith("windows", home, nil), platform.ModeDev, filepath.Join(home, "AppData", "Roaming", "Marshal-dev")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := platform.DataDir(tc.env, tc.mode)
			if err != nil {
				t.Fatalf("DataDir returned an error: %v", err)
			}
			if got != tc.want {
				t.Errorf("DataDir = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDataDirNeedsHome(t *testing.T) {
	if _, err := platform.DataDir(envWith("linux", "", nil), platform.ModeNormal); err == nil {
		t.Fatal("DataDir with no home folder returned no error")
	}
}

func TestDevAndNormalFoldersDiffer(t *testing.T) {
	env := envWith("linux", filepath.Join("home", "sam"), nil)
	normal, _ := platform.DataDir(env, platform.ModeNormal)
	dev, _ := platform.DataDir(env, platform.ModeDev)
	if normal == dev {
		t.Fatalf("normal and dev share the folder %q", normal)
	}
}
