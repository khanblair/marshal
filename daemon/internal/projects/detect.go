package projects

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The language labels the app shows. Monorepo is what a repository with several packages is
// called, whatever the packages are written in.
const (
	LanguageGo         = "Go"
	LanguageTypeScript = "TypeScript"
	LanguageJavaScript = "JavaScript"
	LanguageRust       = "Rust"
	LanguagePython     = "Python"
	LanguageJava       = "Java"
	LanguageRuby       = "Ruby"
	LanguageCSharp     = "C#"
	LanguageMonorepo   = "Monorepo"
	LanguageUnknown    = "Unknown"
)

const (
	// maxMarkerBytes is the most of a file that detection reads. A manifest is small, and a huge
	// file that only looks like one is not worth loading.
	maxMarkerBytes = 1 << 20
	// goSourceHeadBytes is how much of a Go file is read to find its package clause.
	goSourceHeadBytes = 4096
	// minMonorepoPackages is how many packages make a repository a monorepo.
	minMonorepoPackages = 2
)

// Detection is what the top folder of a repository says about the project.
type Detection struct {
	// Language is one of the Language constants.
	Language string
	// IsMonorepo is true when the repository has at least two package folders.
	IsMonorepo bool
	// Packages are the package folders of a monorepo, relative to the root with forward slashes,
	// in name order. It is empty, never nil, when the repository is not a monorepo.
	Packages []string
	// DevCommand is a best guess at the command that starts the dev server, or empty. It is shown
	// in the project settings, and the person can change it.
	DevCommand string
}

// topLevel is what is in the top folder of a repository.
type topLevel struct {
	root  string
	files map[string]struct{}
	dirs  map[string]struct{}
}

func (t topLevel) has(names ...string) bool {
	for _, name := range names {
		if _, ok := t.files[name]; ok {
			return true
		}
	}
	return false
}

func (t topLevel) hasSuffix(suffixes ...string) bool {
	for name := range t.files {
		for _, suffix := range suffixes {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
	}
	return false
}

func (t topLevel) path(elem ...string) string {
	return filepath.Join(append([]string{t.root}, elem...)...)
}

// Detect reads the marker files in the top folder of a repository and works out its language,
// whether it is a monorepo, its packages, and a guess at its dev command. It uses only the
// standard library and never runs anything from the repository.
//
// Detection is a best effort. A marker file that is missing, too big, or not valid is treated as
// if it said nothing, because a broken package.json must not stop a person from adding the
// project. Only a top folder that cannot be listed is an error.
func Detect(root string) (Detection, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Detection{}, fmt.Errorf("look inside the repository folder: %w", err)
	}
	top := topLevel{root: root, files: map[string]struct{}{}, dirs: map[string]struct{}{}}
	for _, entry := range entries {
		if entry.IsDir() {
			top.dirs[entry.Name()] = struct{}{}
		} else {
			top.files[entry.Name()] = struct{}{}
		}
	}
	found := Detection{
		Language:   languageOf(top),
		DevCommand: devCommandOf(top),
		Packages:   []string{},
	}
	if packages := workspacePackages(top); len(packages) >= minMonorepoPackages {
		found.IsMonorepo, found.Packages, found.Language = true, packages, LanguageMonorepo
	}
	return found, nil
}

// languageOf picks the language from the marker files. The order matters when a repository has
// several: a marker that only one ecosystem uses (go.mod, Cargo.toml) wins over package.json,
// which many repositories keep for tooling, and the Python markers come last because a
// requirements.txt turns up in repositories of every kind.
func languageOf(top topLevel) string {
	switch {
	case top.has("go.mod", "go.work"):
		return LanguageGo
	case top.has("Cargo.toml"):
		return LanguageRust
	case top.has("pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"):
		return LanguageJava
	case top.hasSuffix(".csproj", ".sln", ".slnx"):
		return LanguageCSharp
	case top.has("Gemfile"):
		return LanguageRuby
	case top.has("package.json"):
		return scriptLanguage(top)
	case top.has("pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "Pipfile"):
		return LanguagePython
	}
	return LanguageUnknown
}

// packageManifest is the part of package.json that detection reads.
type packageManifest struct {
	Scripts         map[string]string `json:"scripts"`
	PackageManager  string            `json:"packageManager"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Workspaces      json.RawMessage   `json:"workspaces"`
}

// readManifest reads package.json. It gives an empty manifest when the file is missing or broken.
func readManifest(top topLevel) packageManifest {
	var manifest packageManifest
	data, ok := readMarker(top.path("package.json"))
	if !ok || json.Unmarshal(data, &manifest) != nil {
		return packageManifest{}
	}
	return manifest
}

// scriptLanguage tells TypeScript from JavaScript: a tsconfig.json, or typescript among the
// dependencies, means TypeScript.
func scriptLanguage(top topLevel) string {
	if top.has("tsconfig.json") {
		return LanguageTypeScript
	}
	manifest := readManifest(top)
	_, inDeps := manifest.Dependencies["typescript"]
	_, inDevDeps := manifest.DevDependencies["typescript"]
	if inDeps || inDevDeps {
		return LanguageTypeScript
	}
	return LanguageJavaScript
}

// devCommandOf guesses the dev command. A package.json dev script comes first, then a Go main
// package, a Rust binary, and a Python module that is clearly meant to be run.
func devCommandOf(top topLevel) string {
	if command := nodeDevCommand(top); command != "" {
		return command
	}
	switch {
	case top.has("go.mod") && hasGoMain(top):
		return "go run ."
	case top.has("Cargo.toml") && fileExists(top.path("src", "main.rs")):
		return "cargo run"
	}
	if module := pythonModule(top); module != "" {
		return "python -m " + module
	}
	return ""
}

// nodeDevCommand is the command for the dev script of package.json. The lock file says which
// package manager the project uses, then the packageManager field, then npm is the fallback.
func nodeDevCommand(top topLevel) string {
	manifest := readManifest(top)
	if strings.TrimSpace(manifest.Scripts["dev"]) == "" {
		return ""
	}
	switch {
	case top.has("pnpm-lock.yaml"):
		return "pnpm dev"
	case top.has("yarn.lock"):
		return "yarn dev"
	case top.has("package-lock.json"):
		return "npm run dev"
	case strings.HasPrefix(manifest.PackageManager, "pnpm@") || top.has("pnpm-workspace.yaml"):
		return "pnpm dev"
	case strings.HasPrefix(manifest.PackageManager, "yarn@"):
		return "yarn dev"
	}
	return "npm run dev"
}

var goMainClause = regexp.MustCompile(`(?m)^package main\b`)

// hasGoMain reports whether a Go file in the top folder is in package main. Files that Go skips
// (tests, and files marked "go:build ignore", which are usually generators) do not count.
func hasGoMain(top topLevel) bool {
	names := make([]string, 0, len(top.files))
	for name := range top.files {
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		head, ok := readHead(top.path(name), goSourceHeadBytes)
		if ok && goMainClause.Match(head) && !bytes.Contains(head, []byte("go:build ignore")) {
			return true
		}
	}
	return false
}

var moduleName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// pythonModule returns the name of the one top folder that is a package with a __main__.py, which
// is what "python -m name" runs. With none or several it returns nothing, since a guess would be
// wrong as often as right.
func pythonModule(top topLevel) string {
	if !top.has("pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "Pipfile") {
		return ""
	}
	var found []string
	for name := range top.dirs {
		if moduleName.MatchString(name) && fileExists(top.path(name, "__main__.py")) {
			found = append(found, name)
		}
	}
	if len(found) != 1 {
		return ""
	}
	return found[0]
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// readMarker reads a whole marker file, up to maxMarkerBytes. It reports false when the file is
// missing or cannot be read.
func readMarker(path string) ([]byte, bool) {
	return readHead(path, maxMarkerBytes)
}

// readHead reads at most limit bytes from the start of a file.
func readHead(path string, limit int64) ([]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, false
	}
	return data, true
}
