package preview

// Finding a browser to drive. Marshal bundles none: it uses the Chrome or Edge the person already
// has. When neither is installed the screenshot check is skipped with a clear notice and never
// silently passed (docs/library-docs.md).

import (
	"os"
	"os/exec"
	"runtime"
)

// browserCandidates are the places Chrome and Edge install themselves, by platform.
func browserCandidates() map[string][]string {
	return map[string][]string{
		"darwin": {
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Microsoft Edge Beta.app/Contents/MacOS/Microsoft Edge Beta",
		},
		"windows": {
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		},
	}
}

// browserNames are the programs to look for on the PATH, which is how a browser is usually found on
// Linux and how a Homebrew or snap Chrome is found on macOS.
func browserNames() []string {
	return []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
		"microsoft-edge", "microsoft-edge-stable",
	}
}

// browserLookup is the real BrowserFinder. It keeps nothing: every look reads the machine, so a
// browser installed while Marshal is running is found the next time a screenshot is asked for.
type browserLookup struct{}

// Find returns the first browser Marshal can drive, wherever this machine keeps it.
func (browserLookup) Find() (string, bool) { return lookForBrowser() }

// lookForBrowser is what browserLookup reads: the first browser Marshal can drive, wherever this
// machine keeps it.
func lookForBrowser() (string, bool) {
	if path, found := browserIn(browserCandidates()[runtime.GOOS]); found {
		return path, true
	}
	for _, name := range browserNames() {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

// browserIn returns the first path that is a file Marshal can run. It is separate from
// lookForBrowser so the choice can be tested without a browser installed anywhere.
func browserIn(paths []string) (string, bool) {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}
