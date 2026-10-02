package googleclient

import "testing"

func TestAClientNeedsBothPartsToBeValid(t *testing.T) {
	for _, tc := range []struct {
		client Client
		want   bool
	}{
		{Client{ID: "id", Secret: "secret"}, true},
		{Client{ID: "id"}, false},
		{Client{Secret: "secret"}, false},
		{Client{ID: " ", Secret: "secret"}, false},
		{Client{}, false},
	} {
		if got := tc.client.Valid(); got != tc.want {
			t.Errorf("%+v valid = %v, want %v", tc.client, got, tc.want)
		}
	}
}

// A checkout has no client.json (it is not in git), so a build from one has no Google client of its
// own, and Settings asks the person for theirs. A machine that has the file will see it here instead.
func TestBundledIsEmptyOrCompleteNeverHalf(t *testing.T) {
	got := Bundled()
	if got != (Client{}) && !got.Valid() {
		t.Errorf("Bundled() = %+v, want nothing or a whole client", got)
	}
}
