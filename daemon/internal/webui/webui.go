// Package webui embeds the built web app (apps/web/dist), so the daemon can serve it itself: the
// desktop shell points its window straight at the daemon's own address instead of bundling a
// second copy of the UI at a different origin, and a plain browser can do the same thing over
// Tailscale. dist/ is empty (only a placeholder) until scripts/copy-web-dist.mjs copies the real
// build in, which every build and release does before the daemon compiles.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS is the built web app, rooted at what dist/ holds: index.html, its scripts and styles, and
// every other asset `pnpm --filter web build` produces. Empty (only its placeholder) means the
// copy step has not run, and the caller falls back to answering every address the same way it did
// before this package existed.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is a literal, always-embedded directory: Sub only fails on a bad path, never on
		// missing content, so this cannot happen outside a typo in the embed directive above.
		panic("webui: " + err.Error())
	}
	return sub
}

// Available is true once dist/ holds more than its placeholder, i.e. a real build was copied in.
func Available() bool {
	entries, err := fs.ReadDir(FS(), ".")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}
