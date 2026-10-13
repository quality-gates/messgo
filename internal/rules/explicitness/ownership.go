package explicitness

import (
	"go/ast"
	"go/token"
	"maps"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/util"
)

type storage struct {
	typ              ast.Expr
	outer, reachable bool
	ambient          bool
	origins          map[*ast.Object]bool
}

var ambientStorage = storage{outer: true, reachable: true, ambient: true}

type state map[*ast.Object]storage

type ownershipSnapshot struct {
	position token.Pos
	values   state
}

type ownershipSnapshots []ownershipSnapshot

func initialState(s *scope) state {
	current := maps.Clone(s.captures)
	if current == nil {
		current = state{}
	}
	for object, typ := range s.params {
		current[object] = parameterStorage(s, object, typ)
	}
	for _, result := range s.fn.Results {
		if result.Ident != nil && result.Ident.Obj != nil {
			current[result.Ident.Obj] = storage{typ: result.Field.Type}
		}
	}
	if s.recv != nil {
		current[s.recv] = parameterStorage(s, s.recv, s.fn.Decl.Recv.List[0].Type)
	}
	return current
}

func parameterStorage(s *scope, object *ast.Object, typ ast.Expr) storage {
	fact := typeStorage(typ, s.fn.File.PackageTypeDefs)
	fact.origins = map[*ast.Object]bool{object: true}
	return fact
}

func inherit(s *scope, parent *scope, position token.Pos) {
	s.captures = maps.Clone(parent.ownership.stateAt(position))
}

func (snapshots ownershipSnapshots) stateAt(position token.Pos) state {
	var values state
	best := token.NoPos
	for _, snapshot := range snapshots {
		if snapshot.position <= position && snapshot.position >= best {
			values, best = snapshot.values, snapshot.position
		}
	}
	return values
}

func joinStates(states ...state) state {
	out := state{}
	for _, values := range states {
		for object, fact := range values {
			if prior, ok := out[object]; ok {
				fact = joinStorage(prior, fact)
			}
			out[object] = fact
		}
	}
	return out
}

func sameStates(a, b state) bool {
	if len(a) != len(b) {
		return false
	}
	for object, fact := range a {
		other, ok := b[object]
		if !ok || !sameStorage(fact, other) {
			return false
		}
	}
	return true
}

func sameStorage(a, b storage) bool {
	return a.outer == b.outer && a.reachable == b.reachable && a.ambient == b.ambient && maps.Equal(a.origins, b.origins)
}

func joinStorage(a, b storage) storage {
	if a.typ == nil {
		a.typ = b.typ
	}
	a.outer = a.outer || b.outer
	a.reachable = a.reachable || b.reachable
	a.ambient = a.ambient || b.ambient
	if len(b.origins) == 0 {
		return a
	}
	origins := maps.Clone(a.origins)
	if origins == nil {
		origins = map[*ast.Object]bool{}
	}
	maps.Copy(origins, b.origins)
	a.origins = origins
	return a
}

func contained(fact storage) storage {
	return storage{reachable: fact.outer || fact.reachable, ambient: fact.ambient, origins: fact.origins}
}

func elementOf(fact storage) storage {
	if !fact.reachable {
		return storage{}
	}
	return storage{outer: true, reachable: true, ambient: fact.ambient, origins: fact.origins}
}

func storageOf(s *scope, expression ast.Expr, current state) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return identStorage(s, value, current)
	case *ast.SliceExpr:
		return storageOf(s, value.X, current)
	case *ast.StarExpr:
		return storageOf(s, value.X, current)
	case *ast.TypeAssertExpr:
		return storageOf(s, value.X, current)
	case *ast.UnaryExpr:
		return unaryStorage(s, value, current)
	case *ast.CallExpr:
		return callStorage(s, value, current)
	case *ast.CompositeLit:
		return compositeStorage(s, value, current)
	case *ast.BasicLit, *ast.FuncLit, *ast.BinaryExpr:
		return storage{}
	}
	return selectedStorage(s, expression, current)
}

func identStorage(s *scope, id *ast.Ident, current state) storage {
	if s.isGlobal(id) {
		return ambientStorage
	}
	if id.Obj == nil || id.Obj.Kind != ast.Var {
		return storage{}
	}
	if fact, tracked := current[id.Obj]; tracked {
		return fact
	}
	return ambientStorage
}

func unaryStorage(s *scope, value *ast.UnaryExpr, current state) storage {
	fact := storageOf(s, value.X, current)
	if value.Op == token.ARROW {
		return elementOf(fact)
	}
	return fact
}

func selectedStorage(s *scope, expression ast.Expr, current state) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.SelectorExpr:
		if s.qualified(value) != "" {
			return ambientStorage
		}
		fact := storageOf(s, value.X, current)
		fact.outer = fact.outer || fact.reachable
		return fact
	case *ast.IndexExpr:
		fact := storageOf(s, value.X, current)
		fact.outer = fact.reachable
		return fact
	}
	return ambientStorage
}

func compositeStorage(s *scope, value *ast.CompositeLit, current state) storage {
	fact := storage{typ: value.Type}
	for _, element := range value.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			element = pair.Value
		}
		fact = joinStorage(fact, contained(storageOf(s, element, current)))
	}
	return fact
}

func builtinName(call *ast.CallExpr) string {
	if name, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && name.Obj == nil {
		return name.Name
	}
	return ""
}

func typeStorage(typ ast.Expr, definitions map[string]ast.Expr) storage {
	sharing := util.TypeSharing(typ, definitions)
	return storage{typ: typ, outer: sharing.Outer, reachable: sharing.Reachable}
}

type returnSummaries struct {
	done   map[*model.Function]storage
	active map[*model.Function]bool
}

func newReturnSummaries() *returnSummaries {
	return &returnSummaries{done: map[*model.Function]storage{}, active: map[*model.Function]bool{}}
}

var builtinFunctions = setOf("append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover")

func callStorage(s *scope, call *ast.CallExpr, current state) storage {
	if fact, known := libraryStorage(s, call, current); known {
		return fact
	}
	if conversion(s, call.Fun) {
		return argumentsStorage(s, call.Args, current)
	}
	if callee := model.ResolveCall(s.fn, call); callee != nil && callee.Body != nil {
		return mappedStorage(s, callEdge{call: call, callee: callee}, returnedStorage(s, callee), current)
	}
	return unresolvedStorage(s, call, current)
}

func libraryStorage(s *scope, call *ast.CallExpr, current state) (storage, bool) {
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && len(call.Args) > 0 {
		switch s.qualified(selector) {
		case "slices.Clone", "maps.Clone":
			fact := storageOf(s, call.Args[0], current)
			fact.outer = false
			return fact, true
		}
	}
	name := builtinName(call)
	switch {
	case name == "append" && len(call.Args) > 0:
		return appendStorage(s, call, current), true
	case builtinFunctions[name]:
		return storage{}, true
	}
	return storage{}, false
}

func appendStorage(s *scope, call *ast.CallExpr, current state) storage {
	fact := storageOf(s, call.Args[0], current)
	for _, arg := range call.Args[1:] {
		child := storageOf(s, arg, current)
		if call.Ellipsis.IsValid() {
			child = elementOf(child)
		}
		fact = joinStorage(fact, contained(child))
	}
	return fact
}

func argumentsStorage(s *scope, args []ast.Expr, current state) storage {
	fact := storage{}
	for _, arg := range args {
		fact = joinStorage(fact, storageOf(s, arg, current))
	}
	return fact
}

func conversion(s *scope, fun ast.Expr) bool {
	switch value := util.UnwrapTypeInstance(ast.Unparen(fun)).(type) {
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StructType:
		return true
	case *ast.StarExpr:
		return conversion(s, value.X)
	case *ast.Ident:
		if value.Obj != nil {
			return value.Obj.Kind == ast.Typ
		}
		return util.IsBasicType(value.Name) || value.Name == "any" || value.Name == "error" || s.fn.File.PackageTypeDefs[value.Name] != nil
	}
	return false
}

func unresolvedStorage(s *scope, call *ast.CallExpr, current state) storage {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ambientStorage
	}
	fact := argumentsStorage(s, call.Args, current)
	if s.qualified(selector) != "" {
		return fact
	}
	return joinStorage(fact, storageOf(s, selector.X, current))
}

func returnedStorage(s *scope, callee *model.Function) storage {
	summaries := s.returns
	if fact, done := summaries.done[callee]; done {
		return fact
	}
	if summaries.active[callee] {
		return ambientStorage
	}
	summaries.active[callee] = true
	fact := returnStorage(newScope(callee, true, summaries))
	delete(summaries.active, callee)
	summaries.done[callee] = fact
	return fact
}

func returnStorage(s *scope) storage {
	out := storage{}
	if s.fn.Body == nil {
		return ambientStorage
	}
	ast.Inspect(s.fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if ok {
			out = joinStorage(out, resultStorage(s, ret))
		}
		_, literal := n.(*ast.FuncLit)
		return !literal
	})
	return out
}

func resultStorage(s *scope, ret *ast.ReturnStmt) storage {
	current := s.ownership.stateAt(ret.Pos())
	out := storage{}
	for _, result := range ret.Results {
		out = joinStorage(out, storageOf(s, result, current))
	}
	if len(ret.Results) > 0 {
		return out
	}
	for _, result := range s.fn.Results {
		if result.Ident != nil && result.Ident.Obj != nil {
			out = joinStorage(out, current[result.Ident.Obj])
		}
	}
	return out
}

func mappedStorage(s *scope, edge callEdge, summary storage, current state) storage {
	if len(summary.origins) == 0 {
		return storage{outer: summary.outer, reachable: summary.reachable, ambient: summary.ambient}
	}
	out := storage{}
	if summary.ambient {
		out = storage{outer: summary.outer, reachable: summary.reachable, ambient: true}
	}
	for origin := range summary.origins {
		actual := originArgument(s, edge, origin, current)
		if actual.outer || actual.reachable || actual.ambient {
			out = joinStorage(out, storage{outer: summary.outer, reachable: summary.reachable, ambient: actual.ambient, origins: actual.origins})
		}
	}
	return out
}

func originArgument(s *scope, edge callEdge, origin *ast.Object, current state) storage {
	if recv, _ := receiverObject(edge.callee.Decl); recv != nil && recv == origin {
		if selector, ok := ast.Unparen(edge.call.Fun).(*ast.SelectorExpr); ok {
			return storageOf(s, selector.X, current)
		}
		return ambientStorage
	}
	index := paramIndex(edge.callee, origin)
	if index < 0 {
		return ambientStorage
	}
	args, variadic := parameterArguments(edge, index)
	if len(args) == 0 && !variadic {
		return ambientStorage
	}
	if variadic {
		return contained(argumentsStorage(s, args, current))
	}
	return storageOf(s, args[0], current)
}
