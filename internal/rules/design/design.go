// Package design implements PHPMD's Design ruleset, adapted to Go. PHP-only
// rules (EvalExpression, NumberOfChildren, DepthOfInheritance) have no Go
// analog and are omitted.
package design

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("PHPMD\\Rule\\Design\\ExitExpression", func() rule.Rule { return &ExitExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\GotoStatement", func() rule.Rule { return &GotoStatement{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CountInLoopExpression", func() rule.Rule { return &CountInLoopExpression{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\DevelopmentCodeFragment", func() rule.Rule { return &DevelopmentCodeFragment{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\EmptyCatchBlock", func() rule.Rule { return &EmptyCatchBlock{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\CouplingBetweenObjects", func() rule.Rule { return &CouplingBetweenObjects{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\GlobalVariable", func() rule.Rule { return &GlobalVariable{Base: rule.NewBase()} })
	rule.Register("PHPMD\\Rule\\Design\\LackOfCohesionOfMethods", func() rule.Rule { return &LackOfCohesionOfMethods{Base: rule.NewBase()} })
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
type GlobalVariable struct{ *rule.Base }

func (r *GlobalVariable) ApplyFile(c *rule.Context) {
	mutated := c.File.MutatedGlobals
	if mutated == nil {
		// Analyzed in isolation (e.g. a single file): fall back to scanning
		// just this file for mutations.
		mutated = util.MutatedGlobalNames([]*ast.File{c.File.Syntax})
	}
	reportImmutable := c.Props().Bool("report-immutable", false)
	for _, g := range packageVars(c.File.Syntax) {
		if mutated[g.name] || reportImmutable {
			line := c.File.Fset.Position(g.pos).Line
			c.Report(line, line, g.name)
		}
	}
}

type pkgVar struct {
	name string
	pos  token.Pos
}

// packageVars returns every package-level variable declared in the file (one
// entry per name), skipping the blank identifier. Constants and locals are not
// package-level vars and are excluded by construction.
func packageVars(f *ast.File) []pkgVar {
	var out []pkgVar
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name != "_" {
					out = append(out, pkgVar{name.Name, name.Pos()})
				}
			}
		}
	}
	return out
}

// ----- ExitExpression -----------------------------------------------------
//
// Detects calls to os.Exit / syscall.Exit (the Go analog of PHP's exit/die).

type ExitExpression struct{ *rule.Base }

func (r *ExitExpression) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	for _, call := range util.Calls(fn.Body, fn.File.Fset) {
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
	if fn.Body == nil {
		return
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if b, ok := n.(*ast.BranchStmt); ok && b.Tok == token.GOTO {
			found = true
			return false
		}
		return true
	})
	if found {
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
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return true
		}
		ast.Inspect(fs.Cond, func(cn ast.Node) bool {
			if ce, ok := cn.(*ast.CallExpr); ok {
				name := util.CalleeName(ce.Fun)
				if loopCountFuncs[name] {
					line := fn.File.Fset.Position(fs.Pos()).Line
					c.ReportFuncAt(fn, line, line, name, "for")
				}
			}
			return true
		})
		return true
	})
}
func (r *CountInLoopExpression) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- DevelopmentCodeFragment --------------------------------------------

type DevelopmentCodeFragment struct{ *rule.Base }

func (r *DevelopmentCodeFragment) check(c *rule.Context, fn *model.Function) {
	if fn.Body == nil {
		return
	}
	unwanted := map[string]bool{}
	for _, f := range util.SplitToList(c.Props().String("unwanted-functions", "println,print")) {
		unwanted[strings.ToLower(strings.TrimSpace(f))] = true
	}
	image := fn.Name
	if fn.IsMethod() {
		image = fn.Receiver + "::" + fn.Name
	}
	for _, call := range util.Calls(fn.Body, fn.File.Fset) {
		if unwanted[strings.ToLower(call.Name)] {
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
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok || ifs.Body == nil || len(ifs.Body.List) != 0 {
			return true
		}
		if conditionChecksNil(ifs.Cond) {
			line := fn.File.Fset.Position(ifs.Pos()).Line
			c.ReportFuncAt(fn, line, line, fn.Name)
		}
		return true
	})
}

func conditionChecksNil(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok && (be.Op == token.NEQ || be.Op == token.EQL) {
			if isNilIdent(be.X) || isNilIdent(be.Y) {
				found = true
			}
		}
		return true
	})
	return found
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}
func (r *EmptyCatchBlock) ApplyFunc(c *rule.Context, fn *model.Function) { r.check(c, fn) }

// ----- CouplingBetweenObjects ---------------------------------------------
//
// Counts the number of distinct (non-builtin) named types a class depends on
// through its field types and method signatures — the Go analog of pdepend's
// CBO metric.

type CouplingBetweenObjects struct{ *rule.Base }

func (r *CouplingBetweenObjects) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maximum", 13)
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
	if cbo >= threshold {
		c.ReportClass(class, class.Name, cbo, threshold)
	}
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

type LackOfCohesionOfMethods struct{ *rule.Base }

func (r *LackOfCohesionOfMethods) ApplyClass(c *rule.Context, class *model.Class) {
	threshold := c.Props().Int("maximum", 1)
	if lcom := lcom4(class); lcom > threshold {
		c.ReportClass(class, class.Name, lcom, threshold)
	}
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
		usedFields, calledMethods := receiverUses(m, fields, methodIdx)
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
		if f := accessorFieldOf(m, fields); f != "" {
			accessorOf[m.Name] = f
		}
	}
	return methodIdx, accessorOf
}

// accessorFieldOf returns the field a trivial getter or setter wraps: the
// method's whole body is `return r.field`, or `r.field = v` with a plain
// identifier or literal v. Anything more (computation, validation, touching a
// second field) makes the method a real behavior carrier and returns "".
func accessorFieldOf(m *model.Function, fields map[string]bool) string {
	if m.Body == nil || len(m.Body.List) != 1 {
		return ""
	}
	switch s := m.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(s.Results) == 1 {
			return fieldSelectorName(s.Results[0], m.RecvName, fields)
		}
	case *ast.AssignStmt:
		return setterFieldOf(s, m.RecvName, fields)
	}
	return ""
}

// setterFieldOf returns the field assigned by a trivial setter statement
// (`r.field = v` with a plain identifier or literal v), or "".
func setterFieldOf(s *ast.AssignStmt, recvName string, fields map[string]bool) string {
	if s.Tok == token.ASSIGN && len(s.Lhs) == 1 && len(s.Rhs) == 1 && isPlainValue(s.Rhs[0]) {
		return fieldSelectorName(s.Lhs[0], recvName, fields)
	}
	return ""
}

// isPlainValue reports whether e is a bare identifier or literal.
func isPlainValue(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	}
	return false
}

// fieldSelectorName returns the field name if e is `<recvName>.<field>`.
func fieldSelectorName(e ast.Expr, recvName string, fields map[string]bool) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if ok && id.Name == recvName && fields[sel.Sel.Name] {
		return sel.Sel.Name
	}
	return ""
}

// receiverUses scans a method body for selector expressions rooted at the
// receiver variable and splits them into used field names and called sibling
// method names. Field and method names cannot collide in Go, so the
// classification is unambiguous; a method value reference (`f := r.m`) counts
// the same as a call, since it ties the methods together just as strongly.
func receiverUses(m *model.Function, fields map[string]bool, methods map[string]int) (usedFields, calledMethods []string) {
	if m.Body == nil || m.RecvName == "" || m.RecvName == "_" {
		return nil, nil
	}
	seen := map[string]bool{}
	ast.Inspect(m.Body, func(n ast.Node) bool {
		name := receiverSelector(n, m.RecvName)
		if name == "" || seen[name] {
			return true
		}
		_, isMethod := methods[name]
		switch {
		case fields[name]:
			seen[name] = true
			usedFields = append(usedFields, name)
		case isMethod:
			seen[name] = true
			calledMethods = append(calledMethods, name)
		}
		return true
	})
	return usedFields, calledMethods
}

// receiverSelector returns the selected name if n is a selector expression on
// the given receiver variable, else "".
func receiverSelector(n ast.Node, recvName string) string {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if id, ok := sel.X.(*ast.Ident); ok && id.Name == recvName {
		return sel.Sel.Name
	}
	return ""
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
