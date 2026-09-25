package catalog

import "testing"

func TestParseSemver(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{"claude code", "2.1.282 (Claude Code)", "2.1.282", true},
		{"gemini", "0.35.1", "0.35.1", true},
		{"with a newline", "0.35.1\n", "0.35.1", true},
		{"a word before it", "codex-cli 0.42.0", "0.42.0", true},
		{"a leading v", "v1.2.3", "1.2.3", true},
		{"in brackets", "tool (1.2.3)", "1.2.3", true},
		{"a pre-release", "0.36.0-preview.1", "0.36.0-preview.1", true},
		{"a build tag is dropped", "1.2.3+build5", "1.2.3", true},
		{"the first of several lines", "warning: old\n3.4.5\n6.7.8", "3.4.5", true},
		{"no version", "hello world", "", false},
		{"two numbers", "1.2", "", false},
		{"four numbers", "1.2.3.4", "", false},
		{"not a number", "1.x.3", "", false},
		{"empty parts", "1..3", "", false},
		{"too many digits", "1.2.1234567890", "", false},
		{"empty", "", "", false},
		{"a path is not a version", "/usr/lib/1.2.3/bin", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseSemver(tt.text)
			if ok != tt.ok || (ok && got.String() != tt.want) {
				t.Errorf("ParseSemver(%q) = %q, %v, want %q, %v", tt.text, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSemverAtLeast(t *testing.T) {
	v := Semver{Major: 2, Minor: 1, Patch: 282}
	tests := []struct {
		name                string
		v                   Semver
		major, minor, patch int
		want                bool
	}{
		{"the same version", v, 2, 1, 282, true},
		{"an older patch", v, 2, 1, 259, true},
		{"a newer patch is not reached", v, 2, 1, 283, false},
		{"an older minor", v, 2, 0, 999, true},
		{"a newer minor is not reached", v, 2, 2, 0, false},
		{"an older major", v, 1, 99, 99, true},
		{"a newer major is not reached", v, 3, 0, 0, false},
		{"a pre-release is before its release", Semver{Major: 2, Minor: 1, Patch: 282, Pre: "rc.1"}, 2, 1, 282, false},
		{"a pre-release of a later patch", Semver{Major: 2, Minor: 1, Patch: 283, Pre: "rc.1"}, 2, 1, 282, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.AtLeast(tt.major, tt.minor, tt.patch); got != tt.want {
				t.Errorf("%s.AtLeast(%d, %d, %d) = %v, want %v", tt.v, tt.major, tt.minor, tt.patch, got, tt.want)
			}
		})
	}
}
