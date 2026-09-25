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
var sentTo = make(chan int, 1)
var closed = make(chan struct{})

var readOnly int
var negated int
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
	sentTo <- 1
	close(closed)
	_ = readOnly
	_ = -negated // a unary operator other than & only reads
	shadowed := 5 // local shadow via :=; must NOT count
	_ = shadowed
}
`)

	got := MutatedGlobalNames([]*ast.File{f})

	for _, name := range []string{"reassigned", "incremented", "elemWritten", "fieldWritten", "addressed", "rangedInto", "sentTo", "closed"} {
		if !got[name] {
			t.Errorf("expected %q to be detected as mutated; got %v", name, got)
		}
	}
	for _, name := range []string{"readOnly", "negated", "shadowed", "Konst"} {
		if got[name] {
			t.Errorf("%q must not be detected as mutated; got %v", name, got)
		}
	}
}

// TestMutatedGlobalNamesCallForms checks that delete, clear, copy and each
// in-place sort change the package variable in their first argument, and that
// calls which only read do not. A local variable with the name of an import is
// not the package.
func TestMutatedGlobalNamesCallForms(t *testing.T) {
	f := parseFile(t, `
package p

import (
	"slices"
	s "sort"
)

var copied, sliced, sliceStable, sorted, stable, strs, ints, floats []int
var slicesSorted, sortFunc, sortStableFunc, reversed []int
var searched, sortedCopy, measured, shadowSorted []int
var deleted, cleared = map[int]int{}, map[int]int{}

type sorter struct{}

func (sorter) Sort(xs []int) {}

func shadow() {
	slices := sorter{}
	slices.Sort(shadowSorted)
}

func work(src []int, less func(i, j int) bool, cmp func(a, b int) int) {
	delete(deleted, 1)
	clear(cleared)
	_ = len(measured)
	copy(copied, src)
	s.Slice(sliced, less)
	s.SliceStable(sliceStable, less)
	s.Sort(sorted)
	s.Stable(stable)
	s.Strings(strs)
	s.Ints(ints)
	s.Float64s(floats)
	slices.Sort(slicesSorted)
	slices.SortFunc(sortFunc, cmp)
	slices.SortStableFunc(sortStableFunc, cmp)
	slices.Reverse(reversed)
	_ = s.SearchInts(searched, 1)
	_ = slices.Sorted(slices.Values(sortedCopy))
}
`)

	got := MutatedGlobalNames([]*ast.File{f})

	for _, name := range []string{"deleted", "cleared", "copied", "sliced", "sliceStable", "sorted", "stable", "strs", "ints", "floats", "slicesSorted", "sortFunc", "sortStableFunc", "reversed"} {
		if !got[name] {
			t.Errorf("expected %q to be detected as mutated; got %v", name, got)
		}
	}
	for _, name := range []string{"searched", "sortedCopy", "measured", "shadowSorted"} {
		if got[name] {
			t.Errorf("%q is only read; must not be detected as mutated; got %v", name, got)
		}
	}
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

func TestImportHelpers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		spec        *ast.ImportSpec
		importPath  string
		qualifier   string
		wantMatches bool
	}{
		{name: "default name", spec: &ast.ImportSpec{}, importPath: "example.com/thing", qualifier: "thing", wantMatches: true},
		{name: "explicit alias", spec: &ast.ImportSpec{Name: ast.NewIdent("o")}, importPath: "os", qualifier: "o", wantMatches: true},
		{name: "mismatched name", spec: &ast.ImportSpec{}, importPath: "os", qualifier: "syscall"},
		{name: "blank import", spec: &ast.ImportSpec{Name: ast.NewIdent("_")}, importPath: "os", qualifier: "_"},
		{name: "dot import", spec: &ast.ImportSpec{Name: ast.NewIdent(".")}, importPath: "os", qualifier: "."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := importMatchesQualifier(tc.spec, tc.importPath, tc.qualifier); got != tc.wantMatches {
				t.Fatalf("importMatchesQualifier() = %t, want %t", got, tc.wantMatches)
			}
		})
	}

	for _, tc := range []struct {
		name string
		spec *ast.ImportSpec
		want string
		ok   bool
	}{
		{name: "nil spec"},
		{name: "missing path", spec: &ast.ImportSpec{}},
		{name: "invalid path literal", spec: &ast.ImportSpec{Path: &ast.BasicLit{Value: "os"}}},
		{name: "quoted path", spec: &ast.ImportSpec{Path: &ast.BasicLit{Value: `"os"`}}, want: "os", ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := importPath(tc.spec)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("importPath() = %q, %t; want %q, %t", got, ok, tc.want, tc.ok)
			}
		})
	}
}
