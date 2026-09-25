package gitx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestParseVersion(t *testing.T) {
	tests := map[string]gitx.Version{
		"git version 2.53.0":                 {Major: 2, Minor: 53},
		"git version 2.39.5 (Apple Git-154)": {Major: 2, Minor: 39},
		"git version 2.38.0.windows.1":       {Major: 2, Minor: 38},
	}
	for output, want := range tests {
		got, err := gitx.ParseVersion(output)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", output, got, err, want)
		}
	}
	if _, err := gitx.ParseVersion("not git"); err == nil {
		t.Error("ParseVersion accepted text with no version")
	}
}

func TestAtLeast(t *testing.T) {
	min := gitx.MinVersion()
	tests := []struct {
		v    gitx.Version
		want bool
	}{
		{gitx.Version{Major: 2, Minor: 37}, false},
		{gitx.Version{Major: 2, Minor: 38}, true},
		{gitx.Version{Major: 2, Minor: 53}, true},
		{gitx.Version{Major: 3, Minor: 0}, true},
		{gitx.Version{Major: 1, Minor: 99}, false},
	}
	for _, tc := range tests {
		if got := tc.v.AtLeast(min); got != tc.want {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", tc.v, min, got, tc.want)
		}
	}
}

func TestInstalledGitIsNewEnough(t *testing.T) {
	if err := gitx.New().CheckVersion(context.Background()); err != nil {
		t.Fatalf("CheckVersion: %v", err)
	}
}

func TestRunReportsFailuresWithGitsOwnMessage(t *testing.T) {
	_, err := gitx.New().Run(context.Background(), t.TempDir(), "rev-parse", "--is-inside-work-tree")
	var gitErr *gitx.Error
	if !errors.As(err, &gitErr) {
		t.Fatalf("error = %v, want a *gitx.Error", err)
	}
	if gitErr.Stderr == "" {
		t.Error("the error carries no message from Git")
	}
}
