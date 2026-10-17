package explicitness

import "maps"

// compactState keeps heap references instead of copies of their cells.
// Copies can grow without bound when a loop builds a cyclic object graph.
func compactState(current state) state {
	current = cloneState(current)
	for object, fact := range current.bindings {
		current.bindings[object] = compactStorage(fact, current)
	}
	for call, fact := range current.calls {
		current.calls[call] = compactStorage(fact, current)
	}
	for allocation, cells := range current.heap {
		current.heap[allocation] = compactCells(cells, current)
	}
	return current
}

func compactCells(cells map[string]storage, current state) map[string]storage {
	out := maps.Clone(cells)
	for key, fact := range cells {
		out[key] = compactStorage(fact, current)
	}
	return out
}

func compactStorage(fact storage, current state) storage {
	if heapBacked(fact, current) {
		fact.cells = nil
	} else {
		fact.cells = compactCells(fact.cells, current)
	}
	return fact
}

func heapBacked(fact storage, current state) bool {
	if len(fact.references) == 0 {
		return false
	}
	for reference := range fact.references {
		if _, found := current.heap[reference.allocation]; !found {
			return false
		}
	}
	return true
}
