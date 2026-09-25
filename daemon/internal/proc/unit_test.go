package proc

import (
	"slices"
	"testing"
)

func TestBuildEnv(t *testing.T) {
	tests := []struct {
		name  string
		base  []string
		extra []string
		want  []string
	}{
		{
			name: "keeps only the allowed names",
			base: []string{"PATH=/bin", "SECRET=x", "HOME=/h", "MARSHAL_TOKEN=y", "TMPDIR=/t"},
			want: []string{"PATH=/bin", "HOME=/h", "TMPDIR=/t"},
		},
		{
			name:  "extra entries are added and win",
			base:  []string{"PATH=/bin", "HOME=/h"},
			extra: []string{"HOME=/other", "MARSHAL_X=1"},
			want:  []string{"PATH=/bin", "HOME=/other", "MARSHAL_X=1"},
		},
		{
			name:  "a later extra entry wins over an earlier one",
			extra: []string{"A=1", "A=2"},
			want:  []string{"A=2"},
		},
		{
			name:  "entries without a name or an equals sign are ignored",
			base:  []string{"PATH", "=x"},
			extra: []string{"NOEQUALS", "=v"},
			want:  []string{},
		},
		{
			name: "a value may hold an equals sign",
			base: []string{"PATH=/a=b"},
			want: []string{"PATH=/a=b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEnv(tt.base, tt.extra)
			if !slices.Equal(got, tt.want) {
				t.Errorf("buildEnv = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTailBuffer(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"shorter than the limit", 10, []string{"abc"}, "abc"},
		{"grows across writes", 10, []string{"abc", "def"}, "abcdef"},
		{"drops the oldest bytes", 5, []string{"abc", "def"}, "bcdef"},
		{"a single big write keeps its end", 4, []string{"0123456789"}, "6789"},
		{"a write of exactly the limit", 4, []string{"xx", "abcd"}, "abcd"},
		{"a split character at the start is dropped", 3, []string{"aébc"}, "bc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tail := &tailBuffer{limit: tt.limit}
			for _, w := range tt.writes {
				n, err := tail.Write([]byte(w))
				if err != nil || n != len(w) {
					t.Fatalf("Write(%q) = %d, %v, want %d and no error", w, n, err, len(w))
				}
			}
			if got := tail.String(); got != tt.want {
				t.Errorf("tail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllowedNamesForWindowsAgents(t *testing.T) {
	// Node-based agents need these on Windows to find a shell and the programs on PATH.
	for _, name := range []string{
		"COMSPEC", "PATHEXT", "WINDIR", "PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA",
	} {
		if !allowedName(name) {
			t.Errorf("allowedName(%q) = false, want true", name)
		}
	}
	// The usual spellings are only the same name where names are case insensitive.
	for _, name := range []string{"ComSpec", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
		if got := allowedName(name); got != foldEnvNames {
			t.Errorf("allowedName(%q) = %v, want %v", name, got, foldEnvNames)
		}
	}
	// Names that only look like them stay out.
	for _, name := range []string{"COMSPEC2", "PROGRAMFILES(X64)", "PROGRAMW6432", "USERNAME"} {
		if allowedName(name) {
			t.Errorf("allowedName(%q) = true, want false", name)
		}
	}
}

func TestBuildEnvPassesTheWindowsNames(t *testing.T) {
	base := []string{"COMSPEC=C:\\Windows\\System32\\cmd.exe", "PATHEXT=.COM;.EXE", "SECRET=x", "PROGRAMDATA=C:\\ProgramData"}
	want := []string{"COMSPEC=C:\\Windows\\System32\\cmd.exe", "PATHEXT=.COM;.EXE", "PROGRAMDATA=C:\\ProgramData"}
	if got := buildEnv(base, nil); !slices.Equal(got, want) {
		t.Errorf("buildEnv = %q, want %q", got, want)
	}
}

func TestSameNameAndAllowed(t *testing.T) {
	if !allowedName("PATH") || allowedName("MARSHAL_TOKEN") || allowedName("") {
		t.Error("allowedName gave the wrong answer for PATH, MARSHAL_TOKEN, or an empty name")
	}
	if !sameName("A", "A") || sameName("A", "B") {
		t.Error("sameName compares wrongly")
	}
	if sameName("a", "A") != foldEnvNames {
		t.Error("sameName does not follow foldEnvNames")
	}
}
