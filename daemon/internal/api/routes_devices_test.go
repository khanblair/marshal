package api_test

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/devices"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// codeShape is how a pairing code reads: three characters, a dash, three more, none of the letters
// a person mistakes for a digit.
var codeShape = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{3}-[0-9A-HJKMNP-TV-Z]{3}$`)

// pair asks the pairing route to exchange a code for a device token, with no token of its own.
func (st *stack) pair(req protocol.PairDeviceRequest) reply {
	st.t.Helper()
	return st.doWith("", http.MethodPost, "/v1/devices/pair", req)
}

// issueCode asks for a pairing code the way the desktop app's Pair a device button does.
func (st *stack) issueCode() protocol.PairingCode {
	st.t.Helper()
	answer := st.do(http.MethodPost, "/v1/me/devices/pairing-code", nil).want(st.t, http.StatusOK)
	return decode[protocol.PairingCode](st.t, answer)
}

// The flow of a phone pairing: the desktop app reads a code off its screen, the phone types it in
// and gets a token, and from then on that token is the phone's. The code is spent by the pairing,
// so it cannot be read off the screen twice.
func TestAPairingCodePairsADevice(t *testing.T) {
	st := newStack(t)
	code := st.issueCode()
	if !codeShape.MatchString(code.Code) {
		t.Fatalf("code = %q, want three characters, a dash, and three more", code.Code)
	}
	if !code.ExpiresAt.Time().After(code.ServerTime.Time()) {
		t.Errorf("expiresAt %v is not after serverTime %v", code.ExpiresAt.Time(), code.ServerTime.Time())
	}
	if got := code.ExpiresAt.Time().Sub(code.ServerTime.Time()); got != devices.CodeTTL {
		t.Errorf("a code lives for %v, want %v", got, devices.CodeTTL)
	}

	paired := decode[protocol.PairDeviceResponse](t,
		st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "Blair's iPhone", Kind: protocol.DeviceKindMobile}).
			want(t, http.StatusOK))
	if paired.Token == "" {
		t.Fatal("the answer carries no token, so the device cannot sign in")
	}
	if paired.Device.Kind != protocol.DeviceKindMobile || paired.Device.Name != "Blair's iPhone" {
		t.Errorf("device = %+v, want the kind and name that were asked for", paired.Device)
	}
	if paired.Device.Revoked {
		t.Error("a device that was just paired is not revoked")
	}

	// The token it was given is the device's own, and it signs in as a mobile device.
	me := decode[protocol.WhoAmI](t,
		st.doWith(paired.Token, http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK))
	if me.DeviceKind != protocol.DeviceKindMobile || me.DeviceID != paired.Device.ID {
		t.Errorf("whoami = %+v, want device %s of kind mobile", me, paired.Device.ID)
	}

	// The paired device is in the list, beside the ones the daemon made for itself.
	list := decode[protocol.DeviceList](t, st.do(http.MethodGet, "/v1/me/devices", nil).want(t, http.StatusOK))
	if len(list.Devices) < 2 {
		t.Fatalf("devices = %+v, want at least the paired one and one the daemon made", list.Devices)
	}
	found := false
	for _, device := range list.Devices {
		if device.ID == paired.Device.ID {
			found = true
			if device.Revoked {
				t.Error("the paired device is listed as revoked")
			}
		}
	}
	if !found {
		t.Errorf("the paired device is not in the list: %+v", list.Devices)
	}

	// The code is spent: a second attempt with the same code is refused.
	st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "Someone else", Kind: protocol.DeviceKindMobile}).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

// A code that was never issued, a code with a letter that cannot be in one, and a code typed in
// any spacing all get the same refusal: nothing about which codes are live is told apart.
func TestAPairingCodeThatIsNotRightIsRefused(t *testing.T) {
	st := newStack(t)
	for _, code := range []string{"", "ABC-DEF", "ZZZ-ZZZ", "7QX-2L", "7QX-2LDA", "7qx-2ld", "!!!"} {
		t.Run("code "+code, func(t *testing.T) {
			st.pair(protocol.PairDeviceRequest{Code: code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
				apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
		})
	}
}

// A code stops working when its five minutes are up, measured on the daemon's own clock rather
// than by waiting.
func TestAPairingCodeExpires(t *testing.T) {
	st := newStack(t)
	code := st.issueCode()
	st.advance(devices.CodeTTL)
	st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
}

// Guessing at a code does not stay open: after five misses the code itself is thrown away, so
// the guesser gains nothing and the person is asked to make a new one.
func TestAPairingCodeStopsWorkingAfterTooManyGuesses(t *testing.T) {
	st := newStack(t)
	code := st.issueCode()
	for range 5 {
		st.pair(protocol.PairDeviceRequest{Code: "ZZZ-ZZZ", Name: "A phone", Kind: protocol.DeviceKindMobile}).
			apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	}
	// The code the person was shown no longer works either: it was guessed at.
	st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	// A new code pairs at once, so the way out is one button.
	fresh := st.issueCode()
	st.pair(protocol.PairDeviceRequest{Code: fresh.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
		want(t, http.StatusOK)
}

// Pairing is asked for with a device kind the daemon owns: the owner token and the dev token are
// written by the daemon itself and cannot be asked for by a phone.
func TestAPairingRefusesTheKindsTheDaemonOwns(t *testing.T) {
	st := newStack(t)
	for _, kind := range []protocol.DeviceKind{
		protocol.DeviceKindCLI, protocol.DeviceKindDev, protocol.DeviceKindDesktop, "smartwatch", "",
	} {
		t.Run(string(kind), func(t *testing.T) {
			code := st.issueCode()
			got := st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "A phone", Kind: kind})
			if got.Status != http.StatusBadRequest {
				t.Fatalf("status = %d for kind %q, want 400; body: %s", got.Status, kind, got.Body)
			}
			if err := decode[protocol.ErrorResponse](t, got).Error; err.Code != protocol.ErrorCodeInvalidArgument {
				t.Errorf("code = %q, want invalid_argument", err.Code)
			}
		})
	}
}

// An empty or absurdly long name is refused before a device row is made, with a sentence that says
// what to type instead.
func TestAPairingChecksTheDeviceName(t *testing.T) {
	st := newStack(t)
	for _, name := range []string{"   ", string(make([]byte, 65))} {
		code := st.issueCode()
		got := st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: name, Kind: protocol.DeviceKindMobile})
		if got.Status != http.StatusBadRequest {
			t.Fatalf("status = %d for name %q, want 400; body: %s", got.Status, name, got.Body)
		}
	}
}

// A revoked device loses access, and the paired device that made it happen is still signed in.
// The daemon's token cache is what makes revocation take a few seconds rather than one request
// (internal/api/auth.go), which is the trade that cache exists for.
func TestAPairedDeviceLosesAccessWhenItIsRevoked(t *testing.T) {
	st := newStack(t)
	code := st.issueCode()
	paired := decode[protocol.PairDeviceResponse](t,
		st.pair(protocol.PairDeviceRequest{Code: code.Code, Name: "Blair's iPhone", Kind: protocol.DeviceKindMobile}).
			want(t, http.StatusOK))
	st.doWith(paired.Token, http.MethodGet, "/v1/auth/whoami", nil).want(t, http.StatusOK)

	st.do(http.MethodDelete, "/v1/me/devices/"+paired.Device.ID, nil).want(t, http.StatusNoContent)

	// The cache entry the paired device made has to age out before the store is asked again. The
	// window is internal/api/auth.go's own authCacheTTL, which is deliberately a few seconds.
	st.advance(10 * time.Second)
	st.doWith(paired.Token, http.MethodGet, "/v1/auth/whoami", nil).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)

	// The owner's own token is untouched by it.
	st.do(http.MethodGet, "/v1/me/devices", nil).want(t, http.StatusOK)
	// And the revoked device is still in the list, so the screen can say it was removed.
	list := decode[protocol.DeviceList](t, st.do(http.MethodGet, "/v1/me/devices", nil).want(t, http.StatusOK))
	for _, device := range list.Devices {
		if device.ID == paired.Device.ID && !device.Revoked {
			t.Error("the revoked device is listed as still paired")
		}
	}
}

// Revoking a device that is not this person's, or one that does not exist, is not found: whether
// it exists is not this person's business.
func TestARevokedDeviceThatIsNotYoursIsNotFound(t *testing.T) {
	st := newStack(t)
	for _, id := range []string{sampleCardID, "not-an-id"} {
		st.do(http.MethodDelete, "/v1/me/devices/"+id, nil).
			apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	}
}

// With no devices service no device address exists at all, and no device can be paired.
func TestNoDeviceRoutesWithoutTheService(t *testing.T) {
	st := newStack(t, withoutDevices())
	st.do(http.MethodGet, "/v1/me/devices", nil).want(t, http.StatusNotFound)
	st.do(http.MethodPost, "/v1/me/devices/pairing-code", nil).want(t, http.StatusNotFound)
	st.doWith("", http.MethodPost, "/v1/devices/pair",
		protocol.PairDeviceRequest{Code: "ABC-DEF", Name: "A phone", Kind: protocol.DeviceKindMobile}).
		want(t, http.StatusNotFound)
}

// The pairing route takes no token, which is the whole point: a device being paired has none yet.
// What authorizes it is the code.
func TestThePairingRouteNeedsNoToken(t *testing.T) {
	st := newStack(t)
	if st.doWith("", http.MethodPost, "/v1/me/devices/pairing-code", nil).Status != http.StatusUnauthorized {
		t.Fatal("asking for a code without a token should be refused: a code is only made for a device already signed in")
	}
	code := st.issueCode()
	req := protocol.PairDeviceRequest{Code: code.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}
	if r := st.pair(req); r.Status != http.StatusOK {
		t.Fatalf("pairing without a token = %d, want 200; body: %s", r.Status, r.Body)
	}
}

// Pairing is one code at a time: a second code replaces the first, so the screen always shows the
// one that works and an old screenshot of a code is dead.
func TestASecondPairingCodeReplacesTheFirst(t *testing.T) {
	st := newStack(t)
	first := st.issueCode()
	second := st.issueCode()
	st.pair(protocol.PairDeviceRequest{Code: first.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
		apiError(t, http.StatusUnauthorized, protocol.ErrorCodeUnauthorized)
	st.pair(protocol.PairDeviceRequest{Code: second.Code, Name: "A phone", Kind: protocol.DeviceKindMobile}).
		want(t, http.StatusOK)
}

// Pairing is a device signing in, not a request that reads someone's data: the pairing route
// answers before any bearer token is accepted from it.
func TestAPairingRefusesABadBody(t *testing.T) {
	st := newStack(t)
	st.doWith("", http.MethodPost, "/v1/devices/pair", "{not json").
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}
