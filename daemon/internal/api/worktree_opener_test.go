package api_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/api"
)

// The system opener decides which program to start and with what arguments. Its Run is a fake, so
// no test starts one.

type ran struct {
	calls []string
	fail  map[string]bool
}

func (r *ran) run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if r.fail[name] {
		return errors.New("failed")
	}
	return nil
}

func installed(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, have := range names {
			if have == name {
				return "/usr/local/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestTheSystemOpenerRevealsAFolderInTheFileManager(t *testing.T) {
	for os, want := range map[string]string{"darwin": "open -R /work/card", "linux": "xdg-open /work/card"} {
		fake := &ran{}
		opener := api.SystemOpener{Run: fake.run, LookPath: installed(), OS: os}
		if err := opener.Open(context.Background(), "/work/card", "finder"); err != nil {
			t.Fatalf("%s: %v", os, err)
		}
		if len(fake.calls) != 1 || fake.calls[0] != want {
			t.Errorf("%s ran %v, want %q", os, fake.calls, want)
		}
	}
}

func TestTheSystemOpenerPrefersTheEditorsCommand(t *testing.T) {
	fake := &ran{}
	opener := api.SystemOpener{Run: fake.run, LookPath: installed("code"), OS: "darwin"}
	if err := opener.Open(context.Background(), "/work/card", "editor"); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "code /work/card" {
		t.Errorf("ran %v, want only the editor", fake.calls)
	}
}

func TestTheSystemOpenerFallsBackToTheFileManagerWithoutAnEditor(t *testing.T) {
	missing := &ran{}
	opener := api.SystemOpener{Run: missing.run, LookPath: installed(), OS: "darwin"}
	if err := opener.Open(context.Background(), "/work/card", "editor"); err != nil {
		t.Fatal(err)
	}
	if len(missing.calls) != 1 || missing.calls[0] != "open /work/card" {
		t.Errorf("with no editor ran %v, want the file manager", missing.calls)
	}

	broken := &ran{fail: map[string]bool{"code": true}}
	opener = api.SystemOpener{Run: broken.run, LookPath: installed("code"), OS: "darwin"}
	if err := opener.Open(context.Background(), "/work/card", "editor"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"code /work/card", "open /work/card"}; strings.Join(broken.calls, ";") != strings.Join(want, ";") {
		t.Errorf("with an editor that fails ran %v, want %v", broken.calls, want)
	}
}

func TestTheSystemOpenerReportsAProgramThatFailsAndRefusesOtherChoices(t *testing.T) {
	fake := &ran{fail: map[string]bool{"open": true}}
	opener := api.SystemOpener{Run: fake.run, LookPath: installed(), OS: "darwin"}
	if err := opener.Open(context.Background(), "/work/card", "finder"); err == nil {
		t.Error("a file manager that failed was not reported")
	}
	fake.calls = nil
	if err := opener.Open(context.Background(), "/work/card", "rm -rf"); err == nil || len(fake.calls) != 0 {
		t.Errorf("another choice ran %v with error %v", fake.calls, err)
	}
}
