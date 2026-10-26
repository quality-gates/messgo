package explicitness

import (
	"go/ast"
	"go/token"
	"maps"
	"strings"
)

func newReference(s *scope, site ast.Node) reference {
	return reference{allocation: allocation{site: site, invocation: s.invocation, context: s.context}}
}

func addressStorage(s *scope, expression *ast.UnaryExpr, current state) storage {
	target := ast.Unparen(expression.X)
	if id, named := target.(*ast.Ident); named && id.Obj != nil && !s.isGlobal(id) {
		return bindingAddress(s, expression, id.Obj, valueStorage(s, id, current), current)
	}
	if pointer, indirect := target.(*ast.StarExpr); indirect {
		return valueStorage(s, pointer.X, current)
	}
	if container, key := cellTarget(target); container != nil {
		fact := valueStorage(s, container, current)
		base := fact
		if len(base.references) == 0 {
			base = addressStorage(s, &ast.UnaryExpr{Op: token.AND, X: container}, current)
		}
		return projectedAddress(base, fact, key, current)
	}
	return unprojectedAddress(s, expression, valueStorage(s, target, current))
}

func unprojectedAddress(s *scope, expression *ast.UnaryExpr, fact storage) storage {
	target := ast.Unparen(expression.X)
	if _, literal := target.(*ast.CompositeLit); literal {
		if len(fact.references) == 0 {
			fact.references = map[reference]bool{newReference(s, target): true}
		}
		return fact
	}
	return storage{typ: &ast.StarExpr{Star: expression.OpPos, X: fact.typ}, outer: fact.ambient, ambient: fact.ambient,
		references: map[reference]bool{newReference(s, target): true}, cells: map[string]storage{addressCell: fact}}
}

func projectedAddress(base, container storage, key string, current state) storage {
	references := map[reference]bool{}
	for ref := range base.references {
		if ref.path == "" {
			if _, addressed := allocationCells(base, ref, current)[addressCell]; addressed {
				ref.path = addressCell
			}
		}
		ref.path = appendCell(ref.path, key)
		references[ref] = true
	}
	return storage{outer: container.outer, ambient: container.ambient, origins: container.origins, references: references, cells: base.cells}
}

func appendCell(path, key string) string {
	if path == recursiveStoragePath {
		return path
	}
	if path == "" {
		return key
	}
	return path + "/" + key
}

func allocationCells(fact storage, ref reference, current state) map[string]storage {
	if cells, found := current.heap[ref.allocation]; found {
		return cells
	}
	return fact.cells
}

func referenceCells(fact storage, ref reference, current state) map[string]storage {
	if ref.path == recursiveStoragePath {
		return map[string]storage{addressCell: recursiveStorageContents(fact, current)}
	}
	cells := allocationCells(fact, ref, current)
	if ref.path == "" {
		return cells
	}
	value := storage{outer: fact.outer, ambient: fact.ambient, origins: fact.origins, cells: cells}
	for _, key := range strings.Split(ref.path, "/") {
		value = selectedCell(value, key, current, key == "*" || strings.HasPrefix(key, "["))
	}
	return map[string]storage{addressCell: value}
}

func writeReference(current state, fact storage, ref reference, key string, incoming storage, strong bool) state {
	if ref.path == recursiveStoragePath {
		return writeRecursiveStorage(current, ref, incoming)
	}
	cells := allocationCells(fact, ref, current)
	strong = strong && !ref.summary
	if ref.path == "" {
		current.heap = maps.Clone(current.heap)
		current.heap[ref.allocation] = updatedCells(cells, key, incoming, strong)
		return publishAddress(current, ref, key, current.heap[ref.allocation][key])
	}
	path := ref.path
	if key != addressCell {
		path = appendCell(path, key)
	}
	current, root := writeStoragePath(current, storage{cells: cells}, strings.Split(path, "/"), incoming, strong)
	current.heap = maps.Clone(current.heap)
	current.heap[ref.allocation] = root.cells
	ref.path = ""
	return publishAddress(current, ref, addressCell, root.cells[addressCell])
}

func writeStoragePath(current state, fact storage, path []string, incoming storage, strong bool) (state, storage) {
	if len(path) == 0 {
		if !strong {
			incoming = joinStorage(fact, incoming)
		}
		return current, incoming
	}
	key := path[0]
	strong = strong && key != "*"
	child := selectedCell(fact, key, current, key == "*" || strings.HasPrefix(key, "["))
	current, child = writeStoragePath(current, child, path[1:], incoming, strong)
	if len(fact.references) == 0 {
		fact.cells = updatedCells(cellsOf(fact, current), key, child, strong)
		return current, fact
	}
	for ref := range fact.references {
		current = writeReference(current, fact, ref, key, child, strong && len(fact.references) == 1)
	}
	return current, fact
}
