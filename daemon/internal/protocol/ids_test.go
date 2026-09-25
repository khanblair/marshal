package protocol_test

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestValidProjectID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"api", true},
		{"web-dashboard", true},
		{"a1", true},
		{"ab", true},
		{"a-", true},
		{"project-42", true},
		{strings.Repeat("a", 24), true},
		{"", false},
		{"a", false},
		{strings.Repeat("a", 25), false},
		{"1api", false},
		{"-api", false},
		{"Api", false},
		{"my_project", false},
		{"my project", false},
		{"café", false},
		{"a#1", false},
	}
	for _, tc := range tests {
		if got := protocol.ValidProjectID(tc.id); got != tc.want {
			t.Errorf("ValidProjectID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestSlugFromName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "API", "api"},
		{"spaces become one hyphen", "Web  Dashboard", "web-dashboard"},
		{"punctuation runs collapse", "my_project!!v2", "my-project-v2"},
		{"edges are trimmed", "  --Mobile App--  ", "mobile-app"},
		{"accents are replaced", "Café Menu", "caf-menu"},
		{"cut to 24", "the quick brown fox jumps over the lazy dog", "the-quick-brown-fox-jump"},
		{"a cut that ends in a hyphen is trimmed again", "aaaaaaaaaaaaaaaaaaaaaaa bbb", "aaaaaaaaaaaaaaaaaaaaaaa"},
		{"empty", "", "project"},
		{"only symbols", "!!!", "project"},
		{"starts with a digit", "3d viewer", "project"},
		{"one letter is too short", "x", "project"},
		{"every result is valid", "ééé", "project"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := protocol.SlugFromName(tc.in)
			if got != tc.want {
				t.Errorf("SlugFromName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !protocol.ValidProjectID(got) {
				t.Errorf("SlugFromName(%q) = %q, which is not a valid project id", tc.in, got)
			}
		})
	}
}

func TestNumberedProjectID(t *testing.T) {
	long := strings.Repeat("a", 24)
	tests := []struct {
		id   string
		n    int
		want string
	}{
		{"api", 1, "api"},
		{"api", 2, "api-2"},
		{"api", 13, "api-13"},
		{long, 2, strings.Repeat("a", 22) + "-2"},
		{long, 100, strings.Repeat("a", 20) + "-100"},
		{"aaaaaaaaaaaaaaaaaaaaaa-b", 2, "aaaaaaaaaaaaaaaaaaaaaa-2"},
		{"aaaaaaaaaaaaaaaaaaaaa-bc", 10, "aaaaaaaaaaaaaaaaaaaaa-10"},
	}
	for _, tc := range tests {
		got := protocol.NumberedProjectID(tc.id, tc.n)
		if got != tc.want || !protocol.ValidProjectID(got) {
			t.Errorf("NumberedProjectID(%q, %d) = %q, want %q (valid id)", tc.id, tc.n, got, tc.want)
		}
	}
}

func TestCardKeyString(t *testing.T) {
	tests := []struct {
		key  protocol.CardKey
		want string
	}{
		{protocol.CardKey{ProjectID: "api", Number: 1}, "api#1"},
		{protocol.CardKey{ProjectID: "web-dashboard", Number: 12}, "web-dashboard#12"},
		{protocol.CardKey{ProjectID: "app-2-go", Number: 4096}, "app-2-go#4096"},
	}
	for _, tc := range tests {
		if got := tc.key.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestParseCardKey(t *testing.T) {
	good := []protocol.CardKey{
		{ProjectID: "api", Number: 1},
		{ProjectID: "web-dashboard", Number: 12},
		{ProjectID: "app-2-go", Number: 4096},
		{ProjectID: "ab", Number: 2147483647},
	}
	for _, key := range good {
		got, err := protocol.ParseCardKey(key.String())
		if err != nil || got != key {
			t.Errorf("ParseCardKey(%q) = %+v, %v; want %+v", key.String(), got, err, key)
		}
	}
	bad := []string{
		"", "api", "api#", "#12", "api#0", "api#012", "api#-1", "api#+1", "api#1.5", "api#x",
		"API#1", "1api#1", "api#1#2", "api #1", "api#2147483648", "api#99999999999999999999",
	}
	for _, in := range bad {
		if got, err := protocol.ParseCardKey(in); err == nil {
			t.Errorf("ParseCardKey(%q) = %+v, want an error", in, got)
		}
	}
}

func TestNewID(t *testing.T) {
	ff := bytes.Repeat([]byte{0xFF}, 10)
	counting := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	tests := []struct {
		name    string
		at      time.Time
		entropy []byte
		want    string
	}{
		{"time zero and no randomness", time.UnixMilli(0), make([]byte, 10), strings.Repeat("0", 26)},
		{"one millisecond", time.UnixMilli(1), make([]byte, 10), "0000000001" + strings.Repeat("0", 16)},
		{"32 milliseconds carries into the next character", time.UnixMilli(32), make([]byte, 10), "0000000010" + strings.Repeat("0", 16)},
		{"the largest time", time.UnixMilli(1<<48 - 1), make([]byte, 10), "7ZZZZZZZZZ" + strings.Repeat("0", 16)},
		{"all randomness bits set", time.UnixMilli(0), ff, strings.Repeat("0", 10) + strings.Repeat("Z", 16)},
		{"a real moment", time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC), counting, "01M3C107JB041061050R3GG28A"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := protocol.NewID(tc.at, bytes.NewReader(tc.entropy))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
			if !protocol.ValidID(got) {
				t.Errorf("%s is not a valid id", got)
			}
		})
	}
}

func TestNewIDSortsByTime(t *testing.T) {
	base := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	var ids []string
	for i, entropy := range [][]byte{bytes.Repeat([]byte{0xFF}, 10), make([]byte, 10), bytes.Repeat([]byte{0x80}, 10)} {
		id, err := protocol.NewID(base.Add(time.Duration(i)*time.Millisecond), bytes.NewReader(entropy))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("ids made in time order do not sort in time order: %v", ids)
	}
}

func TestNewIDRefusesWhatItCannotEncode(t *testing.T) {
	zeros := bytes.NewReader(make([]byte, 10))
	if _, err := protocol.NewID(time.UnixMilli(-1), zeros); err == nil {
		t.Error("a time before 1970 made an id, want an error")
	}
	if _, err := protocol.NewID(time.UnixMilli(1<<48), zeros); err == nil {
		t.Error("a time past the 48-bit range made an id, want an error")
	}
	boom := errors.New("no randomness")
	if _, err := protocol.NewID(time.UnixMilli(0), iotest.ErrReader(boom)); !errors.Is(err, boom) {
		t.Errorf("a failing reader gave %v, want it to wrap the reader's error", err)
	}
	if _, err := protocol.NewID(time.UnixMilli(0), bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Error("a short reader made an id, want an error")
	}
}

func TestValidID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"01M3C107JB041061050R3GG28A", true},
		{strings.Repeat("0", 26), true},
		{"7ZZZZZZZZZ" + strings.Repeat("Z", 16), true},
		{"", false},
		{strings.Repeat("0", 25), false},
		{strings.Repeat("0", 27), false},
		{"8" + strings.Repeat("0", 25), false},
		{strings.Repeat("0", 25) + "I", false},
		{strings.Repeat("0", 25) + "U", false},
		{strings.Repeat("0", 25) + "a", false},
	}
	for _, tc := range tests {
		if got := protocol.ValidID(tc.id); got != tc.want {
			t.Errorf("ValidID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}
