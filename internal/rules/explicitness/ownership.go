package explicitness

import (
	"go/ast"
	"go/token"
	"maps"
)

// storage separates an allocation from the objects reachable through it.
type storage struct {
	typ              ast.Expr
	outer, reachable bool
	origins          map[*ast.Object]bool
}

type ownershipSnapshot struct {
	position token.Pos
	values   map[*ast.Object]storage
}

func (s *scope) trackOwnership() {
	current := maps.Clone(s.captures)
	if current == nil {
		current = map[*ast.Object]storage{}
	}
	for object, typ := range s.params {
		fact := typeStorage(typ, map[ast.Expr]bool{}, s.fn.File.PackageTypeDefs)
		fact.origins = map[*ast.Object]bool{object: true}
		current[object] = fact
	}
	for _, result := range s.fn.Results {
		if result.Ident != nil {
			current[result.Ident.Obj] = storage{}
		}
	}
	s.ownership = []ownershipSnapshot{{s.fn.Body.Pos(), maps.Clone(current)}}
	var stack []ast.Node
	ast.Inspect(s.fn.Body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		conditional := conditionalParents(stack)
		stack = append(stack, node)
		current = updateStorage(s, node, current, conditional)
		if _, assignment := node.(*ast.AssignStmt); assignment {
			s.ownership = append(s.ownership, ownershipSnapshot{node.End(), maps.Clone(current)})
		}
		if _, declaration := node.(*ast.ValueSpec); declaration {
			s.ownership = append(s.ownership, ownershipSnapshot{node.End(), maps.Clone(current)})
		}
		return true
	})
}

func updateStorage(s *scope, node ast.Node, current map[*ast.Object]storage, conditional bool) map[*ast.Object]storage {
	switch node := node.(type) {
	case *ast.AssignStmt:
		return assignmentStorage(s, node, current, conditional)
	case *ast.ValueSpec:
		if len(node.Names) == len(node.Values) {
			for i, name := range node.Names {
				current[name.Obj] = storageOf(s, node.Values[i], current)
			}
		}
	}
	return current
}

func assignmentStorage(s *scope, assignment *ast.AssignStmt, current map[*ast.Object]storage, conditional bool) map[*ast.Object]storage {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		return current
	}
	next := maps.Clone(current)
	for i, target := range assignment.Lhs {
		id, named := target.(*ast.Ident)
		if !named || id.Obj == nil {
			continue
		}
		fact := storageOf(s, assignment.Rhs[i], current)
		if conditional {
			fact = joinStorage(current[id.Obj], fact)
		}
		next[id.Obj] = fact
	}
	return next
}

func (s *scope) inherit(parent *scope, position token.Pos) {
	s.captures = maps.Clone(parent.storageAt(position))
}

func conditionalParents(stack []ast.Node) bool {
	for _, parent := range stack {
		switch parent.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return true
		}
	}
	return false
}

func joinStorage(a, b storage) storage {
	if a.typ == nil {
		a.typ = b.typ
	}
	a.outer = a.outer || b.outer
	a.reachable = a.reachable || b.reachable
	origins := maps.Clone(a.origins)
	if origins == nil {
		origins = map[*ast.Object]bool{}
	}
	maps.Copy(origins, b.origins)
	a.origins = origins
	return a
}

func storageOf(s *scope, expression ast.Expr, current map[*ast.Object]storage) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.Ident:
		if s.isGlobal(value) {
			return storage{outer: true, reachable: true}
		}
		return current[value.Obj]
	case *ast.SliceExpr:
		return storageOf(s, value.X, current)
	case *ast.StarExpr:
		return storageOf(s, value.X, current)
	case *ast.UnaryExpr:
		return storageOf(s, value.X, current)
	case *ast.CallExpr:
		return callStorage(s, value, current)
	case *ast.CompositeLit:
		return compositeStorage(s, value, current)
	}
	return selectedStorage(s, expression, current)
}

func selectedStorage(s *scope, expression ast.Expr, current map[*ast.Object]storage) storage {
	switch value := ast.Unparen(expression).(type) {
	case *ast.SelectorExpr:
		fact := storageOf(s, value.X, current)
		fact.outer = fact.outer || fact.reachable
		return fact
	case *ast.IndexExpr:
		fact := storageOf(s, value.X, current)
		fact.outer = fact.reachable
		return fact
	}
	return storage{}
}

func compositeStorage(s *scope, value *ast.CompositeLit, current map[*ast.Object]storage) storage {
	fact := storage{typ: value.Type}
	for _, element := range value.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			element = pair.Value
		}
		child := storageOf(s, element, current)
		fact = joinStorage(fact, storage{reachable: child.outer || child.reachable, origins: child.origins})
	}
	return fact
}

func callStorage(s *scope, call *ast.CallExpr, current map[*ast.Object]storage) storage {
	if len(call.Args) == 0 {
		return storage{}
	}
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		name := s.qualified(selector)
		if name == "slices.Clone" || name == "maps.Clone" {
			fact := storageOf(s, call.Args[0], current)
			fact.outer = false
			return fact
		}
	}
	if builtinName(call) == "make" || builtinName(call) == "new" {
		return storage{}
	}
	if builtinName(call) == "append" {
		return appendStorage(s, call.Args, current)
	}
	// Unmodeled results may alias their arguments.
	fact := storage{}
	for _, arg := range call.Args {
		fact = joinStorage(fact, storageOf(s, arg, current))
	}
	return fact
}

func builtinName(call *ast.CallExpr) string {
	if name, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && name.Obj == nil {
		return name.Name
	}
	return ""
}

func appendStorage(s *scope, args []ast.Expr, current map[*ast.Object]storage) storage {
	fact := storageOf(s, args[0], current)
	for _, arg := range args[1:] {
		child := storageOf(s, arg, current)
		fact = joinStorage(fact, storage{reachable: child.outer || child.reachable, origins: child.origins})
	}
	return fact
}

func (s *scope) storageAt(position token.Pos) map[*ast.Object]storage {
	var values map[*ast.Object]storage
	for _, snapshot := range s.ownership {
		if snapshot.position <= position {
			values = snapshot.values
		}
	}
	return values
}

func typeStorage(typ ast.Expr, seen map[ast.Expr]bool, definitions map[string]ast.Expr) storage {
	if typ == nil {
		return storage{}
	}
	if seen[typ] {
		return storage{typ: typ, outer: true, reachable: true}
	}
	seen[typ] = true
	fact := storage{typ: typ}
	switch value := ast.Unparen(typ).(type) {
	case *ast.ArrayType:
		fact.outer = value.Len == nil
		fact.reachable = reachableType(value.Elt, seen, definitions)
	case *ast.MapType:
		fact.outer = true
		fact.reachable = reachableType(value.Value, seen, definitions)
	case *ast.StarExpr, *ast.Ellipsis, *ast.InterfaceType:
		fact.outer, fact.reachable = true, true
	case *ast.ChanType:
		fact.outer = true
	case *ast.StructType:
		fact.reachable = fieldsShare(value.Fields, seen, definitions)
	case *ast.Ident:
		return namedStorage(value, seen, definitions)
	}
	return fact
}

func reachableType(typ ast.Expr, seen map[ast.Expr]bool, definitions map[string]ast.Expr) bool {
	fact := typeStorage(typ, maps.Clone(seen), definitions)
	return fact.outer || fact.reachable
}

func fieldsShare(fields *ast.FieldList, seen map[ast.Expr]bool, definitions map[string]ast.Expr) bool {
	for _, field := range fields.List {
		if reachableType(field.Type, seen, definitions) {
			return true
		}
	}
	return false
}

func namedStorage(id *ast.Ident, seen map[ast.Expr]bool, definitions map[string]ast.Expr) storage {
	if id.Obj != nil {
		if spec, ok := id.Obj.Decl.(*ast.TypeSpec); ok {
			return typeStorage(spec.Type, seen, definitions)
		}
	}
	if definition := definitions[id.Name]; definition != nil {
		return typeStorage(definition, seen, definitions)
	}
	return storage{typ: id}
}
