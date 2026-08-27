// Package rules_test runs each built-in ruleset against crafted Go fixtures and
// asserts the exact set of rules that fire, with their lines. This is the
// automated counterpart to the manual phpmd parity checks: it pins down the
// behavior of every rule analog.
package rules_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func TestUnusedVariableRulesPreserveBindingAndDeclarationForms(t *testing.T) {
	src := `
func inspect(unusedParam, capturedParam int, values []int) {
	read, writeOnly := 1, 2
	_ = read
	writeOnly = 3
	for usedKey, unusedValue := range values {
		_ = usedKey
	}
	captured := 1
	closure := func() {
		unusedParam := 1
		_ = unusedParam
		_ = capturedParam
		_ = captured
	}
	_ = closure
}
`
	hits := analyze(t, src, "unusedcode")
	mustHave(t, hits, "UnusedFormalParameter", "UnusedLocalVariable")
	want := []hit{
		{rule: "UnusedFormalParameter", line: 2},
		{rule: "UnusedLocalVariable", line: 3},
		{rule: "UnusedLocalVariable", line: 6},
	}
	if !slices.Equal(hits, want) {
		t.Fatalf("unused-variable hits = %v, want %v", hits, want)
	}
}

func TestUnusedMemberRulesPreserveFileWideSelectionForms(t *testing.T) {
	src := `
const mapKey = "key"

type widget struct {
	selected int
	literal  int
	mapKey   int
	unused   int
}

func (w *widget) selectedMethod() {}
func (w *widget) unusedMethod()   {}

func inspect(w widget) {
	_ = w.selected
	w.selectedMethod()
	_ = widget{literal: 1}
	_ = map[string]int{mapKey: 1}
}
`
	hits := analyze(t, src, "unusedcode")
	mustHave(t, hits, "UnusedPrivateField", "UnusedPrivateMethod")
	want := []hit{
		{rule: "UnusedPrivateField", line: 7},
		{rule: "UnusedPrivateField", line: 8},
		{rule: "UnusedPrivateMethod", line: 12},
	}
	if !slices.Equal(hits, want) {
		t.Fatalf("unused-member hits = %v, want %v", hits, want)
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

func TestBooleanGetMethodNameSkipsParameterizedByDefault(t *testing.T) {
	hits := analyze(t, `
func getReady(force bool) bool { return force }
`, "naming")
	mustNotHave(t, hits, "BooleanGetMethodName")
}

func TestBooleanGetMethodNameFlagsParameterizedWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rs.xml")
	xml := `<ruleset name="t">
  <rule ref="naming/BooleanGetMethodName">
    <properties><property name="checkParameterizedMethods" value="true"/></properties>
  </rule>
</ruleset>`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	hits := analyze(t, `
func getReady(force bool) bool { return force }
`, path)
	mustHave(t, hits, "BooleanGetMethodName")
}

func TestUnusedFormalParameterSkipsInterfaceMethods(t *testing.T) {
	hits := analyze(t, `
type I interface {
	Do(value int)
}
`, "unusedcode")
	mustNotHave(t, hits, "UnusedFormalParameter")
}

func TestShortVariableIgnoresBlank(t *testing.T) {
	hits := analyze(t, `
func consume(_ int) {}
`, "naming")
	mustNotHave(t, hits, "ShortVariable")
}

func TestEmptyCatchBlockIgnoresEqualityNilCheck(t *testing.T) {
	hits := analyze(t, `
func f(pointer *int) {
	if pointer == nil {}
}
`, "design")
	mustNotHave(t, hits, "EmptyCatchBlock")
}

func TestDuplicatedArrayKeyReportsPackageLevel(t *testing.T) {
	hits := analyze(t, `
var values = map[string]int{
	"x": 1,
	"x": 2,
}
`, "cleancode")
	mustHave(t, hits, "DuplicatedArrayKey")
}

func TestDuplicatedArrayKeyUsesConstantValue(t *testing.T) {
	hits := analyze(t, `
func f() {
	_ = map[int]int{1: 1, 01: 2}
}
`, "cleancode")
	mustHave(t, hits, "DuplicatedArrayKey")
}

func TestGlobalVariableFlagsDelete(t *testing.T) {
	hits := analyze(t, `
var cache = map[string]int{"x": 1}
func reset() { delete(cache, "x") }
`, "design")
	mustHave(t, hits, "GlobalVariable")
}

func TestGlobalVariableFlagsClear(t *testing.T) {
	hits := analyze(t, `
var cache = map[string]int{"x": 1}
func reset() { clear(cache) }
`, "design")
	mustHave(t, hits, "GlobalVariable")
}

func TestGlobalVariableIgnoresReadOnlyCall(t *testing.T) {
	hits := analyze(t, `
var cache = map[string]int{"x": 1}
func peek() { println(len(cache)) }
`, "design")
	mustNotHave(t, hits, "GlobalVariable")
}

func TestUnusedFormalParameterRespectsClosureShadowing(t *testing.T) {
	src := `
func outer(x int) {
	func() {
		x := 1
		_ = x
	}()
}
`
	hits := analyze(t, src, "opinionated")
	mustHave(t, hits, "UnusedFormalParameter")
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

func TestCouplingBetweenObjectsIgnoresBuiltinMapType(t *testing.T) {
	rulesetPath := filepath.Join(t.TempDir(), "coupling.xml")
	rulesetXML := []byte(`<ruleset name="coupling">
  <rule ref="design/CouplingBetweenObjects">
    <properties><property name="maximum" value="1"/></properties>
  </rule>
</ruleset>
`)
	if err := os.WriteFile(rulesetPath, rulesetXML, 0o600); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}

	hits := analyze(t, `
type Box struct {
	values map[string]int
}
`, rulesetPath)
	mustNotHave(t, hits, "CouplingBetweenObjects")
}

// globalVarFixture exercises every classification the GlobalVariable rule must
// make. The mutated vars are written in different ways (reassign, ++, element
// write, address-of); the const-like vars are only ever read; constants, the
// blank identifier, locals, params and shadowing must never be flagged.
const globalVarFixture = `
var counter = 0
var registry = map[string]int{}
var current *node
var sink int

var ErrThing = mkErr()
var table = []int{1, 2, 3}

const MaxRetries = 3

const (
	alpha = 1
	beta  = 2
)

var _ = setup()

func work(n int) int {
	counter++
	registry["k"] = n
	current = nil
	p := &sink
	local := table[0]
	_ = p
	return local + counter
}

type node struct{}

func mkErr() error { return nil }
func setup() int   { return 0 }
`

// flaggedGlobals runs the GlobalVariable rule over the fixture with the given
// ruleset and returns the set of variable names it reports.
func flaggedGlobals(t *testing.T, rulesetID string) map[string]bool {
	t.Helper()
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+globalVarFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sets, err := (&ruleset.Loader{}).Load(rulesetID)
	if err != nil {
		t.Fatalf("load %q: %v", rulesetID, err)
	}
	got := map[string]bool{}
	for _, v := range rule.Analyze(f, sets) {
		if v.Rule.Name() == "GlobalVariable" {
			got[v.Args[0].(string)] = true
		}
	}
	return got
}

// neverFlagged are declarations that must not be reported under any setting:
// constants, the blank identifier, locals, and a parameter.
var neverFlagged = []string{"MaxRetries", "alpha", "beta", "_", "local", "p", "n"}

// TestGlobalVariableDefaultFlagsOnlyMutated verifies the default behavior:
// only package-level variables that are actually mutated are reported, so
// effectively-constant globals (sentinel errors, lookup tables) stay quiet.
func TestGlobalVariableDefaultFlagsOnlyMutated(t *testing.T) {
	got := flaggedGlobals(t, "design")

	// Mutated in various ways: reassigned, ++, element write, address taken.
	wantMutated := []string{"counter", "registry", "current", "sink"}
	for _, w := range wantMutated {
		if !got[w] {
			t.Errorf("expected mutated global %q to be flagged; got %v", w, got)
		}
	}
	// Read-only globals must stay silent by default.
	for _, immutable := range []string{"ErrThing", "table"} {
		if got[immutable] {
			t.Errorf("read-only global %q should not be flagged by default; got %v", immutable, got)
		}
	}
	for _, bad := range neverFlagged {
		if got[bad] {
			t.Errorf("non-global %q wrongly flagged; got %v", bad, got)
		}
	}
	if len(got) != len(wantMutated) {
		t.Errorf("flagged %d globals, want %d: %v", len(got), len(wantMutated), got)
	}
}

// TestGlobalVariableReportImmutable verifies that report-immutable=true also
// surfaces read-only package-level variables, while still never flagging
// constants, the blank identifier, or locals.
func TestGlobalVariableReportImmutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ruleset.xml")
	xml := `<?xml version="1.0"?>
<ruleset name="t">
  <rule ref="design/GlobalVariable">
    <properties><property name="report-immutable" value="true"/></properties>
  </rule>
</ruleset>`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}

	got := flaggedGlobals(t, path)

	wantAll := []string{"counter", "registry", "current", "sink", "ErrThing", "table"}
	for _, w := range wantAll {
		if !got[w] {
			t.Errorf("with report-immutable, expected %q to be flagged; got %v", w, got)
		}
	}
	for _, bad := range neverFlagged {
		if got[bad] {
			t.Errorf("non-global %q wrongly flagged; got %v", bad, got)
		}
	}
	if len(got) != len(wantAll) {
		t.Errorf("flagged %d globals, want %d: %v", len(got), len(wantAll), got)
	}
}

// lcomViolations runs the given ruleset over a fixture and returns the
// LackOfCohesionOfMethods violations, keyed by class name with the reported
// LCOM4 value, so tests assert on behavior (which classes fire, with what
// value) rather than line numbers.
func lcomViolations(t *testing.T, src, rulesetID string) map[string]int {
	t.Helper()
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sets, err := (&ruleset.Loader{}).Load(rulesetID)
	if err != nil {
		t.Fatalf("load %q: %v", rulesetID, err)
	}
	got := map[string]int{}
	for _, v := range rule.Analyze(f, sets) {
		if v.Rule.Name() == "LackOfCohesionOfMethods" {
			got[v.Args[0].(string)] = v.Args[1].(int)
		}
	}
	return got
}

// lcomDisjointFixture is the canonical LCOM4 violation: two clusters of
// methods that each work on their own field and never touch the other's.
const lcomDisjointFixture = `
type server struct {
	conns map[string]int
	stats map[string]int
}

func (s *server) accept(addr string) { s.conns[addr] = 1 }

func (s *server) closeAll() int {
	total := 0
	for range s.conns {
		total++
	}
	return total
}

func (s *server) record(k string) { s.stats[k]++ }

func (s *server) snapshot() int {
	n := 0
	for _, v := range s.stats {
		n += v
	}
	return n
}
`

func TestLackOfCohesionFlagsDisjointMethodGroups(t *testing.T) {
	got := lcomViolations(t, lcomDisjointFixture, "design")
	if got["server"] != 2 {
		t.Errorf("expected server to be flagged with LCOM4 = 2; got %v", got)
	}
	// The rule is part of the default go ruleset too.
	hits := analyze(t, lcomDisjointFixture, "go")
	mustHave(t, hits, "LackOfCohesionOfMethods")
}

// TestLackOfCohesionCohesiveClass verifies the no-fire cases: a class whose
// disjoint field groups are bridged by a method using both, a class bridged by
// an intra-class call, and a class with a single communicating method.
func TestLackOfCohesionCohesiveClass(t *testing.T) {
	src := lcomDisjointFixture + `
func (s *server) report() int { return len(s.conns) + s.snapshot() }

type queue struct {
	items []int
	limit int
}

func (q *queue) push(v int) {
	q.items = append(q.items, v)
}

func (q *queue) full() bool { return len(q.items) >= q.limit }

func (q *queue) setLimit(l int) { q.limit = l + 0 }

type single struct{ x int }

func (s *single) double() int { return s.x * 2 }
`
	hits := analyze(t, src, "design")
	mustNotHave(t, hits, "LackOfCohesionOfMethods")
}

// TestLackOfCohesionIgnoresDataCarriers verifies that trivial getters/setters
// and stateless helpers are not counted: a plain data carrier must not score
// LCOM4 = number of fields.
func TestLackOfCohesionIgnoresDataCarriers(t *testing.T) {
	src := `
type config struct {
	host string
	port int
	tls  bool
}

func (c *config) Host() string     { return c.host }
func (c *config) SetHost(h string) { c.host = h }
func (c *config) Port() int        { return c.port }
func (c *config) SetPort(p int)    { c.port = p }
func (c *config) TLS() bool        { return c.tls }
func (c *config) fresh() *config   { return &config{} }
`
	hits := analyze(t, src, "design")
	mustNotHave(t, hits, "LackOfCohesionOfMethods")
}

// TestLackOfCohesionAccessorCallCountsAsFieldUse: a method reaching a field
// only through that field's getter still joins the field's group, so the
// class below is one connected component, not two.
func TestLackOfCohesionAccessorCallCountsAsFieldUse(t *testing.T) {
	src := `
type meter struct {
	count int
	limit int
}

func (m *meter) Count() int { return m.count }

func (m *meter) bump(n int) { m.count += n }

func (m *meter) over() bool { return m.Count() > m.limit }

func (m *meter) widen(l int) { m.limit = l * 2 }
`
	hits := analyze(t, src, "design")
	mustNotHave(t, hits, "LackOfCohesionOfMethods")
}

// TestLackOfCohesionMaximumProperty verifies the threshold is configurable:
// with maximum=2 the two-group fixture is acceptable.
func TestLackOfCohesionMaximumProperty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ruleset.xml")
	xml := `<?xml version="1.0"?>
<ruleset name="t">
  <rule ref="design/LackOfCohesionOfMethods">
    <properties><property name="maximum" value="2"/></properties>
  </rule>
</ruleset>`
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}
	got := lcomViolations(t, lcomDisjointFixture, path)
	if len(got) != 0 {
		t.Errorf("with maximum=2, expected no violations; got %v", got)
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

func TestLongMethodIgnoreWhitespaceOverride(t *testing.T) {
	src := `
func spaced() {
	// explanation

	value := 1
	_ = value
}
`
	for _, tc := range []struct {
		ignoreWhitespace bool
		wantViolation    bool
	}{
		{ignoreWhitespace: false, wantViolation: true},
		{ignoreWhitespace: true, wantViolation: false},
	} {
		t.Run(fmt.Sprintf("ignore-whitespace=%t", tc.ignoreWhitespace), func(t *testing.T) {
			hits := analyze(t, src, codesizeRuleset(t, "LongMethod", 6, tc.ignoreWhitespace))
			if got := has(hits, "LongMethod"); got != tc.wantViolation {
				t.Fatalf("LongMethod violation = %t, want %t; hits = %v", got, tc.wantViolation, hits)
			}
		})
	}
}

func TestLongClassIgnoreWhitespaceOverride(t *testing.T) {
	src := `
type widget struct {
	// explanation

	value int
}

func (w widget) method() {
	// explanation

	_ = w.value
}
`
	for _, tc := range []struct {
		ignoreWhitespace bool
		wantViolation    bool
	}{
		{ignoreWhitespace: false, wantViolation: true},
		{ignoreWhitespace: true, wantViolation: false},
	} {
		t.Run(fmt.Sprintf("ignore-whitespace=%t", tc.ignoreWhitespace), func(t *testing.T) {
			hits := analyze(t, src, codesizeRuleset(t, "LongClass", 8, tc.ignoreWhitespace))
			if got := has(hits, "LongClass"); got != tc.wantViolation {
				t.Fatalf("LongClass violation = %t, want %t; hits = %v", got, tc.wantViolation, hits)
			}
		})
	}
}

func codesizeRuleset(t *testing.T, ruleName string, minimum int, ignoreWhitespace bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ruleset.xml")
	xml := fmt.Sprintf(`<ruleset name="test">
  <rule name="%s" class="PHPMD\Rule\Design\%s">
    <properties>
      <property name="minimum" value="%d"/>
      <property name="ignore-whitespace" value="%t"/>
    </properties>
  </rule>
</ruleset>
`, ruleName, ruleName, minimum, ignoreWhitespace)
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}
	return path
}
