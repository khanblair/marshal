package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

var pageNow = time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC)

func TestPageGolden(t *testing.T) {
	items := []protocol.CardKey{{ProjectID: "api", Number: 12}, {ProjectID: "web-dashboard", Number: 3}}
	testutil.Golden(t, "page", protocol.NewPage(items, "eyJuIjoxM30", pageNow))
}

func TestPageNeverEncodesItemsAsNull(t *testing.T) {
	got, err := json.Marshal(protocol.NewPage[string](nil, "", pageNow))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"items":[],"nextCursor":"","serverTime":"2026-09-25T10:15:30.123Z"}`
	if string(got) != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}
