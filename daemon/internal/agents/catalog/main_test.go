package catalog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// The fake agent programs of these tests are this test binary under another name. When it is
// started as "claude", "gemini", or "codex" with the argument --version, it does what the text
// file beside it says (see runFakeProgram), and otherwise it is the test binary as usual.
func TestMain(m *testing.M) {
	runFakeProgram()
	code := m.Run()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// fakeNames are the program names that the fakes answer to.
var fakeNames = map[string]bool{"claude": true, "gemini": true, "codex": true}

// runFakeProgram plays a fake program if this process was started as one, and exits. The file
// "<program>.txt" beside the program says what to do: "hang" never answers, "exit N" prints to
// standard error and exits with N, "spam" prints far more than a version, and anything else is
// printed as it is.
func runFakeProgram() {
	exe, err := os.Executable()
	if err != nil || len(os.Args) != 2 || os.Args[1] != "--version" {
		return
	}
	if !fakeNames[strings.TrimSuffix(filepath.Base(exe), ".exe")] {
		return
	}
	script, err := os.ReadFile(exe + ".txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no script beside the fake program")
		os.Exit(2)
	}
	text := strings.TrimSpace(string(script))
	switch {
	case text == "hang":
		time.Sleep(time.Minute)
	case strings.HasPrefix(text, "exit "):
		fmt.Fprintln(os.Stderr, "boom")
		code, _ := strconv.Atoi(strings.TrimPrefix(text, "exit "))
		os.Exit(code)
	case text == "spam":
		fmt.Print(strings.Repeat("x", 1<<20))
	default:
		fmt.Println(string(script))
	}
	os.Exit(0)
}

// installFake puts a fake program of the given name in dir, with the text that it will answer to
// --version, and returns its path. The program is a link to the test binary, or a copy of it where
// links are not possible.
func installFake(t *testing.T, dir, name, answer string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, name)
	if err := os.Link(src, dst); err != nil {
		copyFile(t, src, dst)
	}
	if err := os.WriteFile(dst+".txt", []byte(answer), 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

// copyFile copies an executable.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
