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
