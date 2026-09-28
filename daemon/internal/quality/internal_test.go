package quality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The checks themselves, the language readers, the profile rules, and the small helpers are tested
// directly here: they are pure functions over text, so nothing is started, read, or written. The
// service that drives them over a store and Git is tested in service_test.go.

// newFile is a source file the card added, so every line of it is its own.
func newFile(path, language, content string) sourceFile {
	return sourceFile{Path: path, Language: language, New: true, Now: content}
}

// editedFile is a file the card changed in place: `added` are the line numbers of the worktree
// version the card added, and base is what the target branch has.
func editedFile(path, language, content, base string, added ...int) sourceFile {
	lines := map[int]bool{}
	for _, line := range added {
		lines[line] = true
	}
	return sourceFile{Path: path, Language: language, Now: content, Base: base, Added: lines}
}

// profileWith returns the default profile with one threshold changed.
func profileWith(change func(*protocol.SmellProfile)) protocol.SmellProfile {
	profile := protocol.DefaultSmellProfile()
	change(&profile)
	return profile
}

// onlyCheck turns every built-in check but one off, so a test of one check is not disturbed by the
// others. The check left on keeps its default severity.
func onlyCheck(check protocol.SmellCheck) protocol.SmellProfile {
	profile := protocol.DefaultSmellProfile()
	settings := make([]protocol.SmellCheckSetting, 0, len(protocol.SmellCheckValues()))
	for _, known := range protocol.SmellCheckValues() {
		setting := defaultSettingOf(known)
		setting.Enabled = known == check
		settings = append(settings, setting)
	}
	profile.Checks = settings
	return profile
}

func TestLargeFileFlagsAFileTheCardPushedOverTheLimit(t *testing.T) {
	big := strings.Repeat("// padding line\n", 900)
	small := strings.Repeat("// padding line\n", 800)
	cases := []struct {
		name  string
		file  sourceFile
		found bool
	}{
		{"a new file over the limit", newFile("a.go", "go", big), true},
		{"a new file exactly at the limit", newFile("a.go", "go", small), false},
		{"a file the target branch already had too long", editedFile("a.go", "go", big, big, 1), false},
		{"a file grown over the limit", editedFile("a.go", "go", big, small, 1), true},
		{"a file whose base could not be read", editedFile("a.go", "go", big, "", 1), true},
	}
	profile := profileWith(func(p *protocol.SmellProfile) { p.MaxFileLines = 800 })
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, ok := largeFile(profile, tc.file)
			if ok != tc.found {
				t.Fatalf("found = %v, wanted %v", ok, tc.found)
			}
			if ok && found.Check != protocol.SmellCheckLargeFile {
				t.Fatalf("the finding names the large-file check, got %q", found.Check)
			}
		})
	}
}

func TestLongLinesSkipALineWithNoSpaces(t *testing.T) {
	profile := profileWith(func(p *protocol.SmellProfile) { p.MaxLineLength = 20 })
	content := "short\n" +
		strings.Repeat("a", 26) + "\n" + // long but nothing to wrap
		"a long line with plenty of spaces\n"
	found := longLines(profile, newFile("a.go", "go", content))
	if len(found) != 1 {
		t.Fatalf("one line is both long and wrappable, got %d findings", len(found))
	}
	if found[0].Line != 3 {
		t.Fatalf("the finding names the long line, got line %d", found[0].Line)
	}
}

func TestMagicNumbersSkipCommentsAndNumberTables(t *testing.T) {
	profile := onlyCheck(protocol.SmellCheckMagicNumber)
	content := "x := 1\n" +
		"// the answer is 3000\n" +
		"1, 2, 3\n" +
		"timeout := 3000\n"
	found := checkBuiltins(profile, []sourceFile{newFile("a.go", "go", content)})
	if len(found) != 1 {
		t.Fatalf("only the bare number in code is a finding, got %d: %+v", len(found), found)
	}
	if found[0].Line != 4 || !strings.Contains(found[0].Message, "3000") {
		t.Fatalf("the finding names the number and its line, got %+v", found[0])
	}
}

func TestDeepNestingFlagsOnlyTheLineAddedTooDeep(t *testing.T) {
	profile := onlyCheck(protocol.SmellCheckDeepNesting)
	profile.MaxNesting = 1
	content := "func f() {\n\tfor i := range xs {\n\t\tuse(i)\n\t}\n}\n"
	found := checkBuiltins(profile, []sourceFile{editedFile("a.go", "go", content, content, 3)})
	if len(found) != 1 {
		t.Fatalf("one added line nests too deep, got %d: %+v", len(found), found)
	}
	if found[0].Line != 3 {
		t.Fatalf("the finding names the deep line, got %d", found[0].Line)
	}
}

func TestLongFunctionsBlameOnlyAFunctionTheCardTouched(t *testing.T) {
	profile := onlyCheck(protocol.SmellCheckLongFunction)
	profile.MaxFunctionLines = 3
	content := "func small() {\n\tx := 1\n}\nfunc big() {\n\ta := 1\n\tb := 2\n\tc := 3\n\td := 4\n}\n"
	untouched := checkBuiltins(profile, []sourceFile{editedFile("a.go", "go", content, content, 2)})
	if len(untouched) != 0 {
		t.Fatalf("a long function the card never touched is not blamed on it, got %+v", untouched)
	}
	touched := checkBuiltins(profile, []sourceFile{editedFile("a.go", "go", content, content, 7)})
	if len(touched) != 1 {
		t.Fatalf("a long function the card grew is one finding, got %d", len(touched))
	}
	if touched[0].Line != 4 || !strings.Contains(touched[0].Message, "big") {
		t.Fatalf("the finding names the function and where it starts, got %+v", touched[0])
	}
}

func TestLongParameterListsFlagAFunctionOverTheLimit(t *testing.T) {
	profile := onlyCheck(protocol.SmellCheckLongParameterList)
	profile.MaxParameters = 2
	content := "func many(a int, b int, c int) {\n\tx := 1\n}\n"
	found := checkBuiltins(profile, []sourceFile{newFile("a.go", "go", content)})
	if len(found) != 1 {
		t.Fatalf("one function takes too many parameters, got %d", len(found))
	}
	if !strings.Contains(found[0].Message, "3 parameters") {
		t.Fatalf("the finding counts the parameters, got %q", found[0].Message)
	}
}

func TestDuplicateBlocksReportTheSecondCopyOnly(t *testing.T) {
	block := "const alpha = 1;\nconst beta = 2;\nconst gamma = 3;\nconst delta = 4;\nconst epsilon = 5;\nconst zeta = 6;\n"
	profile := onlyCheck(protocol.SmellCheckDuplicateBlock)
	found := checkBuiltins(profile, []sourceFile{
		newFile("first.js", "javascript", block),
		newFile("second.js", "javascript", block),
	})
	if len(found) != 1 {
		t.Fatalf("one block copied once is one finding, got %d: %+v", len(found), found)
	}
	if found[0].File != "second.js" || found[0].Line != 1 {
		t.Fatalf("the finding names the copy, got %s:%d", found[0].File, found[0].Line)
	}
	if got := found[0].family(); got != protocol.SmellFamilyDispensables {
		t.Fatalf("a duplicated block is a dispensable, got %q", got)
	}
}

func TestGeneratedFilesAndUnknownLanguagesAreSkipped(t *testing.T) {
	for _, path := range []string{
		"package-lock.json", "pnpm-lock.yaml", "go.sum",
		"src/app.min.js", "api/things.pb.go", "gen/things_generated.go",
		"node_modules/pkg/index.js", "vendor/lib/a.go", "src/generated/api.ts",
	} {
		if !generatedFile(path) {
			t.Errorf("%s is not code a person wrote", path)
		}
	}
	for _, path := range []string{"src/main.go", "README.md", "src/util.test.js"} {
		if generatedFile(path) {
			t.Errorf("%s is code a person wrote", path)
		}
	}

	// A file in a language no scanner knows is still checked for being too large, but not for
	// shape.
	profile := onlyCheck(protocol.SmellCheckLongFunction)
	profile.MaxFunctionLines = 1
	content := "func big() {\n\ta := 1\n\tb := 2\n}\n"
	found := checkBuiltins(profile, []sourceFile{newFile("notes.txt", "", content)})
	if len(found) != 0 {
		t.Fatalf("a file in an unknown language is not read for shape, got %+v", found)
	}
}

func TestLanguageOfAndCommentLine(t *testing.T) {
	languages := map[string]string{
		"a.go": "go", "a.js": "javascript", "a.mjs": "javascript", "a.ts": "typescript",
		"a.tsx": "typescript", "a.py": "python", "A.JAVA": "java", "a.rb": "ruby",
		"a.rs": "rust", "a.cpp": "brace", "a.txt": "", "a.md": "",
	}
	for path, want := range languages {
		if got := languageOf(path); got != want {
			t.Errorf("languageOf(%q) = %q, wanted %q", path, got, want)
		}
	}
	for _, tc := range []struct {
		language, line string
		want           bool
	}{
		{"go", "// a comment", true},
		{"go", "/* a comment", true},
		{"go", "* a doc line", true},
		{"javascript", "// a comment", true},
		{"python", "# a comment", true},
		{"ruby", "# a comment", true},
		{"go", "x := 1", false},
		{"python", "x = 1", false},
		{"", "# not read", false},
	} {
		if got := commentLine(tc.language, tc.line); got != tc.want {
			t.Errorf("commentLine(%q, %q) = %v, wanted %v", tc.language, tc.line, got, tc.want)
		}
	}
}

func TestCountLinesAndCaps(t *testing.T) {
	if got := countLines(""); got != 0 {
		t.Fatalf("an empty file has no lines, got %d", got)
	}
	if got := countLines("a\nb\n"); got != 2 {
		t.Fatalf("a trailing newline ends the last line, got %d", got)
	}
	if got := countLines("a\nb"); got != 2 {
		t.Fatalf("two lines, got %d", got)
	}
	found := make([]rawFinding, 60)
	if got := len(capPerFile(found)); got != maxFindingsPerFile {
		t.Fatalf("a check reports at most %d findings per file, got %d", maxFindingsPerFile, got)
	}
}

func TestAddedLineSelection(t *testing.T) {
	added := newFile("a.go", "go", "one\ntwo\n")
	if got := len(addedLines(added)); got != 2 {
		t.Fatalf("a file the card added is all its own, got %d lines", got)
	}
	edited := editedFile("a.go", "go", "one\ntwo\nthree\n", "one\n", 2)
	lines := addedLines(edited)
	if len(lines) != 1 || lines[0].Number != 2 {
		t.Fatalf("only the added line is read, got %+v", lines)
	}
	if !touchesAdded(edited, 1, 2) || touchesAdded(edited, 3, 3) {
		t.Fatal("touchesAdded must report only a range with an added line in it")
	}
	if !touchesAdded(added, 99, 100) {
		t.Fatal("a file the card added is entirely its own")
	}
	significant := significantAddedLines(newFile("a.js", "javascript", "x := 1\n}\n\nconst value = 2;\n"))
	if len(significant) != 1 || !strings.Contains(significant[0].Text, "const value") {
		t.Fatalf("a blank line, a lone brace, and a short line are not compared, got %+v", significant)
	}
}

func TestScanFunctionsReadsTheLanguagesItKnows(t *testing.T) {
	goSource := "func (r *Repo) Save(name string, count int) {\n\tr.x = 1\n}\n"
	functions := scanFunctions("go", goSource)
	if len(functions) != 1 {
		t.Fatalf("one Go method, got %d", len(functions))
	}
	if functions[0].Name != "Save" || functions[0].Params != 2 {
		t.Fatalf("the method's name and parameters are read, got %+v", functions[0])
	}
	if functions[0].StartLine != 1 || functions[0].EndLine != 3 {
		t.Fatalf("the method's span is read, got %d..%d", functions[0].StartLine, functions[0].EndLine)
	}

	py := scanFunctions("python", "def a(x):\n    return x\n\ndef b():\n    pass\n")
	if len(py) != 2 || py[0].Name != "a" || py[0].Params != 1 || py[1].Name != "b" {
		t.Fatalf("the Python functions are read, got %+v", py)
	}

	js := scanFunctions("javascript", "function foo(a, b) {\n  return a;\n}\n")
	if len(js) != 1 || js[0].Name != "foo" || js[0].Params != 2 {
		t.Fatalf("the JavaScript function is read, got %+v", js)
	}

	// A declaration's name is the name, not the word before it.
	java := scanFunctions("java", "public static void render(String title) {\n    draw(title);\n}\n")
	if len(java) != 1 || java[0].Name != "render" || java[0].Params != 1 {
		t.Fatalf("the declaration's name is read past its keywords, got %+v", java)
	}

	if got := scanFunctions("", "func f() {}"); got != nil {
		t.Fatalf("a language no scanner knows answers nothing, got %+v", got)
	}
}

func TestParameterAndNameReading(t *testing.T) {
	cases := map[string]int{
		"f()":           0,
		"f(a int)":      1,
		"f(a, b, c)":    3,
		"f(g(a, b), c)": 2,
		"nothing here":  0,
	}
	for text, want := range cases {
		if got := countParenthesised(text); got != want {
			t.Errorf("countParenthesised(%q) = %d, wanted %d", text, got, want)
		}
	}
	if got := firstIdentifier("123 abc"); got != "abc" {
		t.Errorf("firstIdentifier skips a leading number, got %q", got)
	}
	if got := goFuncName("func (r *Repo) Save(x int) {"); got != "Save" {
		t.Errorf("goFuncName skips the receiver, got %q", got)
	}
	if got := goParamCount("func (r *Repo) Save(x int) {"); got != 1 {
		t.Errorf("goParamCount skips the receiver, got %d", got)
	}
}

func TestNestingDepthsAndIndentation(t *testing.T) {
	brace := nestingDepths("go", "func f() {\n\tif a {\n\t\tuse()\n\t}\n}\n")
	want := []int{0, 1, 2, 2, 1}
	if len(brace) != len(want) {
		t.Fatalf("one depth per line, got %d", len(brace))
	}
	for i := range want {
		if brace[i] != want[i] {
			t.Fatalf("line %d nests %d deep, wanted %d", i+1, brace[i], want[i])
		}
	}
	indented := nestingDepths("python", "def a():\n    x = 1\n")
	if len(indented) != 2 || indented[1] != 1 {
		t.Fatalf("an indented body nests one deep, got %v", indented)
	}
	if got := indentWidth("\t  x"); got != 6 {
		t.Fatalf("a tab is four spaces, got %d", got)
	}
}

func TestStripCodeRemovesCommentsAndStringContents(t *testing.T) {
	if got := stripCode("x := 1 // a comment", "go"); got != "x := 1 " {
		t.Fatalf("a line comment is removed, got %q", got)
	}
	if got := stripCode(`x := "a//b"`, "go"); got != "x := " {
		t.Fatalf("a // inside a string is not a comment, got %q", got)
	}
	if got := stripCode("x = 1  # a comment", "python"); got != "x = 1  " {
		t.Fatalf("a python comment is removed, got %q", got)
	}
	if got := stripCode(`x := "a\"b" ; y`, "go"); !strings.Contains(got, "y") {
		t.Fatalf("an escaped quote does not end the string early, got %q", got)
	}
}

func TestFamilyOfEveryCheck(t *testing.T) {
	want := map[protocol.SmellCheck]protocol.SmellFamily{
		protocol.SmellCheckLongFunction:      protocol.SmellFamilyBloaters,
		protocol.SmellCheckLargeFile:         protocol.SmellFamilyBloaters,
		protocol.SmellCheckLongParameterList: protocol.SmellFamilyBloaters,
		protocol.SmellCheckDeepNesting:       protocol.SmellFamilyObfuscators,
		protocol.SmellCheckLongLine:          protocol.SmellFamilyObfuscators,
		protocol.SmellCheckMagicNumber:       protocol.SmellFamilyLexicalAbusers,
		protocol.SmellCheckDuplicateBlock:    protocol.SmellFamilyDispensables,
	}
	if len(want) != len(protocol.SmellCheckValues()) {
		t.Fatalf("every check has a family in this test: %d vs %d", len(want), len(protocol.SmellCheckValues()))
	}
	for check, family := range want {
		if got := familyOf(check); got != family {
			t.Errorf("familyOf(%q) = %q, wanted %q", check, got, family)
		}
	}
}

func TestProfileResolutionAndBounds(t *testing.T) {
	resolved := resolveProfile("web", nil)
	if resolved.ProjectID != "web" || len(resolved.Checks) != len(protocol.SmellCheckValues()) {
		t.Fatalf("a project with no profile resolves to the defaults, got %+v", resolved)
	}

	stored := &protocol.SmellProfile{
		MaxFunctionLines: 120,
		Checks: []protocol.SmellCheckSetting{
			{Check: protocol.SmellCheckLongLine, Enabled: false, Severity: protocol.SmellSeverityInfo},
		},
		Linters: []protocol.SmellLinter{{Name: "eslint", Command: []string{"eslint"}}},
	}
	merged := resolveProfile("web", stored)
	if merged.MaxFunctionLines != 120 {
		t.Fatalf("a stored threshold wins, got %d", merged.MaxFunctionLines)
	}
	if merged.MaxFileLines != protocol.DefaultSmellProfile().MaxFileLines {
		t.Fatalf("a threshold the profile did not name keeps its default, got %d", merged.MaxFileLines)
	}
	if setting := settingOf(merged, protocol.SmellCheckLongLine); setting.Enabled {
		t.Fatal("a check the profile turned off is off")
	}
	if len(merged.Linters) != 1 || merged.Linters[0].Name != "eslint" {
		t.Fatalf("a project's own linters are carried through, got %+v", merged.Linters)
	}

	if _, err := checkProfile("web", protocol.SmellProfile{MaxFunctionLines: 1}); err == nil {
		t.Fatal("a threshold under its bound is refused")
	}
	if _, err := checkProfile("web", protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
		{Check: "nope", Enabled: true, Severity: protocol.SmellSeverityWarning},
	}}); err == nil {
		t.Fatal("an unknown check is refused")
	}
	if _, err := checkProfile("web", protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
		{Check: protocol.SmellCheckLongLine, Severity: "loud"},
	}}); err == nil {
		t.Fatal("an unknown severity is refused")
	}
	if err := checkBound("field", 0, 5, 10); err != nil {
		t.Fatal("an unnamed value is left for the defaults")
	}
	if err := checkBound("field", 11, 5, 10); err == nil {
		t.Fatal("a value over its bound is refused")
	}
}

func TestCheckLintersRefusesWhatCannotBeRun(t *testing.T) {
	if _, err := checkLinters([]protocol.SmellLinter{{Command: []string{"eslint"}}}); err == nil {
		t.Fatal("a linter with no name is refused")
	}
	if _, err := checkLinters([]protocol.SmellLinter{{Name: "eslint"}}); err == nil {
		t.Fatal("a linter with no program is refused")
	}
	if _, err := checkLinters([]protocol.SmellLinter{
		{Name: "eslint", Command: []string{"eslint"}, Family: "nope"},
	}); err == nil {
		t.Fatal("a linter with an unknown family is refused")
	}
	tooMany := make([]protocol.SmellLinter, 0, maxLinters+1)
	for i := 0; i <= maxLinters; i++ {
		tooMany = append(tooMany, protocol.SmellLinter{Name: "l", Command: []string{"l"}})
	}
	if _, err := checkLinters(tooMany); err == nil {
		t.Fatal("too many linters are refused")
	}
	got, err := checkLinters([]protocol.SmellLinter{{Name: " eslint ", Command: []string{"eslint"}}})
	if err != nil {
		t.Fatalf("a plain linter is accepted: %v", err)
	}
	if got[0].Name != "eslint" || got[0].Family != protocol.SmellFamilyLexicalAbusers {
		t.Fatalf("a linter's name is trimmed and its family defaults, got %+v", got[0])
	}
}

func TestProfileJSONRoundTrip(t *testing.T) {
	if got, err := decodeProfile("web", "  "); err != nil || got.ProjectID != "" {
		t.Fatalf("an empty row reads as no profile, got %+v, %v", got, err)
	}
	if _, err := decodeProfile("web", "{not json"); err == nil {
		t.Fatal("a row that will not parse is a fault")
	}
	encoded, err := encodeProfile(protocol.DefaultSmellProfile())
	if err != nil {
		t.Fatalf("encode a profile: %v", err)
	}
	back, err := decodeProfile("web", encoded)
	if err != nil {
		t.Fatalf("decode a profile: %v", err)
	}
	if back.MaxFunctionLines != protocol.DefaultSmellProfile().MaxFunctionLines {
		t.Fatalf("a profile round-trips, got %+v", back)
	}
}

func TestLinterOutputParsing(t *testing.T) {
	got := parseLinterOutput("eslint v9\nsrc/a.js:12:1: Unexpected var\ngarbage\nsrc/b.js:7: no space after comma\n",
		protocol.SmellFamilyCouplers)
	if len(got) != 2 {
		t.Fatalf("only the two `file:line: message` lines are findings, got %+v", got)
	}
	if got[0].File != "src/a.js" || got[0].Line != 12 || got[0].Family != protocol.SmellFamilyCouplers {
		t.Fatalf("the first finding is read with the profile's family, got %+v", got[0])
	}
	if got[1].File != "src/b.js" || got[1].Line != 7 {
		t.Fatalf("a line without a column is read, got %+v", got[1])
	}
	for _, line := range []string{"", "banner", "src/a.js:0: zero", "src/a.js:x: bad", "src/a.js:12: "} {
		if _, _, _, ok := splitLinterLine(line); ok {
			t.Errorf("%q is not a finding line", line)
		}
	}
}

func TestCommandLinterNeedsAProgramAndAName(t *testing.T) {
	if _, err := NewCommandLinter(protocol.SmellLinter{Name: "l"}); err == nil {
		t.Fatal("a linter with no command is refused")
	}
	if _, err := NewCommandLinter(protocol.SmellLinter{Command: []string{"l"}}); err == nil {
		t.Fatal("a linter with no name is refused")
	}
	linter, err := NewCommandLinter(protocol.SmellLinter{Name: "golangci-lint", Command: []string{"golangci-lint", "run"}})
	if err != nil {
		t.Fatalf("a plain linter is built: %v", err)
	}
	if linter.Name() != "golangci-lint" {
		t.Fatalf("the linter's name is its label, got %q", linter.Name())
	}
	var warned []string
	built := buildLinters(protocol.SmellProfile{Linters: []protocol.SmellLinter{
		{Name: "broken", Command: nil},
		{Name: "fine", Command: []string{"fine"}},
	}}, func(name string, _ error) { warned = append(warned, name) })
	if len(built) != 1 || len(warned) != 1 || warned[0] != "broken" {
		t.Fatalf("a linter that cannot be built is skipped with a warning, got %d built, %v warned", len(built), warned)
	}
}

func TestCommonLanguage(t *testing.T) {
	goFiles := []sourceFile{{Language: "go"}, {Language: "go"}}
	if got := commonLanguage(goFiles); got != "go" {
		t.Fatalf("files of one language share it, got %q", got)
	}
	if got := commonLanguage([]sourceFile{{Language: "go"}, {Language: "python"}}); got != "" {
		t.Fatalf("files of several languages share none, got %q", got)
	}
	if got := commonLanguage([]sourceFile{{Language: ""}, {Language: "go"}}); got != "go" {
		t.Fatalf("a file with no language does not spoil the shared one, got %q", got)
	}
}

func TestMessagesToTheAgent(t *testing.T) {
	finding := protocol.SmellFinding{
		File: "src/a.go", Line: 12, Smell: "long-function", Family: protocol.SmellFamilyBloaters,
		Severity: protocol.SmellSeverityBlocking, Status: protocol.SmellStatusOpen,
		Message: "too long", Suggestion: "split it",
	}
	if line := findingLine(finding); line != "src/a.go:12 (long-function, bloaters)" {
		t.Fatalf("a finding's line names its place and sort, got %q", line)
	}
	text := blockingText([]protocol.SmellFinding{finding, {
		File: "b.go", Severity: protocol.SmellSeverityWarning, Status: protocol.SmellStatusOpen,
	}})
	if !strings.Contains(text, "src/a.go:12") || strings.Contains(text, "b.go") {
		t.Fatalf("only blocking findings go to the agent, got %q", text)
	}
	fix := fixText(finding)
	if !strings.Contains(fix, "split it") || !strings.Contains(fix, "src/a.go:12") {
		t.Fatalf("the ask-to-fix names the place and the suggestion, got %q", fix)
	}
	long := fixText(protocol.SmellFinding{Suggestion: strings.Repeat("x", maxFixTextChars*2)})
	if len(long) != maxFixTextChars {
		t.Fatalf("the message is bounded, got %d characters", len(long))
	}
}

func TestToWireAndNotFound(t *testing.T) {
	row := db.SmellFinding{
		ID: "id", CardID: "card", CommitSha: "sha", Family: "bloaters", Smell: "large-file",
		File: "a.go", Line: 4, Severity: "blocking", Message: "big", Suggestion: "split",
		Status: "open", DismissReason: "",
	}
	got := toWire(row)
	if got.ID != "id" || got.Line != 4 || got.Family != protocol.SmellFamilyBloaters ||
		got.Severity != protocol.SmellSeverityBlocking || got.Status != protocol.SmellStatusOpen {
		t.Fatalf("a stored finding reads back as its wire shape, got %+v", got)
	}
	short := notFoundFinding("abc")
	if short.Code != protocol.ErrorCodeNotFound || short.Details["id"] != "abc" {
		t.Fatalf("an unknown finding is not_found, got %+v", short)
	}
	longID := strings.Repeat("x", maxEchoedFindingID*2)
	if echoed := notFoundFinding(longID).Details["id"]; len(echoed) != maxEchoedFindingID {
		t.Fatalf("an enormous id is cut before it is echoed, got %d characters", len(echoed))
	}
}

func TestReadingWorktreeFilesStaysInsideTheWorktree(t *testing.T) {
	dir := t.TempDir()
	if err := writeWorktreeFile(dir, "src/a.go", "package a\n"); err != nil {
		t.Fatalf("write a worktree file: %v", err)
	}
	if got := readWorktreeFile(dir, "src/a.go"); got != "package a\n" {
		t.Fatalf("a worktree file is read, got %q", got)
	}
	if got := readWorktreeFile(dir, "../outside.go"); got != "" {
		t.Fatalf("a path that leaves the worktree is not read, got %q", got)
	}
	if got := readWorktreeFile(dir, "src/missing.go"); got != "" {
		t.Fatalf("a file that is gone is not checked, got %q", got)
	}
	if folderExists("") || folderExists(dir+"/src/a.go") {
		t.Fatal("folderExists is true only for a folder")
	}
}

func TestABuiltFromFindingCarriesItsOwnFamilyAndName(t *testing.T) {
	builtin := rawFinding{Check: protocol.SmellCheckMagicNumber}
	if builtin.family() != protocol.SmellFamilyLexicalAbusers || builtin.smellName() != "magic-number" {
		t.Fatalf("a built-in finding takes its check's family and name, got %q, %q", builtin.family(), builtin.smellName())
	}
	linted := rawFinding{Check: protocol.SmellCheckMagicNumber, Family: protocol.SmellFamilyCouplers, Smell: "no-var"}
	if linted.family() != protocol.SmellFamilyCouplers || linted.smellName() != "no-var" {
		t.Fatalf("a linter's finding keeps its own family and rule name, got %q, %q", linted.family(), linted.smellName())
	}
}

func TestASettingComesFromTheProfileOrDefaults(t *testing.T) {
	if got := defaultSettingOf(protocol.SmellCheckLargeFile); !got.Enabled || got.Severity != protocol.SmellSeverityBlocking {
		t.Fatalf("the defaults block a large file, got %+v", got)
	}
	profile := protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
		{Check: protocol.SmellCheckLongLine, Enabled: false, Severity: protocol.SmellSeverityInfo},
	}}
	if got := settingOf(profile, protocol.SmellCheckLongLine); got.Enabled || got.Severity != protocol.SmellSeverityInfo {
		t.Fatalf("a check the profile names uses the profile's answer, got %+v", got)
	}
	if got := settingOf(profile, protocol.SmellCheckMagicNumber); !got.Enabled {
		t.Fatalf("a check the profile does not name keeps its default, got %+v", got)
	}
}

func TestNumberTablesAreNotMagicNumbers(t *testing.T) {
	if !allNumbers("1, 2, 3: 4 | 5") {
		t.Fatal("a line of numbers and separators is a table")
	}
	if allNumbers("value := 3000") {
		t.Fatal("a line with a name in it is not a table")
	}
	if magicNumberRe.FindStringSubmatch("x = 1") != nil {
		t.Fatal("a one-digit number is not a magic number")
	}
	if magicNumberRe.FindStringSubmatch("x = 3000") == nil {
		t.Fatal("a four-digit number is a magic number")
	}
}

// writeWorktreeFile writes one file of a fake worktree.
func writeWorktreeFile(dir, name, content string) error {
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
