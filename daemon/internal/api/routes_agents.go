package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents/catalog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// listAgents is GET /v1/agents: the agents this daemon can start, from the catalog's kept answer.
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.catalog.List(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// refreshAgents is POST /v1/agents/refresh: look for the agents again, whatever is kept.
func (s *Server) refreshAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.catalog.Refresh(r.Context())
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, list)
}

// testAgent is POST /v1/agents/{id}/test: look at one agent program again and say what was found,
// without sending any prompt. The id is an agent kind or one of the other tools the catalog lists.
func (s *Server) testAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.catalog.Test(r.Context(), id)
	if errors.Is(err, catalog.ErrUnknownAgent) {
		s.writeError(w, notFoundID("agent", id))
		return
	}
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

// saveAgentSettings is PUT /v1/agents/{id}/settings: the path to use for an agent and whether it is
// on. A path is checked first, so a file that is not the agent's program is refused with the reason
// and nothing is saved. It answers the agent as the catalog lists it now.
func (s *Server) saveAgentSettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	list, err := s.catalog.List(ctx)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	kind, known := findAgent(list, id)
	if !known {
		s.writeError(w, notFoundID("agent", id))
		return
	}
	if kind == protocol.AgentKindBuiltin {
		s.writeError(w, protocol.InvalidArgument("Marshal's own agent has no program to choose, and it is not turned off here."))
		return
	}
	var in protocol.AgentSettingsRequest
	if err := s.decodeJSON(r, &in); err != nil {
		s.writeError(w, err)
		return
	}
	path := strings.TrimSpace(in.Path)
	// A path is checked only when it is a new one. Turning an agent off must still work when the
	// program saved earlier has been moved or deleted, which is when the person wants it off.
	prev, err := s.agentPrefs.Preference(ctx, kind)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	if path != "" && path != prev.Path {
		if _, err := catalog.CheckProgram(ctx, kind, path); err != nil {
			s.writeError(w, protocol.InvalidArgument(err.Error()).With("path", path))
			return
		}
	}
	if err := s.agentPrefs.SetPreference(ctx, kind, catalog.Preference{Path: path, Off: in.Off}); err != nil {
		s.writeError(w, translate(err))
		return
	}
	// The save has happened, so the answer always says what was saved. A list that did not refresh
	// is said so in the log, and the agent is shown with the saved settings over the last list.
	answer := list
	if fresh, err := s.catalog.Refresh(ctx); err != nil {
		s.log.Warn("the agent list did not refresh after a save", "kind", string(kind), "err", err)
	} else {
		answer = fresh
	}
	agent := listedAgent(answer, kind)
	agent.CustomPath, agent.Off = path, in.Off
	s.writeJSON(w, http.StatusOK, agent)
}

// listedAgent returns the listed agent of a kind, which is always listed when its kind was.
func listedAgent(list protocol.AgentCatalog, kind protocol.AgentKind) protocol.Agent {
	for _, agent := range list.Agents {
		if agent.Kind == kind {
			return agent
		}
	}
	return protocol.Agent{Kind: kind}
}

// findAgent returns the kind of a listed agent with the id, or false when the list has none.
func findAgent(list protocol.AgentCatalog, id string) (protocol.AgentKind, bool) {
	for _, agent := range list.Agents {
		if string(agent.Kind) == id {
			return agent.Kind, true
		}
	}
	return "", false
}
