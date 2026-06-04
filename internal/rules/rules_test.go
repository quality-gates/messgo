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
