package protocol

// The wire shape of the folder browser (docs/backend-checklist.md B1.x, N3): where a person picks the
// folder of a repository from, when the page they use cannot open the operating system's own folder
// dialog. The daemon lists the folders of its own computer, because that is where the repository is.

// MaxFolderEntries is the most folders one listing carries. A folder with more is cut, and Truncated
// says so.
const MaxFolderEntries = 500

// FolderEntry is one folder inside the listed one.
type FolderEntry struct {
	// Name is the folder's own name.
	Name string `json:"name"`
	// Path is the folder's full path, which the next listing asks for.
	Path string `json:"path"`
	// IsGitRepo is true when the folder holds a Git repository.
	IsGitRepo bool `json:"isGitRepo"`
}

// FolderListing is the answer to GET /v1/folders?path=.
type FolderListing struct {
	// Path is the folder that was listed, as a full path.
	Path string `json:"path"`
	// Parent is the folder above it, or empty at the top of the disk.
	Parent string `json:"parent"`
	// Home is the person's home folder, where a browser starts.
	Home string `json:"home"`
	// IsGitRepo is true when the listed folder itself holds a Git repository.
	IsGitRepo bool `json:"isGitRepo"`
	// Folders are the folders inside it, by name, with the hidden ones left out. Never null.
	Folders []FolderEntry `json:"folders"`
	// Truncated is true when the folder holds more than MaxFolderEntries folders.
	Truncated bool `json:"truncated"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}
