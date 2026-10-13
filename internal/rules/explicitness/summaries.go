package explicitness

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

type callEdge struct {
	call      *ast.CallExpr
	callee    *model.Function
	rangeBody bool
	factory   *callEdge
}

type callableSummary struct {
	facts  *result
	scope  *scope
	calls  []callEdge
	parent *model.Function
}

type effectGraph struct {
	ctx       *rule.Context
	nodes     map[*model.Function]*callableSummary
	order     []*model.Function
	literals  map[*ast.FuncLit]*model.Function
	returning map[*model.Function]bool
	mutations *storageMutations
	strict    bool
}

func analyze(c *rule.Context, root *model.Function, strict bool) *effectGraph {
	graph := &effectGraph{
		ctx:       c,
		nodes:     map[*model.Function]*callableSummary{},
		literals:  map[*ast.FuncLit]*model.Function{},
		returning: map[*model.Function]bool{},
		mutations: newStorageMutations(),
		strict:    strict,
	}
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
	s := newScope(fn, strict, g.mutations)
	if parent != nil {
		inherit(s, g.nodes[parent].scope, fn.Decl.Type.Pos())
		trackOwnership(s)
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
	if fn.Body == nil {
		node.facts.unknown = append(node.facts.unknown, effect{what: "unavailable function body"})
		return
	}
	inspectExecuted(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			g.collectCall(c, node, n, strict)
		case *ast.RangeStmt:
			g.collectRange(c, node, n, strict)
		}
		return true
	})
}

func (g *effectGraph) collectCall(c *rule.Context, node *callableSummary, call *ast.CallExpr, strict bool) {
	callee := g.resolveCallee(node.scope.fn, call.Fun)
	if callee != nil {
		g.add(c, callee, nil, strict)
		node.calls = append(node.calls, callEdge{call: call, callee: callee, factory: g.factoryEdge(node.scope.fn, call.Fun, callee)})
	} else if !establishedCall(node.scope, call) {
		node.facts.unknown = append(node.facts.unknown, unresolvedCallEffect(node.scope, call))
	}
	addArgumentCallbacks(g, c, node.scope.fn, call, strict)
	g.callbackEdges(node, call)
}

func (g *effectGraph) collectRange(c *rule.Context, node *callableSummary, loop *ast.RangeStmt, strict bool) {
	if callee := g.resolveCallee(node.scope.fn, loop.X); callee != nil {
		g.add(c, callee, nil, strict)
		call := &ast.CallExpr{Fun: loop.X, Lparen: loop.X.End()}
		node.calls = append(node.calls, callEdge{call: call, callee: callee, rangeBody: true})
		return
	}
	if rangesOverFunction(node.scope, loop.X) {
		node.facts.unknown = append(node.facts.unknown, node.scope.effect(loop.X, "range over function"))
	}
}

func (g *effectGraph) factoryEdge(fn *model.Function, expression ast.Expr, callee *model.Function) *callEdge {
	call := factoryCall(fn, expression, map[*ast.Object]bool{})
	if call == nil {
		return nil
	}
	factory := model.ResolveCall(fn, call)
	if factory == nil || g.nodes[callee].parent != factory {
		return nil
	}
	return &callEdge{call: call, callee: factory}
}

func factoryCall(fn *model.Function, expression ast.Expr, seen map[*ast.Object]bool) *ast.CallExpr {
	switch value := ast.Unparen(expression).(type) {
	case *ast.CallExpr:
		return value
	case *ast.Ident:
		if value.Obj == nil || value.Obj.Kind != ast.Var || seen[value.Obj] || !singleBinding(fn.File, value.Obj) {
			return nil
		}
		seen[value.Obj] = true
		return factoryCall(fn, bindingExpression(value.Obj), seen)
	}
	return nil
}

func rangesOverFunction(s *scope, x ast.Expr) bool {
	id, ok := ast.Unparen(x).(*ast.Ident)
	if !ok || id.Obj == nil {
		return false
	}
	typ := s.params[id.Obj]
	if typ == nil {
		typ = s.ownership.stateAt(id.Pos()).bindings[id.Obj].typ
	}
	return functionType(s, typ, map[ast.Expr]bool{})
}

func functionType(s *scope, typ ast.Expr, seen map[ast.Expr]bool) bool {
	typ = util.UnwrapTypeInstance(ast.Unparen(typ))
	if typ == nil || seen[typ] {
		return false
	}
	seen[typ] = true
	switch value := typ.(type) {
	case *ast.FuncType:
		return true
	case *ast.SelectorExpr:
		name := s.qualified(value)
		return name == "iter.Seq" || name == "iter.Seq2"
	case *ast.Ident:
		if value.Obj == nil {
			return functionType(s, s.fn.File.PackageTypeDefs[value.Name], seen)
		}
		spec, ok := value.Obj.Decl.(*ast.TypeSpec)
		return ok && functionType(s, spec.Type, seen)
	}
	return false
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

func (g *effectGraph) resolveCallee(fn *model.Function, expression ast.Expr) *model.Function {
	if boundReceiverSelection(fn, expression) {
		return model.ResolveCall(fn, &ast.CallExpr{Fun: util.UnwrapTypeInstance(ast.Unparen(expression))})
	}
	return g.resolve(fn, expression)
}

func boundReceiverSelection(fn *model.Function, expression ast.Expr) bool {
	sel, ok := util.UnwrapTypeInstance(ast.Unparen(expression)).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := sel.X.(*ast.Ident)
	return !ok || model.ImportedPackagePath(fn.File, qualifier) == ""
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
	if boundReceiverSelection(fn, expression) {
		return nil
	}
	return model.ResolveCall(fn, &ast.CallExpr{Fun: expression})
}

func returnedFunction(g *effectGraph, fn *model.Function, call *ast.CallExpr) *model.Function {
	callee := model.ResolveCall(fn, call)
	if callee == nil || callee.Body == nil || g.returning[callee] {
		return nil
	}
	g.returning[callee] = true
	defer delete(g.returning, callee)
	g.add(g.ctx, callee, nil, g.strict)
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
	ret, ok := n.(*ast.ReturnStmt)
	if s.ambiguous || !ok {
		return !s.ambiguous
	}
	var next *model.Function
	if len(ret.Results) == 1 {
		next = g.resolveBinding(callee, ret.Results[0], map[*ast.Object]bool{})
	}
	if next == nil {
		s.ambiguous = true
		return false
	}
	return s.choose(next)
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
		count += bindingsIn(n, object)
		return true
	})
	if _, declared := object.Decl.(*ast.ValueSpec); declared {
		return count == 0
	}
	return count == 1
}

func bindingsIn(node ast.Node, object *ast.Object) int {
	count := 0
	switch node := node.(type) {
	case *ast.AssignStmt:
		for _, target := range node.Lhs {
			if id, ok := target.(*ast.Ident); ok && id.Obj == object {
				count++
			}
		}
	case *ast.UnaryExpr:
		if id, ok := ast.Unparen(node.X).(*ast.Ident); ok && node.Op == token.AND && id.Obj == object {
			count = 2
		}
	}
	return count
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
			node.calls = append(node.calls, callEdge{call: elementCall(call, index, len(callee.Params)), callee: callee})
		}
	}
}

func elementCall(call *ast.CallExpr, index, params int) *ast.CallExpr {
	invocation := &ast.CallExpr{Fun: call.Fun, Lparen: call.Lparen, Rparen: call.Rparen}
	for i := 0; i < params && index > 0; i++ {
		collection := call.Args[min(i, index-1)]
		invocation.Args = append(invocation.Args, &ast.IndexExpr{X: collection, Index: ast.NewIdent("_"), Lbrack: collection.End()})
	}
	return invocation
}

func (g *effectGraph) propagate(fn *model.Function, edge callEdge) bool {
	node, called := g.nodes[fn], g.nodes[edge.callee]
	changed := false
	for _, pair := range [][2]*[]effect{{&node.facts.inputs, &called.facts.inputs}, {&node.facts.outputs, &called.facts.outputs}} {
		if addTranslatedFacts(node.scope, edge, pair[0], *pair[1]) {
			changed = true
		}
	}
	for _, fact := range called.facts.unknown {
		if propagateUnknown(g, node.scope, edge, fact) {
			changed = true
		}
	}
	return changed
}

func addTranslatedFacts(s *scope, edge callEdge, target *[]effect, facts []effect) bool {
	changed := false
	for _, fact := range facts {
		if addTranslatedEffects(target, translateEffect(s, edge, fact)) {
			changed = true
		}
	}
	return changed
}

func propagateUnknown(g *effectGraph, s *scope, edge callEdge, fact effect) bool {
	index := paramIndex(edge.callee, fact.binding)
	if fact.callable && edge.rangeBody && index == 0 {
		return false
	}
	if fact.callable && index >= 0 && index < len(edge.call.Args) {
		if actual := g.resolve(s.fn, edge.call.Args[index]); actual != nil {
			return propagateKnownCallable(g, s, edge, fact, actual)
		}
	}
	return addTranslatedEffects(&g.nodes[s.fn].facts.unknown, translateEffect(s, edge, fact))
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
			for _, local := range translateEffect(g.nodes[edge.callee].scope, callbackEdge, actualFact) {
				if addTranslatedEffects(pair[0], translateEffect(s, edge, local)) {
					changed = true
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

func translateEffect(s *scope, edge callEdge, fact effect) []effect {
	fact.line = s.effect(edge.call, "").line
	switch {
	case fact.callable:
		return []effect{callableEffect(s, edge, fact)}
	case fact.binding == nil:
		return []effect{fact}
	case fact.bindingAccess:
		return variableEffects(s, fact)
	}
	if index := paramIndex(edge.callee, fact.binding); index >= 0 {
		return parameterEffects(s, edge, index, fact)
	}
	if recv, _ := receiverObject(edge.callee.Decl); recv != nil && recv == fact.binding {
		return receiverEffects(s, edge, fact)
	}
	if edge.factory != nil {
		if index := paramIndex(edge.factory.callee, fact.binding); index >= 0 {
			return parameterEffects(s, *edge.factory, index, fact)
		}
	}
	return capturedEffects(s, edge, fact)
}

func capturedEffects(s *scope, edge callEdge, fact effect) []effect {
	if _, local := s.ownership.stateAt(edge.call.Pos()).bindings[fact.binding]; local {
		id := ast.NewIdent(fact.binding.Name)
		id.Obj = fact.binding
		id.NamePos = edge.call.Pos()
		return argumentEffects(s, id, fact)
	}
	if owns(s, fact.binding) {
		return nil
	}
	return []effect{fact}
}

func receiverEffects(s *scope, edge callEdge, fact effect) []effect {
	sel, ok := util.UnwrapTypeInstance(ast.Unparen(edge.call.Fun)).(*ast.SelectorExpr)
	if !ok {
		fact.binding = nil
		return []effect{fact}
	}
	if !receiverTyped(s, sel.X, edge.callee) {
		fact.deep = true
	}
	return argumentEffects(s, sel.X, fact)
}

func receiverTyped(s *scope, x ast.Expr, method *model.Function) bool {
	typ := storageOf(s, x, s.ownership.stateAt(x.Pos())).typ
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	id, ok := util.UnwrapTypeInstance(typ).(*ast.Ident)
	return ok && id.Name == method.Receiver
}

func variableEffects(s *scope, fact effect) []effect {
	if owns(s, fact.binding) {
		return nil
	}
	return []effect{fact}
}

func owns(s *scope, object *ast.Object) bool {
	pos := object.Pos()
	return pos >= s.fn.Decl.Type.Pos() && pos < s.fn.Body.End()
}

func paramIndex(fn *model.Function, object *ast.Object) int {
	if object == nil {
		return -1
	}
	for index, param := range fn.Params {
		if param.Ident != nil && param.Ident.Obj == object {
			return index
		}
	}
	return -1
}

func parameterEffects(s *scope, edge callEdge, index int, fact effect) []effect {
	args, variadic := parameterArguments(edge, index)
	if !variadic {
		if len(args) == 0 {
			fact.binding = nil
			return []effect{fact}
		}
		return argumentEffects(s, args[0], fact)
	}
	if !fact.deep {
		return nil
	}
	var out []effect
	for _, arg := range args {
		out = append(out, argumentEffects(s, arg, fact)...)
	}
	return out
}

func parameterArguments(edge callEdge, index int) ([]ast.Expr, bool) {
	params := edge.callee.Params
	_, ellipsis := params[index].Field.Type.(*ast.Ellipsis)
	if ellipsis && index == len(params)-1 && !edge.call.Ellipsis.IsValid() {
		if index >= len(edge.call.Args) {
			return nil, true
		}
		return edge.call.Args[index:], true
	}
	if index >= len(edge.call.Args) {
		return nil, false
	}
	return edge.call.Args[index : index+1], false
}

func callableEffect(s *scope, edge callEdge, fact effect) effect {
	binding := fact.binding
	if index := paramIndex(edge.callee, binding); index >= 0 {
		binding = argumentObject(edge, index)
	} else if !visible(s, binding, edge.call.Pos()) {
		binding = nil
	}
	args, ok := substituteArguments(s, edge, fact.callableArgs)
	if binding == nil || !ok {
		fact.binding, fact.callable, fact.callableArgs = nil, false, nil
		return fact
	}
	fact.binding, fact.callableArgs = binding, args
	return fact
}

func argumentObject(edge callEdge, index int) *ast.Object {
	if index >= len(edge.call.Args) {
		return nil
	}
	id, ok := ast.Unparen(edge.call.Args[index]).(*ast.Ident)
	if !ok || id.Obj == nil || id.Obj.Kind != ast.Var {
		return nil
	}
	return id.Obj
}

func visible(s *scope, object *ast.Object, position token.Pos) bool {
	if _, tracked := s.ownership.stateAt(position).bindings[object]; tracked {
		return true
	}
	return owns(s, object)
}

func substituteArguments(s *scope, edge callEdge, args []ast.Expr) ([]ast.Expr, bool) {
	out := make([]ast.Expr, 0, len(args))
	for _, arg := range args {
		next, ok := substitute(s, edge, arg)
		if !ok {
			return nil, false
		}
		out = append(out, next)
	}
	return out, true
}

func substitute(s *scope, edge callEdge, expression ast.Expr) (ast.Expr, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return substituteIdent(s, edge, value)
	case *ast.BasicLit:
		return value, true
	case *ast.ParenExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return x })
	case *ast.StarExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return &ast.StarExpr{Star: x.Pos(), X: x} })
	case *ast.UnaryExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return &ast.UnaryExpr{OpPos: x.Pos(), Op: value.Op, X: x} })
	case *ast.SelectorExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return &ast.SelectorExpr{X: x, Sel: value.Sel} })
	case *ast.IndexExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return &ast.IndexExpr{X: x, Index: value.Index, Lbrack: x.End()} })
	case *ast.SliceExpr:
		return rebuild(s, edge, value.X, func(x ast.Expr) ast.Expr { return &ast.SliceExpr{X: x, Lbrack: x.End()} })
	}
	return nil, false
}

func rebuild(s *scope, edge callEdge, inner ast.Expr, wrap func(ast.Expr) ast.Expr) (ast.Expr, bool) {
	x, ok := substitute(s, edge, inner)
	if !ok {
		return nil, false
	}
	return wrap(x), true
}

func substituteIdent(s *scope, edge callEdge, id *ast.Ident) (ast.Expr, bool) {
	if id.Obj == nil || id.Obj.Kind != ast.Var || s.isGlobal(id) {
		return id, true
	}
	if index := paramIndex(edge.callee, id.Obj); index >= 0 {
		args, variadic := parameterArguments(edge, index)
		if variadic || len(args) == 0 {
			return nil, false
		}
		return args[0], true
	}
	_, tracked := s.ownership.stateAt(edge.call.Pos()).bindings[id.Obj]
	return id, tracked
}

func argumentEffects(s *scope, argument ast.Expr, fact effect) []effect {
	storage := storageOf(s, argument, s.ownership.stateAt(argument.Pos()))
	if fact.storageRead && !storage.ambient {
		return nil
	}
	if !storage.outer && !(fact.deep && storage.reachable) {
		return nil
	}
	id := util.RootIdent(argument)
	if id != nil {
		fact.what = argumentPart(s, id, fact.what)
	}
	return originEffects(fact, storage)
}

func argumentPart(s *scope, id *ast.Ident, what string) string {
	if prefix := flowPrefix(what); prefix != "" {
		return prefix + id.Name
	}
	switch {
	case s.isGlobal(id):
		return "package variable " + id.Name
	case s.isReceiver(id) && s.strict && receiverEffect(effect{what: what}):
		return what
	case s.isReceiver(id):
		return "write through receiver " + id.Name
	}
	return "write through parameter " + id.Name
}

func originEffects(fact effect, storage storage) []effect {
	var out []effect
	for origin := range storage.origins {
		fact.binding = origin
		out = append(out, fact)
	}
	if len(out) == 0 || storage.ambient {
		fact.binding = nil
		out = append(out, fact)
	}
	return out
}

func reportableEffects(fn *model.Function, effects []effect, skipReceiver bool) []effect {
	var out []effect
	for _, fact := range effects {
		if skipReceiver && receiverEffect(fact) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func addEffect(effects *[]effect, next effect) bool {
	for index, prior := range *effects {
		if sameEffect(prior, next) {
			if next.line > 0 && (prior.line == 0 || next.line < prior.line) {
				(*effects)[index].line = next.line
				return true
			}
			return false
		}
	}
	*effects = append(*effects, next)
	return true
}

func sameEffect(a, b effect) bool {
	return a.what == b.what && a.binding == b.binding && a.deep == b.deep && a.callable == b.callable && a.bindingAccess == b.bindingAccess && a.storageRead == b.storageRead
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
func (r *UnknownEffect) ApplyFile(c *rule.Context) { applyModule(c, r.ApplyFunc) }

func (r *UnknownEffect) ApplyFunc(c *rule.Context, fn *model.Function) {
	fn = moduleOwner(fn)
	graph := analyze(c, fn, r.includeReceiver)
	for _, callable := range graph.owned(fn) {
		facts := graph.nodes[callable].facts
		unknown := append([]effect(nil), facts.unknown...)
		report(c, callable, reportableEffects(callable, unknown, false))
	}
}

var pureBuiltins = setOf("len", "cap", "make", "new", "append", "copy", "delete", "clear", "close", "min", "max", "complex", "real", "imag", "panic", "recover", "print", "println", "bool", "byte", "rune", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128")

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

func flowPrefix(what string) string {
	for _, prefix := range flowPrefixes {
		if strings.HasPrefix(what, prefix) {
			return prefix
		}
	}
	return ""
}

var flowPrefixes = []string{"send on ", "receive from ", "write to ", "read from "}

func receiverEffect(fact effect) bool { return strings.HasPrefix(fact.what, "receiver ") }
