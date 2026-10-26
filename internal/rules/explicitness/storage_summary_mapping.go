package explicitness

import (
	"fmt"
	"go/ast"
	"maps"
	"slices"
	"strings"
)

const storageSummaryContext = "#summary/"

// Ordered traversal gives equivalent entries the same allocation numbering.
func storageKeys[K comparable, V any](m map[K]V) []K {
	keys := slices.Collect(maps.Keys(m))
	slices.SortFunc(keys, func(a, b K) int { return strings.Compare(fmt.Sprintf("%#v", a), fmt.Sprintf("%#v", b)) })
	return keys
}

func mapStorage(fact storage, rename func(allocation) []allocation) storage {
	fact.references = mapStorageReferences(fact.references, rename)
	cells := fact.cells
	fact.cells = nil
	if cells != nil {
		fact.cells = map[string]storage{}
	}
	for _, key := range storageKeys(cells) {
		fact.cells[key] = mapStorage(cells[key], rename)
	}
	return fact
}

func mapStorageReferences(refs map[reference]bool, rename func(allocation) []allocation) map[reference]bool {
	if refs == nil {
		return nil
	}
	out := map[reference]bool{}
	for _, ref := range storageKeys(refs) {
		for _, a := range rename(ref.allocation) {
			mapped := ref
			mapped.allocation = a
			if a.summary && mapped.path != "" {
				mapped.path = recursiveStoragePath
			}
			out[mapped] = true
		}
	}
	return out
}

func mapStorageState(current state, rename func(allocation) []allocation) state {
	out := state{bindings: map[*ast.Object]storage{}, heap: map[allocation]map[string]storage{}, addresses: map[*ast.Object]map[reference]bool{}, calls: map[*ast.CallExpr]storage{}}
	for _, object := range storageKeys(current.bindings) {
		out.bindings[object] = mapStorage(current.bindings[object], rename)
	}
	for _, object := range storageKeys(current.addresses) {
		out.addresses[object] = mapStorageReferences(current.addresses[object], rename)
	}
	for _, a := range storageKeys(current.heap) {
		cells := map[string]storage{}
		for _, key := range storageKeys(current.heap[a]) {
			cells[key] = mapStorage(current.heap[a][key], rename)
		}
		// Recursive allocation abstraction may merge contexts of one site.
		for _, mapped := range rename(a) {
			out.heap[mapped] = joinCells(out.heap[mapped], cells)
		}
	}
	for _, call := range storageKeys(current.calls) {
		out.calls[call] = mapStorage(current.calls[call], rename)
	}
	return out
}
