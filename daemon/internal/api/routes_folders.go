package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// getFolders is GET /v1/folders?path=: the folders inside one folder of this computer, for the
// screens that ask for a repository's folder from a page that cannot open the system's own dialog.
// It answers folders only, never files, and leaves out the hidden ones. With no path it lists the
// person's home folder. It reads the disk as the daemon's own user, behind the same token as every
// other route.
func (s *Server) getFolders(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		s.writeError(w, protocol.Unavailable("Marshal cannot find your home folder."))
		return
	}
	dir, err := folderToList(r.URL.Query().Get("path"), home)
	if err != nil {
		s.writeError(w, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.writeError(w, folderReadError(err, dir))
		return
	}
	out := protocol.FolderListing{
		Path: dir, Home: home, IsGitRepo: isGitRepo(dir), Folders: []protocol.FolderEntry{},
		ServerTime: protocol.NewTimestamp(s.now()),
	}
	if parent := filepath.Dir(dir); parent != dir {
		out.Parent = parent
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || !isDirectory(dir, entry) {
			continue
		}
		if len(out.Folders) == protocol.MaxFolderEntries {
			out.Truncated = true
			break
		}
		path := filepath.Join(dir, entry.Name())
		out.Folders = append(out.Folders, protocol.FolderEntry{Name: entry.Name(), Path: path, IsGitRepo: isGitRepo(path)})
	}
	sort.Slice(out.Folders, func(i, j int) bool {
		return strings.ToLower(out.Folders[i].Name) < strings.ToLower(out.Folders[j].Name)
	})
	s.writeJSON(w, http.StatusOK, out)
}

// folderToList turns the asked path into a clean full path: empty is the home folder, a leading ~
// is the home folder, and anything else has to be a full path already.
func folderToList(asked, home string) (string, error) {
	asked = strings.TrimSpace(asked)
	switch {
	case asked == "" || asked == "~":
		return home, nil
	case strings.HasPrefix(asked, "~/"):
		return filepath.Join(home, asked[2:]), nil
	case !filepath.IsAbs(asked):
		return "", protocol.InvalidArgument("Give the full path of a folder, such as /Users/you/code.")
	}
	return filepath.Clean(asked), nil
}

// isDirectory is true for a folder, and for a link that leads to one.
func isDirectory(dir string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&fs.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, entry.Name()))
	return err == nil && info.IsDir()
}

// isGitRepo is true when the folder has a .git entry: a folder for an ordinary repository, a file
// for a worktree or a submodule.
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func folderReadError(err error, dir string) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return protocol.NotFound("folder").With("path", dir)
	case errors.Is(err, fs.ErrPermission):
		return protocol.Refused("Marshal is not allowed to open that folder.").With("path", dir)
	}
	return protocol.InvalidArgument("That is not a folder Marshal can open.").With("path", dir)
}
