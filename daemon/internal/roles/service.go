package roles

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The lengths a role's fields may have. They are generous: a long role prompt is normal, and these
// are here to stop a runaway paste, not to shape what a person writes. The names are the longest of
// the small ones because a role's name is what a card, a chat target, and the screens call it, and
// chats limit a role name to 60 characters too (maxRoleChars in internal/chats/service.go).
const (
	maxRoleNameChars = 60
	maxDescChars     = 300
	maxInstrChars    = 20_000
	maxAgentChars    = 80
	maxModelChars    = 120
	maxBackupChars   = 120
	maxLabelChars    = 40
	maxTagChars      = 60
	maxTags          = 50
	maxLimitValue    = 1_000_000
)

// Roles returns every role, with the overridden flag set for the project being looked at. The list
// is global: a project changes only whether a role reads as overridden, never which roles exist. A
// project id that no longer names a project is not an error here - it simply has no overrides, which
// is what a client that kept a stale id should see.
func (s *Service) Roles(ctx context.Context, projectID string) (protocol.RoleList, error) {
	var rows []db.Role
	overridden := map[string]bool{}
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if rows, err = q.ListRoles(ctx); err != nil {
			return fmt.Errorf("list roles: %w", err)
		}
		if projectID == "" {
			return nil
		}
		overrides, err := q.ListRoleOverridesByProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("list the role overrides of project %s: %w", projectID, err)
		}
		for _, override := range overrides {
			overridden[override.RoleID] = true
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	list, err := toRoles(rows, overridden)
	if err != nil {
		return protocol.RoleList{}, err
	}
	return protocol.NewRoleList(list, s.now()), nil
}

// Role returns one role by its name.
func (s *Service) Role(ctx context.Context, name, projectID string) (protocol.Role, error) {
	row, overridden, err := s.find(ctx, name, projectID)
	if err != nil {
		return protocol.Role{}, err
	}
	return toRole(row, overridden)
}

// CreateRole adds a role, or imports one. The name is trimmed and must not be in use, and a name
// that is taken is a conflict rather than a silent replace: importing a role must never overwrite
// one that is already there.
func (s *Service) CreateRole(ctx context.Context, projectID string, in protocol.CreateRoleRequest) (protocol.RoleList, error) {
	name := strings.TrimSpace(in.Name)
	if err := checkName(name); err != nil {
		return protocol.RoleList{}, err
	}
	spec, err := checkSpec(in.Spec)
	if err != nil {
		return protocol.RoleList{}, err
	}
	encoded, err := encodeSpec(spec)
	if err != nil {
		return protocol.RoleList{}, err
	}
	id, err := s.newID()
	if err != nil {
		return protocol.RoleList{}, fmt.Errorf("make a role id: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.GetRoleByName(ctx, name); err == nil {
			return conflictName(name)
		} else if !store.IsNotFound(err) {
			return fmt.Errorf("look for a role called %q: %w", name, err)
		}
		params := db.CreateRoleParams{ID: id, Name: name, IsStarter: 0, SpecJSON: encoded}
		if err := q.CreateRole(ctx, params); err != nil {
			return fmt.Errorf("add the role %q: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	s.log.Info("added a role", "role_id", id, "name", name)
	return s.Roles(ctx, projectID)
}

// renamedRole applies a validated new name to row, once it has checked that no other role already
// claims it. It is its own function rather than an inline block of UpdateRole's write so that the
// rename's own conflict check - a lookup, then two ways that lookup can turn out - reads as one
// decision instead of adding another layer of nesting to the transaction around it.
//
// A newName equal to row's own name is not a rename at all, and is left alone rather than looked up:
// a role kept at its own name never conflicts with itself.
func renamedRole(ctx context.Context, q *db.Queries, newName string, row db.Role) (db.Role, error) {
	if newName == row.Name {
		return row, nil
	}
	other, err := q.GetRoleByName(ctx, newName)
	switch {
	case err == nil && other.ID != row.ID:
		return row, conflictName(newName)
	case err != nil && !store.IsNotFound(err):
		return row, fmt.Errorf("look for a role called %q: %w", newName, err)
	}
	row.Name = newName
	return row, nil
}

// preparedRoleUpdate is what UpdateRole is ready to write, once every part of the request it was
// asked for has been checked: the new name, trimmed and validated, and the spec, checked and
// re-encoded to store. Either is left at its zero value when that part of the request was not
// asked for, exactly as UpdateRole's own "in.Name != nil" and "in.Spec != nil" checks read them.
type preparedRoleUpdate struct {
	newName string
	encoded string
}

// prepareRoleUpdate validates the parts of an update request that were asked for, before
// UpdateRole opens a transaction for them - a bad name or a bad spec is refused without ever
// starting one.
func prepareRoleUpdate(in protocol.UpdateRoleRequest) (preparedRoleUpdate, error) {
	var out preparedRoleUpdate
	if in.Name != nil {
		out.newName = strings.TrimSpace(*in.Name)
		if err := checkName(out.newName); err != nil {
			return preparedRoleUpdate{}, err
		}
	}
	if in.Spec != nil {
		checked, err := checkSpec(*in.Spec)
		if err != nil {
			return preparedRoleUpdate{}, err
		}
		encoded, err := encodeSpec(checked)
		if err != nil {
			return preparedRoleUpdate{}, err
		}
		out.encoded = encoded
	}
	return out, nil
}

// UpdateRole renames a role, replaces its spec, or both. A body that sets nothing answers with the
// list as it is - after checking the role is there, so an empty edit of a name that is not a role is
// still a not found. A rename is a change to the role itself, not to one project's view of it: roles
// are global, and the screens rename the role, not a copy of it.
func (s *Service) UpdateRole(ctx context.Context, name string, in protocol.UpdateRoleRequest) (protocol.RoleList, error) {
	if in.Name == nil && in.Spec == nil {
		if _, err := s.Role(ctx, name, ""); err != nil {
			return protocol.RoleList{}, err
		}
		return s.Roles(ctx, "")
	}
	prepared, err := prepareRoleUpdate(in)
	if err != nil {
		return protocol.RoleList{}, err
	}
	var id string
	err = s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetRoleByName(ctx, name)
		if err != nil {
			return notFoundRoleFrom(err, name)
		}
		id = row.ID
		if in.Name != nil {
			if row, err = renamedRole(ctx, q, prepared.newName, row); err != nil {
				return err
			}
		}
		if in.Spec != nil {
			row.SpecJSON = prepared.encoded
		}
		changed, err := q.UpdateRole(ctx, db.UpdateRoleParams{Name: row.Name, SpecJSON: row.SpecJSON, ID: row.ID})
		if err != nil {
			return fmt.Errorf("update the role %q: %w", name, err)
		}
		if changed == 0 {
			return notFoundRole(name)
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	s.log.Info("edited a role", "role_id", id, "name", prepared.newName)
	return s.Roles(ctx, "")
}

// DeleteRole removes a role a person made. A role Marshal ships is refused: it is reset instead, so
// the eight roles the screens are built around can never be removed by a stray request.
func (s *Service) DeleteRole(ctx context.Context, name, projectID string) (protocol.RoleList, error) {
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetRoleByName(ctx, name)
		if err != nil {
			return notFoundRoleFrom(err, name)
		}
		if row.IsStarter != 0 {
			return protocol.Refused(fmt.Sprintf(
				"%s is one of Marshal's own roles, so it cannot be deleted. Reset it instead.", row.Name))
		}
		changed, err := q.DeleteRole(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("delete the role %q: %w", name, err)
		}
		if changed == 0 {
			return notFoundRole(name)
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	s.log.Info("deleted a role", "name", name)
	return s.Roles(ctx, projectID)
}

// ResetRole removes one project's override of a role. It clears the role's overridden flag for that
// project and does nothing else: the role's own spec is left exactly as it is, so a role a person
// edited stays edited. That is what the prototype's confirmResetRole does, and its confirm text is
// the only thing that suggests otherwise.
func (s *Service) ResetRole(ctx context.Context, name, projectID string) (protocol.RoleList, error) {
	if projectID == "" {
		return protocol.RoleList{}, protocol.InvalidArgument("A reset needs the project to reset it for.").With("project", "")
	}
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetRoleByName(ctx, name)
		if err != nil {
			return notFoundRoleFrom(err, name)
		}
		if err := q.DeleteRoleOverride(ctx, db.DeleteRoleOverrideParams{RoleID: row.ID, ProjectID: projectID}); err != nil {
			return fmt.Errorf("clear the override of role %q for project %s: %w", name, projectID, err)
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	s.log.Info("reset a role", "name", name, "project_id", projectID)
	return s.Roles(ctx, projectID)
}

// SetRoleOverride gives one project its own version of a role. The project must exist, and the spec
// is checked the same way a role's own spec is.
func (s *Service) SetRoleOverride(ctx context.Context, name, projectID string, spec protocol.RoleSpec) (protocol.RoleList, error) {
	if projectID == "" {
		return protocol.RoleList{}, protocol.InvalidArgument("An override needs the project it is for.").With("project", "")
	}
	checked, err := checkSpec(spec)
	if err != nil {
		return protocol.RoleList{}, err
	}
	encoded, err := encodeSpec(checked)
	if err != nil {
		return protocol.RoleList{}, err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFoundProjectFrom(err, projectID)
		}
		row, err := q.GetRoleByName(ctx, name)
		if err != nil {
			return notFoundRoleFrom(err, name)
		}
		params := db.SetRoleOverrideParams{RoleID: row.ID, ProjectID: projectID, SpecJSON: encoded}
		if err := q.SetRoleOverride(ctx, params); err != nil {
			return fmt.Errorf("set the override of role %q for project %s: %w", name, projectID, err)
		}
		return nil
	})
	if err != nil {
		return protocol.RoleList{}, err
	}
	s.log.Info("overrode a role", "name", name, "project_id", projectID)
	return s.Roles(ctx, projectID)
}

// find reads one role and says whether the project being looked at keeps its own version of it.
func (s *Service) find(ctx context.Context, name, projectID string) (db.Role, bool, error) {
	var (
		row        db.Role
		overridden bool
	)
	err := s.store.Read(ctx, func(q *db.Queries) error {
		var err error
		if row, err = q.GetRoleByName(ctx, name); err != nil {
			return notFoundRoleFrom(err, name)
		}
		if projectID == "" {
			return nil
		}
		if _, err := q.GetRoleOverride(ctx, db.GetRoleOverrideParams{RoleID: row.ID, ProjectID: projectID}); err != nil {
			if store.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("read the override of role %q for project %s: %w", name, projectID, err)
		}
		overridden = true
		return nil
	})
	if err != nil {
		return db.Role{}, false, err
	}
	return row, overridden, nil
}

// toRoles builds the wire roles from rows, in the order the rows came.
func toRoles(rows []db.Role, overridden map[string]bool) ([]protocol.Role, error) {
	list := make([]protocol.Role, 0, len(rows))
	for _, row := range rows {
		role, err := toRole(row, overridden[row.ID])
		if err != nil {
			return nil, err
		}
		list = append(list, role)
	}
	return list, nil
}

// toRole builds the wire role from a row.
func toRole(row db.Role, overridden bool) (protocol.Role, error) {
	spec, err := decodeSpec(row.SpecJSON)
	if err != nil {
		return protocol.Role{}, fmt.Errorf("read the spec of role %s: %w", row.ID, err)
	}
	return protocol.Role{
		ID: row.ID, Name: row.Name, Starter: row.IsStarter != 0, Overridden: overridden, Spec: spec,
	}, nil
}

// encodeSpec turns a role's body into the JSON the table stores. A role with no skills or no MCP
// servers stores an empty list, never null.
func encodeSpec(spec protocol.RoleSpec) (string, error) {
	if spec.Skills == nil {
		spec.Skills = []string{}
	}
	if spec.MCP == nil {
		spec.MCP = []string{}
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("encode the role spec: %w", err)
	}
	return string(data), nil
}

// decodeSpec reads a role's body from the JSON the table stores.
func decodeSpec(encoded string) (protocol.RoleSpec, error) {
	var spec protocol.RoleSpec
	if err := json.Unmarshal([]byte(encoded), &spec); err != nil {
		return protocol.RoleSpec{}, err
	}
	if spec.Skills == nil {
		spec.Skills = []string{}
	}
	if spec.MCP == nil {
		spec.MCP = []string{}
	}
	return spec, nil
}

// checkName refuses a name that cannot be a role's, before the store is touched.
func checkName(name string) error {
	switch {
	case name == "":
		return protocol.InvalidArgument("Give the role a name.").With("name", name)
	case utf8.RuneCountInString(name) > maxRoleNameChars:
		return protocol.InvalidArgument(fmt.Sprintf(
			"Role names can have at most %d characters.", maxRoleNameChars)).With("name", name)
	case strings.Contains(name, "/"):
		// A role is addressed by its name in a route, so a slash would split the address in two.
		return protocol.InvalidArgument("Role names cannot contain a slash.").With("name", name)
	}
	return nil
}

// checkSpec refuses a role body that cannot be saved, before the store is touched. It answers the
// spec with its lists made non-nil, which is what gets stored.
func checkSpec(spec protocol.RoleSpec) (protocol.RoleSpec, error) {
	if err := checkLimits(spec.Limits); err != nil {
		return protocol.RoleSpec{}, err
	}
	for _, field := range []struct {
		what, value string
		most        int
	}{
		{"desc", spec.Desc, maxDescChars},
		{"instr", spec.Instr, maxInstrChars},
		{"agent", spec.Agent, maxAgentChars},
		{"model", spec.Model, maxModelChars},
		{"think", spec.Think, maxLabelChars},
		{"perm", spec.Perm, maxLabelChars},
		{"strength", spec.Strength, maxLabelChars},
		{"backup", spec.Backup, maxBackupChars},
	} {
		if utf8.RuneCountInString(field.value) > field.most {
			return protocol.RoleSpec{}, protocol.InvalidArgument(fmt.Sprintf(
				"The role's %s can have at most %d characters.", field.what, field.most)).With(field.what, field.value)
		}
	}
	if err := checkTags("skills", spec.Skills); err != nil {
		return protocol.RoleSpec{}, err
	}
	if err := checkTags("MCP servers", spec.MCP); err != nil {
		return protocol.RoleSpec{}, err
	}
	if spec.Skills == nil {
		spec.Skills = []string{}
	}
	if spec.MCP == nil {
		spec.MCP = []string{}
	}
	return spec, nil
}

// checkLimits refuses a negative limit, or one so large it would not be a limit.
func checkLimits(limits protocol.RoleLimits) error {
	for _, each := range []struct {
		what  string
		value int
	}{
		{"time", limits.Time},
		{"cost", limits.Cost},
		{"rounds", limits.Rounds},
	} {
		switch {
		case each.value < 0:
			return protocol.InvalidArgument("A role's limits cannot be negative.").With(each.what, fmt.Sprint(each.value))
		case each.value > maxLimitValue:
			return protocol.InvalidArgument(fmt.Sprintf(
				"A role's %s limit can be at most %d.", each.what, maxLimitValue)).With(each.what, fmt.Sprint(each.value))
		}
	}
	return nil
}

// checkTags refuses a list of names that has too many entries or one that is too long.
func checkTags(what string, tags []string) error {
	if len(tags) > maxTags {
		return protocol.InvalidArgument(fmt.Sprintf("A role can have at most %d %s.", maxTags, what)).With("count", fmt.Sprint(len(tags)))
	}
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			return protocol.InvalidArgument(fmt.Sprintf("A role's %s cannot include an empty name.", what))
		}
		if utf8.RuneCountInString(tag) > maxTagChars {
			return protocol.InvalidArgument(fmt.Sprintf(
				"A role's %s names can have at most %d characters.", what, maxTagChars)).With("name", tag)
		}
	}
	return nil
}
