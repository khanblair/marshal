package codemap

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// What the walk leaves out. The map is meant to be light: a repository's history, its installed
// dependencies and its build output are not the project's own source, and indexing them costs time
// and memory for answers nobody wants.

// skipDirs are folders that are never descended into. Hidden folders are skipped as well, by name,
// which covers .git, .venv, .next and the like without listing them here.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"venv":         true,
	"__pycache__":  true,
	"Pods":         true,
	"DerivedData":  true,
	"coverage":     true,
}

// skipExt are the extensions that are never source: pictures, media, archives, fonts, compiled
// objects and lock files. Handing them to ctags would be work with no answer at the end of it.
var skipExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
	".ico": true, ".icns": true, ".pdf": true, ".svg": true,
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".7z": true, ".tar": true,
	".jar": true, ".war": true, ".class": true, ".pyc": true, ".o": true, ".a": true,
	".so": true, ".dylib": true, ".dll": true, ".exe": true, ".bin": true, ".wasm": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".webm": true, ".wav": true,
	".lock": true, ".sum": true, ".snap": true, ".map": true,
	".db": true, ".sqlite": true, ".sqlite3": true,
}

// skipSuffix are the names that are source by extension but not by content: a bundled file is one
// enormous line, and ctags reading it is work with no useful answer at the end.
var skipSuffix = []string{".min.js", ".min.css", ".bundle.js", ".chunk.js"}

// maxFileBytes is the largest file the map hands to ctags. A megabyte is far past any hand-written
// source file and well inside what a generated or bundled one looks like; skipping those is most of
// what keeps the index light.
const maxFileBytes = 1 << 20

// sourceFiles answers every file of a repository that is worth reading symbols from, as paths
// relative to root in name order. It is the one full walk the map ever does.
func sourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				// The repository itself is not there, or cannot be read. The caller has to know:
				// an index of nothing would be answered as "this project has no such name".
				return err
			}
			// A folder inside it that cannot be read is skipped rather than failing the whole
			// index. One unreadable directory should not cost an agent every answer.
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := entry.Name()
		switch {
		case entry.IsDir():
			if path == root {
				return nil
			}
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		case entry.Type()&fs.ModeSymlink != 0:
			return nil
		case !entry.Type().IsRegular():
			return nil
		case strings.HasPrefix(name, "."):
			return nil
		case skipExt[strings.ToLower(filepath.Ext(name))]:
			return nil
		case hasSuffixFold(name, skipSuffix):
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxFileBytes {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	return files, nil
}

// hasSuffixFold reports whether the name ends with any of the suffixes, ignoring case.
func hasSuffixFold(name string, suffixes []string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
