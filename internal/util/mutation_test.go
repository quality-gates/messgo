package util

import (
	"go/ast"
	"testing"
)

// TestMutationsClassifyOperations checks the facts that the shared query
// reports for each form of mutation in a function body.
func TestMutationsClassifyOperations(t *testing.T) {
	f := parseFile(t, `
package p

import (
	"slices"
	s "sort"
)

func work(m map[int]int, xs, ys []int, ch chan int, n int) {
	n = 1
	n += 1
	n++
	for n = range xs {
	}
	_ = &n
	ch <- 1
	delete(m, 1)
	clear(m)
	copy(xs, ys)
	close(ch)
	(copy)(xs, ys)
	s.Ints(xs)
	(slices.Sort)(xs)
	slices.Sort[[]int](xs)
	_ = append(xs[:1], 2)
	_ = append(xs[:1:1], 2)
	_ = len(xs)
	ys := xs
	_ = ys
}
`)
	type fact struct {
		target string
		kind   MutationKind
		reads  bool
		shared bool
	}
	var got []fact
	ast.Inspect(f, func(n ast.Node) bool {
		for _, m := range Mutations(f, n) {
			got = append(got, fact{RootIdent(m.Target).Name, m.Kind, m.Reads, m.Shared})
		}
		return true
	})
	want := []fact{
		{"n", MutationAssign, false, false},
		{"n", MutationUpdate, true, false},
		{"n", MutationUpdate, true, false},
		{"n", MutationAssign, false, false},
		{"n", MutationAddress, false, false},
		{"ch", MutationSend, true, true},
		{"m", MutationCall, false, true},
		{"m", MutationCall, false, true},
		{"xs", MutationCall, true, true},
		{"ch", MutationCall, true, true},
		{"xs", MutationCall, true, true},
		{"xs", MutationCall, true, true},
		{"xs", MutationCall, true, true},
		{"xs", MutationCall, true, true},
		{"xs", MutationCall, true, true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mutations, want %d:\n got: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mutation %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestMutationsIgnoreShadowedCallees checks that a local function or variable
// that shadows a builtin or an imported package is not a mutation.
func TestMutationsIgnoreShadowedCallees(t *testing.T) {
	f := parseFile(t, `
package p

import "sort"

func copy(dst, src []int) {}

func work(xs, ys []int) {
	copy(xs, ys)
	(copy)(xs, ys)
	sort := struct{ Ints func([]int) }{}
	sort.Ints(xs)
}
`)
	ast.Inspect(f, func(n ast.Node) bool {
		if ms := Mutations(f, n); len(ms) > 0 {
			t.Errorf("unexpected mutations %+v", ms)
		}
		return true
	})
}

// TestMutatedGlobalNamesNormalizesCallees checks that package analysis sees a
// mutating call through parentheses and generic instantiation, and an append
// into the array of a global, and ignores a shadowed builtin.
func TestMutatedGlobalNamesNormalizesCallees(t *testing.T) {
	f := parseFile(t, `
package p

import "slices"

var copied, sorted, instantiated, shadowed, appended, capped []int

func clear(xs []int) {}

func work(src []int) {
	(copy)(copied, src)
	(slices.Sort)(sorted)
	slices.Sort[[]int](instantiated)
	clear(shadowed)
	_ = append(appended[:1], 2)
	_ = append(capped[:1:1], 2)
}
`)
	got := MutatedGlobalNames([]*ast.File{f})
	for _, name := range []string{"copied", "sorted", "instantiated", "appended"} {
		if !got[name] {
			t.Errorf("expected %q to be detected as mutated; got %v", name, got)
		}
	}
	if got["shadowed"] {
		t.Errorf("shadowed is passed to a local no-op named clear; got %v", got)
	}
	if got["capped"] {
		t.Errorf("a three-index slice limits the capacity, so append copies capped; got %v", got)
	}
}
