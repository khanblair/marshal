// Package trust marks a folder as trusted in a CLI's own settings, so that the CLI does not stop
// on its "do you trust this folder?" question in a terminal nobody is watching. It is used for
// folders Marshal made (a card's worktree) and for folders a person chose as a project.
package trust

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	claudeProjects = "projects"
	claudeAccepted = "hasTrustDialogAccepted"
	geminiTrusted  = "TRUST_FOLDER"
	fileMode       = 0o600
	dirMode        = 0o700
)

// Folder marks dir as trusted for every CLI Marshal knows how to do it for. A CLI whose settings
// file is missing is skipped. The errors are joined so that one failing CLI hides no other.
func Folder(dir string) error {
	return errors.Join(Claude(dir), Gemini(dir))
}

// Claude sets hasTrustDialogAccepted for dir in Claude Code's global settings file. A file that is
// missing or not a JSON object is left alone and reported, because Claude Code owns it and asks
// its own question the first time it runs.
func Claude(dir string) error {
	path, err := claudeFile()
	if err != nil {
		return err
	}
	return editJSON(path, func(root map[string]json.RawMessage) (bool, error) {
		projects := map[string]map[string]json.RawMessage{}
		if raw, ok := root[claudeProjects]; ok {
			if err := json.Unmarshal(raw, &projects); err != nil {
				return false, fmt.Errorf("read the projects in %s: %w", path, err)
			}
		}
		entry := projects[dir]
		if entry == nil {
			entry = map[string]json.RawMessage{}
		}
		if string(entry[claudeAccepted]) == "true" {
			return false, nil
		}
		entry[claudeAccepted] = json.RawMessage("true")
		projects[dir] = entry
		raw, err := json.Marshal(projects)
		if err != nil {
			return false, err
		}
		root[claudeProjects] = raw
		return true, nil
	})
}

// Gemini adds dir to Gemini CLI's list of trusted folders. The file is created when it is missing.
func Gemini(dir string) error {
	path := os.Getenv("GEMINI_CLI_TRUSTED_FOLDERS_PATH")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".gemini", "trustedFolders.json")
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("{}"), fileMode); err != nil {
			return err
		}
	}
	return editJSON(path, func(root map[string]json.RawMessage) (bool, error) {
		want := json.RawMessage(`"` + geminiTrusted + `"`)
		if string(root[dir]) == string(want) {
			return false, nil
		}
		root[dir] = want
		return true, nil
	})
}

// claudeFile is Claude Code's global settings file, which lives in the config folder when
// CLAUDE_CONFIG_DIR is set, and in the home folder otherwise.
func claudeFile() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// editJSON reads a JSON object, lets change edit it, and writes it back through a temporary file
// so that a crash never leaves half a file. Keys the change does not touch are kept as they were.
func editJSON(path string, change func(map[string]json.RawMessage) (bool, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	changed, err := change(root)
	if err != nil || !changed {
		return err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(fileMode)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".marshal-trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
