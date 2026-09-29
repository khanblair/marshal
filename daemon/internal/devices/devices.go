// Package devices owns the paired clients of a daemon: the rows the `devices` table holds, the
// short-lived code that pairs a new one, and revoking one (B9.2, build-plan task 9.2).
//
// A device is paired once, from the desktop app, and then signs in with the token it was given.
// The room the daemon listens in is what makes a short code enough: it is on this machine and on
// the person's own tailnet, so the only clients that can reach the pairing route are this machine
// and the person's own devices, and it is never exposed through Funnel (architecture.md section
// 13).
package devices

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// CodeTTL is how long a pairing code works. It is short on purpose: the code is read off one
	// screen and typed into another, and a code that outlives that is only a longer window for
	// someone else to use it.
	CodeTTL = 5 * time.Minute
	// codeLength is how many characters a code has, in two groups of three. Six characters from
	// the 32-character alphabet below are about a billion possibilities, which no one guessing by
	// hand can walk in one code's life.
	codeLength = 6
	// codeGroup is how many characters go before the dash, for reading the code aloud.
	codeGroup = 3
	// maxPairFailures is how many wrong codes the live code absorbs before it is thrown away. A
	// guess that misses does not cost the code its life at once - a person mistypes - but a code
	// that has been guessed at five times stops working at all, so guessing is never an open door
	// and a person who mistypes is asked to make a new one.
	maxPairFailures = 5
	// maxDeviceNameBytes caps what a person may call a device, before the name is shown anywhere.
	maxDeviceNameBytes = 64
)

// codeAlphabet is Crockford base32 without I, L, O, and U: the letters a person reading a code off
// a screen mistakes for 1, 0, or each other are left out, so every code has exactly one reading.
const codeAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Option changes how the service is built.
type Option func(*Service)

// WithClock replaces the clock, so a test can let a code expire without waiting.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithLogger replaces the logger.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) { s.log = log }
}

// Service owns the devices and the one pairing code that is live at a time.
type Service struct {
	store *store.Store
	log   *slog.Logger
	now   func() time.Time

	mu sync.Mutex
	// code is the one live pairing code. A second code replaces the first: the screen shows one
	// code, so there is one code.
	code     string
	expires  time.Time
	failures int
}

// NewService makes the service over a store.
func NewService(st *store.Store, opts ...Option) *Service {
	s := &Service{store: st, log: slog.New(slog.DiscardHandler), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// New is what Create needs beyond the store and the clock, bundled so the function stays inside
// the parameter limit.
type New struct {
	UserID, Name string
	Kind         protocol.DeviceKind
	Token        string
}

// Create makes a device row and stores the hash of its token. It is the one place a device is
// created, so the owner's start-up devices and a paired device cannot drift apart.
func Create(ctx context.Context, st *store.Store, now time.Time, d New) (string, error) {
	id, err := protocol.NewID(now, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("make a device id: %w", err)
	}
	err = st.Write(ctx, func(q *db.Queries) error {
		return q.CreateDevice(ctx, db.CreateDeviceParams{
			ID: id, UserID: d.UserID, Name: d.Name, Kind: string(d.Kind),
			TokenHash: store.HashToken(d.Token), PairedAt: store.Millis(now),
		})
	})
	if err != nil {
		return "", fmt.Errorf("save the %s device: %w", d.Kind, err)
	}
	return id, nil
}

// List answers GET /v1/devices: every device of the person, oldest first, revoked ones included.
func (s *Service) List(ctx context.Context, userID string) (protocol.DeviceList, error) {
	rows, err := s.store.Queries().ListDevices(ctx, userID)
	if err != nil {
		return protocol.DeviceList{}, protocol.Internal().
			WithCause(fmt.Errorf("list devices: %w", err))
	}
	out := make([]protocol.Device, 0, len(rows))
	for _, row := range rows {
		out = append(out, deviceOf(row))
	}
	return protocol.NewDeviceList(out, s.now()), nil
}

// IssueCode answers POST /v1/devices/pairing-code: it makes a code and throws away any code that
// was live before, so the screen always shows the one that works.
func (s *Service) IssueCode() protocol.PairingCode {
	now := s.now()
	expires := now.Add(CodeTTL)
	s.mu.Lock()
	s.code = newCode()
	s.expires = expires
	s.failures = 0
	code := s.code
	s.mu.Unlock()
	return protocol.PairingCode{
		Code:       formatCode(code),
		ExpiresAt:  protocol.NewTimestamp(expires),
		ServerTime: protocol.NewTimestamp(now),
	}
}

// Pair answers POST /v1/devices/pair: it exchanges a live code for a device token. The code is
// spent by a successful pairing, so a code that was read over a shoulder is useless afterwards.
//
// Every failure is the same answer, whatever went wrong: a code that never existed, an expired
// one, and a wrong one are indistinguishable, so nothing about a live code leaks to a guesser.
func (s *Service) Pair(ctx context.Context, req protocol.PairDeviceRequest) (protocol.PairDeviceResponse, error) {
	now := s.now()
	if err := s.spendCode(req.Code, now); err != nil {
		return protocol.PairDeviceResponse{}, err
	}
	name, err := deviceName(req.Name, req.Kind)
	if err != nil {
		return protocol.PairDeviceResponse{}, err
	}
	userID, err := s.ownerID(ctx)
	if err != nil {
		return protocol.PairDeviceResponse{}, err
	}
	token, err := platform.NewToken()
	if err != nil {
		return protocol.PairDeviceResponse{}, err
	}
	id, err := Create(ctx, s.store, now, New{UserID: userID, Name: name, Kind: req.Kind, Token: token})
	if err != nil {
		return protocol.PairDeviceResponse{}, protocol.Internal().WithCause(err)
	}
	s.log.Info("paired a device", "device_id", id, "kind", req.Kind)
	return protocol.PairDeviceResponse{
		Token: token,
		Device: protocol.Device{
			ID: id, Name: name, Kind: req.Kind,
			PairedAt:   protocol.NewTimestamp(now),
			LastSeenAt: nil,
			Revoked:    false,
		},
		ServerTime: protocol.NewTimestamp(now),
	}, nil
}

// Revoke answers DELETE /v1/devices/{id}: the device loses access at once. "At once" is within
// the token cache's own few seconds (internal/api/auth.go), which is the point of that cache
// being short; the database is asked again on the next request after it.
func (s *Service) Revoke(ctx context.Context, userID, deviceID string) error {
	if !protocol.ValidID(deviceID) {
		return protocol.NotFound("device").With("id", deviceID)
	}
	rows, err := s.store.Queries().ListDevices(ctx, userID)
	if err != nil {
		return protocol.Internal().WithCause(fmt.Errorf("list devices: %w", err))
	}
	if !hasDevice(rows, deviceID) {
		// A device of someone else is not found rather than forbidden: whether it exists is not
		// this person's business.
		return protocol.NotFound("device").With("id", deviceID)
	}
	var affected int64
	err = s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		affected, err = q.RevokeDevice(ctx, db.RevokeDeviceParams{
			RevokedAt: store.Millis(s.now()), ID: deviceID,
		})
		return err
	})
	if err != nil {
		return protocol.Internal().WithCause(fmt.Errorf("revoke a device: %w", err))
	}
	if affected == 0 {
		return protocol.NotFound("device").With("id", deviceID)
	}
	s.log.Info("revoked a device", "device_id", deviceID)
	return nil
}

// spendCode checks a code and spends it on success. Every refusal is Unauthorized, whether the
// code was wrong, gone, or past its life, so nothing about which codes are live is told apart.
func (s *Service) spendCode(given string, now time.Time) error {
	normalized := normalizeCode(given)
	if normalized == "" {
		return protocol.Unauthorized()
	}
	sum := sha256.Sum256([]byte(normalized))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || !now.Before(s.expires) {
		s.forget()
		return protocol.Unauthorized()
	}
	want := sha256.Sum256([]byte(s.code))
	if subtle.ConstantTimeCompare(sum[:], want[:]) != 1 {
		s.failures++
		if s.failures >= maxPairFailures {
			// The code has been guessed at too much. It is thrown away, and so is any chance to use
			// it: the person is asked for a new one, and the guesser has gained nothing.
			s.forget()
		}
		return protocol.Unauthorized()
	}
	s.forget()
	return nil
}

// forget drops the live code and the guesses made against it. The caller holds the lock.
func (s *Service) forget() {
	s.code = ""
	s.expires = time.Time{}
	s.failures = 0
}

// ownerID is the person a code pairs for. Marshal is single-person today, so it is the owner.
func (s *Service) ownerID(ctx context.Context) (string, error) {
	owner, err := s.store.Queries().GetOwner(ctx)
	if err != nil {
		return "", protocol.Internal().WithCause(fmt.Errorf("read the owner: %w", err))
	}
	return owner.ID, nil
}

// deviceName checks what a person called the device, or refuses the pairing with a sentence that
// says what to type instead.
func deviceName(given string, kind protocol.DeviceKind) (string, error) {
	name := strings.TrimSpace(given)
	switch {
	case name == "":
		return "", protocol.InvalidArgument("Give the device a name, such as \"Blair's iPhone\", so it can be told apart from the others.")
	case len(name) > maxDeviceNameBytes:
		return "", protocol.InvalidArgument("That device name is too long. Use 64 characters or fewer.")
	}
	switch kind {
	case protocol.DeviceKindMobile, protocol.DeviceKindWeb:
		return name, nil
	case protocol.DeviceKindDesktop, protocol.DeviceKindCLI, protocol.DeviceKindDev:
		// The owner token and the dev token are written by the daemon itself, so they cannot be
		// asked for by a device trying to pair.
		return "", protocol.InvalidArgument("Marshal does not pair a device of that kind. Pair a phone, a tablet, or a browser.")
	default:
		return "", protocol.InvalidArgument("Marshal does not know that kind of device. Pair a phone, a tablet, or a browser.")
	}
}

// newCode makes the characters a code is made of: six of them, all from the alphabet, with no
// grouping. It is stored as it is made and formatted only for the screen, so what is compared is
// the plain six characters and a dash can never be the difference between two codes.
func newCode() string {
	raw := make([]byte, codeLength)
	for i := range raw {
		raw[i] = codeAlphabet[randomIndex(len(codeAlphabet))]
	}
	return string(raw)
}

// formatCode groups a code for reading: three characters, a dash, and three more.
func formatCode(raw string) string {
	if len(raw) != codeLength {
		return raw
	}
	return raw[:codeGroup] + "-" + raw[codeGroup:]
}

// randomIndex picks a number below n from the system's randomness. n is small enough that taking
// a whole byte and folding it keeps the distribution even, which for a pairing code is plenty.
func randomIndex(n int) int {
	buf := []byte{0}
	if _, err := rand.Read(buf); err != nil {
		// The system's randomness is not something a pairing code can be made without, and a
		// panic here is better than a code a person cannot trust. It has never been seen.
		panic(fmt.Errorf("read randomness for a pairing code: %w", err))
	}
	return int(buf[0]) % n
}

// normalizeCode uppercases a typed code and drops the dashes and spaces, so the code works however
// it is typed. I and L read as 1 and O reads as 0, the same way the alphabet leaves them out.
func normalizeCode(given string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(given) {
		switch r {
		case '-', ' ', '\t':
			continue
		case 'I', 'L':
			r = '1'
		case 'O':
			r = '0'
		}
		if !strings.ContainsRune(codeAlphabet, r) {
			return ""
		}
		out.WriteRune(r)
	}
	normalized := out.String()
	if len(normalized) != codeLength {
		return ""
	}
	return normalized
}

// deviceOf maps a stored row to the wire type.
func deviceOf(row db.ListDevicesRow) protocol.Device {
	return protocol.Device{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       protocol.DeviceKind(row.Kind),
		PairedAt:   store.Timestamp(row.PairedAt),
		LastSeenAt: store.OptionalTimestamp(row.LastSeenAt),
		Revoked:    row.RevokedAt != nil,
	}
}

// hasDevice reports whether the person has a device with this id, revoked or not.
func hasDevice(rows []db.ListDevicesRow, id string) bool {
	return slicesContainFunc(rows, func(row db.ListDevicesRow) bool { return row.ID == id })
}

// slicesContainFunc is a tiny stand-in for slices.ContainsFunc that keeps the check above readable.
func slicesContainFunc[T any](items []T, want func(T) bool) bool {
	for _, item := range items {
		if want(item) {
			return true
		}
	}
	return false
}
