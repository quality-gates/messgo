package explicitness

import "maps"

const recursiveStoragePath = "$recursive"

// A recursive projection represents an arbitrary descendant, not an arbitrary
// finite path depth. Flatten its provenance, and weakly write every possible
// cell so widening cannot hide a later caller-visible write.
func recursiveStorageContents(fact storage, current state) storage {
	out := flattenedStorage(fact)
	for _, cells := range current.heap {
		for _, child := range cells {
			out = joinStorage(out, flattenedStorage(child))
		}
	}
	return out
}

func flattenedStorage(fact storage) storage {
	cells := fact.cells
	fact.cells, fact.references = nil, nil
	for _, child := range cells {
		fact = joinStorage(fact, flattenedStorage(child))
	}
	return fact
}

func markRecursiveStorageMutation(w *ownershipFlow, ref reference) {
	if ref.path == recursiveStoragePath {
		for a := range w.current.heap {
			w.s.mutations.mutatedAllocations[a] = true
		}
	}
}

func writeRecursiveStorage(current state, ref reference, incoming storage) state {
	current.heap = maps.Clone(current.heap)
	if _, found := current.heap[ref.allocation]; !found {
		current.heap[ref.allocation] = nil
	}
	for a, cells := range current.heap {
		current.heap[a] = updatedCells(cells, "*", incoming, false)
		mirror := reference{allocation: a}
		if value, found := current.heap[a][addressCell]; found {
			current = publishAddress(current, mirror, addressCell, value)
		}
	}
	return current
}
