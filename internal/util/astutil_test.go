package util

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseFile(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "f.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// TestMutatedGlobalNamesCrossFile is the headline check: a variable declared in
// one file but mutated in another file of the same package must be detected.
func TestMutatedGlobalNamesCrossFile(t *testing.T) {
	declFile := parseFile(t, `
package p

var Timeout = 30
var ErrThing = mkErr()
var table = []int{1, 2, 3}

func mkErr() error { return nil }
`)
	mutFile := parseFile(t, `
package p

func reset() {
	Timeout = 60   // cross-file mutation
	_ = table[0]   // read only
}
`)

	got := MutatedGlobalNames([]*ast.File{declFile, mutFile})

	if !got["Timeout"] {
		t.Errorf("Timeout is reassigned in another file; should be detected as mutated, got %v", got)
	}
	if got["ErrThing"] {
		t.Errorf("ErrThing is never mutated; should not be reported, got %v", got)
	}
	if got["table"] {
		t.Errorf("table is only read; should not be reported, got %v", got)
	}
	if got["mkErr"] {
		t.Errorf("mkErr is a function, not a package var; should not be reported, got %v", got)
	}
}

// TestMutatedGlobalNamesForms checks each mutation form, and that locals,
// constants, field/element writes on non-globals, and := shadows are handled.
func TestMutatedGlobalNamesForms(t *testing.T) {
	f := parseFile(t, `
package p

var reassigned int
var incremented int
var elemWritten = map[string]int{}
var fieldWritten = struct{ X int }{}
var addressed int
var rangedInto int

var readOnly int
var shadowed int

const Konst = 1

func work(items []int) {
	reassigned = 1
	incremented++
	elemWritten["k"] = 2
	fieldWritten.X = 3
	p := &addressed
	_ = p
	for rangedInto = range items {
	}
	_ = readOnly
	shadowed := 5 // local shadow via :=; must NOT count
	_ = shadowed
}
`)

	got := MutatedGlobalNames([]*ast.File{f})

	for _, name := range []string{"reassigned", "incremented", "elemWritten", "fieldWritten", "addressed", "rangedInto"} {
		if !got[name] {
			t.Errorf("expected %q to be detected as mutated; got %v", name, got)
		}
	}
	for _, name := range []string{"readOnly", "shadowed", "Konst"} {
		if got[name] {
			t.Errorf("%q must not be detected as mutated; got %v", name, got)
		}
	}
}

func TestDefineIdentsSkipsRedeclaredIdentifiers(t *testing.T) {
	file := parseFile(t, `
package p

func pair() (int, error) { return 1, nil }

func f() (n int) {
	n, err := pair()
	_ = err
	return
}
`)
	assign := firstDefineAssign(t, file)
	got := defineIdentNames(assign)
	if len(got) != 1 || got[0] != "err" {
		t.Fatalf("defineIdents() = %v, want [err]", got)
	}
}

func TestDefineIdentsKeepsFreshLocalsOnMixedLine(t *testing.T) {
	file := parseFile(t, `
package p

func f() (n int) {
	n, unused := 1, 2
	return
}
`)
	assign := firstDefineAssign(t, file)
	got := defineIdentNames(assign)
	if len(got) != 1 || got[0] != "unused" {
		t.Fatalf("defineIdents() = %v, want [unused]", got)
	}
}

func TestDefineIdentsSkipsUnresolvedIdentifiers(t *testing.T) {
	assign := &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{&ast.Ident{Name: "x"}},
	}
	if got := defineIdents(assign); len(got) != 0 {
		t.Fatalf("defineIdents() = %v, want none when Obj is nil", got)
	}
}

func firstDefineAssign(t *testing.T, file *ast.File) *ast.AssignStmt {
	t.Helper()
	var assign *ast.AssignStmt
	ast.Inspect(file, func(n ast.Node) bool {
		a, ok := n.(*ast.AssignStmt)
		if ok && a.Tok == token.DEFINE && assign == nil {
			assign = a
		}
		return true
	})
	if assign == nil {
		t.Fatal("fixture had no := assignment")
	}
	return assign
}

func defineIdentNames(assign *ast.AssignStmt) []string {
	var names []string
	for _, id := range defineIdents(assign) {
		names = append(names, id.Name)
	}
	return names
}

func TestTypeAliasNames(t *testing.T) {
	src := `package p
type original struct{}
type alias = original
type chained = alias
type external = other.Thing
type literal = struct{ A int }
type selfish = selfish
type real original
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "a.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := TypeAliasNames([]*ast.File{f})
	want := map[string]string{"alias": "original", "chained": "alias"}
	if len(got) != len(want) {
		t.Fatalf("TypeAliasNames = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("TypeAliasNames[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestResolveTypeAlias(t *testing.T) {
	aliases := map[string]string{"a": "b", "b": "original", "loop": "loop2", "loop2": "loop"}
	tests := []struct{ in, want string }{
		{"a", "original"},
		{"b", "original"},
		{"original", "original"},
		{"missing", "missing"},
		{"loop", "loop"},
	}
	for _, tt := range tests {
		if got := ResolveTypeAlias(aliases, tt.in); got != tt.want {
			t.Errorf("ResolveTypeAlias(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
