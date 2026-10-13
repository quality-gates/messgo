package explicitness

import (
	"go/ast"
	"go/token"
	"maps"

	"github.com/quality-gates/messgo/internal/util"
)

func storeCell(w *ownershipFlow, target ast.Expr, incoming storage) {
	container, key := cellTarget(target)
	if container == nil {
		return
	}
	fact := valueStorage(w.s, container, w.current)
	if writeAddressField(w, fact, key, incoming) {
		return
	}
	if len(fact.references) > 0 {
		writeReferences(w, fact, key, incoming)
		return
	}
	storeValueCell(w, container, key, incoming)
}

func cellTarget(target ast.Expr) (ast.Expr, string) {
	switch value := ast.Unparen(target).(type) {
	case *ast.SelectorExpr:
		return value.X, "." + value.Sel.Name
	case *ast.IndexExpr:
		return value.X, indexCell(value.Index)
	case *ast.StarExpr:
		return value.X, addressCell
	}
	return nil, ""
}

func storeValueCell(w *ownershipFlow, container ast.Expr, key string, incoming storage) {
	id, direct := ast.Unparen(container).(*ast.Ident)
	if !direct || id.Obj == nil {
		return
	}
	fact, tracked := w.current.bindings[id.Obj]
	if !tracked {
		return
	}
	fact.cells = updatedCells(fact.cells, key, incoming, key != "*")
	w.current = applyBindingChanges(w.current, bindingChange{id.Obj, fact})
}

func writeReferences(w *ownershipFlow, fact storage, key string, incoming storage) {
	next := registerStorage(cloneState(w.current), incoming)
	next.heap = maps.Clone(next.heap)
	for reference := range fact.references {
		cells, found := next.heap[reference]
		if !found {
			cells = fact.cells
		}
		strong := len(fact.references) == 1 && key != "*"
		next.heap[reference] = updatedCells(cells, key, incoming, strong)
		next = publishAddress(next, reference, key, incoming)
	}
	w.current = next
}

func updatedCells(cells map[string]storage, key string, incoming storage, strong bool) map[string]storage {
	out := maps.Clone(cells)
	if out == nil {
		out = map[string]storage{}
	}
	if !strong {
		incoming = joinStorage(priorCell(out, key), incoming)
		if key == "*" {
			for cell, fact := range out {
				out[cell] = joinStorage(fact, incoming)
			}
		}
	}
	out[key] = incoming
	return out
}

func storeElement(w *ownershipFlow, container ast.Expr, incoming storage) {
	fact := valueStorage(w.s, container, w.current)
	if len(fact.references) > 0 {
		writeReferences(w, fact, "*", incoming)
	} else {
		storeValueCell(w, container, "*", incoming)
	}
}

func markStorageMutation(w *ownershipFlow, target ast.Expr) {
	id := util.RootIdent(target)
	if id != nil && id.Obj != nil && target == ast.Expr(id) {
		w.s.mutations.mutatedObjects[id.Obj] = true
	}
	container := writtenContainer(target, false)
	fact := valueStorage(w.s, container, w.current)
	if id != nil && id.Obj != nil && len(fact.references) == 0 {
		w.s.mutations.mutatedContents[id.Obj] = true
	}
	for reference := range fact.references {
		w.s.mutations.mutatedAllocations[reference] = true
		if reference.binding != nil {
			w.s.mutations.mutatedObjects[reference.binding] = true
		}
	}
}

func markNodeMutation(w *ownershipFlow, node ast.Node) {
	switch n := node.(type) {
	case *ast.AssignStmt:
		if n.Tok != token.DEFINE {
			for _, target := range n.Lhs {
				markStorageMutation(w, target)
			}
		}
	case *ast.IncDecStmt:
		markStorageMutation(w, n.X)
	}
}

func markCallMutation(w *ownershipFlow, call *ast.CallExpr) {
	if target := mutationArgument(w.s, call); target != nil {
		for reference := range valueStorage(w.s, target, w.current).references {
			w.s.mutations.mutatedAllocations[reference] = true
		}
	}
}

func mutationArgument(s *scope, call *ast.CallExpr) ast.Expr {
	if len(call.Args) == 0 {
		return nil
	}
	mutates := false
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		_, mutates = builtinWrites[builtinName(call)]
		mutates = mutates || resliceAppend(fun, call)
	case *ast.SelectorExpr:
		mutates = util.InPlaceChanges[s.qualified(fun)]
	}
	if mutates {
		return call.Args[0]
	}
	return nil
}

func capturedMutable(s *scope, id *ast.Ident, contents bool) bool {
	if s.mutations.mutatedObjects[id.Obj] {
		return true
	}
	if !contents {
		return false
	}
	if s.mutations.mutatedContents[id.Obj] {
		return true
	}
	current := s.ownership.stateAt(id.Pos())
	fact := current.bindings[id.Obj]
	resolved := resolveStorage(fact, current)
	if resolved.outer || resolved.reachable || resolved.ambient {
		return true
	}
	return mutatedStorage(s, fact, current, map[allocation]bool{})
}

func mutatedStorage(s *scope, fact storage, current state, seen map[allocation]bool) bool {
	for reference := range fact.references {
		if s.mutations.mutatedAllocations[reference] {
			return true
		}
		if seen[reference] {
			return false
		}
		seen[reference] = true
	}
	for _, child := range cellsOf(fact, current) {
		if mutatedStorage(s, child, current, maps.Clone(seen)) {
			return true
		}
	}
	return false
}
