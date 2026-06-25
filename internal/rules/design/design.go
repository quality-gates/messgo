// Package design implements PHPMD's Design ruleset, adapted to Go. PHP-only
// rules (EvalExpression, NumberOfChildren, DepthOfInheritance) have no Go
// analog and are omitted.
package design

import (
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\Design\\ExitExpression", func() rule.Rule { return &ExitExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\GotoStatement", func() rule.Rule { return &GotoStatement{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CountInLoopExpression", func() rule.Rule { return &CountInLoopExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\DevelopmentCodeFragment", newDevelopmentCodeFragment)
	rule.Register("PHPMD\\Rule\\Design\\EmptyCatchBlock", func() rule.Rule { return &EmptyCatchBlock{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CouplingBetweenObjects", newCouplingBetweenObjects)
	rule.Register("PHPMD\\Rule\\Design\\GlobalVariable", newGlobalVariable)
	rule.Register("PHPMD\\Rule\\Design\\LackOfCohesionOfMethods", newLackOfCohesionOfMethods)
}

// ----- GlobalVariable -----------------------------------------------------
//
// Flags mutable package-level variables (top-level `var` declarations). Global
// mutable state hurts testability and is unsafe under concurrency.
//
// By default only variables that are actually mutated somewhere in the package
// are reported — reassigned, incremented/decremented, written through
// (`g.f = x`, `g[k] = v`), or having their address taken. Package-level
// variables that are only ever read after initialization (sentinel errors,
// compiled regexps, lookup tables) are effectively constant and stay silent, so
// the genuinely risky shared state is not drowned out. Set the `report-immutable`
// property to also flag those read-only globals.
//
// Mutation analysis is package-wide: a variable declared in one file but
// reassigned in another is correctly reported. Only the file's top-level
// declarations are inspected for what to report, so locals are never flagged;
// constants and the blank identifier (`var _ = ...`) are ignored.
type GlobalVariable struct {
	*rule.Base
	reportImmutable bool
}

func newGlobalVariable() rule.Rule {
	return &GlobalVariable{Base: rule.NewBase()}
}

func (r *GlobalVariable) Configure(props rule.Properties) error {
	r.reportImmutable = props.Bool("report-immutable", false)
	return nil
}

func (r *GlobalVariable) ApplyFile(c *rule.Context) {
	mutated := c.File.MutatedPackageGlobals()
	for _, g := range c.File.PackageVars() {
		if mutated[g.Name] || r.reportImmutable {
			c.Report(g.Line, g.Line, g.Name)
		}
	}
}

// ----- ExitExpression -----------------------------------------------------
//
// Detects calls to os.Exit / syscall.Exit (the Go analog of PHP's exit/die).

type ExitExpression struct{ *rule.Base }

func (r *ExitExpression) check(c *rule.Context, fn *model.Function) {
	for _, call := range fn.Calls() {
		if call.Name == "os.Exit" || call.Name == "syscall.Exit" {
			c.ReportFuncAt(fn, call.Line, call.Line, string(fn.NodeType()), fn.Name)
			return
		}
	}
}
func (r *ExitExpression) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- GotoStatement ------------------------------------------------------

type GotoStatement struct{ *rule.Base }

func (r *GotoStatement) check(c *rule.Context, fn *model.Function) {
	if fn.HasGoto() {
		c.ReportFunc(fn, string(fn.NodeType()), fn.Name)
	}
}
func (r *GotoStatement) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CountInLoopExpression ----------------------------------------------
//
// Detects len()/cap() calls in a loop condition, which are re-evaluated every
// iteration (the Go analog of count()/sizeof() in PHP loops).

type CountInLoopExpression struct{ *rule.Base }

var loopCountFuncs = map[string]bool{"len": true, "cap": true}

func (r *CountInLoopExpression) check(c *rule.Context, fn *model.Function) {
	for _, call := range fn.LoopConditionCalls(loopCountFuncs) {
		c.ReportFuncAt(fn, call.Line, call.Line, call.Name, "for")
	}
}
func (r *CountInLoopExpression) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- DevelopmentCodeFragment --------------------------------------------

type DevelopmentCodeFragment struct {
	*rule.Base
	unwantedFunctions map[string]bool
}

func newDevelopmentCodeFragment() rule.Rule {
	return &DevelopmentCodeFragment{Base: rule.NewBase()}
}

func (r *DevelopmentCodeFragment) Configure(props rule.Properties) error {
	r.unwantedFunctions = map[string]bool{}
	for _, f := range util.SplitToList(props.String("unwanted-functions", "println,print")) {
		r.unwantedFunctions[strings.ToLower(strings.TrimSpace(f))] = true
	}
	return nil
}

func (r *DevelopmentCodeFragment) check(c *rule.Context, fn *model.Function) {
	image := fn.Name
	if fn.IsMethod() {
		image = fn.Receiver + "::" + fn.Name
	}
	for _, call := range fn.Calls() {
		if r.unwantedFunctions[strings.ToLower(call.Name)] {
			c.ReportFuncAt(fn, call.Line, call.Line, string(fn.NodeType()), image, call.Name)
		}
	}
}
func (r *DevelopmentCodeFragment) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- EmptyCatchBlock ----------------------------------------------------
//
// Go has no try/catch. The analog of a swallowed exception is an empty
// error-handling block: `if err != nil { }`.

type EmptyCatchBlock struct{ *rule.Base }

func (r *EmptyCatchBlock) check(c *rule.Context, fn *model.Function) {
	for _, line := range fn.EmptyNilCheckBlockLines() {
		c.ReportFuncAt(fn, line, line, fn.Name)
	}
}
func (r *EmptyCatchBlock) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CouplingBetweenObjects ---------------------------------------------
//
// Counts the number of distinct (non-builtin) named types a class depends on
// through its field types and method signatures — the Go analog of pdepend's
// CBO metric.

func designClassNameMeasurement(class *model.Class, value int) rule.ThresholdMeasurement {
	return rule.ThresholdMeasurement{Value: value, Args: []any{class.Name}}
}

type CouplingBetweenObjects struct {
	*rule.Base
	*rule.ThresholdRule
}

func newCouplingBetweenObjects() rule.Rule {
	r := &CouplingBetweenObjects{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maximum",
		Default:     13,
		Boundary:    rule.AtOrAbove,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *CouplingBetweenObjects) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	types := map[string]bool{}
	collect := func(t string) {
		if name := baseTypeName(t); name != "" && !builtinTypes[name] {
			types[name] = true
		}
	}
	for _, f := range class.Fields {
		collect(f.Type)
	}
	for _, m := range class.Methods {
		for _, p := range m.Params {
			collect(p.Type)
		}
		for _, res := range m.Results {
			collect(res.Type)
		}
	}
	cbo := len(types)
	return designClassNameMeasurement(class, cbo), true
}

// baseTypeName strips pointer/slice/map decorations to the leading type name.
func baseTypeName(t string) string {
	t = strings.TrimLeft(t, "*[]")
	t = strings.TrimPrefix(t, "...")
	if i := strings.IndexByte(t, '['); i >= 0 {
		t = t[:i]
	}
	if i := strings.IndexByte(t, '.'); i >= 0 {
		t = t[i+1:]
	}
	return strings.TrimLeft(t, "*")
}

// ----- LackOfCohesionOfMethods ----------------------------------------------
//
// Computes the LCOM4 cohesion metric for a class: methods are nodes of a
// graph, with an edge between two methods when they use a common struct field
// or when one calls the other through the receiver. The metric is the number
// of connected components; a value above 1 means the class contains disjoint
// method groups that share no state, i.e. it bundles unrelated
// responsibilities and is a candidate for splitting.
//
// Two refinements keep the metric honest on idiomatic Go:
//
//   - Methods that use no fields and neither call nor are called by another
//     method (pure helpers, interface stubs) are left out of the count.
//   - Trivial accessors — a body that only returns one field, or only assigns
//     a plain value to one field — are not graph nodes. They carry no behavior
//     of their own, so counting them flags plain data carriers (a struct with
//     one getter per field scores LCOM4 = number of fields). A call to an
//     accessor instead counts as a use of the underlying field.
//
// Analysis is per file, matching how methods are attached to their class
// elsewhere in messgo.

type LackOfCohesionOfMethods struct {
	*rule.Base
	*rule.ThresholdRule
}

func newLackOfCohesionOfMethods() rule.Rule {
	r := &LackOfCohesionOfMethods{Base: rule.NewBase()}
	r.ThresholdRule = rule.NewThresholdRule(rule.ThresholdDeclaration{
		Property:    "maximum",
		Default:     1,
		Boundary:    rule.Above,
		NodeKind:    rule.ThresholdClass,
		ClassMetric: r.measure,
	})
	return r
}

func (r *LackOfCohesionOfMethods) measure(_ *rule.Context, class *model.Class) (rule.ThresholdMeasurement, bool) {
	return designClassNameMeasurement(class, lcom4(class)), true
}

// lcom4 returns the number of connected components among the class's
// communicating methods (those that touch a field or participate in an
// intra-class call). A class with no such methods is trivially cohesive (1).
func lcom4(class *model.Class) int {
	fields := fieldNameSet(class)
	methodIdx, accessorOf := indexMethods(class, fields)
	g := newCohesionGraph(len(class.Methods))
	for i, m := range class.Methods {
		if accessorOf[m.Name] != "" {
			continue
		}
		usedFields, calledMethods := m.ReceiverUses(fields, methodIdx)
		for _, f := range usedFields {
			g.addFieldUse(i, f)
		}
		for _, callee := range calledMethods {
			if f := accessorOf[callee]; f != "" {
				g.addFieldUse(i, f)
			} else {
				g.addCall(i, methodIdx[callee])
			}
		}
	}
	return g.components()
}

func fieldNameSet(class *model.Class) map[string]bool {
	fields := map[string]bool{}
	for _, f := range class.Fields {
		fields[f.Name] = true
	}
	return fields
}

// indexMethods maps each method name to its position in class.Methods, and
// each trivial accessor's name to the field it wraps.
func indexMethods(class *model.Class, fields map[string]bool) (methodIdx map[string]int, accessorOf map[string]string) {
	methodIdx = map[string]int{}
	accessorOf = map[string]string{}
	for i, m := range class.Methods {
		methodIdx[m.Name] = i
		if f := m.AccessorField(fields); f != "" {
			accessorOf[m.Name] = f
		}
	}
	return methodIdx, accessorOf
}

// cohesionGraph is a union-find over a class's methods. Methods become
// "active" (counted) once they use a field or sit on either end of a call.
type cohesionGraph struct {
	parent     []int
	active     []bool
	fieldOwner map[string]int // field name -> first method seen using it
}

func newCohesionGraph(n int) *cohesionGraph {
	g := &cohesionGraph{
		parent:     make([]int, n),
		active:     make([]bool, n),
		fieldOwner: map[string]int{},
	}
	for i := range g.parent {
		g.parent[i] = i
	}
	return g
}

func (g *cohesionGraph) addFieldUse(method int, field string) {
	g.active[method] = true
	if owner, ok := g.fieldOwner[field]; ok {
		g.union(method, owner)
		return
	}
	g.fieldOwner[field] = method
}

func (g *cohesionGraph) addCall(caller, callee int) {
	g.active[caller], g.active[callee] = true, true
	g.union(caller, callee)
}

func (g *cohesionGraph) union(a, b int) {
	g.parent[g.find(a)] = g.find(b)
}

// find returns the union-find root of x, halving paths as it goes.
func (g *cohesionGraph) find(x int) int {
	for g.parent[x] != x {
		g.parent[x] = g.parent[g.parent[x]]
		x = g.parent[x]
	}
	return x
}

// components counts distinct roots among active methods; a class with no
// active methods is trivially cohesive (1).
func (g *cohesionGraph) components() int {
	roots := map[int]bool{}
	for i, on := range g.active {
		if on {
			roots[g.find(i)] = true
		}
	}
	if len(roots) == 0 {
		return 1
	}
	return len(roots)
}

var builtinTypes = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "byte": true, "rune": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"error": true, "any": true, "interface{}": true, "struct{}": true,
}
