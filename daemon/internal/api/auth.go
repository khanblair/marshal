package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// authCacheTTL is how long a token that was found in the database is remembered. It is the
	// most time a revoked token can still work, and it is short on purpose so that revoking a
	// device takes effect within seconds without the database being asked on every request.
	authCacheTTL = 5 * time.Second
	// touchInterval is the most often a device's last-seen time is written. A write per request
	// would keep the only database writer busy for no gain.
	touchInterval = time.Minute
	// maxAuthCacheEntries bounds the cache. Tokens that were never valid are not cached, so this
	// only matters when many devices are active at once.
	maxAuthCacheEntries = 256
	// maxTokenBytes cuts off absurd input before it is hashed.
	maxTokenBytes = 512
	bearerPrefix  = "Bearer "
)

// Caller is the device and user behind an authenticated request.
type Caller struct {
	// DeviceID is the opaque id of the device whose token was used.
	DeviceID string
	// DeviceKind is what sort of client the device is.
	DeviceKind protocol.DeviceKind
	// UserID is the opaque id of the user who owns the device.
	UserID string
}

// Principal returns the caller that the token check put in the request context. It reports false
// on a route that has no token check, so a handler on a public route must not expect one.
func Principal(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerKey).(Caller)
	return caller, ok
}

// DevAccess lets a dev daemon accept its dev token from this machine. A normal daemon ignores
// it, so the dev token file has no effect outside dev mode.
type DevAccess struct {
	// Token is the plain dev token from the dev-token file.
	Token string
	// Caller is the dev device that the token stands for.
	Caller Caller
}

type devCredential struct {
	hash   string
	caller Caller
}

type cachedCaller struct {
	caller  Caller
	expires time.Time
}

// authenticator decides which device a token belongs to. A token is looked up by its SHA-256 hash
// among the active devices, so a revoked device never signs in, and the same answer is given for
// a token that is unknown, malformed, or revoked.
type authenticator struct {
	store *store.Store
	log   *slog.Logger
	now   func() time.Time
	dev   *devCredential // nil unless this is a dev daemon with a dev token

	mu      sync.Mutex // guards the two maps below
	cache   map[string]cachedCaller
	touched map[string]time.Time
}

func newAuthenticator(st *store.Store, log *slog.Logger, now func() time.Time, dev *devCredential) *authenticator {
	return &authenticator{
		store:   st,
		log:     log,
		now:     now,
		dev:     dev,
		cache:   make(map[string]cachedCaller),
		touched: make(map[string]time.Time),
	}
}

func errUnauthorized() *protocol.Error {
	return protocol.NewError(protocol.ErrorCodeUnauthorized,
		"Sign in again. This device's token is missing or no longer valid.")
}

// authenticate returns the caller for a token, or an unauthorized error. remoteAddr is the
// address the connection came from: the dev token is accepted only from a loopback address, and
// only the connection's own address counts, never a header that a proxy or a client could set.
func (a *authenticator) authenticate(ctx context.Context, token, remoteAddr string) (Caller, error) {
	if token == "" || len(token) > maxTokenBytes {
		return Caller{}, errUnauthorized()
	}
	hash := store.HashToken(token)
	if a.dev != nil && isLoopback(remoteAddr) && sameHash(hash, a.dev.hash) {
		a.touch(ctx, a.dev.caller.DeviceID)
		return a.dev.caller, nil
	}
	if caller, ok := a.cached(hash); ok {
		a.touch(ctx, caller.DeviceID)
		return caller, nil
	}
	caller, err := a.lookup(ctx, hash)
	if err != nil {
		return Caller{}, err
	}
	a.remember(hash, caller)
	a.touch(ctx, caller.DeviceID)
	return caller, nil
}

// lookup finds the active device with this token hash.
func (a *authenticator) lookup(ctx context.Context, hash string) (Caller, error) {
	device, err := a.store.Queries().GetActiveDeviceByTokenHash(ctx, hash)
	switch {
	case store.IsNotFound(err):
		return Caller{}, errUnauthorized()
	case err != nil:
		return Caller{}, protocol.Internal().WithCause(fmt.Errorf("look up a device by token: %w", err))
	}
	// The database matched on the hash already. This compares the two again in constant time so
	// no code path decides on a plain string comparison of secrets.
	if !sameHash(device.TokenHash, hash) || device.Kind == string(protocol.DeviceKindDev) {
		// A dev device signs in with the dev token only, and only on a dev daemon.
		return Caller{}, errUnauthorized()
	}
	return Caller{DeviceID: device.ID, DeviceKind: protocol.DeviceKind(device.Kind), UserID: device.UserID}, nil
}

func (a *authenticator) cached(hash string) (Caller, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.cache[hash]
	if !ok || !a.now().Before(entry.expires) {
		delete(a.cache, hash)
		return Caller{}, false
	}
	return entry.caller, true
}

func (a *authenticator) remember(hash string, caller Caller) {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.cache) >= maxAuthCacheEntries {
		for key, entry := range a.cache {
			if !now.Before(entry.expires) {
				delete(a.cache, key)
			}
		}
	}
	if len(a.cache) >= maxAuthCacheEntries {
		clear(a.cache)
	}
	a.cache[hash] = cachedCaller{caller: caller, expires: now.Add(authCacheTTL)}
}

// touch records that a device was seen, at most once per touchInterval. A failure is logged and
// never stops the request: knowing when a device last called is not worth refusing it.
func (a *authenticator) touch(ctx context.Context, deviceID string) {
	now := a.now()
	a.mu.Lock()
	last, seen := a.touched[deviceID]
	due := !seen || now.Sub(last) >= touchInterval
	if due {
		a.touched[deviceID] = now
	}
	a.mu.Unlock()
	if !due {
		return
	}
	err := a.store.Write(ctx, func(q *db.Queries) error {
		return q.TouchDevice(ctx, db.TouchDeviceParams{LastSeenAt: store.Millis(now), ID: deviceID})
	})
	if err != nil {
		a.log.Warn("record when a device was last seen", "device_id", deviceID, "error", err)
	}
}

// sameHash compares two hashes in constant time.
func sameHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// isLoopback reports whether a connection's remote address (host and port, as net/http gives it)
// is on this machine.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}

// bearerToken reads the token from an Authorization header of the form "Bearer <token>". It
// returns "" when the header is missing or malformed, which every caller treats as no token.
func bearerToken(header string) string {
	if len(header) <= len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}
	token := strings.TrimSpace(header[len(bearerPrefix):])
	if token == "" || strings.ContainsAny(token, " \t") {
		return ""
	}
	return token
}

// authenticated wraps a handler so it runs only for a valid token, and puts the caller in the
// request context. Every answer to a missing, malformed, unknown, or revoked token is the same.
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		caller, err := s.auth.authenticate(r.Context(), token, r.RemoteAddr)
		if err != nil {
			s.denyAccess(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey, caller)))
	})
}

// denyAccess answers a failed check. An unauthorized answer also names the scheme the client
// should use, as the HTTP rules ask.
func (s *Server) denyAccess(w http.ResponseWriter, err error) {
	var perr *protocol.Error
	if errors.As(err, &perr) && perr.Code == protocol.ErrorCodeUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	s.writeError(w, err)
}
