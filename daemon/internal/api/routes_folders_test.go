package api_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The folder browser (GET /v1/folders): folders only, hidden ones left out, sorted, with a mark on
// the ones that hold a Git repository.

func TestFoldersListsOnlyVisibleFoldersByNameAndMarksRepositories(t *testing.T) {
	st := newStack(t)
	root := t.TempDir()
	for _, dir := range []string{"beta", "Alpha", ".hidden", "repo/.git"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := decode[protocol.FolderListing](t, st.do(http.MethodGet, "/v1/folders?path="+url.QueryEscape(root), nil).want(t, http.StatusOK))
	names := []string{}
	for _, f := range got.Folders {
		names = append(names, f.Name)
	}
	if len(names) != 3 || names[0] != "Alpha" || names[1] != "beta" || names[2] != "repo" {
		t.Fatalf("folders = %v", names)
	}
	if !got.Folders[2].IsGitRepo || got.Folders[0].IsGitRepo {
		t.Fatalf("repo marks = %+v", got.Folders)
	}
	if got.Parent != filepath.Dir(root) || got.Path == "" || got.Home == "" {
		t.Fatalf("listing = %+v", got)
	}
}

func TestFoldersStartsAtTheHomeFolderAndKnowsARepositoryItself(t *testing.T) {
	st := newStack(t)
	home, _ := os.UserHomeDir()
	got := decode[protocol.FolderListing](t, st.do(http.MethodGet, "/v1/folders", nil).want(t, http.StatusOK))
	if got.Path != home {
		t.Fatalf("path = %q, want %q", got.Path, home)
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := decode[protocol.FolderListing](t, st.do(http.MethodGet, "/v1/folders?path="+url.QueryEscape(repo), nil).want(t, http.StatusOK))
	if !inside.IsGitRepo || inside.Folders == nil {
		t.Fatalf("listing = %+v", inside)
	}
}

func TestFoldersRefusesWhatIsNotAFolderYouCanOpen(t *testing.T) {
	st := newStack(t)
	file := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	st.do(http.MethodGet, "/v1/folders?path="+url.QueryEscape(filepath.Join(t.TempDir(), "nope")), nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	st.do(http.MethodGet, "/v1/folders?path=relative/dir", nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	st.do(http.MethodGet, "/v1/folders?path="+url.QueryEscape(file), nil).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}
