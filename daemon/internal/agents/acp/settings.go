package acp

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// selectOption finds the option of the given kind that offers a list of values, such as the model.
func (c controls) selectOption(category sdk.SessionConfigOptionCategory) *sdk.SessionConfigOptionSelect {
	for _, o := range c.options {
		if o.Select != nil && o.Select.Category != nil && *o.Select.Category == category {
			return o.Select
		}
	}
	return nil
}

// choices lists the values of a select option, whether or not they are grouped.
func choices(sel *sdk.SessionConfigOptionSelect) []sdk.SessionConfigSelectOption {
	var out []sdk.SessionConfigSelectOption
	if sel.Options.Ungrouped != nil {
		out = append(out, *sel.Options.Ungrouped...)
	}
	if sel.Options.Grouped != nil {
		for _, group := range *sel.Options.Grouped {
			out = append(out, group.Options...)
		}
	}
	return out
}

// normalize makes two spellings of a word comparable: "auto-edits", "autoEdits", and "Auto edits"
// all become "autoedits".
func normalize(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(word) {
		if r != '-' && r != '_' && r != ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// applySettings gives the agent the model, thinking mode, and permission mode that were asked for,
// as far as the agent offers controls for them. A setting that the agent has no control for is
// left alone, and Applied says so. A control that does not know the value is an error, because
// running with another model or mode than the one asked for would be worse than not starting.
func (s *session) applySettings(
	ctx context.Context, spec agents.StartSpec, ctl controls,
) (agents.Applied, error) {
	var applied agents.Applied
	var err error
	if spec.Model != "" {
		applied.Model, err = s.applyOption(ctx, ctl, sdk.SessionConfigOptionCategoryModel,
			setting{what: "model", want: spec.Model})
		if err != nil {
			return applied, err
		}
	}
	if spec.Thinking != "" {
		want := mapped(s.adapter.cfg.ThinkingModes, spec.Thinking)
		applied.Thinking, err = s.applyOption(ctx, ctl, sdk.SessionConfigOptionCategoryThoughtLevel,
			setting{what: "thinking mode", want: want})
		if err != nil {
			return applied, err
		}
	}
	if spec.PermissionMode != "" {
		want := mapped(s.adapter.cfg.PermissionModes, spec.PermissionMode)
		applied.PermissionMode, err = s.applyMode(ctx, ctl, want)
	}
	return applied, err
}

// mapped returns what the agent calls a setting, or the setting itself.
func mapped(names map[string]string, value string) string {
	if name, ok := names[value]; ok {
		return name
	}
	return value
}

// setting is one setting that was asked for: what it is called in a message, and the value wanted.
type setting struct {
	what string
	want string
}

// applyOption sets a select option to the value that matches want. It returns false, with no
// error, when the agent has no such option.
func (s *session) applyOption(
	ctx context.Context, ctl controls, category sdk.SessionConfigOptionCategory, st setting,
) (bool, error) {
	sel := ctl.selectOption(category)
	if sel == nil {
		return false, nil
	}
	all := choices(sel)
	pick, ok := matchChoice(all, st.want)
	if !ok {
		return false, unsupported(st.what, st.want, choiceNames(all))
	}
	if pick.Value == sel.CurrentValue {
		return true, nil
	}
	_, err := s.conn.SetSessionConfigOption(ctx, sdk.SetSessionConfigOptionRequest{
		ValueId: &sdk.SetSessionConfigOptionValueId{
			ConfigId: sel.Id, SessionId: s.currentID(), Value: pick.Value,
		},
	})
	if err != nil {
		return false, fmt.Errorf("set the %s to %q: %w", st.what, st.want, err)
	}
	return true, nil
}

// applyMode sets the permission mode, through the session's modes or else through an option.
func (s *session) applyMode(ctx context.Context, ctl controls, want string) (bool, error) {
	if ctl.modes == nil {
		return s.applyOption(ctx, ctl, sdk.SessionConfigOptionCategoryMode,
			setting{what: "permission mode", want: want})
	}
	for _, m := range ctl.modes.AvailableModes {
		if !sameWord(want, string(m.Id)) && !sameWord(want, m.Name) {
			continue
		}
		if m.Id == ctl.modes.CurrentModeId {
			return true, nil
		}
		if _, err := s.conn.SetSessionMode(ctx, sdk.SetSessionModeRequest{
			SessionId: s.currentID(), ModeId: m.Id,
		}); err != nil {
			return false, fmt.Errorf("set the permission mode to %q: %w", want, err)
		}
		return true, nil
	}
	names := make([]string, 0, len(ctl.modes.AvailableModes))
	for _, m := range ctl.modes.AvailableModes {
		names = append(names, string(m.Id))
	}
	return false, unsupported("permission mode", want, names)
}

// currentID returns the agent's session id.
func (s *session) currentID() sdk.SessionId {
	return sdk.SessionId(s.sessionID())
}

// sameWord compares two words the way normalize does.
func sameWord(a, b string) bool {
	return normalize(a) == normalize(b)
}

// matchChoice finds the option whose value or name is the wanted word.
func matchChoice(all []sdk.SessionConfigSelectOption, want string) (sdk.SessionConfigSelectOption, bool) {
	for _, c := range all {
		if sameWord(want, string(c.Value)) || sameWord(want, c.Name) {
			return c, true
		}
	}
	return sdk.SessionConfigSelectOption{}, false
}

// choiceNames lists the values that an option offers, for a message.
func choiceNames(all []sdk.SessionConfigSelectOption) []string {
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, string(c.Value))
	}
	return names
}

// unsupported is the error for a setting whose value the agent does not offer.
func unsupported(what, want string, offered []string) error {
	return fmt.Errorf("%w: %s %q (it offers: %s)", agents.ErrUnsupportedSetting, what, want,
		strings.Join(offered, ", "))
}
