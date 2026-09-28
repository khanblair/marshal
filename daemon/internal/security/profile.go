package security

// Profile says what an agent may do at all, whatever its permission mode
// (docs/marshal-product-scope.md section 14.3). A mode says how often a person is asked; a profile
// says what is allowed even then. A field that is false refuses its action in every mode except
// bypass, and no mode can widen it.
//
// Solo use has one owner and no screen to change a profile yet, so the profile decides little today
// beyond what Marshal ships with. It exists now because the mode is not the only word a permission
// decision is made from, and because a card or a role that carries a narrower profile is a matter of
// setting one field, not of writing the check again.
type Profile struct {
	// ReadFiles allows reading and searching.
	ReadFiles bool
	// EditFiles allows writing, moving, and deleting files.
	EditFiles bool
	// RunCommands allows running a command that is not one of the narrower commands below.
	RunCommands bool
	// PushBranches allows pushing to a remote.
	PushBranches bool
	// InstallPackages allows installing or updating a dependency.
	InstallPackages bool
	// Network allows reaching out to a network address.
	Network bool
	// Deploys allows running a deploy. Unlike the rest, a deploy is still sent to a person by the
	// deploy rule even when the profile allows it, in every mode except bypass.
	Deploys bool
}

// DefaultProfile is the profile Marshal ships with: everything a person's own agent normally does.
// It is not a permissive accident - the checks that matter are the blocklist and the deploy rule,
// which apply on top of it in every mode except bypass.
func DefaultProfile() Profile {
	return Profile{
		ReadFiles: true, EditFiles: true, RunCommands: true, PushBranches: true,
		InstallPackages: true, Network: true, Deploys: true,
	}
}

// NothingAllowed is a profile that refuses every action it covers. It is the shape a test uses to
// show that a profile is enforced at all.
func NothingAllowed() Profile { return Profile{} }

// Check says whether the profile allows an action. It reports false when the profile has nothing to
// say about the action, which leaves the decision to the mode and the rest of the harness.
func (p Profile) Check(a Action) (Finding, bool) {
	// Every case below sets allowed itself, and the switch's default returns before it is ever
	// read - there is no case that relies on a starting value.
	var allowed bool
	switch a {
	case ActionRead:
		allowed = p.ReadFiles
	case ActionEdit:
		allowed = p.EditFiles
	case ActionCommand:
		allowed = p.RunCommands
	case ActionPush:
		allowed = p.PushBranches
	case ActionInstall:
		allowed = p.InstallPackages
	case ActionNetwork:
		allowed = p.Network
	case ActionDeploy:
		allowed = p.Deploys
	default:
		return Finding{}, false
	}
	if allowed {
		return Finding{}, false
	}
	return Finding{Verdict: VerdictBlock, Rule: RuleProfile}, true
}
