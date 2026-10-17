package explicitness

import (
	"go/ast"
	"maps"
	"strconv"

	"github.com/quality-gates/messgo/internal/util"
)

func joinedReferences(a, b map[reference]bool) map[reference]bool {
	out := maps.Clone(a)
	if out == nil {
		out = map[reference]bool{}
	}
	maps.Copy(out, b)
	return out
}

func joinCells(a, b map[string]storage) map[string]storage {
	out := map[string]storage{}
	for key, fact := range a {
		out[key] = joinStorage(fact, priorCell(b, key))
	}
	for key, fact := range b {
		out[key] = joinStorage(fact, priorCell(a, key))
	}
	return out
}

func priorCell(cells map[string]storage, key string) storage {
	if fact, found := cells[key]; found {
		return fact
	}
	return cells["*"]
}

func sameCells(a, b map[string]storage) bool {
	if len(a) != len(b) {
		return false
	}
	for key, fact := range a {
		other, found := b[key]
		if !found || !sameStorage(fact, other) {
			return false
		}
	}
	return true
}

func joinHeaps(a, b map[allocation]map[string]storage) map[allocation]map[string]storage {
	out := maps.Clone(a)
	if out == nil {
		out = map[allocation]map[string]storage{}
	}
	for reference, cells := range b {
		out[reference] = joinCells(out[reference], cells)
	}
	return out
}

func sameHeaps(a, b map[allocation]map[string]storage) bool {
	if len(a) != len(b) {
		return false
	}
	for reference, cells := range a {
		other, found := b[reference]
		if !found || !sameCells(cells, other) {
			return false
		}
	}
	return true
}

func registerStorage(current state, fact storage) state {
	for reference := range fact.references {
		if _, found := current.heap[reference.allocation]; !found {
			current.heap = maps.Clone(current.heap)
			if current.heap == nil {
				current.heap = map[allocation]map[string]storage{}
			}
			current.heap[reference.allocation] = fact.cells
		}
		current = registerAddress(current, reference)
	}
	for _, child := range fact.cells {
		current = registerStorage(current, child)
	}
	return current
}

func cellsOf(fact storage, current state) map[string]storage {
	if len(fact.references) == 0 {
		return fact.cells
	}
	var out map[string]storage
	for reference := range fact.references {
		out = joinCells(out, referenceCells(fact, reference, current))
	}
	return out
}

func resolveStorage(fact storage, current state) storage {
	return resolveReferences(fact, current, map[reference]bool{})
}

func resolveReferences(fact storage, current state, seen map[reference]bool) storage {
	cells := cellsOf(fact, current)
	for reference := range fact.references {
		if seen[reference] {
			return fact
		}
		seen[reference] = true
	}
	for _, child := range cells {
		fact = joinStorage(fact, contained(resolveReferences(child, current, maps.Clone(seen))))
	}
	return fact
}

func frozenStorage(fact storage, current state) storage {
	return freezeReferences(fact, current, map[reference]bool{})
}

func freezeReferences(fact storage, current state, seen map[reference]bool) storage {
	for reference := range fact.references {
		if seen[reference] {
			return fact
		}
		seen[reference] = true
	}
	fact.cells = maps.Clone(cellsOf(fact, current))
	for key, child := range fact.cells {
		fact.cells[key] = freezeReferences(child, current, maps.Clone(seen))
	}
	return fact
}

func selectedCell(fact storage, key string, current state, index bool) storage {
	return selectedCellSeen(fact, key, current, index, map[allocation]bool{})
}

func selectedCellSeen(fact storage, key string, current state, index bool, seen map[allocation]bool) storage {
	cells := cellsOf(fact, current)
	if value, addressed := cells[addressCell]; addressed && key != addressCell && followAddress(value, seen) {
		return selectedCellSeen(value, key, current, index, seen)
	}
	if selected, found := readCell(cells, key); found {
		return selected
	}
	resolved := resolveStorage(fact, current)
	if index {
		return elementOf(resolved)
	}
	resolved.outer = resolved.outer || resolved.reachable
	resolved.references, resolved.cells = nil, nil
	return resolved
}

// followAddress reports whether $value is a further pointee. A storage whose
// allocation is already on this selection is a cycle, not another pointee.
func followAddress(fact storage, seen map[allocation]bool) bool {
	if len(fact.references) == 0 {
		return true
	}
	for ref := range fact.references {
		if seen[ref.allocation] {
			return false
		}
	}
	for ref := range fact.references {
		seen[ref.allocation] = true
	}
	return true
}

func readCell(cells map[string]storage, key string) (storage, bool) {
	if len(cells) > 0 && key == "*" {
		selected := storage{}
		for _, child := range cells {
			selected = joinStorage(selected, child)
		}
		return selected, true
	}
	if selected, found := cells[key]; found {
		return selected, true
	}
	wildcard, found := cells["*"]
	return wildcard, found
}

func indexCell(index ast.Expr) string {
	if literal, ok := ast.Unparen(index).(*ast.BasicLit); ok {
		return "[" + literal.Value + "]"
	}
	return "*"
}

func literalCell(key ast.Expr) string {
	if id, ok := key.(*ast.Ident); ok {
		return "." + id.Name
	}
	return indexCell(key)
}

func compositeCell(s *scope, typ ast.Expr, index int) string {
	if structure, ok := resolvedType(s, typ).(*ast.StructType); ok {
		var fields []string
		for _, field := range structure.Fields.List {
			for _, name := range field.Names {
				fields = append(fields, "."+name.Name)
			}
		}
		if index < len(fields) {
			return fields[index]
		}
	}
	return "[" + strconv.Itoa(index) + "]"
}

func resolvedType(s *scope, typ ast.Expr) ast.Expr {
	seen := map[ast.Expr]bool{}
	for typ != nil && !seen[typ] {
		seen[typ] = true
		id, named := util.UnwrapTypeInstance(typ).(*ast.Ident)
		if !named {
			return typ
		}
		if id.Obj == nil {
			typ = s.fn.File.PackageTypeDefs[id.Name]
			continue
		}
		spec, declared := id.Obj.Decl.(*ast.TypeSpec)
		if !declared {
			return typ
		}
		typ = spec.Type
	}
	return typ
}
