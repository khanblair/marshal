package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// addProject adds a fresh copy of a fixture repository as a project, through the API, and returns
// the project and the path of the copy.
func (st *stack) addProject(fixture string) (protocol.Project, string) {
	st.t.Helper()
	path := testutil.Fixture(st.t, fixture)
	return st.addProjectAt(path), path
}

// addProjectAt adds the repository at path as a project, through the API.
func (st *stack) addProjectAt(path string) protocol.Project {
	st.t.Helper()
	body := protocol.CreateProjectRequest{Source: protocol.ProjectSourceFolder, Path: path}
	r := st.do(http.MethodPost, "/v1/projects", body).want(st.t, http.StatusCreated)
	return decode[protocol.Project](st.t, r)
}

// addCard adds a card with a title to a project, through the API.
func (st *stack) addCard(projectID, title string) protocol.Card {
	st.t.Helper()
	r := st.do(http.MethodPost, "/v1/projects/"+projectID+"/cards", protocol.CreateCardRequest{Title: title})
	return decode[protocol.Card](st.t, r.want(st.t, http.StatusCreated))
}

// digestTree returns the hash of every file under root except the ones in .git, by relative path.
// Two digests are equal when the files that people see are the same.
func digestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	digest := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		digest[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("read the files under %s: %v", root, err)
	}
	return digest
}
