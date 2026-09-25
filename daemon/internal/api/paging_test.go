package api_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/api"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantCursor string
		wantErr    bool
	}{
		{"nothing given", "", 50, "", false},
		{"a limit", "limit=10", 10, "", false},
		{"the largest limit", "limit=200", 200, "", false},
		{"a limit over the maximum is cut", "limit=5000", 200, "", false},
		{"a cursor", "cursor=eyJuIjoxM30", 50, "eyJuIjoxM30", false},
		{"both", "limit=25&cursor=abc", 25, "abc", false},
		{"not a number", "limit=many", 0, "", true},
		{"a decimal", "limit=1.5", 0, "", true},
		{"zero", "limit=0", 0, "", true},
		{"negative", "limit=-3", 0, "", true},
		{"a huge number", "limit=99999999999999999999", 0, "", true},
		{"a huge cursor", "cursor=" + strings.Repeat("a", 5000), 0, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/x?"+tc.query, nil)
			limit, cursor, err := api.ParsePage(req)
			if tc.wantErr {
				assertInvalidArgument(t, err)
				return
			}
			if err != nil || limit != tc.wantLimit || cursor != tc.wantCursor {
				t.Errorf("ParsePage = %d, %q, %v; want %d, %q", limit, cursor, err, tc.wantLimit, tc.wantCursor)
			}
		})
	}
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("err = %v, want an invalid_argument error", err)
	}
}

type position struct {
	Created string `json:"c"`
	ID      string `json:"id"`
}

func TestCursorRoundTrips(t *testing.T) {
	in := position{Created: "2026-09-25T10:15:30.123Z", ID: "01M3C107JB041061050R3GG28A"}
	cursor, err := api.EncodeCursor(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(cursor, "+/=") {
		t.Errorf("cursor %q is not URL-safe base64 without padding", cursor)
	}
	out, found, err := api.DecodeCursor[position](cursor)
	if err != nil || !found || out != in {
		t.Errorf("DecodeCursor = %+v, %v, %v; want %+v", out, found, err, in)
	}
}

func TestEmptyCursorMeansTheFirstPage(t *testing.T) {
	out, found, err := api.DecodeCursor[position]("")
	if err != nil || found || out != (position{}) {
		t.Errorf("DecodeCursor(\"\") = %+v, %v, %v; want the zero value, false, nil", out, found, err)
	}
}

func TestBadCursorsAreInvalidArguments(t *testing.T) {
	for _, cursor := range []string{"!!!", "not base64 at all", "bm90IGpzb24", strings.Repeat("A", 2000)} {
		_, found, err := api.DecodeCursor[position](cursor)
		if found {
			t.Errorf("DecodeCursor(%q) reported a position", cursor)
		}
		assertInvalidArgument(t, err)
	}
}

func TestEncodeCursorReportsWhatItCannotEncode(t *testing.T) {
	if _, err := api.EncodeCursor(make(chan int)); err == nil {
		t.Error("EncodeCursor of a channel returned no error")
	}
}
