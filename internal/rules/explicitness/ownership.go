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
	references       map[reference]bool
	cells            map[string]storage
	targets          map[*ast.BlockStmt]*model.Function
}

var ambientStorage = storage{outer: true, reachable: true, ambient: true}

type allocation struct {
	site       ast.Node
	binding    *ast.Object
	invocation *ast.CallExpr
	context    string
	summary    bool // may represent multiple allocations in a recursive SCC
}

type reference struct {
	allocation
	path string
}

type state struct {
	bindings  map[*ast.Object]storage
	heap      map[allocation]map[string]storage
	calls     map[*ast.CallExpr]storage
	addresses map[*ast.Object]map[reference]bool
}

func cloneState(current state) state {
	return state{bindings: maps.Clone(current.bindings), heap: maps.Clone(current.heap), calls: maps.Clone(current.calls), addresses: maps.Clone(current.addresses)}
}

type ownershipSnapshot struct {
	position token.Pos
	values   state
}

type ownershipSnapshots []ownershipSnapshot

func initialState(s *scope) state {
	current := freshScopeState(s)
	if current.bindings == nil {
		current.bindings = map[*ast.Object]storage{}
	}
	if current.heap == nil {
		current.heap = map[allocation]map[string]storage{}
	}
	for object, typ := range s.params {
		current.bindings[object] = parameterStorage(s, object, typ)
	}
	for _, result := range s.fn.Results {
		if result.Ident != nil && result.Ident.Obj != nil {
			current.bindings[result.Ident.Obj] = storage{typ: result.Field.Type}
		}
	}
	if s.recv != nil {
		current.bindings[s.recv] = parameterStorage(s, s.recv, s.fn.Decl.Recv.List[0].Type)
	}
	return current
}

func parameterStorage(s *scope, object *ast.Object, typ ast.Expr) storage {
	fact := typeStorage(typ, s.fn.File.PackageTypeDefs)
	fact.origins = map[*ast.Object]bool{object: true}
	return fact
}

func inherit(s *scope, parent *scope, position token.Pos) {
	s.captures = cloneState(parent.ownership.stateAt(position))
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
	out := state{bindings: map[*ast.Object]storage{}}
	for _, values := range states {
		for object, fact := range values.bindings {
			if prior, ok := out.bindings[object]; ok {
				fact = joinStorage(prior, fact)
			}
			out.bindings[object] = fact
		}
		out.heap = joinHeaps(out.heap, values.heap)
		out.calls = joinCalls(out.calls, values.calls)
		out.addresses = joinAddresses(out.addresses, values.addresses)
	}
	for _, fact := range out.bindings {
		out = registerStorage(out, fact)
	}
	for _, fact := range out.calls {
		out = registerStorage(out, fact)
	}
	return out
}

func sameStates(a, b state) bool {
	if len(a.bindings) != len(b.bindings) || !sameHeaps(a.heap, b.heap) || !sameCalls(a.calls, b.calls) || !sameAddresses(a.addresses, b.addresses) {
		return false
	}
	for object, fact := range a.bindings {
		other, ok := b.bindings[object]
		if !ok || !sameStorage(fact, other) {
			return false
		}
	}
	return true
}

func sameStorage(a, b storage) bool {
	return a.outer == b.outer && a.reachable == b.reachable && a.ambient == b.ambient && maps.Equal(a.origins, b.origins) && maps.Equal(a.references, b.references) && sameCells(a.cells, b.cells) && sameTargets(a.targets, b.targets)
}

func joinStorage(a, b storage) storage {
	if a.typ == nil {
		a.typ = b.typ
	}
	a.outer = a.outer || b.outer
	a.reachable = a.reachable || b.reachable
	a.ambient = a.ambient || b.ambient
	a.references = joinedReferences(a.references, b.references)
	a.cells = joinCells(a.cells, b.cells)
	a.targets = joinedTargets(a.targets, b.targets)
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
	return resolveStorage(valueStorage(s, expression, current), current)
}

func valueStorage(s *scope, expression ast.Expr, current state) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return identStorage(s, value, current)
	case *ast.SliceExpr:
		return valueStorage(s, value.X, current)
	case *ast.StarExpr:
		return dereferenceStorage(valueStorage(s, value.X, current), current)
	case *ast.TypeAssertExpr:
		return valueStorage(s, value.X, current)
	case *ast.UnaryExpr:
		return unaryStorage(s, value, current)
	case *ast.CallExpr:
		return callStorage(s, value, current)
	case *ast.CompositeLit:
		return compositeStorage(s, value, current)
	case *ast.BasicLit, *ast.BinaryExpr:
		return storage{}
	}
	return selectedStorage(s, expression, current)
}

func identStorage(s *scope, id *ast.Ident, current state) storage {
	if s.isGlobal(id) {
		sharing, known := s.fn.File.PackageGlobalSharings[id.Name]
		typ := s.fn.File.PackageGlobalTypes[id.Name]
		if !known && s.fn.File.PackageGlobalSharings == nil {
			sharing, known = util.GlobalSharings([]*ast.File{s.fn.File.Syntax}, s.fn.File.PackageTypeDefs)[id.Name]
			typ = util.GlobalValueTypes([]*ast.File{s.fn.File.Syntax}, s.fn.File.PackageTypeDefs)[id.Name]
		}
		if known {
			return storage{typ: typ, outer: sharing.Outer, reachable: sharing.Reachable, ambient: true}
		}
		return ambientStorage
	}
	if id.Obj == nil || id.Obj.Kind != ast.Var {
		return functionStorage(s, id)
	}
	if fact, tracked := current.bindings[id.Obj]; tracked {
		return addressedBinding(current, id.Obj, fact)
	}
	return ambientStorage
}

func unaryStorage(s *scope, value *ast.UnaryExpr, current state) storage {
	fact := valueStorage(s, value.X, current)
	if value.Op == token.ARROW {
		return elementOf(fact)
	}
	if value.Op == token.AND {
		return addressStorage(s, value, current)
	}
	return fact
}

func selectedStorage(s *scope, expression ast.Expr, current state) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.FuncLit:
		return functionStorage(s, value)
	case *ast.SelectorExpr:
		if s.qualified(value) != "" {
			return ambientStorage
		}
		parent := valueStorage(s, value.X, current)
		return typedSelection(s, parent, "."+value.Sel.Name, current, false)
	case *ast.IndexExpr:
		parent := valueStorage(s, value.X, current)
		return typedSelection(s, parent, indexCell(value.Index), current, true)
	}
	return ambientStorage
}

func typedSelection(s *scope, parent storage, key string, current state, index bool) storage {
	selected := selectedCell(parent, key, current, index)
	if _, known := readCell(cellsOf(parent, current), key); known {
		return selected
	}
	typ := selectedType(s, parent.typ, key, index)
	if typ == nil {
		return selected
	}
	selected.typ = typ
	if !parent.outer {
		sharing := typeStorage(typ, s.fn.File.PackageTypeDefs)
		selected.outer, selected.reachable = sharing.outer, sharing.reachable
	}
	return selected
}

func selectedType(s *scope, typ ast.Expr, key string, index bool) ast.Expr {
	resolved := resolvedType(s, typ)
	if index {
		if array, ok := resolved.(*ast.ArrayType); ok && array.Len != nil {
			return array.Elt
		}
		return nil
	}
	structure, ok := resolved.(*ast.StructType)
	if !ok {
		return nil
	}
	for _, field := range structure.Fields.List {
		for _, name := range field.Names {
			if key == "."+name.Name {
				return field.Type
			}
		}
	}
	return nil
}

func compositeStorage(s *scope, value *ast.CompositeLit, current state) storage {
	fact := storage{typ: value.Type, cells: map[string]storage{}}
	for index, element := range value.Elts {
		key := compositeCell(s, value.Type, index)
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			key = literalCell(pair.Key)
			element = pair.Value
		}
		fact.cells[key] = valueStorage(s, element, current)
	}
	if typeStorage(value.Type, s.fn.File.PackageTypeDefs).outer {
		fact.references = map[reference]bool{newReference(s, value): true}
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

type storageMutations struct {
	mutatedObjects     map[*ast.Object]bool
	mutatedContents    map[*ast.Object]bool
	mutatedAllocations map[allocation]bool
}

func newStorageMutations() *storageMutations {
	return &storageMutations{mutatedObjects: map[*ast.Object]bool{}, mutatedContents: map[*ast.Object]bool{}, mutatedAllocations: map[allocation]bool{}}
}

var builtinFunctions = setOf("append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover")

func callStorage(s *scope, call *ast.CallExpr, current state) storage {
	if fact, known := libraryStorage(s, call, current); known {
		return fact
	}
	if conversion(s, call.Fun) {
		return argumentsStorage(s, call.Args, current)
	}
	if fact, executed := current.calls[call]; executed {
		return fact
	}
	return unresolvedStorage(s, call, current)
}

func libraryStorage(s *scope, call *ast.CallExpr, current state) (storage, bool) {
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok && len(call.Args) > 0 {
		switch s.qualified(selector) {
		case "slices.Clone", "maps.Clone":
			fact := frozenStorage(valueStorage(s, call.Args[0], current), current)
			fact.reachable = resolveStorage(fact, current).reachable
			fact.outer = false
			fact.references = map[reference]bool{newReference(s, call): true}
			return fact, true
		}
	}
	name := builtinName(call)
	switch {
	case name == "append" && len(call.Args) > 0:
		return appendStorage(s, call, current), true
	case builtinFunctions[name]:
		return builtinStorage(s, call), true
	}
	return storage{}, false
}

func builtinStorage(s *scope, call *ast.CallExpr) storage {
	name := builtinName(call)
	if len(call.Args) > 0 && (name == "make" || name == "new") {
		return storage{typ: call.Args[0], references: map[reference]bool{newReference(s, call): true}}
	}
	return storage{}
}

func appendStorage(s *scope, call *ast.CallExpr, current state) storage {
	fact := frozenStorage(valueStorage(s, call.Args[0], current), current)
	fact.references = map[reference]bool{newReference(s, call): true}
	for _, arg := range call.Args[1:] {
		child := valueStorage(s, arg, current)
		if call.Ellipsis.IsValid() {
			child = selectedCell(child, "*", current, true)
		}
		fact.cells = joinCells(fact.cells, map[string]storage{"*": child})
	}
	return fact
}

func argumentsStorage(s *scope, args []ast.Expr, current state) storage {
	fact := storage{}
	for _, arg := range args {
		fact = joinStorage(fact, valueStorage(s, arg, current))
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

func resultStorage(s *scope, ret *ast.ReturnStmt, current state) storage {
	out := storage{}
	for _, result := range ret.Results {
		out = joinStorage(out, frozenStorage(valueStorage(s, result, current), current))
	}
	if len(ret.Results) > 0 {
		return out
	}
	for _, result := range s.fn.Results {
		if result.Ident != nil && result.Ident.Obj != nil {
			out = joinStorage(out, frozenStorage(current.bindings[result.Ident.Obj], current))
		}
	}
	return out
}

func originArgument(s *scope, edge callEdge, origin *ast.Object, current state) storage {
	if recv, _ := receiverObject(edge.callee.Decl); recv != nil && recv == origin {
		if selector, ok := ast.Unparen(edge.call.Fun).(*ast.SelectorExpr); ok {
			return valueStorage(s, selector.X, current)
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
	return valueStorage(s, args[0], current)
}
