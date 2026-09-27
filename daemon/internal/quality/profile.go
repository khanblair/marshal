package quality

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The bounds a project's smell profile may be set to. They are generous: the point is to stop a
// paste that would make a check meaningless (a line cap of 0, a function cap of a million), not to
// shape the numbers a project picks for its own code.
const (
	minFunctionLines   = 5
	maxFunctionLines   = 5000
	minFileLines       = 20
	maxFileLines       = 200_000
	minParameters      = 1
	maxParameters      = 50
	minNesting         = 1
	maxNesting         = 20
	minLineLength      = 40
	maxLineLength      = 2000
	minDuplicateLines  = 3
	maxDuplicateLines  = 100
	maxProfileJSONSize = 64 << 10
	// A project may name a few of its own linters, but not an unbounded number: each one is a
	// program run on every check, and the point of the profile is the bounds.
	maxLinters      = 8
	maxLinterName   = 60
	maxCommandParts = 32
	maxCommandPart  = 500
)

// familyOf says which of the nine smell families a built-in check belongs to. The family is fixed
// per check, so the mapping is here and not in the profile: a project chooses how much a check
// matters, never what kind of smell it is.
func familyOf(check protocol.SmellCheck) protocol.SmellFamily {
	switch check {
	case protocol.SmellCheckLongFunction, protocol.SmellCheckLargeFile, protocol.SmellCheckLongParameterList:
		return protocol.SmellFamilyBloaters
	case protocol.SmellCheckDeepNesting, protocol.SmellCheckLongLine:
		return protocol.SmellFamilyObfuscators
	case protocol.SmellCheckMagicNumber:
		return protocol.SmellFamilyLexicalAbusers
	case protocol.SmellCheckDuplicateBlock:
		return protocol.SmellFamilyDispensables
	default:
		return protocol.SmellFamilyBloaters
	}
}

// defaultSettingOf is a check's setting when a profile does not name it: the default profile's own
// answer. A check the defaults turn on stays on for a project that never mentioned it.
func defaultSettingOf(check protocol.SmellCheck) protocol.SmellCheckSetting {
	for _, setting := range protocol.DefaultSmellProfile().Checks {
		if setting.Check == check {
			return setting
		}
	}
	return protocol.SmellCheckSetting{Check: check, Enabled: false, Severity: protocol.SmellSeverityWarning}
}

// settingOf reads one check's setting out of a resolved profile, falling back to the default when
// the profile does not name it. A profile only ever names the checks a project changed.
func settingOf(profile protocol.SmellProfile, check protocol.SmellCheck) protocol.SmellCheckSetting {
	for _, setting := range profile.Checks {
		if setting.Check == check {
			return setting
		}
	}
	return defaultSettingOf(check)
}

// resolveProfile fills in every threshold and check a stored profile left out, so the rest of the
// module reads one complete profile and never a zero. A profile that names a check overrides the
// default for that check and leaves the others as they were.
func resolveProfile(projectID string, stored *protocol.SmellProfile) protocol.SmellProfile {
	resolved := protocol.DefaultSmellProfile()
	resolved.ProjectID = projectID
	if stored == nil {
		return resolved
	}
	if stored.MaxFunctionLines > 0 {
		resolved.MaxFunctionLines = stored.MaxFunctionLines
	}
	if stored.MaxFileLines > 0 {
		resolved.MaxFileLines = stored.MaxFileLines
	}
	if stored.MaxParameters > 0 {
		resolved.MaxParameters = stored.MaxParameters
	}
	if stored.MaxNesting > 0 {
		resolved.MaxNesting = stored.MaxNesting
	}
	if stored.MaxLineLength > 0 {
		resolved.MaxLineLength = stored.MaxLineLength
	}
	if stored.DuplicateBlockLines > 0 {
		resolved.DuplicateBlockLines = stored.DuplicateBlockLines
	}
	if len(stored.Checks) > 0 {
		byName := map[protocol.SmellCheck]protocol.SmellCheckSetting{}
		for _, setting := range resolved.Checks {
			byName[setting.Check] = setting
		}
		for _, setting := range stored.Checks {
			byName[setting.Check] = setting
		}
		merged := make([]protocol.SmellCheckSetting, 0, len(protocol.SmellCheckValues()))
		for _, check := range protocol.SmellCheckValues() {
			if setting, ok := byName[check]; ok {
				merged = append(merged, setting)
			}
		}
		resolved.Checks = merged
	}
	// A project's own linters are its first layer of checks and have no default of their own: the
	// built-in profile names none, so whatever a project saved is what runs.
	if len(stored.Linters) > 0 {
		resolved.Linters = stored.Linters
	}
	return resolved
}

// checkProfile refuses a profile that cannot be used. A field left at zero is filled from the
// defaults rather than refused, so a client can send only what it changed; a field outside its
// bounds, an unknown check, an unknown severity, or the same check twice is refused.
func checkProfile(projectID string, in protocol.SmellProfile) (protocol.SmellProfile, error) {
	out := in
	out.ProjectID = projectID
	if err := checkBound("maxFunctionLines", in.MaxFunctionLines, minFunctionLines, maxFunctionLines); err != nil {
		return protocol.SmellProfile{}, err
	}
	if err := checkBound("maxFileLines", in.MaxFileLines, minFileLines, maxFileLines); err != nil {
		return protocol.SmellProfile{}, err
	}
	if err := checkBound("maxParameters", in.MaxParameters, minParameters, maxParameters); err != nil {
		return protocol.SmellProfile{}, err
	}
	if err := checkBound("maxNesting", in.MaxNesting, minNesting, maxNesting); err != nil {
		return protocol.SmellProfile{}, err
	}
	if err := checkBound("maxLineLength", in.MaxLineLength, minLineLength, maxLineLength); err != nil {
		return protocol.SmellProfile{}, err
	}
	if err := checkBound("duplicateBlockLines", in.DuplicateBlockLines, minDuplicateLines, maxDuplicateLines); err != nil {
		return protocol.SmellProfile{}, err
	}
	seen := map[protocol.SmellCheck]bool{}
	checks := make([]protocol.SmellCheckSetting, 0, len(in.Checks))
	for _, setting := range in.Checks {
		if !setting.Check.Valid() {
			return protocol.SmellProfile{}, protocol.InvalidArgument(
				fmt.Sprintf("%q is not a smell check.", string(setting.Check))).
				With("field", "checks").With("check", string(setting.Check))
		}
		if seen[setting.Check] {
			return protocol.SmellProfile{}, protocol.InvalidArgument(
				fmt.Sprintf("The check %q is listed twice.", string(setting.Check))).
				With("field", "checks").With("check", string(setting.Check))
		}
		seen[setting.Check] = true
		if !setting.Severity.Valid() {
			return protocol.SmellProfile{}, protocol.InvalidArgument(
				fmt.Sprintf("%q is not a severity.", string(setting.Severity))).
				With("field", "checks").With("severity", string(setting.Severity))
		}
		checks = append(checks, setting)
	}
	out.Checks = checks
	linters, err := checkLinters(in.Linters)
	if err != nil {
		return protocol.SmellProfile{}, err
	}
	out.Linters = linters
	return out, nil
}

// checkLinters refuses a linter a project could not safely have run: one with no name, one with no
// program, one with too many arguments, one with an argument that is absurdly long, or one whose
// family is not a smell family. The order is kept, so a project's own list reads back as it was set.
func checkLinters(in []protocol.SmellLinter) ([]protocol.SmellLinter, error) {
	if len(in) > maxLinters {
		return nil, protocol.InvalidArgument(
			fmt.Sprintf("A smell profile may name at most %d linters.", maxLinters)).With("field", "linters")
	}
	out := make([]protocol.SmellLinter, 0, len(in))
	for _, linter := range in {
		name := strings.TrimSpace(linter.Name)
		if name == "" || len(name) > maxLinterName {
			return nil, protocol.InvalidArgument("A linter needs a name of up to 60 characters.").
				With("field", "linters")
		}
		if len(linter.Command) == 0 || len(linter.Command) > maxCommandParts {
			return nil, protocol.InvalidArgument("A linter needs a program to run.").
				With("field", "linters").With("linter", name)
		}
		command := make([]string, 0, len(linter.Command))
		for _, part := range linter.Command {
			if strings.TrimSpace(part) == "" || len(part) > maxCommandPart {
				return nil, protocol.InvalidArgument("A linter's command holds an argument Marshal will not run.").
					With("field", "linters").With("linter", name)
			}
			command = append(command, part)
		}
		family := linter.Family
		if family == "" {
			family = protocol.SmellFamilyLexicalAbusers
		}
		if !family.Valid() {
			return nil, protocol.InvalidArgument(
				fmt.Sprintf("%q is not a smell family.", string(linter.Family))).
				With("field", "linters").With("family", string(linter.Family))
		}
		out = append(out, protocol.SmellLinter{Name: name, Command: command, Family: family})
	}
	return out, nil
}

// checkBound refuses a number that is set but out of range. Zero is left for resolveProfile.
func checkBound(field string, value, low, high int) error {
	if value == 0 {
		return nil
	}
	if value < low || value > high {
		return protocol.InvalidArgument(
			fmt.Sprintf("%s must be between %d and %d.", field, low, high)).
			With("field", field)
	}
	return nil
}

// encodeProfile turns a profile into the JSON stored in smell_profiles.spec_json.
func encodeProfile(profile protocol.SmellProfile) (string, error) {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encode the smell profile: %w", err)
	}
	if len(encoded) > maxProfileJSONSize {
		return "", protocol.InvalidArgument("The smell profile is too large.").With("field", "profile")
	}
	return string(encoded), nil
}

// decodeProfile reads the JSON a project saved. A row that cannot be read is an error the caller
// turns into a plain sentence: a profile is written by this module, so a row that will not parse is
// a fault and not something a person did.
func decodeProfile(projectID, raw string) (protocol.SmellProfile, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return protocol.SmellProfile{}, nil
	}
	var stored protocol.SmellProfile
	if err := json.Unmarshal([]byte(trimmed), &stored); err != nil {
		return protocol.SmellProfile{}, fmt.Errorf("read the smell profile of project %s: %w", projectID, err)
	}
	return stored, nil
}
