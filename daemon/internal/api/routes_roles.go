package api

import (
	"net/http"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The role templates and a project's overrides of them (docs/backend-checklist.md B5.1, N18). The
// rules are in internal/roles. These handlers read the path, the query, and the body, call the
// service, and write the answer.
//
// A role is addressed by its name, which is unique, because that is what the screens, a card, and a
// chat target call it. Every route but the read of one role answers with the whole list, so a screen
// redraws itself from one answer whatever changed.
//
// The overridden flag is per project. Each route takes an optional `project` query parameter naming
// the project being looked at; without it every role reports as not overridden, which is what a
// caller that is not looking at a project should see. Only the two routes that change an override
// require it.

// listRoles is GET /v1/roles: every role Marshal knows, Marshal's starters first.
func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	s.writeRoles(w, r, roleProject(r))
}

// getRole is GET /v1/roles/{name}: one role.
func (s *Server) getRole(w http.ResponseWriter, r *http.Request) {
	role, err := s.roles.Role(r.Context(), r.PathValue("name"), roleProject(r))
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, role)
}

// createRole is POST /v1/roles: add a role, or import one. The body is the export document - a name
// and a spec - so a role exported from another machine posts here as it is. A name another role has
// is a conflict: importing must never overwrite a role that is already there.
func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateRoleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if _, err := s.roles.CreateRole(r.Context(), roleProject(r), req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeRoles(w, r, roleProject(r))
}

// updateRole is PATCH /v1/roles/{name}: rename a role, replace its spec, or both. A field that is not
// sent is left as it is. Renaming changes the role itself, not one project's view of it.
func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	var req protocol.UpdateRoleRequest
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if _, err := s.roles.UpdateRole(r.Context(), r.PathValue("name"), req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeRoles(w, r, roleProject(r))
}

// deleteRole is DELETE /v1/roles/{name}: remove a role a person made. One of Marshal's own roles is
// refused with a sentence saying to reset it instead.
func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	if _, err := s.roles.DeleteRole(r.Context(), r.PathValue("name"), roleProject(r)); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeRoles(w, r, roleProject(r))
}

// resetRole is POST /v1/roles/{name}/reset: remove one project's override of a role, which clears the
// role's overridden flag for that project and leaves the role itself as it is.
func (s *Server) resetRole(w http.ResponseWriter, r *http.Request) {
	if _, err := s.roles.ResetRole(r.Context(), r.PathValue("name"), roleProject(r)); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeRoles(w, r, roleProject(r))
}

// setRoleOverride is PUT /v1/roles/{name}/override: give one project its own version of one role. It
// takes the project from the query, because a role's override is stored per project and the body is
// the spec alone.
func (s *Server) setRoleOverride(w http.ResponseWriter, r *http.Request) {
	var req protocol.RoleSpec
	if err := s.decodeJSON(r, &req); err != nil {
		s.writeError(w, err)
		return
	}
	if _, err := s.roles.SetRoleOverride(r.Context(), r.PathValue("name"), roleProject(r), req); err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeRoles(w, r, roleProject(r))
}

// writeRoles sends the whole list, stamped with the daemon's time, with the overridden flag set for
// the project being looked at.
func (s *Server) writeRoles(w http.ResponseWriter, r *http.Request, projectID string) {
	list, err := s.roles.Roles(r.Context(), projectID)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// roleProject is the project a roles request is being made about, or empty when the request is not
// about a project. It is a query parameter rather than a path segment because the roles themselves
// are the resource and the project only changes how they read.
func roleProject(r *http.Request) string {
	return r.URL.Query().Get("project")
}
