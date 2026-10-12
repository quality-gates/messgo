package explicitness

import (
	"go/ast"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

type callEdge struct {
	call   *ast.CallExpr
	callee *model.Function
}

type callableSummary struct {
	facts  *result
	scope  *scope
	calls  []callEdge
	parent *model.Function
}

type effectGraph struct {
	nodes    map[*model.Function]*callableSummary
	order    []*model.Function
	literals map[*ast.FuncLit]*model.Function
}

func analyze(c *rule.Context, root *model.Function, strict bool) *effectGraph {
	graph := &effectGraph{nodes: map[*model.Function]*callableSummary{}, literals: map[*ast.FuncLit]*model.Function{}}
	graph.add(c, root, nil, strict)
	for i := 0; i < len(graph.order); i++ {
		graph.collectCalls(c, graph.order[i], strict)
	}
	for changed := true; changed; {
		changed = false
		for _, fn := range graph.order {
			for _, edge := range graph.nodes[fn].calls {
				if graph.propagate(fn, edge) {
					changed = true
				}
			}
		}
	}
	return graph
}

func (g *effectGraph) add(c *rule.Context, fn, parent *model.Function, strict bool) {
	if g.nodes[fn] != nil {
		return
	}
	s := newScope(c, fn, true)
	if parent != nil {
		s.inherit(g.nodes[parent].scope, fn.Decl.Type.Pos())
		s.trackOwnership()
	}
	facts := scanScope(s)
	g.nodes[fn] = &callableSummary{facts: facts, scope: s, parent: parent}
	g.order = append(g.order, fn)
	if fn.Body == nil {
		return
	}
	inspectExecutedCallables(fn.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		child := literalFunction(fn.File, literal)
		g.literals[literal] = child
		g.add(c, child, fn, strict)
		return false
	})
}

func literalFunction(file *model.File, literal *ast.FuncLit) *model.Function {
	child := &model.Function{Name: "<lambda>", File: file, Body: literal.Body,
		Decl: &ast.FuncDecl{Name: ast.NewIdent("<lambda>"), Type: literal.Type, Body: literal.Body}}
	if literal.Type.Params != nil {
		for _, field := range literal.Type.Params.List {
			for _, name := range field.Names {
				child.Params = append(child.Params, &model.Parameter{Name: name.Name, Ident: name, Field: field})
			}
		}
	}
	return child
}

func (g *effectGraph) owned(fn *model.Function) []*model.Function {
	out := []*model.Function{fn}
	for _, child := range g.order {
		if g.nodes[child].parent == fn {
			out = append(out, g.owned(child)...)
		}
	}
	return out
}

func (g *effectGraph) collectCalls(c *rule.Context, fn *model.Function, strict bool) {
	node := g.nodes[fn]
	if !strict && fn.IsMethod() && receiverAccess(fn) {
		node.facts.unknown = append(node.facts.unknown, node.scope.effect(fn.Body, "receiver excluded by policy"))
	}
	if fn.Body == nil {
		node.facts.unknown = append(node.facts.unknown, effect{what: "unavailable function body"})
		return
	}
	inspectExecuted(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := g.resolve(fn, call.Fun)
		if callee != nil {
			g.add(c, callee, nil, strict)
			node.calls = append(node.calls, callEdge{call, callee})
		} else if !establishedCall(node.scope, call) {
			node.facts.unknown = append(node.facts.unknown, unresolvedCallEffect(node.scope, call))
		}
		addArgumentCallbacks(g, c, fn, call, strict)
		g.callbackEdges(node, call)
		return true
	})
}

func unresolvedCallEffect(s *scope, call *ast.CallExpr) effect {
	e := s.effect(call, "unresolved call")
	id, ok := ast.Unparen(call.Fun).(*ast.Ident)
	if ok && id.Obj != nil && id.Obj.Kind == ast.Var {
		e.binding = id.Obj
		e.callable = true
		e.callableArgs = call.Args
	}
	return e
}

func addArgumentCallbacks(g *effectGraph, c *rule.Context, fn *model.Function, call *ast.CallExpr, strict bool) {
	for _, arg := range call.Args {
		if callee := g.resolve(fn, arg); callee != nil {
			g.add(c, callee, nil, strict)
		}
	}
}

func (g *effectGraph) resolve(fn *model.Function, expression ast.Expr) *model.Function {
	return g.resolveBinding(fn, expression, map[*ast.Object]bool{})
}

func (g *effectGraph) resolveBinding(fn *model.Function, expression ast.Expr, seen map[*ast.Object]bool) *model.Function {
	expression = util.UnwrapTypeInstance(ast.Unparen(expression))
	if literal, ok := expression.(*ast.FuncLit); ok {
		return g.literals[literal]
	}
	if call, ok := expression.(*ast.CallExpr); ok {
		return returnedFunction(g, fn, call)
	}
	if id, ok := expression.(*ast.Ident); ok && id.Obj != nil && id.Obj.Kind == ast.Var {
		if seen[id.Obj] || !singleBinding(fn.File, id.Obj) {
			return nil
		}
		seen[id.Obj] = true
		return g.resolveBinding(fn, bindingExpression(id.Obj), seen)
	}
	return model.ResolveCall(fn, &ast.CallExpr{Fun: expression})
}

func returnedFunction(g *effectGraph, fn *model.Function, call *ast.CallExpr) *model.Function {
	callee := model.ResolveCall(fn, call)
	if callee == nil || callee.Body == nil {
		return nil
	}
	selection := &returnSelection{}
	inspectExecuted(callee.Body, func(n ast.Node) bool {
		return selection.visit(g, callee, n)
	})
	if selection.ambiguous {
		return nil
	}
	return selection.returned
}

type returnSelection struct {
	returned  *model.Function
	ambiguous bool
}

func (s *returnSelection) visit(g *effectGraph, callee *model.Function, n ast.Node) bool {
	if s.ambiguous {
		return false
	}
	next := returnedCallBinding(g, callee, n)
	if next == nil {
		return true
	}
	return s.choose(next)
}

func returnedCallBinding(g *effectGraph, callee *model.Function, n ast.Node) *model.Function {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil
	}
	return g.resolveBinding(callee, ret.Results[0], map[*ast.Object]bool{})
}

func (s *returnSelection) choose(next *model.Function) bool {
	if s.returned != nil && s.returned != next {
		s.ambiguous = true
		return false
	}
	s.returned = next
	return true
}

func bindingExpression(object *ast.Object) ast.Expr {
	switch decl := object.Decl.(type) {
	case *ast.AssignStmt:
		if len(decl.Rhs) == 1 {
			return decl.Rhs[0]
		}
	case *ast.ValueSpec:
		if len(decl.Values) == 1 {
			return decl.Values[0]
		}
	}
	return nil
}

func singleBinding(file *model.File, object *ast.Object) bool {
	count := 0
	ast.Inspect(file.Syntax, func(n ast.Node) bool {
		if assignment, ok := n.(*ast.AssignStmt); ok {
			for _, target := range assignment.Lhs {
				if id, ok := target.(*ast.Ident); ok && id.Obj == object {
					count++
				}
			}
		}
		return true
	})
	if _, declared := object.Decl.(*ast.ValueSpec); declared {
		return count == 0
	}
	return count == 1
}

func (g *effectGraph) callbackEdges(node *callableSummary, call *ast.CallExpr) {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	indices := immediateCallbacks[node.scope.qualified(selector)]
	for _, index := range indices {
		if index >= len(call.Args) {
			continue
		}
		callee := g.resolve(node.scope.fn, call.Args[index])
		if callee == nil {
			node.facts.unknown = append(node.facts.unknown, node.scope.effect(call, "unresolved callback"))
		} else {
			node.calls = append(node.calls, callEdge{call, callee})
		}
	}
}

func (g *effectGraph) propagate(fn *model.Function, edge callEdge) bool {
	node, called := g.nodes[fn], g.nodes[edge.callee]
	changed := false
	for _, pair := range [][2]*[]effect{{&node.facts.inputs, &called.facts.inputs}, {&node.facts.outputs, &called.facts.outputs}} {
		for _, fact := range *pair[1] {
			for _, translated := range g.translateEffect(node.scope, edge, fact) {
				if addEffect(pair[0], translated) {
					changed = true
				}
			}
		}
	}
	for _, fact := range called.facts.unknown {
		if propagateUnknown(g, node.scope, edge, fact) {
			changed = true
		}
	}
	return changed
}

func propagateUnknown(g *effectGraph, s *scope, edge callEdge, fact effect) bool {
	if !fact.callable {
		return addTranslatedEffects(&g.nodes[s.fn].facts.unknown, g.translateEffect(s, edge, fact))
	}
	for index, param := range edge.callee.Params {
		if param.Ident.Obj == fact.binding && index < len(edge.call.Args) {
			return propagateCallableArgument(g, s, edge, fact, edge.call.Args[index])
		}
	}
	return addTranslatedEffects(&g.nodes[s.fn].facts.unknown, g.translateEffect(s, edge, fact))
}

func propagateCallableArgument(g *effectGraph, s *scope, edge callEdge, fact effect, argument ast.Expr) bool {
	actual := g.resolve(s.fn, argument)
	if actual == nil {
		fact.line = s.effect(edge.call, "").line
		if id, ok := ast.Unparen(argument).(*ast.Ident); ok && id.Obj != nil {
			fact.binding = id.Obj
		} else {
			fact.binding = nil
			fact.callable = false
			fact.callableArgs = nil
		}
		return addEffect(&g.nodes[s.fn].facts.unknown, fact)
	}
	return propagateKnownCallable(g, s, edge, fact, actual)
}

func propagateKnownCallable(g *effectGraph, s *scope, edge callEdge, fact effect, actual *model.Function) bool {
	actualNode := g.nodes[actual]
	if actualNode == nil {
		return false
	}
	invocation := &ast.CallExpr{Fun: ast.NewIdent(actual.Name), Args: fact.callableArgs}
	invocation.Lparen = edge.call.Lparen
	callbackEdge := callEdge{call: invocation, callee: actual}
	changed := false
	for _, pair := range [][2]*[]effect{{&g.nodes[s.fn].facts.inputs, &actualNode.facts.inputs}, {&g.nodes[s.fn].facts.outputs, &actualNode.facts.outputs}, {&g.nodes[s.fn].facts.unknown, &actualNode.facts.unknown}} {
		for _, actualFact := range *pair[1] {
			for _, local := range g.translateEffect(g.nodes[edge.callee].scope, callbackEdge, actualFact) {
				for _, translated := range g.translateEffect(s, edge, local) {
					if addEffect(pair[0], translated) {
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func addTranslatedEffects(effects *[]effect, translated []effect) bool {
	changed := false
	for _, fact := range translated {
		if addEffect(effects, fact) {
			changed = true
		}
	}
	return changed
}

func (g *effectGraph) translateEffect(s *scope, edge callEdge, fact effect) []effect {
	if g.nodes[edge.callee].parent != s.fn {
		fact.line = s.effect(edge.call, "").line
	}
	if fact.binding == nil {
		return []effect{fact}
	}
	for index, param := range edge.callee.Params {
		if param.Ident.Obj == fact.binding && index < len(edge.call.Args) {
			return argumentEffects(s, edge.call.Args[index], fact)
		}
	}
	if _, local := s.storageAt(edge.call.Pos())[fact.binding]; local {
		id := ast.NewIdent(fact.binding.Name)
		id.Obj = fact.binding
		id.NamePos = edge.call.Pos()
		return argumentEffects(s, id, fact)
	}
	if fact.binding.Pos() >= s.fn.Body.Pos() && fact.binding.Pos() < s.fn.Body.End() {
		return nil
	}
	// A deferred closure's captured binding belongs to its lexical environment.
	return []effect{fact}
}

func argumentEffects(s *scope, argument ast.Expr, fact effect) []effect {
	storage := storageOf(s, argument, s.storageAt(argument.Pos()))
	if !storage.outer && !(fact.deep && storage.reachable) {
		return nil
	}
	id := util.RootIdent(argument)
	if id != nil {
		fact.what = "write through parameter " + id.Name
		if s.isGlobal(id) {
			fact.what = "package variable " + id.Name
		}
	}
	var out []effect
	for origin := range storage.origins {
		fact.binding = origin
		out = append(out, fact)
	}
	if len(out) == 0 {
		fact.binding = nil
		out = append(out, fact)
	}
	return out
}

func reportableEffects(g *effectGraph, fn *model.Function, effects []effect, skipReceiver bool) []effect {
	var out []effect
	for _, fact := range effects {
		if skipReceiver && receiverEffect(fact) {
			continue
		}
		if privateParentCapture(g, fn, fact) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func privateParentCapture(g *effectGraph, fn *model.Function, fact effect) bool {
	if fact.binding == nil {
		return false
	}
	for parent := g.nodes[fn].parent; parent != nil; parent = g.nodes[parent].parent {
		scope := g.nodes[parent].scope
		storage, ok := scope.storageAt(fn.Decl.Type.Pos())[fact.binding]
		if !ok {
			continue
		}
		return !storage.outer && !storage.reachable
	}
	return false
}

func addEffect(effects *[]effect, next effect) bool {
	for _, prior := range *effects {
		if prior.what == next.what && prior.binding == next.binding && prior.deep == next.deep && prior.callable == next.callable {
			return false
		}
	}
	*effects = append(*effects, next)
	return true
}

func establishedCall(s *scope, call *ast.CallExpr) bool {
	switch fun := util.UnwrapTypeInstance(ast.Unparen(call.Fun)).(type) {
	case *ast.Ident:
		if fun.Obj != nil {
			return fun.Obj.Kind == ast.Typ
		}
		return pureBuiltins[fun.Name] || s.fn.File.PackageTypeDefs[fun.Name] != nil
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.StarExpr:
		return true
	case *ast.SelectorExpr:
		name := s.qualified(fun)
		if knownOperation(name) {
			return true
		}
		if knownReceiverOperation(s, fun) {
			return true
		}
		if call, ok := ast.Unparen(fun.X).(*ast.CallExpr); ok {
			return establishedCall(s, call)
		}
	}
	return false
}

func knownOperation(name string) bool {
	_, writer := streamWriters[name]
	_, reader := streamReaders[name]
	_, callback := immediateCallbacks[name]
	return pureOperations[name] || ambientInputs[name] || ambientOutputs[name] || util.InPlaceChanges[name] || writer || reader || callback
}

// UnknownEffect preserves uncertainty even when a callable has known actions.
type UnknownEffect struct {
	*rule.Base
	includeReceiver bool
}

func newUnknownEffect() rule.Rule { return &UnknownEffect{Base: rule.NewBase(receiverProperty)} }
func (r *UnknownEffect) Configure(props rule.Properties) error {
	r.includeReceiver = props.Bool(receiverProperty, false)
	return nil
}
func (r *UnknownEffect) ApplyFunc(c *rule.Context, fn *model.Function) {
	graph := analyze(c, fn, r.includeReceiver)
	for _, callable := range graph.owned(fn) {
		facts := graph.nodes[callable].facts
		unknown := append([]effect(nil), facts.unknown...)
		report(c, callable, reportableEffects(graph, callable, unknown, false))
	}
}

func receiverAccess(fn *model.Function) bool {
	recv, _ := receiverObject(fn.Decl)
	found := false
	inspectExecuted(fn.Body, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok && id.Obj == recv {
			found = true
		}
		return true
	})
	return found
}

var pureBuiltins = setOf("len", "cap", "make", "new", "append", "copy", "delete", "clear", "min", "max", "complex", "real", "imag", "panic", "recover", "print", "println", "bool", "byte", "rune", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128")

var pureOperations = setOf(
	"slices.Clone", "slices.Index", "slices.Contains", "slices.Equal", "slices.Compare", "slices.IsSorted", "maps.Clone", "maps.Equal",
	"fmt.Sprintf", "fmt.Sprint", "fmt.Sprintln", "fmt.Errorf", "errors.New", "errors.Join", "strconv.Itoa", "strconv.Atoi", "strconv.FormatInt", "strconv.ParseInt",
	"strings.ToUpper", "strings.ToLower", "strings.TrimSpace", "strings.Trim", "strings.Split", "strings.Join", "strings.Contains", "strings.HasPrefix", "strings.HasSuffix", "strings.ReplaceAll", "strings.Index", "strings.Repeat", "strings.Compare",
	"math.Abs", "math.Sqrt", "math.Pow", "math.Floor", "math.Ceil", "math.Round", "math.Min", "math.Max", "math.Sin", "math.Cos",
)

var immediateCallbacks = map[string][]int{
	"slices.DeleteFunc": {1}, "slices.CompactFunc": {1}, "slices.SortFunc": {1}, "slices.SortStableFunc": {1}, "slices.IndexFunc": {1}, "slices.ContainsFunc": {1}, "slices.EqualFunc": {2}, "slices.CompareFunc": {2}, "slices.IsSortedFunc": {1}, "sort.Slice": {1}, "sort.SliceStable": {1}, "path/filepath.Walk": {1}, "path/filepath.WalkDir": {1},
}

func knownReceiverOperation(s *scope, selector *ast.SelectorExpr) bool {
	id, ok := ast.Unparen(selector.X).(*ast.Ident)
	if !ok || id.Obj == nil {
		return false
	}
	typ := s.params[id.Obj]
	if typ == nil {
		return false
	}
	if selector.Sel.Name != "Done" {
		return false
	}
	expr, ok := ast.Unparen(typ).(*ast.SelectorExpr)
	if !ok || expr.Sel.Name != "Context" {
		return false
	}
	pkg, ok := ast.Unparen(expr.X).(*ast.Ident)
	return ok && model.ImportedPackagePath(s.fn.File, pkg) == "context"
}

func receiverEffect(fact effect) bool { return strings.HasPrefix(fact.what, "receiver ") }
