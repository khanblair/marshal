package gemini

import (
	"fmt"
	"slices"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// withModel moves the model of a start spec into the environment of the process, where Gemini CLI
// reads it, and takes it out of the spec so that the ACP adapter does not look for a model control
// that Gemini does not offer. It returns the model that was asked for, or an empty string.
func withModel(spec agents.StartSpec) (agents.StartSpec, string, error) {
	model := spec.Model
	if model == "" {
		return spec, "", nil
	}
	if !validModel(model) {
		return spec, "", fmt.Errorf("%w: model %q is not a model name", agents.ErrUnsupportedSetting, model)
	}
	spec.Model = ""
	spec.Env = append(slices.Clone(spec.Env), modelVariable+"="+model)
	return spec, model, nil
}

// withModelApplied records on the handle that the model was taken, since the ACP adapter did not
// see it.
func withModelApplied(h agents.SessionHandle, model string) agents.SessionHandle {
	h.Capabilities.ModelSwitching = true
	if model != "" {
		h.Model = model
		h.Applied.Model = true
	}
	return h
}

// validModel says whether text can be a model name: letters, digits, and the marks that names such
// as "gemini-2.5-pro" and "models/gemini-2.5-pro" use. The name goes into an environment variable,
// so anything else is refused instead of passed on.
func validModel(name string) bool {
	if len(name) > maxModelLength {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == ':' || r == '/':
		default:
			return false
		}
	}
	return true
}

// maxModelLength is the longest model name that is accepted.
const maxModelLength = 100
