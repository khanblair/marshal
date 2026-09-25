package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestDecodeJSONReadsAValidBody(t *testing.T) {
	s, _ := newTestServer()
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"a"}`))
	var dst struct {
		Name string `json:"name"`
	}
	if err := s.decodeJSON(r, &dst); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
	if dst.Name != "a" {
		t.Errorf("Name = %q, want %q", dst.Name, "a")
	}
}

func TestDecodeJSONRejections(t *testing.T) {
	s, _ := newTestServer()
	tests := []struct {
		name          string
		body          string
		wantInMessage string
	}{
		{"empty body", "", "empty"},
		{"not JSON", "not json", "valid JSON"},
		{"unknown field", `{"nope":1}`, `"nope"`},
		{"wrong type", `{"name":1}`, `"name"`},
		{"more than one value", `{"name":"a"}{"name":"b"}`, "single JSON object"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.body))
			var dst struct {
				Name string `json:"name"`
			}
			perr := decodeErrOf(t, s.decodeJSON(r, &dst))
			if perr.Code != protocol.ErrorCodeInvalidArgument {
				t.Errorf("code = %q, want %q", perr.Code, protocol.ErrorCodeInvalidArgument)
			}
			if !strings.Contains(perr.Message, tc.wantInMessage) {
				t.Errorf("message = %q, want it to mention %q", perr.Message, tc.wantInMessage)
			}
		})
	}
}

func TestDecodeJSONEnforcesTheSizeLimit(t *testing.T) {
	s, _ := newTestServer()
	s.limits.MaxBodyBytes = 4
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"a"}`))
	var dst struct {
		Name string `json:"name"`
	}
	perr := decodeErrOf(t, s.decodeJSON(r, &dst))
	if !strings.Contains(perr.Message, "larger than Marshal accepts") {
		t.Errorf("message = %q, want it to mention the size limit", perr.Message)
	}
}

func decodeErrOf(t *testing.T, err error) *protocol.Error {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error is not a *protocol.Error: %v", err)
	}
	return perr
}
