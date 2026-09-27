package protocol

import "time"

// The wire shape of the code-smell checks (docs/architecture.md section 17, docs/backend-checklist.md
// B5.8, build-plan 5.20, docs/marshal-product-scope.md section 15.4). A finding is one smell the
// checks found in one card's diff: where it is, what kind it is, how much it matters, why, and what
// to do about it. A profile is one project's own version of the checks: which run, at what
// thresholds, and which block.
//
// A finding is read from the daemon and acted on through its own two calls (ask the agent to fix it,
// or dismiss it with a reason), never kept on the client alone, because a finding is a fact about
// the commit it was found in and a client that decided on its own that one was fixed would disagree
// with the daemon the next time it re-read the card.

// SmellFamily is the kind a smell belongs to: the nine families the product scope groups smells
// into (section 15.4), following the catalog by Jerzyk and Madeyski (2023). The family is fixed
// per check, so a screen can group a card's findings by family without knowing the checks.
type SmellFamily string

const (
	// SmellFamilyBloaters is code that has grown too large: long functions, large files, long
	// parameter lists.
	SmellFamilyBloaters SmellFamily = "bloaters"
	// SmellFamilyChangePreventers is one change that forces edits in many places: shotgun surgery,
	// divergent change.
	SmellFamilyChangePreventers SmellFamily = "change-preventers"
	// SmellFamilyCouplers is too much dependence between parts: feature envy, reaching into another
	// module's internals, long message chains.
	SmellFamilyCouplers SmellFamily = "couplers"
	// SmellFamilyDataDealers is data passed around more than it needs to be: middle men, global
	// mutable data.
	SmellFamilyDataDealers SmellFamily = "data-dealers"
	// SmellFamilyDispensables is code that could be removed: dead code, duplicated code,
	// speculative generality.
	SmellFamilyDispensables SmellFamily = "dispensables"
	// SmellFamilyFunctionalAbusers is hidden changes in state: mutations and side effects a name
	// does not suggest.
	SmellFamilyFunctionalAbusers SmellFamily = "functional-abusers"
	// SmellFamilyLexicalAbusers is names and comments that mislead: magic numbers, mysterious names,
	// outdated comments.
	SmellFamilyLexicalAbusers SmellFamily = "lexical-abusers"
	// SmellFamilyObfuscators is code harder to follow than it needs to be: complex boolean
	// expressions, clever code, deep nesting.
	SmellFamilyObfuscators SmellFamily = "obfuscators"
	// SmellFamilyObjectOrientedAbusers is poor use of types and inheritance: repeated switches on
	// the same value, similar types with different method names.
	SmellFamilyObjectOrientedAbusers SmellFamily = "object-oriented-abusers"
)

// SmellFamilyValues lists every smell family, in the order the product scope names them.
func SmellFamilyValues() []SmellFamily {
	return []SmellFamily{
		SmellFamilyBloaters, SmellFamilyChangePreventers, SmellFamilyCouplers,
		SmellFamilyDataDealers, SmellFamilyDispensables, SmellFamilyFunctionalAbusers,
		SmellFamilyLexicalAbusers, SmellFamilyObfuscators, SmellFamilyObjectOrientedAbusers,
	}
}

// Valid reports whether f is a smell family.
func (f SmellFamily) Valid() bool {
	for _, known := range SmellFamilyValues() {
		if known == f {
			return true
		}
	}
	return false
}

// SmellCheck is one of Marshal's own built-in checks. A check's name is what a finding's Smell
// carries for a built-in finding, and what a project's smell profile turns on and off. A project
// linter's findings carry the linter's own rule name instead, which is why SmellFinding.Smell is a
// plain string and not this type.
type SmellCheck string

const (
	// SmellCheckLongFunction is a function that is far longer than the profile allows.
	SmellCheckLongFunction SmellCheck = "long-function"
	// SmellCheckLargeFile is a file that is far longer than the profile allows.
	SmellCheckLargeFile SmellCheck = "large-file"
	// SmellCheckLongParameterList is a function with more parameters than the profile allows.
	SmellCheckLongParameterList SmellCheck = "long-parameter-list"
	// SmellCheckDeepNesting is a block nested deeper than the profile allows.
	SmellCheckDeepNesting SmellCheck = "deep-nesting"
	// SmellCheckLongLine is a line longer than the profile allows.
	SmellCheckLongLine SmellCheck = "long-line"
	// SmellCheckMagicNumber is a bare number in code that should be a named constant.
	SmellCheckMagicNumber SmellCheck = "magic-number"
	// SmellCheckDuplicateBlock is a block of lines that appears more than once in the card's own
	// new code.
	SmellCheckDuplicateBlock SmellCheck = "duplicate-block"
)

// SmellCheckValues lists every built-in check.
func SmellCheckValues() []SmellCheck {
	return []SmellCheck{
		SmellCheckLongFunction, SmellCheckLargeFile, SmellCheckLongParameterList,
		SmellCheckDeepNesting, SmellCheckLongLine, SmellCheckMagicNumber, SmellCheckDuplicateBlock,
	}
}

// Valid reports whether c is a built-in check.
func (c SmellCheck) Valid() bool {
	for _, known := range SmellCheckValues() {
		if known == c {
			return true
		}
	}
	return false
}

// SmellSeverity is how much a finding matters.
type SmellSeverity string

const (
	// SmellSeverityBlocking is a finding that stops the card: it goes back to the agent to fix
	// before the card can move to review.
	SmellSeverityBlocking SmellSeverity = "blocking"
	// SmellSeverityWarning is a finding shown to the Reviewer and to the person, that does not stop
	// the card.
	SmellSeverityWarning SmellSeverity = "warning"
	// SmellSeverityInfo is a finding shown only in the card's checks.
	SmellSeverityInfo SmellSeverity = "info"
)

// SmellSeverityValues lists every severity, from the one that matters most to the least.
func SmellSeverityValues() []SmellSeverity {
	return []SmellSeverity{SmellSeverityBlocking, SmellSeverityWarning, SmellSeverityInfo}
}

// Valid reports whether s is a smell severity.
func (s SmellSeverity) Valid() bool {
	for _, known := range SmellSeverityValues() {
		if known == s {
			return true
		}
	}
	return false
}

// SmellStatus is what became of a finding.
type SmellStatus string

const (
	// SmellStatusOpen is a finding nobody has acted on.
	SmellStatusOpen SmellStatus = "open"
	// SmellStatusFixed is a finding whose agent was asked to fix it and the code changed.
	SmellStatusFixed SmellStatus = "fixed"
	// SmellStatusDismissed is a finding a person waved away, with a reason.
	SmellStatusDismissed SmellStatus = "dismissed"
)

// SmellStatusValues lists every status.
func SmellStatusValues() []SmellStatus {
	return []SmellStatus{SmellStatusOpen, SmellStatusFixed, SmellStatusDismissed}
}

// Valid reports whether s is a smell status.
func (s SmellStatus) Valid() bool {
	for _, known := range SmellStatusValues() {
		if known == s {
			return true
		}
	}
	return false
}

// SmellFinding is one smell the checks found in one card's diff.
type SmellFinding struct {
	// ID is the finding's own opaque id. The calls that act on a finding address it.
	ID string `json:"id"`
	// CardID is the card the finding is about.
	CardID string `json:"cardId"`
	// Commit is the commit the finding was found in. It is empty for a finding from uncommitted
	// work. An answer is cached per commit, and a finding in an older commit is never re-blamed on
	// a newer one.
	Commit string `json:"commit"`
	// Family is the kind of smell the finding is.
	Family SmellFamily `json:"family"`
	// Smell is the rule's own name: a built-in check's name, or a project linter's rule.
	Smell string `json:"smell"`
	// File is the file the finding is in, relative to the repository, with forward slashes.
	File string `json:"file"`
	// Line is the line the finding is on. Zero when the finding is about the whole file.
	Line int `json:"line"`
	// Severity is how much the finding matters.
	Severity SmellSeverity `json:"severity"`
	// Message says why it matters, in one sentence.
	Message string `json:"message"`
	// Suggestion is the refactoring to make, in one sentence.
	Suggestion string `json:"suggestion"`
	// Status is open, fixed, or dismissed.
	Status SmellStatus `json:"status"`
	// DismissReason is why a person waved the finding away. Empty unless Status is dismissed.
	DismissReason string `json:"dismissReason"`
}

// SmellFindingList is the answer to GET /v1/cards/{id}/findings: the findings of one card, as of one
// commit, newest first is not guaranteed, so a client that cares sorts them.
type SmellFindingList struct {
	// CardID is the card the list is about.
	CardID string `json:"cardId"`
	// Commit is the commit the findings are as of, empty when they came from uncommitted work.
	Commit string `json:"commit"`
	// Findings are the card's findings. Never null.
	Findings []SmellFinding `json:"findings"`
	// Blocking is how many of Findings block the card. It is carried so a screen can say the
	// number without counting, and so the move to review can be refused without a second read.
	Blocking int `json:"blocking"`
	// Checked is when the checks last ran for this card, or null when they never have: a card that
	// has not been checked yet is not the same as one checked at the beginning of time.
	Checked *Timestamp `json:"checked"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewSmellFindingList makes an answer stamped with the daemon's time. A nil list becomes an empty
// one, so the JSON has [] and never null.
func NewSmellFindingList(cardID, commit string, findings []SmellFinding, checked, now time.Time) SmellFindingList {
	out := make([]SmellFinding, len(findings))
	copy(out, findings)
	blocking := 0
	for _, finding := range out {
		if finding.Status == SmellStatusOpen && finding.Severity == SmellSeverityBlocking {
			blocking++
		}
	}
	answer := SmellFindingList{
		CardID: cardID, Commit: commit, Findings: out, Blocking: blocking,
		ServerTime: NewTimestamp(now),
	}
	if !checked.IsZero() {
		when := NewTimestamp(checked)
		answer.Checked = &when
	}
	return answer
}

// DismissFindingRequest is the body of POST /v1/cards/{id}/findings/{findingId}/dismiss. The reason
// is required: a finding is waved away on purpose, and the reason is what the auto lessons of a
// later phase learn from, so an empty one is refused.
type DismissFindingRequest struct {
	// Reason is why the finding is being dismissed, in the person's own words.
	Reason string `json:"reason"`
}

// SmellCheckedEventData is the payload of quality.checked (architecture.md 11.2): the card's
// findings as they now stand, so a client redraws the card's checks panel from one event and never
// asks again for what the event already carries.
type SmellCheckedEventData struct {
	// CardID is the card whose checks finished.
	CardID string `json:"cardId"`
	// Commit is the commit the checks ran for.
	Commit string `json:"commit"`
	// Findings are the card's findings for that commit.
	Findings []SmellFinding `json:"findings"`
	// Blocking is how many of them block the card.
	Blocking int `json:"blocking"`
}

// SmellCheckSetting is one built-in check as a project has set it: whether it runs, and how much
// its findings matter.
type SmellCheckSetting struct {
	// Check is the built-in check this setting is for.
	Check SmellCheck `json:"check"`
	// Enabled says whether the check runs. A check that is off produces no findings at all.
	Enabled bool `json:"enabled"`
	// Severity is how much a finding from this check matters.
	Severity SmellSeverity `json:"severity"`
}

// SmellProfile is one project's own version of the smell checks (architecture.md section 17.3,
// product scope 15.4). It is kept as one JSON document per project, and a project with no profile
// uses DefaultSmellProfile. It is language-aware through its checks: a check that does not fit a
// file's language is not applied to that file, so one profile serves a project of several
// languages.
//
// These are the project's own thresholds, and never Marshal's: Marshal's `.golangci.yml` and
// `.jscpd.json` are thresholds for Marshal's own code, not a smell profile for arbitrary
// agent-written code in a card's project.
type SmellProfile struct {
	// ProjectID is the project the profile is for.
	ProjectID string `json:"projectId"`
	// MaxFunctionLines is the longest a new function may be before it is flagged.
	MaxFunctionLines int `json:"maxFunctionLines"`
	// MaxFileLines is the longest a file may be before it is flagged.
	MaxFileLines int `json:"maxFileLines"`
	// MaxParameters is the most parameters a new function may have.
	MaxParameters int `json:"maxParameters"`
	// MaxNesting is the deepest a new block may nest.
	MaxNesting int `json:"maxNesting"`
	// MaxLineLength is the longest a new line may be.
	MaxLineLength int `json:"maxLineLength"`
	// DuplicateBlockLines is how many lines in a row must repeat before the copy is called a
	// duplicated block.
	DuplicateBlockLines int `json:"duplicateBlockLines"`
	// Checks says which built-in checks run and how much each one matters. A check the list does
	// not name uses its default.
	Checks []SmellCheckSetting `json:"checks"`
	// Linters are the project's own linters, if it has them (product scope 15.4, layer 1). Each is
	// a program Marshal runs in the card's worktree, on the files the card changed, with the
	// project's own configuration. A project with no linters has none, which is the usual case for
	// a fresh install: the built-in checks run either way.
	Linters []SmellLinter `json:"linters,omitempty"`
}

// SmellLinter is one of a project's own linters. Marshal runs its command in the card's worktree
// with the changed files appended, reads the `file:line: message` lines it prints, and files what
// it finds under the family named here, because a linter's own rules do not map onto Marshal's nine
// families on their own.
type SmellLinter struct {
	// Name labels the linter in a finding's message, such as "golangci-lint".
	Name string `json:"name"`
	// Command is the program and its arguments. The changed files are appended as the last
	// arguments, and it is run directly and never through a shell.
	Command []string `json:"command"`
	// Family is the smell family this linter's findings are filed under.
	Family SmellFamily `json:"family"`
}

// DefaultSmellProfile is what a project with no profile of its own is checked with. Only the few
// clear cases block - a new function or file far over the length limit, and a large duplicated
// block - and everything else warns, which is what the product scope asks for (section 15.4).
func DefaultSmellProfile() SmellProfile {
	return SmellProfile{
		MaxFunctionLines:    80,
		MaxFileLines:        800,
		MaxParameters:       5,
		MaxNesting:          4,
		MaxLineLength:       160,
		DuplicateBlockLines: 6,
		Checks: []SmellCheckSetting{
			{Check: SmellCheckLongFunction, Enabled: true, Severity: SmellSeverityBlocking},
			{Check: SmellCheckLargeFile, Enabled: true, Severity: SmellSeverityBlocking},
			{Check: SmellCheckDuplicateBlock, Enabled: true, Severity: SmellSeverityBlocking},
			{Check: SmellCheckLongParameterList, Enabled: true, Severity: SmellSeverityWarning},
			{Check: SmellCheckDeepNesting, Enabled: true, Severity: SmellSeverityWarning},
			{Check: SmellCheckLongLine, Enabled: true, Severity: SmellSeverityWarning},
			{Check: SmellCheckMagicNumber, Enabled: true, Severity: SmellSeverityWarning},
		},
	}
}
