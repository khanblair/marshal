package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestKnownDirs(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(name string) string { return vars[name] }
	}
	tests := []struct {
		name string
		goos string
		home string
		vars map[string]string
		want []string
	}{
		{
			name: "macos",
			goos: "darwin",
			home: "/Users/pat",
			want: []string{
				"/Users/pat/.local/bin", "/Users/pat/.claude/local", "/Users/pat/.npm-global/bin",
				"/opt/homebrew/bin", "/usr/local/bin",
			},
		},
		{
			name: "linux with an npm prefix",
			goos: "linux",
			home: "/home/pat",
			vars: map[string]string{"NPM_CONFIG_PREFIX": "/opt/npm"},
			want: []string{
				"/home/pat/.local/bin", "/home/pat/.claude/local", "/home/pat/.npm-global/bin",
				"/opt/npm/bin", "/opt/homebrew/bin", "/usr/local/bin",
			},
		},
		{
			name: "windows",
			goos: "windows",
			home: `C:\Users\pat`,
			vars: map[string]string{"APPDATA": `C:\Users\pat\AppData\Roaming`, "LOCALAPPDATA": `C:\Users\pat\AppData\Local`},
			want: []string{
				filepath.Join(`C:\Users\pat`, ".local", "bin"),
				filepath.Join(`C:\Users\pat`, ".claude", "local"),
				filepath.Join(`C:\Users\pat`, ".npm-global", "bin"),
				filepath.Join(`C:\Users\pat\AppData\Roaming`, "npm"),
				filepath.Join(`C:\Users\pat\AppData\Local`, "Programs"),
			},
		},
		{
			name: "no home folder leaves out what needs it",
			goos: "linux",
			want: []string{"/opt/homebrew/bin", "/usr/local/bin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := knownDirs(tt.goos, tt.home, env(tt.vars))
			if !slices.Equal(got, tt.want) {
				t.Errorf("knownDirs =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestJoinPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	got := joinPath("/a", strings.Join([]string{"/b", "/a", "/c"}, sep), []string{"/c", "/d", ""})
	want := strings.Join([]string{"/a", "/b", "/c", "/d"}, sep)
	if got != want {
		t.Errorf("joinPath = %q, want %q", got, want)
	}
}

func TestProgramEnvPutsTheProgramsFolderFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/somewhere/else")
	env := ProgramEnv(filepath.Join(dir, "gemini"))
	if len(env) != 1 || !strings.HasPrefix(env[0], "PATH=") {
		t.Fatalf("ProgramEnv = %q, want one PATH entry", env)
	}
	dirs := filepath.SplitList(strings.TrimPrefix(env[0], "PATH="))
	if len(dirs) < 2 || dirs[0] != dir || dirs[1] != "/somewhere/else" {
		t.Errorf("PATH = %q, want the program's folder first and the daemon's PATH after it", dirs)
	}
}
