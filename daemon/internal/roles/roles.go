// Package roles owns Marshal's role templates: the eight starter roles it ships, the roles a person
// makes by duplicating or importing one, and a project's own version of a role
// (docs/backend-checklist.md B5.1, docs/architecture.md section 10, inventory N18). The API layer
// reaches it through the Roles interface, and nothing outside it reads or writes its tables.
//
// A role is a name and a spec. The spec is the whole editable body - the description, the
// instructions, the agent, the model, the thinking and permission labels, the strength, the backup
// model, the skills, the MCP servers, and the role's own time, cost, and round limits - kept as one
// JSON document so the shape can grow without a migration.
//
// A role is global to the install, not a project's. A project that needs different instructions or a
// cheaper model for one role keeps an override - its own spec for that role - and the role then
// reads as "overridden" for that project. Resetting a role removes the project's override and
// nothing else: it clears the flag and leaves the role's own spec alone, which is what the
// prototype's confirmResetRole does (apps/web/src/views/settings/role-actions.ts). Removing a
// project takes its overrides with it, and removing a role takes every project's override of it.
//
// Nothing here publishes an event. A settings screen reads the roles when it opens and writes one
// when a person saves it, and no other screen's contents change when a role does: a card carries its
// role by name, and the harness reads a role's limits when the card runs, not when a role is saved.
package roles

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// Roles is what the API layer needs to manage Marshal's role templates.
type Roles interface {
	// Roles returns every role, with the overridden flag set for the project being looked at. An
	// empty project id reports every role as not overridden, because no project is being looked at.
	Roles(ctx context.Context, projectID string) (protocol.RoleList, error)
	// Role returns one role by its name.
	Role(ctx context.Context, name, projectID string) (protocol.Role, error)
	// CreateRole adds a role, or imports one. The name must not already be in use.
	CreateRole(ctx context.Context, projectID string, in protocol.CreateRoleRequest) (protocol.RoleList, error)
	// UpdateRole renames a role, replaces its spec, or both. A rename changes the role itself,
	// not one project's view of it: roles are global, and the screens rename the role.
	UpdateRole(ctx context.Context, name string, in protocol.UpdateRoleRequest) (protocol.RoleList, error)
	// DeleteRole removes a role a person made. A starter role is refused: it is reset, not deleted.
	DeleteRole(ctx context.Context, name, projectID string) (protocol.RoleList, error)
	// ResetRole removes the project's override of a role, which clears the role's overridden flag
	// for that project and leaves the role's own spec as it is.
	ResetRole(ctx context.Context, name, projectID string) (protocol.RoleList, error)
	// SetRoleOverride gives one project its own version of one role.
	SetRoleOverride(ctx context.Context, name, projectID string, spec protocol.RoleSpec) (protocol.RoleList, error)
	// EnsureStarters adds any of Marshal's starter roles that are missing, and leaves a role a
	// person has edited as it is. It is safe to call at every start-up.
	EnsureStarters(ctx context.Context) error
}

var _ Roles = (*Service)(nil)

// Deps are the parts the service is built from.
type Deps struct {
	// Store is the open database.
	Store *store.Store
}

// Service implements Roles. It is safe for use by many goroutines.
type Service struct {
	store   *store.Store
	log     *slog.Logger
	now     func() time.Time
	entropy io.Reader
	// idMu makes ids come from the entropy reader one at a time, so a test reader needs no lock.
	idMu sync.Mutex
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithEntropy sets where ids get their random part. The default is crypto/rand.
func WithEntropy(r io.Reader) Option {
	return func(s *Service) { s.entropy = r }
}

// WithLogger sets the logger. The default logs nothing.
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

// New builds a Service. The store is required.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Store == nil {
		return nil, errors.New("the roles module needs the store")
	}
	s := &Service{
		store:   deps.Store,
		log:     slog.New(slog.DiscardHandler),
		now:     time.Now,
		entropy: rand.Reader,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// newID makes an opaque id from the clock and the entropy reader.
func (s *Service) newID() (string, error) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	return protocol.NewID(s.now(), s.entropy)
}
