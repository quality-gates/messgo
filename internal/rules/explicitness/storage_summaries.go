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
	current      *storageSummary
	pending      []*storageSummary
	queued       map[*storageSummary]bool
	bodyCalls    map[*ast.BlockStmt]map[*ast.BlockStmt]bool
	inlineSites  map[string]*ast.Ident
}

type storageSummary struct {
	entry                  state
	fn                     *model.Function
	computing, initialized bool
	callees, callers       map[*storageSummary]bool
	strict                 bool
	exit                   state
	result                 storage
	mutations              *storageMutations
	context                string
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
		c.queued = map[*storageSummary]bool{}
		c.bodyCalls = map[*ast.BlockStmt]map[*ast.BlockStmt]bool{}
		c.inlineSites = map[string]*ast.Ident{}
		c.current, c.pending = nil, nil
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
			collectStorageIdentifier(fn, n, d)
		case *ast.SelectorExpr:
			// Method values introduce the same recursive dependencies as calls.
			collectStorageDependencies(model.ResolveCall(fn, &ast.CallExpr{Fun: n}), d)
		case *ast.FuncLit:
			collectStorageDependencies(literalFunction(fn.File, n), d)
			return false
		}
		return true
	})
}

func storageEntry(cache *storageSummaryCache, fn *model.Function, current state) (state, *storageDependencies) {
	base := cache.depends(fn)
	d := &storageDependencies{objects: maps.Clone(base.objects), bodies: maps.Clone(base.bodies)}
	// Only this invocation's parameters/results are roots. Transitive callee
	// locals and parameters are freshly initialized, not inherited captures.
	addStorageFields(d, fn.Decl.Type.Params)
	addStorageFields(d, fn.Decl.Type.Results)
	addStorageFields(d, fn.Decl.Recv)
	// Discover dynamic callbacks only from storage the callee can reach. An
	// unrelated inherited function value must not pull in its entire graph.
	for {
		entry := projectStorageEntry(current, d)
		objects, bodies := len(d.objects), len(d.bodies)
		for _, fact := range entry.bindings {
			addStorageTargets(fact, d)
		}
		for _, cells := range entry.heap {
			for _, fact := range cells {
				addStorageTargets(fact, d)
			}
		}
		if objects == len(d.objects) && bodies == len(d.bodies) {
			return entry, d
		}
	}
}

func projectStorageEntry(current state, d *storageDependencies) state {
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
	return entry
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
type storageAllocationMapping map[allocation][]allocation

func abstractStorageEntry(entry state, s *scope, call *ast.CallExpr, recursive bool) (state, storageAllocationMapping) {
	context := callContext(s, call)
	names := map[allocation]allocation{}
	inverse := storageAllocationMapping{}
	rename := func(a allocation) []allocation {
		if named, ok := names[a]; ok {
			return []allocation{named}
		}
		named := allocation{context: "@entry" + strconv.Itoa(len(names)), site: a.site, invocation: a.invocation, binding: a.binding, summary: a.summary}
		// A loop may already contain allocations from this very invocation.
		// Preserve that identity: a repeated allocation must update the existing
		// abstract location, not become a distinct fresh cell in the summary.
		if recursive {
			named.context, named.summary = "@recursive-entry", true
		} else if a.context == context || strings.HasPrefix(a.context, context+"/") {
			named = a
			named.context = storageSummaryContext + strings.TrimPrefix(a.context, context)
			if named.invocation == call {
				named.invocation = nil
			}
		}
		names[a] = named
		inverse[named] = append(inverse[named], a)
		return []allocation{named}
	}
	// A stable traversal is important: maps must not decide alias numbering.
	return mapStorageState(compactState(entry), rename), inverse
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
	if !sameStorageType(a.typ, b.typ) {
		return false
	}
	for key, fact := range a.cells {
		if !sameStorageTypes(fact, b.cells[key]) {
			return false
		}
	}
	return true
}

// Address expressions synthesize fresh pointer type nodes on every walk.
// Compare their shape, not AST identity, while retaining named type identity.
// Other types keep the conservative AST-identity comparison.
func sameStorageType(a, b ast.Expr) bool {
	if a == b {
		return true
	}
	switch a := a.(type) {
	case *ast.StarExpr:
		other, ok := b.(*ast.StarExpr)
		return ok && sameStorageType(a.X, other.X)
	case *ast.Ident:
		other, ok := b.(*ast.Ident)
		return ok && a.Name == other.Name && a.Obj == other.Obj
	}
	return false
}

func (c *storageSummaryCache) lookup(body *ast.BlockStmt, entry state, strict bool) *storageSummary {
	for _, summary := range c.entries[body] {
		if summary.strict == strict && sameStorageEntry(summary.entry, entry) {
			return summary
		}
	}
	return nil
}

// Only caller-visible storage belongs in an exit summary. Exporting every
// private allocation would reproduce the entire call tree in the caller's heap,
// even if every callee body were walked just once.
func storageSummaryExit(flow *ownershipFlow, callee *model.Function, entry state) state {
	exit := state{bindings: map[*ast.Object]storage{}, heap: map[allocation]map[string]storage{}, addresses: map[*ast.Object]map[reference]bool{}}
	for object, fact := range flow.current.bindings {
		if !owns(flow.s, object) && paramIndex(callee, object) < 0 && object != flow.s.recv {
			exit.bindings[object] = fact
			retainStorageHeap(fact, flow.current, exit)
		}
	}
	// Entry locations can still have aliases in the caller, even if the callee
	// overwrote the parameter or disconnected a cell from its result.
	for a := range entry.heap {
		retainAllocation(a, flow.current, exit)
	}
	retainStorageHeap(flow.result, flow.current, exit)
	return exit
}

func instantiateStorageSummary(summary *storageSummary, inverse storageAllocationMapping, s *scope, call *ast.CallExpr, recursive bool, incoming state) (state, storage, *storageMutations) {
	rename := func(a allocation) []allocation {
		if originals, ok := inverse[a]; ok {
			return originals
		}
		if strings.HasPrefix(a.context, summary.context) {
			if recursive {
				// Recursive SCCs share allocation sites instead of growing an
				// unbounded invocation string on every fixed-point iteration.
				a.context, a.summary = callContext(s, call)+"/recursive", true
			} else {
				a.context = callContext(s, call) + strings.TrimPrefix(a.context, summary.context)
			}
			if a.invocation == nil {
				a.invocation = call
			}
		}
		return []allocation{a}
	}
	exit := mapStorageState(summary.exit, rename)
	weakReplayStorage(exit, incoming, inverse)
	return exit, mapStorage(summary.result, rename), mapStorageMutations(summary.mutations, rename)
}

func visibleStorageMutations(m *storageMutations, exit state) *storageMutations {
	out := &storageMutations{mutatedObjects: m.mutatedObjects, mutatedContents: m.mutatedContents, mutatedAllocations: map[allocation]bool{}}
	for a := range m.mutatedAllocations {
		if _, visible := exit.heap[a]; visible {
			out.mutatedAllocations[a] = true
		}
	}
	return out
}

func mapStorageMutations(m *storageMutations, rename func(allocation) []allocation) *storageMutations {
	out := newStorageMutations()
	maps.Copy(out.mutatedObjects, m.mutatedObjects)
	maps.Copy(out.mutatedContents, m.mutatedContents)
	for a := range m.mutatedAllocations {
		for _, mapped := range rename(a) {
			out.mutatedAllocations[mapped] = true
		}
	}
	return out
}

func publishStorageMutations(to, from *storageMutations) {
	maps.Copy(to.mutatedObjects, from.mutatedObjects)
	maps.Copy(to.mutatedContents, from.mutatedContents)
	maps.Copy(to.mutatedAllocations, from.mutatedAllocations)
}
