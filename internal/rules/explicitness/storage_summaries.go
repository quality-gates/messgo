package explicitness

import (
	"go/ast"
	"maps"
	"strconv"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
)

// storageSummaryCache belongs to one rule instance and retains only the current
// file's analysis. No ASTs or caller states escape into a process-global cache.
type storageSummaryCache struct {
	file         *model.File
	entries      map[*ast.BlockStmt][]*storageSummary
	dependencies map[*ast.BlockStmt]*storageDependencies
}

type storageSummary struct {
	entry     state
	active    map[*ast.BlockStmt]bool
	strict    bool
	exit      state
	result    storage
	mutations *storageMutations
	context   string
}

type storageDependencies struct {
	objects map[*ast.Object]bool
	bodies  map[*ast.BlockStmt]bool
}

func (c *storageSummaryCache) forFile(file *model.File) *storageSummaryCache {
	if c.file != file {
		c.file = file
		c.entries = map[*ast.BlockStmt][]*storageSummary{}
		c.dependencies = map[*ast.BlockStmt]*storageDependencies{}
	}
	return c
}

// Dependencies include transitive callees and nested literals, so a capture
// used only by a callback is still part of the entry facts.
func (c *storageSummaryCache) depends(fn *model.Function) *storageDependencies {
	if prior := c.dependencies[fn.Body]; prior != nil {
		return prior
	}
	d := &storageDependencies{objects: map[*ast.Object]bool{}, bodies: map[*ast.BlockStmt]bool{}}
	collectStorageDependencies(fn, d)
	c.dependencies[fn.Body] = d
	return d
}

func collectStorageDependencies(fn *model.Function, d *storageDependencies) {
	if fn == nil || fn.Body == nil || d.bodies[fn.Body] {
		return
	}
	d.bodies[fn.Body] = true
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			if n.Obj != nil {
				d.objects[n.Obj] = true
			}
		case *ast.CallExpr:
			collectStorageDependencies(model.ResolveCall(fn, n), d)
		}
		return true
	})
}

func storageEntry(cache *storageSummaryCache, fn *model.Function, current state) (state, *storageDependencies) {
	base := cache.depends(fn)
	d := &storageDependencies{objects: maps.Clone(base.objects), bodies: maps.Clone(base.bodies)}
	// Callable values can introduce captures outside the statically resolved graph.
	for _, fact := range current.bindings {
		addStorageTargets(fact, d)
	}
	for _, cells := range current.heap {
		for _, fact := range cells {
			addStorageTargets(fact, d)
		}
	}
	entry := state{bindings: map[*ast.Object]storage{}, heap: map[allocation]map[string]storage{}, addresses: map[*ast.Object]map[reference]bool{}}
	for object, fact := range current.bindings {
		if d.objects[object] {
			entry.bindings[object] = fact
			entry.addresses[object] = current.addresses[object]
			retainStorageHeap(fact, current, entry)
			for ref := range current.addresses[object] {
				retainAllocation(ref.allocation, current, entry)
			}
		}
	}
	return entry, d
}

func addStorageTargets(fact storage, d *storageDependencies) {
	for _, fn := range fact.targets {
		collectStorageDependencies(fn, d)
	}
	for _, cell := range fact.cells {
		addStorageTargets(cell, d)
	}
}

func retainStorageHeap(fact storage, current, entry state) {
	for ref := range fact.references {
		retainAllocation(ref.allocation, current, entry)
	}
	for _, cell := range fact.cells {
		retainStorageHeap(cell, current, entry)
	}
}

func retainAllocation(a allocation, current, entry state) {
	if _, seen := entry.heap[a]; seen {
		return
	}
	cells, found := current.heap[a]
	if !found {
		return
	}
	entry.heap[a] = cells
	// Addressed bindings are observable through closures as well as pointers.
	// Keep their mirror binding and address set so publishAddress can update it.
	if fact, found := current.bindings[a.binding]; found {
		entry.bindings[a.binding] = fact
		entry.addresses[a.binding] = current.addresses[a.binding]
		retainStorageHeap(fact, current, entry)
		for ref := range current.addresses[a.binding] {
			retainAllocation(ref.allocation, current, entry)
		}
	}
	for _, fact := range cells {
		retainStorageHeap(fact, current, entry)
	}
}

// Canonical entry allocations retain aliasing but discard caller-specific
// invocation paths. The inverse map instantiates the summary at each call site.
func abstractStorageEntry(entry state, s *scope, call *ast.CallExpr) (state, map[allocation]allocation) {
	context := callContext(s, call)
	names := map[allocation]allocation{}
	inverse := map[allocation]allocation{}
	rename := func(a allocation) allocation {
		if named, ok := names[a]; ok {
			return named
		}
		named := allocation{context: "@entry" + strconv.Itoa(len(names)), binding: a.binding}
		// A loop may already contain allocations from this very invocation.
		// Preserve that identity: a repeated allocation must update the existing
		// abstract location, not become a distinct fresh cell in the summary.
		if a.context == context || strings.HasPrefix(a.context, context+"/") {
			named = a
			named.context = storageSummaryContext + strings.TrimPrefix(a.context, context)
			if named.invocation == call {
				named.invocation = nil
			}
		}
		names[a], inverse[named] = named, a
		return named
	}
	// A stable traversal is important: maps must not decide alias numbering.
	return mapStorageState(entry, rename), inverse
}

func sameStorageEntry(a, b state) bool {
	return sameStates(a, b) && sameBindingTypes(a.bindings, b.bindings) && sameHeapTypes(a.heap, b.heap)
}

func sameBindingTypes(a, b map[*ast.Object]storage) bool {
	for object, fact := range a {
		if !sameStorageTypes(fact, b[object]) {
			return false
		}
	}
	return true
}

func sameHeapTypes(a, b map[allocation]map[string]storage) bool {
	for alloc, cells := range a {
		for key, fact := range cells {
			if !sameStorageTypes(fact, b[alloc][key]) {
				return false
			}
		}
	}
	return true
}

func sameStorageTypes(a, b storage) bool {
	if a.typ != b.typ {
		return false
	}
	for key, fact := range a.cells {
		if !sameStorageTypes(fact, b.cells[key]) {
			return false
		}
	}
	return true
}

func (c *storageSummaryCache) lookup(body *ast.BlockStmt, entry state, active map[*ast.BlockStmt]bool, strict bool) *storageSummary {
	for _, summary := range c.entries[body] {
		if summary.strict == strict && maps.Equal(summary.active, active) && sameStorageEntry(summary.entry, entry) {
			return summary
		}
	}
	return nil
}

func instantiateStorageSummary(summary *storageSummary, inverse map[allocation]allocation, s *scope, call *ast.CallExpr) (state, storage, *storageMutations) {
	rename := func(a allocation) allocation {
		if original, ok := inverse[a]; ok {
			return original
		}
		if strings.HasPrefix(a.context, summary.context) {
			a.context = callContext(s, call) + strings.TrimPrefix(a.context, summary.context)
			if a.invocation == nil {
				a.invocation = call
			}
		}
		return a
	}
	return mapStorageState(summary.exit, rename), mapStorage(summary.result, rename), mapStorageMutations(summary.mutations, rename)
}

func mapStorageMutations(m *storageMutations, rename func(allocation) allocation) *storageMutations {
	out := newStorageMutations()
	maps.Copy(out.mutatedObjects, m.mutatedObjects)
	maps.Copy(out.mutatedContents, m.mutatedContents)
	for a := range m.mutatedAllocations {
		out.mutatedAllocations[rename(a)] = true
	}
	return out
}

func publishStorageMutations(to, from *storageMutations) {
	maps.Copy(to.mutatedObjects, from.mutatedObjects)
	maps.Copy(to.mutatedContents, from.mutatedContents)
	maps.Copy(to.mutatedAllocations, from.mutatedAllocations)
}
