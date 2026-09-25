package protocol_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Rules of the protocol that a test can check by reading the package's own source, so that a
// later change cannot break them without a failing test.

// sourceFiles parses the non-test Go files of the protocol package.
func sourceFiles(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return files
}

func structTypes(files []*ast.File) map[string]*ast.StructType {
	out := map[string]*ast.StructType{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if st, ok := ts.Type.(*ast.StructType); ok {
						out[ts.Name.Name] = st
					}
				}
			}
		}
	}
	return out
}

func TestNoWireFieldUsesABareTime(t *testing.T) {
	for name, st := range structTypes(sourceFiles(t)) {
		for _, field := range st.Fields.List {
			ast.Inspect(field.Type, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Time" {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "time" {
						t.Errorf("%s has a time.Time field; use Timestamp so the wire format stays one", name)
					}
				}
				return true
			})
		}
	}
}

func TestStateResponsesCarryTheServerTime(t *testing.T) {
	structs := structTypes(sourceFiles(t))
	mustCarry := []string{"Health", "Page"}
	for name := range structs {
		if strings.HasSuffix(name, "Snapshot") {
			mustCarry = append(mustCarry, name)
		}
	}
	for _, name := range mustCarry {
		st, ok := structs[name]
		if !ok {
			t.Errorf("%s is missing", name)
			continue
		}
		if !hasServerTime(st) {
			t.Errorf("%s does not have a `ServerTime Timestamp` field tagged serverTime", name)
		}
	}
}

func hasServerTime(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		typ, ok := field.Type.(*ast.Ident)
		if len(field.Names) == 1 && field.Names[0].Name == "ServerTime" && ok && typ.Name == "Timestamp" {
			return field.Tag != nil && strings.Contains(field.Tag.Value, `json:"serverTime"`)
		}
	}
	return false
}

// enumBlocks reads every const block with two or more names from the source and returns the
// string values of the ones that have an explicit type, by type name. A block without a type
// that has exported names would become a union in TypeScript, so it is reported as a problem.
func enumBlocks(t *testing.T, files []*ast.File) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST || len(gen.Specs) < 2 {
				continue
			}
			for _, spec := range gen.Specs {
				addEnumValue(t, out, spec.(*ast.ValueSpec))
			}
		}
	}
	return out
}

func addEnumValue(t *testing.T, out map[string][]string, spec *ast.ValueSpec) {
	t.Helper()
	name := spec.Names[0]
	typ, typed := spec.Type.(*ast.Ident)
	if !typed {
		if name.IsExported() {
			t.Errorf("const %s is exported in a block of untyped constants; declare it on its own so tygo does not turn the block into a union", name.Name)
		}
		return
	}
	lit, ok := spec.Values[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Errorf("const %s of type %s is not a string literal", name.Name, typ.Name)
		return
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name.Name, typ.Name) {
		t.Errorf("const %s should start with its type name %s, or tygo will not build the union", name.Name, typ.Name)
	}
	out[typ.Name] = append(out[typ.Name], value)
}

func TestEveryConstBlockIsAListedEnumInOrder(t *testing.T) {
	inSource := enumBlocks(t, sourceFiles(t))
	listed := allEnums()
	for name, values := range inSource {
		got, ok := listed[name]
		if !ok {
			t.Errorf("%s has a const block but no <Type>Values function in allEnums (enums_test.go)", name)
			continue
		}
		if !equalStrings(got, values) {
			t.Errorf("%s: the const block has %v but %sValues() returns %v", name, values, name, got)
		}
	}
	for name := range listed {
		if _, ok := inSource[name]; !ok {
			t.Errorf("%s is listed in allEnums but has no const block", name)
		}
	}
}
