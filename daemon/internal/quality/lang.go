package quality

import (
	"regexp"
	"strings"
)

// Reading a file's shape: where its functions begin and end, how many parameters each takes, and how
// deeply each line nests. This is deliberately a light read, not a parser: the checks compare the
// card's own added lines with the file around them, and the deeper map of a whole repository is
// Phase 7's. Each scanner says which languages it knows; a file in a language none of them knows is
// still checked line by line, just not for shape.

// funcDecl is one function the scanner found: where it starts and ends (1-based, inclusive), how many
// parameters it takes, and its name when the language gives one cheaply.
type funcDecl struct {
	Name      string
	StartLine int
	EndLine   int
	Params    int
}

// describe names the function for a finding's sentence.
func (f funcDecl) describe() string {
	if f.Name == "" {
		return "This function"
	}
	return "The function " + f.Name
}

// scanFunctions finds the functions of a file in a language the scanner knows. An unknown language
// answers nothing, so a shape check never guesses.
func scanFunctions(language, content string) []funcDecl {
	if content == "" {
		return nil
	}
	lines := codeLines(content, language)
	switch language {
	case "go":
		return scanGo(lines)
	case "python":
		return scanIndented(lines, pythonDefRe)
	case "javascript", "typescript":
		return scanBraced(lines, scriptFuncRe)
	case "java", "rust", "brace":
		return scanBraced(lines, braceFuncRe)
	default:
		return nil
	}
}

// goFuncRe starts a Go function or method.
var goFuncRe = regexp.MustCompile(`^func\b`)

// pythonDefRe starts a Python function or method.
var pythonDefRe = regexp.MustCompile(`^\s*(async\s+)?def\s+(\w+)\s*\(`)

// scriptFuncRe starts a JavaScript or TypeScript function, arrow function, or method.
var scriptFuncRe = regexp.MustCompile(`(^|\s)(async\s+)?function\b|=>\s*\{|^\s*(public|private|protected|static|async|get|set|readonly)*\s*\w+\s*\([^;]*\)\s*:\s*[\w<>\[\]|.\s]*\{`)

// braceFuncRe starts a function in a language whose declarations are mostly types and a name.
var braceFuncRe = regexp.MustCompile(`(^|\s)(fn|func|void|public|private|protected|static|virtual|async|extern)\b[^;]*\(`)

// scanGo finds Go functions and methods by matching braces from the signature's opening brace.
func scanGo(lines []string) []funcDecl {
	var out []funcDecl
	for i := 0; i < len(lines); i++ {
		if !goFuncRe.MatchString(lines[i]) {
			continue
		}
		sigStart := i
		sig, bodyStart, ok := joinSignature(lines, i)
		if !ok {
			continue
		}
		end := endOfBlock(lines, bodyStart)
		out = append(out, funcDecl{
			Name:      goFuncName(sig),
			StartLine: sigStart + 1,
			EndLine:   end + 1,
			Params:    goParamCount(sig),
		})
		i = end
	}
	return out
}

// scanBraced finds functions in a brace language by matching the braces of the block that follows a
// declaration line, and counts the parameters of the first parenthesised group.
func scanBraced(lines []string, re *regexp.Regexp) []funcDecl {
	var out []funcDecl
	for i := 0; i < len(lines); i++ {
		if !re.MatchString(lines[i]) {
			continue
		}
		sig, bodyStart, ok := joinSignature(lines, i)
		if !ok {
			continue
		}
		end := endOfBlock(lines, bodyStart)
		out = append(out, funcDecl{
			Name:      declName(sig),
			StartLine: i + 1,
			EndLine:   end + 1,
			Params:    countParenthesised(sig),
		})
		i = end
	}
	return out
}

// scanIndented finds functions in an indentation-based language: a `def` and everything indented
// under it.
func scanIndented(lines []string, re *regexp.Regexp) []funcDecl {
	var out []funcDecl
	for i := 0; i < len(lines); i++ {
		match := re.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		indent := indentWidth(lines[i])
		end := i
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				end = j
				continue
			}
			if indentWidth(lines[j]) <= indent {
				break
			}
			end = j
		}
		out = append(out, funcDecl{
			Name:      match[2],
			StartLine: i + 1,
			EndLine:   end + 1,
			Params:    countParenthesised(lines[i]),
		})
		i = end
	}
	return out
}

// joinSignature joins the lines from a declaration until the opening brace of its body, and answers
// the signature text and the 0-based line the brace is on. A declaration with no body (an interface
// method, a prototype) is not a function and is skipped.
func joinSignature(lines []string, start int) (sig string, bodyLine int, ok bool) {
	var b strings.Builder
	parens := 0
	for i := start; i < len(lines) && i < start+maxSignatureLines; i++ {
		text := lines[i]
		if i > start {
			b.WriteByte(' ')
		}
		b.WriteString(strings.TrimSpace(text))
		for _, r := range text {
			switch r {
			case '(', '[', '{':
				parens++
			case ')', ']', '}':
				parens--
			}
		}
		if strings.ContainsRune(text, '{') && parens >= 1 {
			return b.String(), i, true
		}
		if parens < 0 {
			return "", 0, false
		}
	}
	return "", 0, false
}

// maxSignatureLines is how far a signature may run before it is taken to be something other than a
// function, so a file of one enormous array does not stall the scanner.
const maxSignatureLines = 20

// endOfBlock returns the 0-based line on which the brace block opened at or after `from` closes.
func endOfBlock(lines []string, from int) int {
	depth := 0
	seen := false
	for i := from; i < len(lines); i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				depth++
				seen = true
			case '}':
				depth--
			}
		}
		if seen && depth <= 0 {
			return i
		}
	}
	return len(lines) - 1
}

// goFuncName reads the function's name out of a Go signature, skipping a method's receiver.
func goFuncName(sig string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sig), "func"))
	if strings.HasPrefix(rest, "(") {
		_, after, ok := splitGroup(rest)
		if !ok {
			return ""
		}
		rest = strings.TrimSpace(after)
	}
	return firstIdentifier(rest)
}

// goParamCount counts a Go signature's parameters, skipping a method's receiver.
func goParamCount(sig string) int {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sig), "func"))
	if strings.HasPrefix(rest, "(") {
		_, after, ok := splitGroup(rest)
		if !ok {
			return 0
		}
		rest = strings.TrimSpace(after)
	}
	return countParenthesised(rest)
}

// countParenthesised counts the top-level items of the first parenthesised group in text, which is
// how many parameters a signature declares.
func countParenthesised(text string) int {
	inner, _, ok := splitGroup(text)
	if !ok {
		return 0
	}
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return 0
	}
	count := 1
	depth := 0
	for _, r := range inner {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				count++
			}
		}
	}
	return count
}

// splitGroup returns the text inside the first `(...)` group in text, and what follows it.
func splitGroup(text string) (inner, rest string, ok bool) {
	start := strings.IndexRune(text, '(')
	if start < 0 {
		return "", "", false
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[start+1 : i], text[i+1:], true
			}
		}
	}
	return "", "", false
}

// firstIdentifier returns the first identifier-looking word of text.
func firstIdentifier(text string) string {
	for _, field := range identifierWords(text) {
		if r := field[0]; r >= '0' && r <= '9' {
			continue
		}
		return field
	}
	return ""
}

// identifierWords splits text into the runs of characters that could be an identifier.
func identifierWords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

// declKeywords are the words a brace-language declaration may start with before the name it
// declares, so a finding says "The function render" and not "The function public".
var declKeywords = map[string]bool{
	"function": true, "fn": true, "func": true, "export": true, "default": true,
	"public": true, "private": true, "protected": true, "static": true, "final": true,
	"abstract": true, "virtual": true, "async": true, "extern": true, "inline": true,
	"const": true, "let": true, "var": true, "get": true, "set": true, "void": true,
}

// declName returns the name a declaration declares: the first identifier-looking word that is not
// one of the words that come before a name. An empty answer means the scanner could not name it,
// which reads as "This function".
func declName(sig string) string {
	for _, word := range identifierWords(sig) {
		if declKeywords[word] {
			continue
		}
		if r := word[0]; r >= '0' && r <= '9' {
			continue
		}
		return word
	}
	return ""
}

// nestingDepths answers, for each line of a file, how deeply that line nests. A line inside one
// function body is depth 1; a line inside a loop inside that function is depth 2.
func nestingDepths(language, content string) []int {
	lines := codeLines(content, language)
	depths := make([]int, len(lines))
	switch language {
	case "python", "ruby":
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			depths[i] = indentWidth(line) / 4
		}
	default:
		depth := 0
		for i, line := range lines {
			depths[i] = depth
			for _, r := range line {
				switch r {
				case '{', '[':
					depth++
				case '}', ']':
					depth--
				}
			}
			if depth < 0 {
				depth = 0
			}
		}
	}
	return depths
}

// indentWidth counts a line's leading spaces, with a tab counting as four.
func indentWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

// codeLines splits a file into lines and drops the comments and the string contents of the language
// it knows, so a brace inside a comment or a string does not change a function's shape or a line's
// nesting. The read is approximate and errs toward keeping code: a missed comment only means a
// possible extra brace, never a wrong file.
func codeLines(content, language string) []string {
	raw := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	out := make([]string, len(raw))
	for i, line := range raw {
		out[i] = stripCode(line, language)
	}
	return out
}

// stripCode removes a line's comment and the contents of its quoted strings.
func stripCode(line, language string) string {
	lineComment := "//"
	switch language {
	case "python", "ruby":
		lineComment = "#"
	}
	var b strings.Builder
	inString := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inString != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inString {
				inString = 0
			}
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			inString = c
			continue
		}
		if c == lineComment[0] && strings.HasPrefix(line[i:], lineComment) {
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}
