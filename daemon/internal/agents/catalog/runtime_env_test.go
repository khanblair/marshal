package catalog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRuntimeEnvFindsToolsFromLoginPathWithoutImportingOtherVariables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell discovery is for Unix GUI services")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "custom-runtime", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, "zsh")
	script := "#!/bin/sh\nprintf 'profile startup text\\n'\nprintf '" + loginPathMarker + "%s\\n' '" + bin + "'\nprintf 'OTHER_VARIABLE=must_not_be_imported\\n'\n"
	if err := os.WriteFile(shell, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	env := RuntimeEnv("", "/bin/sh")
	if len(env) != 1 || !strings.HasPrefix(env[0], "PATH=") || !strings.Contains(env[0], bin) || strings.Contains(env[0], "OTHER_VARIABLE") {
		t.Fatalf("runtime environment did not recover only PATH: %q", env)
	}
}

func TestRuntimeEnvPrefersTheProjectsVirtualEnvironmentAndLocalTools(t *testing.T) {
	project := t.TempDir()
	for _, relative := range []string{".venv/bin", "node_modules/.bin", "vendor/bin"} {
		if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(relative)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := RuntimeEnv(project, "/bin/sh")
	dirs := filepath.SplitList(strings.TrimPrefix(env[0], "PATH="))
	want := []string{filepath.Join(project, ".venv", "bin"), filepath.Join(project, "node_modules", ".bin"), filepath.Join(project, "vendor", "bin")}
	for i, dir := range want {
		if dirs[i] != dir {
			t.Fatalf("project tool order: %v", dirs)
		}
	}
	for _, dir := range dirs {
		if dir == "" || dir == "." {
			t.Fatal("runtime PATH added the current directory")
		}
	}
}

func TestRuntimeEnvPreservesServicePathAlongsideLoginPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell discovery is for Unix GUI services")
	}
	home := t.TempDir()
	loginBin := filepath.Join(home, "login-bin")
	serviceBin := filepath.Join(home, "service-bin")
	shell := filepath.Join(home, "zsh")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '"+loginPathMarker+"%s\\n' '"+loginBin+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", serviceBin)
	env := RuntimeEnv("", "/bin/sh")
	if !strings.Contains(env[0], serviceBin) || !strings.Contains(env[0], loginBin) {
		t.Fatalf("runtime PATH lost service or login directories: %q", env)
	}
}

func TestRuntimeEnvReusesLoginPathCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell discovery is for Unix GUI services")
	}
	home := t.TempDir()
	shell := filepath.Join(home, "zsh")
	writeShell := func(bin string) {
		t.Helper()
		script := "#!/bin/sh\nprintf '" + loginPathMarker + "%s\\n' '" + bin + "'\n"
		if err := os.WriteFile(shell, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	firstBin := filepath.Join(home, "first-bin")
	secondBin := filepath.Join(home, "second-bin")
	writeShell(firstBin)
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/usr/bin:/bin")
	first := RuntimeEnv("", "/bin/sh")[0]
	writeShell(secondBin)
	second := RuntimeEnv("", "/bin/sh")[0]
	if !strings.Contains(first, firstBin) || !strings.Contains(second, firstBin) || strings.Contains(second, secondBin) {
		t.Fatalf("login PATH cache was not reused: first=%q second=%q", first, second)
	}
}

func TestRuntimeEnvFallsBackWhenLoginShellOutputExceedsLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell discovery is for Unix GUI services")
	}
	home := t.TempDir()
	shell := filepath.Join(home, "zsh")
	script := "#!/bin/sh\ni=0; while [ $i -lt 2000 ]; do printf 'startup chatter that fills the output buffer\\n'; i=$((i+1)); done\nprintf '" + loginPathMarker + "/should-not-be-read\\n'\n"
	if err := os.WriteFile(shell, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/service/bin")
	env := RuntimeEnv("", "/bin/sh")
	if !strings.Contains(env[0], "/service/bin") || strings.Contains(env[0], "/should-not-be-read") {
		t.Fatalf("oversized login-shell output did not fall back safely: %q", env)
	}
}

func TestRuntimeEnvBoundsLoginShellTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("login shell discovery is for Unix GUI services")
	}
	home := t.TempDir()
	shell := filepath.Join(home, "zsh")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n/bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/service/bin")
	started := time.Now()
	env := RuntimeEnv("", "/bin/sh")
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("login shell discovery exceeded its timeout: %v", elapsed)
	}
	if len(env) != 1 || !strings.Contains(env[0], "/service/bin") {
		t.Fatalf("timed-out login shell did not fall back to service PATH: %q", env)
	}
}
