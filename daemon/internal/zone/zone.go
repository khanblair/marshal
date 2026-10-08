// Package zone is the time zone Marshal runs its clock in. The person chooses it in their profile;
// until they do, the machine's own zone is used. Everything that decides what day it is, or what
// "8:00" means - schedules, briefs, Home's daily numbers, the calendar's all-day events - reads its
// time from the Clock here, so a zone is chosen once and applies everywhere.
package zone

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Zone names must resolve on a machine with no zone database.

	"github.com/khanblair/marshal/daemon/internal/store"
)

// Source answers the time zone name the person chose, or "" when they chose none.
type Source interface {
	TimeZone(ctx context.Context) (string, error)
}

// StoreSource reads the zone from the daemon's owner, the first person in the users table. A daemon
// with no owner yet has chosen no zone.
type StoreSource struct {
	Store *store.Store
}

// TimeZone is the owner's zone name.
func (s StoreSource) TimeZone(ctx context.Context) (string, error) {
	owner, err := s.Store.Queries().GetOwner(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return owner.TimeZone, nil
}

// Resolve turns a zone name into a location. An empty name, "Local", or a name the zone database
// does not know is the machine's own zone, so a bad stored value never stops the daemon telling time.
func Resolve(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

// Clock is the daemon's clock, in the chosen zone. It is safe for concurrent use.
type Clock struct {
	src Source
	now func() time.Time

	mu        sync.RWMutex
	loc       *time.Location
	listeners []func(*time.Location)
}

// Option changes how a Clock is built.
type Option func(*Clock)

// WithNow sets where the instant comes from. The default is time.Now; a test passes a fixed time.
func WithNow(now func() time.Time) Option {
	return func(c *Clock) {
		if now != nil {
			c.now = now
		}
	}
}

// New makes a clock and reads the zone once. A source that cannot be read leaves the machine's zone.
func New(src Source, opts ...Option) *Clock {
	c := &Clock{src: src, now: time.Now, loc: time.Local}
	for _, opt := range opts {
		opt(c)
	}
	c.Refresh(context.Background())
	return c
}

// Location is the zone times are read in.
func (c *Clock) Location() *time.Location {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loc
}

// Now is the current time in the chosen zone, so its calendar day, weekday, and clock are the
// person's own.
func (c *Clock) Now() time.Time {
	return c.now().In(c.Location())
}

// OnChange registers a function called, outside any lock, each time Refresh finds the zone changed.
func (c *Clock) OnChange(fn func(*time.Location)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, fn)
}

// Refresh reads the zone again and tells the listeners when it is not the one they had. It answers
// whether it changed. Call it after the person saves a new zone.
func (c *Clock) Refresh(ctx context.Context) bool {
	name := ""
	if c.src != nil {
		if read, err := c.src.TimeZone(ctx); err == nil {
			name = read
		}
	}
	next := Resolve(name)
	c.mu.Lock()
	changed := next.String() != c.loc.String()
	c.loc = next
	listeners := append([]func(*time.Location){}, c.listeners...)
	c.mu.Unlock()
	if changed {
		for _, fn := range listeners {
			fn(next)
		}
	}
	return changed
}

// Label is the zone's short name at an instant, such as "EAT", for a message that shows a time.
func Label(t time.Time) string {
	name, _ := t.Zone()
	return name
}
