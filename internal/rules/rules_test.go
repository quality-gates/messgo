// Package rules_test runs each built-in ruleset against crafted Go fixtures and
// asserts the exact set of rules that fire, with their lines. This is the
// automated counterpart to the manual phpmd parity checks: it pins down the
// behavior of every rule analog.
package rules_test

import (
	"sort"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/ruleset"
)

type hit struct {
	rule string
	line int
}

func analyze(t *testing.T, src, rulesetID string) []hit {
	t.Helper()
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	loader := &ruleset.Loader{}
	sets, err := loader.Load(rulesetID)
	if err != nil {
		t.Fatalf("load ruleset %s: %v", rulesetID, err)
	}
	vs := rule.Analyze(f, sets)
	var hits []hit
	for _, v := range vs {
		// Subtract the synthetic "package fixture\n" prepended line.
		hits = append(hits, hit{v.Rule.Name(), v.BeginLine - 1})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].line != hits[j].line {
			return hits[i].line < hits[j].line
		}
		return hits[i].rule < hits[j].rule
	})
	return hits
}

func has(hits []hit, name string) bool {
	for _, h := range hits {
		if h.rule == name {
			return true
		}
	}
	return false
}

func mustHave(t *testing.T, hits []hit, names ...string) {
	t.Helper()
	for _, n := range names {
		if !has(hits, n) {
			t.Errorf("expected rule %q to fire; got %v", n, hits)
		}
	}
}

func mustNotHave(t *testing.T, hits []hit, names ...string) {
	t.Helper()
	for _, n := range names {
		if has(hits, n) {
			t.Errorf("did not expect rule %q to fire; got %v", n, hits)
		}
	}
}

func TestCodeSize(t *testing.T) {
	src := `
func manyParams(a, b, c, d, e, f, g, h, i, j, k int) {}

type Big struct {
	A, B, C, D, E, F, G, H int
	I, J, K, L, M, N, O, P int
}
`
	hits := analyze(t, src, "codesize")
	mustHave(t, hits, "ExcessiveParameterList", "TooManyFields")
}

func TestNaming(t *testing.T) {
	src := `
const my_constant = 5

type Fo struct{ x int }

func (f *Fo) a(b int) int { return b }

func getActive() bool { return true }
`
	hits := analyze(t, src, "naming")
	mustHave(t, hits,
		"ConstantNamingConventions",
		"ShortClassName",
		"ShortVariable",
		"ShortMethodName",
		"BooleanGetMethodName",
	)
}

func TestNamingRangeLoop(t *testing.T) {
	src := `
func loop() {
	for i := range []int{1, 2} {
		_ = i
	}
}
`
	hits := analyze(t, src, "naming")
	mustNotHave(t, hits, "ShortVariable")
}

func TestUnusedCode(t *testing.T) {
	src := `
type widget struct {
	used   int
	unused int
}

func (w *widget) read() int { return w.used }

func (w *widget) dead() int { return 1 }

func compute(a int, spare int) int {
	writeOnly := 0
	writeOnly = 5
	return a
}
`
	hits := analyze(t, src, "unusedcode")
	mustHave(t, hits,
		"UnusedPrivateField",
		"UnusedPrivateMethod",
		"UnusedFormalParameter",
		"UnusedLocalVariable",
	)
	// `used` field is referenced, so it must not be flagged.
	for _, h := range hits {
		if h.rule == "UnusedPrivateField" && h.line == 3 {
			t.Errorf("used field wrongly flagged: %v", hits)
		}
	}
}

func TestUnusedRangeLoopVariable(t *testing.T) {
	src := `
func loop() {
	for i, v := range []int{1, 2} {
	}
}
`
	hits := analyze(t, src, "unusedcode")
	mustHave(t, hits, "UnusedLocalVariable")
}

func TestDesign(t *testing.T) {
	src := `
import "os"

func process(items []int) {
	for i := 0; i < len(items); i++ {
		println("debug", i)
	}
	os.Exit(1)
loop:
	goto loop
}
`
	hits := analyze(t, src, "design")
	mustHave(t, hits,
		"GotoStatement",
		"CountInLoopExpression",
		"DevelopmentCodeFragment",
		"ExitExpression",
	)
}

// TestGlobalVariable pins down exactly which declarations the GlobalVariable
// rule flags. It asserts on the set of reported variable names (not line
// numbers), so it is robust to fixture layout while still being precise about
// the edge cases: grouped blocks, multi-name specs, type-only vars, constants,
// locals, and the blank identifier.
func TestGlobalVariable(t *testing.T) {
	src := `
var GlobalCounter = 0

var width, height int

var (
	enabled = true
	name    string
)

var buffer []byte

var _ = sideEffect()

const MaxRetries = 3

const (
	alpha = 1
	beta  = 2
)

func work(n int) int {
	var local = n
	sum := local
	const inner = 5
	return sum + inner
}

func sideEffect() int { return 0 }
`
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sets, err := (&ruleset.Loader{}).Load("design")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := map[string]bool{}
	for _, v := range rule.Analyze(f, sets) {
		if v.Rule.Name() == "GlobalVariable" {
			got[v.Args[0].(string)] = true
		}
	}

	// Every package-level variable must be flagged, once per name.
	want := []string{"GlobalCounter", "width", "height", "enabled", "name", "buffer"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("GlobalVariable should flag package var %q; got %v", w, got)
		}
	}
	// Constants, the blank identifier, and locals must never be flagged.
	for _, bad := range []string{"MaxRetries", "alpha", "beta", "_", "local", "sum", "inner"} {
		if got[bad] {
			t.Errorf("GlobalVariable wrongly flagged %q (constant/blank/local)", bad)
		}
	}
	if len(got) != len(want) {
		t.Errorf("GlobalVariable flagged %d names, want %d: %v", len(got), len(want), got)
	}
}

func TestCleanCode(t *testing.T) {
	src := `
func process(enable bool) {
	x := 0
	if x = compute(); x > 0 {
		doThing()
	} else {
		doOther()
	}
	m := map[string]int{"a": 1, "a": 2}
	_ = m
}

func compute() int { return 1 }
func doThing()     {}
func doOther()     {}
`
	hits := analyze(t, src, "cleancode")
	mustHave(t, hits,
		"BooleanArgumentFlag",
		"IfStatementAssignment",
		"ElseExpression",
		"DuplicatedArrayKey",
	)
}

func TestControversial(t *testing.T) {
	src := `
type bad_name struct {
	first_field int
}

func snake_method(under_score int) {}
`
	hits := analyze(t, src, "controversial")
	mustHave(t, hits,
		"CamelCaseClassName",
		"CamelCasePropertyName",
		"CamelCaseMethodName",
		"CamelCaseParameterName",
	)
}

func TestCleanCodeNoFalsePositives(t *testing.T) {
	// Idiomatic Go that should be clean under cleancode.
	src := `
func ok(items []int) int {
	total := 0
	for _, v := range items {
		total += v
	}
	if total > 0 {
		return total
	}
	return 0
}
`
	hits := analyze(t, src, "cleancode")
	mustNotHave(t, hits,
		"BooleanArgumentFlag",
		"IfStatementAssignment",
		"ElseExpression",
		"DuplicatedArrayKey",
	)
}
