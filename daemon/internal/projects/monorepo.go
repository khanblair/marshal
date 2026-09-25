package projects

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The workspace files that name a repository's packages. Each reader returns folder patterns such
// as "apps/*". Only the simple shapes are read: a pattern is a folder path whose segments may hold
// * ? or [ ] (never **), and a pattern that starts with ! is an exclusion that is ignored.

// isPackageMarker reports whether a file name makes its folder a package.
func isPackageMarker(name string) bool {
	switch name {
	case "package.json", "go.mod", "Cargo.toml", "pyproject.toml", "setup.py", "pom.xml",
		"build.gradle", "build.gradle.kts", "Gemfile":
		return true
	}
	return strings.HasSuffix(name, ".csproj")
}

// workspacePackages returns the package folders that the repository's workspace files name, as
// sorted folders relative to the root with forward slashes. Only folders that exist and hold a
// package marker are kept.
func workspacePackages(top topLevel) []string {
	var patterns []string
	patterns = append(patterns, pnpmPatterns(top)...)
	patterns = append(patterns, npmPatterns(top)...)
	patterns = append(patterns, goWorkPatterns(top)...)
	patterns = append(patterns, cargoPatterns(top)...)
	found := map[string]struct{}{}
	for _, pattern := range patterns {
		for _, folder := range expandPattern(top.root, pattern) {
			found[folder] = struct{}{}
		}
	}
	packages := make([]string, 0, len(found))
	for folder := range found {
		packages = append(packages, folder)
	}
	sort.Strings(packages)
	return packages
}

// pnpmPatterns reads the packages list of pnpm-workspace.yaml, in the block form
//
//	packages:
//	  - apps/*
//	  - 'packages/*'
//
// and in the inline form `packages: [apps/*, packages/*]`.
func pnpmPatterns(top topLevel) []string {
	data, ok := readMarker(top.path("pnpm-workspace.yaml"))
	if !ok {
		return nil
	}
	var patterns []string
	inPackages := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := stripYAMLComment(scanner.Text())
		text := strings.TrimSpace(line)
		switch {
		case text == "":
		case strings.HasPrefix(text, "-"):
			if inPackages {
				patterns = append(patterns, unquote(text[1:]))
			}
		case line[0] != ' ' && line[0] != '\t':
			var block []string
			block, inPackages = packagesKey(text)
			patterns = append(patterns, block...)
		}
	}
	return patterns
}

// packagesKey reads a top-level "packages:" line of pnpm-workspace.yaml. It returns the patterns
// from an inline list, and whether the following indented "- " lines belong to the packages block.
func packagesKey(text string) (patterns []string, block bool) {
	rest, isKey := strings.CutPrefix(text, "packages:")
	if !isKey {
		return nil, false
	}
	if rest = strings.TrimSpace(rest); rest == "" {
		return nil, true
	}
	return inlineList(rest), false
}

func stripYAMLComment(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return ""
	}
	if i := strings.Index(line, " #"); i >= 0 {
		return line[:i]
	}
	return line
}

// inlineList reads `[a, "b", 'c']`.
func inlineList(text string) []string {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "["), "]"))
	var items []string
	for _, item := range strings.Split(text, ",") {
		if item = unquote(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// unquote trims spaces and one pair of quotes.
func unquote(text string) string {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && (text[0] == '"' || text[0] == '\'') && text[len(text)-1] == text[0] {
		return text[1 : len(text)-1]
	}
	return text
}

// npmPatterns reads the workspaces of package.json: a list, or an object with a packages list.
func npmPatterns(top topLevel) []string {
	raw := readManifest(top).Workspaces
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var object struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return object.Packages
	}
	return nil
}

// goWorkPatterns reads the folders of the use lines of go.work, in both the single and the
// block form.
func goWorkPatterns(top topLevel) []string {
	data, ok := readMarker(top.path("go.work"))
	if !ok {
		return nil
	}
	var patterns []string
	inBlock := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		text := strings.TrimSpace(stripSlashComment(scanner.Text()))
		switch {
		case inBlock && text == ")":
			inBlock = false
		case inBlock:
			patterns = append(patterns, unquote(strings.Trim(text, "`")))
		case text == "use (" || text == "use(":
			inBlock = true
		case strings.HasPrefix(text, "use "):
			patterns = append(patterns, unquote(strings.Trim(strings.TrimPrefix(text, "use "), "` ")))
		}
	}
	return patterns
}

func stripSlashComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

// cargoPatterns reads the members list of the [workspace] table of Cargo.toml. The list may run
// over several lines.
func cargoPatterns(top topLevel) []string {
	data, ok := readMarker(top.path("Cargo.toml"))
	if !ok {
		return nil
	}
	var members strings.Builder
	var state cargoScanState
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		text := strings.TrimSpace(stripTOMLComment(scanner.Text()))
		add, done := state.step(text)
		if add == "" {
			continue
		}
		members.WriteString(add + " ")
		if done {
			break
		}
	}
	return quotedStrings(members.String())
}

// cargoScanState is where cargoPatterns' line-by-line reader is: whether it is inside
// "[workspace]", and whether it has reached the "members" list.
type cargoScanState struct {
	inWorkspace, inMembers bool
}

// step reads one line of Cargo.toml and updates the state. It returns the text to add to the
// members list (empty when the line adds nothing), and whether that text closes the list.
func (s *cargoScanState) step(text string) (add string, done bool) {
	if strings.HasPrefix(text, "[") && !s.inMembers {
		s.inWorkspace = text == "[workspace]"
		return "", false
	}
	if !s.inWorkspace {
		return "", false
	}
	if rest, isKey := strings.CutPrefix(text, "members"); isKey && !s.inMembers {
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, "=") {
			return "", false
		}
		text, s.inMembers = strings.TrimSpace(rest[1:]), true
	}
	if !s.inMembers {
		return "", false
	}
	return text, strings.Contains(text, "]")
}

// stripTOMLComment cuts a # comment, but not a # inside a quoted string.
func stripTOMLComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return line[:i]
		}
	}
	return line
}

// quotedStrings returns the text between each pair of matching quotes.
func quotedStrings(text string) []string {
	var found []string
	for {
		start := strings.IndexAny(text, `"'`)
		if start < 0 {
			return found
		}
		end := strings.IndexByte(text[start+1:], text[start])
		if end < 0 {
			return found
		}
		found = append(found, text[start+1:start+1+end])
		text = text[start+1+end+1:]
	}
}

// expandPattern turns one folder pattern into the existing package folders it names, relative to
// the root with forward slashes. A pattern that leaves the repository, or has **, names nothing.
func expandPattern(root, pattern string) []string {
	pattern = path.Clean(strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./"))
	if pattern == "." || pattern == ".." || strings.HasPrefix(pattern, "../") ||
		strings.HasPrefix(pattern, "!") || path.IsAbs(pattern) || strings.Contains(pattern, "**") {
		return nil
	}
	folders := []string{""}
	for _, segment := range strings.Split(pattern, "/") {
		folders = expandSegment(root, folders, segment)
	}
	var packages []string
	for _, folder := range folders {
		if hasPackageMarker(filepath.Join(root, filepath.FromSlash(folder))) {
			packages = append(packages, folder)
		}
	}
	return packages
}

// expandSegment adds one segment of a pattern to each folder so far. A plain segment is joined
// as it is, and a segment with a wildcard becomes every folder that matches it. Hidden folders and
// node_modules never match a wildcard, like a shell.
func expandSegment(root string, bases []string, segment string) []string {
	var out []string
	for _, base := range bases {
		if !strings.ContainsAny(segment, "*?[") {
			out = append(out, path.Join(base, segment))
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(base)))
		if err != nil {
			continue // a folder that is not there has no matches
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			// A pattern that is not valid matches nothing.
			if ok, err := path.Match(segment, name); err == nil && ok {
				out = append(out, path.Join(base, name))
			}
		}
	}
	return out
}

// hasPackageMarker reports whether a folder holds a file that makes it a package.
func hasPackageMarker(folder string) bool {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && isPackageMarker(entry.Name()) {
			return true
		}
	}
	return false
}
