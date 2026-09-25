package projects_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// tree writes files into a new folder and returns it. A name ending in / makes an empty folder.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	scriptsDev = `{"name":"x","scripts":{"dev":"vite"}}`
	emptyJSON  = `{}`
	goMain     = "package main\n\nfunc main() {}\n"
)

func TestDetectLanguageAndDevCommand(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		language string
		dev      string
	}{
		{"nothing", map[string]string{"README.md": "hi"}, projects.LanguageUnknown, ""},
		{"go library", map[string]string{"go.mod": "module x", "lib.go": "package lib\n"}, projects.LanguageGo, ""},
		{"go main", map[string]string{"go.mod": "module x", "main.go": goMain}, projects.LanguageGo, "go run ."},
		{"go main in a comment header", map[string]string{"go.mod": "module x", "cmd.go": "// Copyright\n\npackage main\n"}, projects.LanguageGo, "go run ."},
		{"go tests do not count", map[string]string{"go.mod": "module x", "main_test.go": goMain}, projects.LanguageGo, ""},
		{"go generator is not the program", map[string]string{"go.mod": "module x", "gen.go": "//go:build ignore\n\n" + goMain}, projects.LanguageGo, ""},
		{"go workspace", map[string]string{"go.work": "go 1.27\n"}, projects.LanguageGo, ""},
		{"go wins over a tooling package.json", map[string]string{"go.mod": "module x", "package.json": emptyJSON}, projects.LanguageGo, ""},
		{"go with a dev script", map[string]string{"go.mod": "module x", "main.go": goMain, "package.json": scriptsDev}, projects.LanguageGo, "npm run dev"},
		{"rust binary", map[string]string{"Cargo.toml": "[package]\n", "src/main.rs": "fn main() {}"}, projects.LanguageRust, "cargo run"},
		{"rust library", map[string]string{"Cargo.toml": "[package]\n", "src/lib.rs": ""}, projects.LanguageRust, ""},
		{"java maven", map[string]string{"pom.xml": "<project/>"}, projects.LanguageJava, ""},
		{"java gradle kotlin", map[string]string{"build.gradle.kts": ""}, projects.LanguageJava, ""},
		{"c sharp project", map[string]string{"App.csproj": "<Project/>"}, projects.LanguageCSharp, ""},
		{"c sharp solution", map[string]string{"App.sln": ""}, projects.LanguageCSharp, ""},
		{"ruby", map[string]string{"Gemfile": "source 'https://rubygems.org'"}, projects.LanguageRuby, ""},
		{"python by pyproject", map[string]string{"pyproject.toml": "[project]"}, projects.LanguagePython, ""},
		{"python by requirements", map[string]string{"requirements.txt": "flask"}, projects.LanguagePython, ""},
		{"python module", map[string]string{"pyproject.toml": "[project]", "app/__init__.py": "", "app/__main__.py": ""}, projects.LanguagePython, "python -m app"},
		{"python with two runnable folders", map[string]string{"pyproject.toml": "[project]", "a/__main__.py": "", "b/__main__.py": ""}, projects.LanguagePython, ""},
		{"python folder without a project file", map[string]string{"app/__main__.py": ""}, projects.LanguageUnknown, ""},
		{"javascript", map[string]string{"package.json": emptyJSON}, projects.LanguageJavaScript, ""},
		{"javascript with a broken package.json", map[string]string{"package.json": "{not json"}, projects.LanguageJavaScript, ""},
		{"typescript by tsconfig", map[string]string{"package.json": emptyJSON, "tsconfig.json": "{}"}, projects.LanguageTypeScript, ""},
		{"typescript by dev dependency", map[string]string{"package.json": `{"devDependencies":{"typescript":"^6"}}`}, projects.LanguageTypeScript, ""},
		{"typescript by dependency", map[string]string{"package.json": `{"dependencies":{"typescript":"^6"}}`}, projects.LanguageTypeScript, ""},
		{"pnpm dev", map[string]string{"package.json": scriptsDev, "pnpm-lock.yaml": ""}, projects.LanguageJavaScript, "pnpm dev"},
		{"yarn dev", map[string]string{"package.json": scriptsDev, "yarn.lock": ""}, projects.LanguageJavaScript, "yarn dev"},
		{"npm dev by lock file", map[string]string{"package.json": scriptsDev, "package-lock.json": "{}"}, projects.LanguageJavaScript, "npm run dev"},
		{"npm dev by default", map[string]string{"package.json": scriptsDev}, projects.LanguageJavaScript, "npm run dev"},
		{"pnpm by the packageManager field", map[string]string{"package.json": `{"packageManager":"pnpm@11.4.0","scripts":{"dev":"vite"}}`}, projects.LanguageJavaScript, "pnpm dev"},
		{"yarn by the packageManager field", map[string]string{"package.json": `{"packageManager":"yarn@4.0.0","scripts":{"dev":"vite"}}`}, projects.LanguageJavaScript, "yarn dev"},
		{"lock file beats packageManager", map[string]string{"package.json": `{"packageManager":"pnpm@11.4.0","scripts":{"dev":"vite"}}`, "yarn.lock": ""}, projects.LanguageJavaScript, "yarn dev"},
		{"an empty dev script is no script", map[string]string{"package.json": `{"scripts":{"dev":"  "}}`}, projects.LanguageJavaScript, ""},
		{"only a test script", map[string]string{"package.json": `{"scripts":{"test":"node --test"}}`}, projects.LanguageJavaScript, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := projects.Detect(tree(t, tc.files))
			if err != nil {
				t.Fatal(err)
			}
			if got.Language != tc.language || got.DevCommand != tc.dev {
				t.Errorf("got language %q, dev %q; want %q, %q", got.Language, got.DevCommand, tc.language, tc.dev)
			}
			if got.IsMonorepo || len(got.Packages) != 0 || got.Packages == nil {
				t.Errorf("a plain repository got monorepo %v, packages %#v; want false and an empty list", got.IsMonorepo, got.Packages)
			}
		})
	}
}

func TestDetectMonorepoPackages(t *testing.T) {
	pkg := func(name string) string { return `{"name":"` + name + `"}` }
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			"pnpm block list",
			map[string]string{
				"pnpm-workspace.yaml":   "packages:\n  - apps/*\n  - 'packages/*'\n  - \"tools/cli\"\n  - '!**/test/**'\n",
				"apps/web/package.json": pkg("web"), "apps/api/package.json": pkg("api"),
				"packages/ui/package.json": pkg("ui"), "tools/cli/package.json": pkg("cli"),
				"apps/empty/": "", "apps/docs/README.md": "no marker",
			},
			[]string{"apps/api", "apps/web", "packages/ui", "tools/cli"},
		},
		{
			"pnpm list without indent and with comments",
			map[string]string{
				"pnpm-workspace.yaml": "# workspace\npackages:\n- apps/*   # apps\n\ncatalog:\n  react: ^19\n",
				"apps/a/package.json": pkg("a"), "apps/b/package.json": pkg("b"),
			},
			[]string{"apps/a", "apps/b"},
		},
		{
			"pnpm inline list",
			map[string]string{
				"pnpm-workspace.yaml": "packages: [apps/*, \"libs/*\"]\n",
				"apps/a/package.json": pkg("a"), "libs/b/package.json": pkg("b"),
			},
			[]string{"apps/a", "libs/b"},
		},
		{
			"pnpm ignores other keys that hold lists",
			map[string]string{
				"pnpm-workspace.yaml":     "onlyBuiltDependencies:\n  - esbuild\npackages:\n  - packages/*\n",
				"packages/a/package.json": pkg("a"), "packages/b/package.json": pkg("b"), "esbuild/package.json": pkg("e"),
			},
			[]string{"packages/a", "packages/b"},
		},
		{
			"package.json workspaces as a list",
			map[string]string{
				"package.json":        `{"workspaces":["apps/*","packages/*"]}`,
				"apps/a/package.json": pkg("a"), "packages/b/package.json": pkg("b"),
			},
			[]string{"apps/a", "packages/b"},
		},
		{
			"package.json workspaces as an object",
			map[string]string{
				"package.json":        `{"workspaces":{"packages":["apps/*"],"nohoist":["**/x"]}}`,
				"apps/a/package.json": pkg("a"), "apps/b/package.json": pkg("b"),
			},
			[]string{"apps/a", "apps/b"},
		},
		{
			"go work block and single use",
			map[string]string{
				"go.work":             "go 1.27\n\nuse (\n\t./services/api // the api\n\t./libs/core\n)\nuse ./tools/gen\n",
				"services/api/go.mod": "module a", "libs/core/go.mod": "module c", "tools/gen/go.mod": "module g",
			},
			[]string{"libs/core", "services/api", "tools/gen"},
		},
		{
			"cargo members over several lines",
			map[string]string{
				"Cargo.toml":          "[workspace]\nresolver = \"2\"\nmembers = [\n  \"crates/*\", # all crates\n  \"cli\",\n]\n\n[workspace.dependencies]\nserde = \"1\"\n",
				"crates/a/Cargo.toml": "[package]", "crates/b/Cargo.toml": "[package]", "cli/Cargo.toml": "[package]",
			},
			[]string{"cli", "crates/a", "crates/b"},
		},
		{
			"cargo members on one line",
			map[string]string{
				"Cargo.toml":   "[workspace]\nmembers = [\"a\", 'b']\n",
				"a/Cargo.toml": "[package]", "b/Cargo.toml": "[package]",
			},
			[]string{"a", "b"},
		},
		{
			"a pattern with two wildcard segments",
			map[string]string{
				"package.json":                   `{"workspaces":["apps/*/packages/*"]}`,
				"apps/x/packages/a/package.json": pkg("a"), "apps/y/packages/b/package.json": pkg("b"),
			},
			[]string{"apps/x/packages/a", "apps/y/packages/b"},
		},
		{
			"double star, parent, and absolute patterns name nothing",
			map[string]string{
				"package.json":   `{"workspaces":["**/*","../outside","/abs","."]}`,
				"a/package.json": pkg("a"), "b/package.json": pkg("b"),
			},
			[]string{},
		},
		{
			"hidden folders and node_modules never match a wildcard",
			map[string]string{
				"package.json":         `{"workspaces":["*"]}`,
				".hidden/package.json": pkg("h"), "node_modules/package.json": pkg("n"),
				"a/package.json": pkg("a"), "b/package.json": pkg("b"),
			},
			[]string{"a", "b"},
		},
		{
			"sources are merged and each folder counts once",
			map[string]string{
				"pnpm-workspace.yaml":     "packages:\n  - packages/*\n",
				"package.json":            `{"workspaces":["packages/*"]}`,
				"packages/a/package.json": pkg("a"), "packages/b/package.json": pkg("b"),
			},
			[]string{"packages/a", "packages/b"},
		},
		{
			"one package is not a monorepo",
			map[string]string{
				"pnpm-workspace.yaml":     "packages:\n  - packages/*\n",
				"packages/a/package.json": pkg("a"),
			},
			[]string{},
		},
		{
			"a package can be marked by any project file",
			map[string]string{
				"package.json":        `{"workspaces":["apps/*"]}`,
				"apps/ios/App.csproj": "", "apps/api/pom.xml": "",
			},
			[]string{"apps/api", "apps/ios"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := projects.Detect(tree(t, tc.files))
			if err != nil {
				t.Fatal(err)
			}
			wantMono := len(tc.want) > 0
			if got.IsMonorepo != wantMono || !reflect.DeepEqual(got.Packages, tc.want) {
				t.Fatalf("got monorepo %v, packages %#v; want %v, %#v", got.IsMonorepo, got.Packages, wantMono, tc.want)
			}
			if wantMono && got.Language != projects.LanguageMonorepo {
				t.Errorf("language = %q, want %q", got.Language, projects.LanguageMonorepo)
			}
		})
	}
}

func TestDetectOnAFolderThatDoesNotExist(t *testing.T) {
	if _, err := projects.Detect(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Detect of a missing folder returned no error")
	}
}

func TestDetectTheFixtureRepositories(t *testing.T) {
	small, err := projects.Detect(testutil.Fixture(t, "small-repo"))
	if err != nil {
		t.Fatal(err)
	}
	if small.Language != projects.LanguageJavaScript || small.IsMonorepo || small.DevCommand != "" || len(small.Packages) != 0 {
		t.Errorf("small-repo = %+v, want a JavaScript repository with no dev command", small)
	}
	mono, err := projects.Detect(testutil.Fixture(t, "monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"packages/api", "packages/shared", "packages/web"}
	if mono.Language != projects.LanguageMonorepo || !mono.IsMonorepo || !reflect.DeepEqual(mono.Packages, want) {
		t.Errorf("monorepo = %+v, want a monorepo with %v", mono, want)
	}
}
