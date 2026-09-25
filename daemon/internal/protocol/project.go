package protocol

import "slices"

// ProjectSource says where a new project comes from.
type ProjectSource string

const (
	// ProjectSourceFolder is a repository that is already on this machine. Path names its top folder.
	ProjectSourceFolder ProjectSource = "folder"
	// ProjectSourceClone is a repository to copy from a server into Dest first.
	ProjectSourceClone ProjectSource = "clone"
)

// ProjectSourceValues lists every project source.
func ProjectSourceValues() []ProjectSource {
	return []ProjectSource{ProjectSourceFolder, ProjectSourceClone}
}

// Valid reports whether s is a project source.
func (s ProjectSource) Valid() bool { return slices.Contains(ProjectSourceValues(), s) }

// ProjectBadges are the counts the sidebar shows beside a project. The daemon works them out from
// the project's cards each time, so they are never stored.
type ProjectBadges struct {
	// Needs is how many cards wait for a person ("Needs you").
	Needs int `json:"needs"`
	// Awake is how many cards have a running agent. It is 0 until agent sessions exist.
	Awake int `json:"awake"`
}

// Project is a repository that Marshal manages, as clients see it.
type Project struct {
	// ID is the project's short id, made from its name when it was added. It never changes.
	ID string `json:"id"`
	// Name is the display name. Renaming changes only this.
	Name string `json:"name"`
	// Path is the top folder of the repository on the machine that runs the daemon.
	Path string `json:"path"`
	// Language is what the daemon found in the repository, for example "Go" or "TypeScript", or
	// "Monorepo" for a repository with several packages. "Unknown" when nothing matched.
	Language string `json:"language"`
	// DefaultBranch is the branch new work starts from.
	DefaultBranch string `json:"defaultBranch"`
	// DevCommand is the command that starts the project's dev server. It is a guess when the
	// project is added, it may be empty, and the person can change it.
	DevCommand string `json:"devCommand"`
	// BypassLocked is true when the project does not allow the bypass permission mode.
	BypassLocked bool `json:"bypassLocked"`
	// IsMonorepo is true when the repository has several packages.
	IsMonorepo bool `json:"isMonorepo"`
	// Packages are the package folders of a monorepo, relative to Path, with forward slashes and
	// in name order. Empty for other projects.
	Packages []string `json:"packages"`
	// CreatedAt is when the project was added to Marshal.
	CreatedAt Timestamp `json:"createdAt"`
	// Badges are the counts for the sidebar.
	Badges ProjectBadges `json:"badges"`
}

// ProjectListSnapshot is the answer to GET /v1/projects: every project, oldest first.
type ProjectListSnapshot struct {
	// Projects are all the projects Marshal manages.
	Projects []Project `json:"projects"`
	// ServerTime is the daemon's time when the list was made.
	ServerTime Timestamp `json:"serverTime"`
}

// CreateProjectRequest is the body of POST /v1/projects. Send Path for a folder, or URL and Dest
// for a clone.
type CreateProjectRequest struct {
	// Source says which of the fields below are used.
	Source ProjectSource `json:"source"`
	// Path is the top folder of a repository on this machine. Used when Source is "folder".
	Path string `json:"path,omitempty"`
	// URL is the address to clone from. Used when Source is "clone".
	URL string `json:"url,omitempty"`
	// Dest is the folder to clone into. It must be new or empty, and outside every project.
	Dest string `json:"dest,omitempty"`
	// Branch is the branch to check out when cloning. Empty means the repository's default.
	Branch string `json:"branch,omitempty"`
	// Name is the display name. Empty means the name of the repository's folder.
	Name string `json:"name,omitempty"`
}

// UpdateProjectRequest is the body of PATCH /v1/projects/{id}. A field that is left out is not
// changed. Changing the name never moves the folder or renames a branch.
type UpdateProjectRequest struct {
	// Name is the new display name. It cannot be empty.
	Name *string `json:"name,omitempty"`
	// DevCommand is the new dev command. An empty string clears it.
	DevCommand *string `json:"devCommand,omitempty"`
	// DefaultBranch is the new default branch. It must exist in the repository.
	DefaultBranch *string `json:"defaultBranch,omitempty"`
	// BypassLocked turns the bypass lock on or off.
	BypassLocked *bool `json:"bypassLocked,omitempty"`
}

// RemoveProjectRequest says what to keep when a project is removed from Marshal. The repository
// folder is never deleted, whatever these say.
type RemoveProjectRequest struct {
	// KeepBranches keeps the branches Marshal made for the project's cards. When false they are
	// deleted, including the ones with work that is not merged.
	KeepBranches bool `json:"keepBranches"`
	// KeepMemory keeps the project's memory folder. When false the folder is deleted.
	KeepMemory bool `json:"keepMemory"`
}
