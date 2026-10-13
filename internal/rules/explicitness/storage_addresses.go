package explicitness

import (
	"go/ast"
	"go/token"
	"maps"
)

const addressCell = "$value"

func freshScopeState(s *scope) state {
	current := cloneState(s.captures)
	for object := range current.addresses {
		if owns(s, object) {
			delete(current.addresses, object)
			delete(current.bindings, object)
		}
	}
	return current
}

func bindingAddress(s *scope, expression *ast.UnaryExpr, object *ast.Object, fact storage, current state) storage {
	references := current.addresses[object]
	if len(references) == 0 {
		site, _ := object.Decl.(ast.Node)
		references = map[allocation]bool{{site: site, binding: object, invocation: s.invocation, context: s.context}: true}
	}
	return storage{typ: &ast.StarExpr{Star: expression.OpPos, X: fact.typ}, references: references, cells: map[string]storage{addressCell: fact}}
}

func recordAddress(w *ownershipFlow, node ast.Node) {
	if value, address := node.(*ast.UnaryExpr); address && value.Op == token.AND {
		w.current = registerStorage(w.current, valueStorage(w.s, value, w.current))
	}
}

func registerAddress(current state, reference allocation) state {
	if reference.binding == nil {
		return current
	}
	current.addresses = maps.Clone(current.addresses)
	if current.addresses == nil {
		current.addresses = map[*ast.Object]map[allocation]bool{}
	}
	current.addresses[reference.binding] = joinedReferences(current.addresses[reference.binding], map[allocation]bool{reference: true})
	return current
}

func addressedBinding(current state, object *ast.Object, fallback storage) storage {
	references := current.addresses[object]
	if len(references) == 0 {
		return fallback
	}
	return dereferenceStorage(storage{references: references}, current)
}

func dereferenceStorage(fact storage, current state) storage {
	if value, addressed := cellsOf(fact, current)[addressCell]; addressed {
		return value
	}
	return fact
}

func writeBindingCell(current state, object *ast.Object, fact storage) state {
	for reference := range current.addresses[object] {
		current.heap = maps.Clone(current.heap)
		current.heap[reference] = updatedCells(current.heap[reference], addressCell, fact, true)
	}
	return current
}

func writeAddressField(w *ownershipFlow, fact storage, key string, incoming storage) bool {
	value, addressed := cellsOf(fact, w.current)[addressCell]
	if !addressed || key == addressCell {
		return false
	}
	value.cells = updatedCells(cellsOf(value, w.current), key, incoming, key != "*")
	writeReferences(w, fact, addressCell, value)
	return true
}

func publishAddress(current state, reference allocation, key string, fact storage) state {
	if key == addressCell && current.addresses[reference.binding][reference] {
		current.bindings = maps.Clone(current.bindings)
		current.bindings[reference.binding] = fact
	}
	return current
}

func joinAddresses(a, b map[*ast.Object]map[allocation]bool) map[*ast.Object]map[allocation]bool {
	out := maps.Clone(a)
	if out == nil {
		out = map[*ast.Object]map[allocation]bool{}
	}
	for object, references := range b {
		out[object] = joinedReferences(out[object], references)
	}
	return out
}

func sameAddresses(a, b map[*ast.Object]map[allocation]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for object, references := range a {
		if !maps.Equal(references, b[object]) {
			return false
		}
	}
	return true
}
