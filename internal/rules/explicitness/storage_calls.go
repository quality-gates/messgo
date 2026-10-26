package explicitness

import (
	"go/ast"
	"maps"
	"strconv"

	"github.com/quality-gates/messgo/internal/model"
)

func callContext(s *scope, call *ast.CallExpr) string {
	return s.context + "/" + strconv.Itoa(int(call.Pos()))
}

func executeStorageCall(w *ownershipFlow, call *ast.CallExpr) bool {
	targets := storageTargets(w.s, call.Fun, w.current)
	if len(targets) == 0 {
		return false
	}
	pre := cloneState(w.current)
	var exits []state
	for _, callee := range targets {
		w.current = cloneState(pre)
		executeKnownStorageCall(w, call, callee)
		exits = append(exits, w.current)
	}
	w.current = joinStates(exits...)
	recordSnapshot(w, call.Pos())
	return true
}

func executeKnownStorageCall(w *ownershipFlow, call *ast.CallExpr, callee *model.Function) {
	if w.active[callee.Body] {
		recordCallResult(w, call, ambientStorage)
		return
	}
	child := untrackedScope(callee, w.s.strict, newStorageMutations())
	child.captures = cloneState(w.current)
	flow := &ownershipFlow{s: child, current: initialState(child), labels: map[string]state{}, active: maps.Clone(w.active), summaries: w.summaries}
	bindCallArguments(w, flow, call, callee)
	entry, dependencies := storageEntry(w.summaries, callee, flow.current)
	entry, inverse := abstractStorageEntry(entry, w.s, call)
	active := map[*ast.BlockStmt]bool{}
	for body := range w.active {
		if dependencies.bodies[body] {
			active[body] = true
		}
	}
	summary := w.summaries.summarize(flow, callee, entry, active)
	var mutations *storageMutations
	flow.current, flow.result, mutations = instantiateStorageSummary(summary, inverse, w.s, call)
	publishStorageMutations(w.s.mutations, mutations)
	// The summary contains only reachable entry storage. Preserve unrelated heap.
	heap := maps.Clone(w.current.heap)
	if heap == nil {
		heap = map[allocation]map[string]storage{}
	}
	maps.Copy(heap, flow.current.heap)
	flow.current.heap = heap
	publishCallState(w, flow, callee)
	recordCallResult(w, call, flow.result)
}

func computeStorageSummary(flow *ownershipFlow, callee *model.Function, entry state, active map[*ast.BlockStmt]bool) *storageSummary {
	// Each computation uses an allocation namespace distinct from its inputs.
	flow.s.context = storageSummaryContext
	flow.current = cloneState(entry)
	flow.active[callee.Body] = true
	recordSnapshot(flow, callee.Body.Pos())
	walkBlock(flow, callee.Body.List)
	exit := storageSummaryExit(flow, callee, entry)
	return &storageSummary{entry: entry, active: active, strict: flow.s.strict, exit: exit, result: flow.result, mutations: visibleStorageMutations(flow.s.mutations, exit), context: flow.s.context}
}

func recordCallResult(w *ownershipFlow, call *ast.CallExpr, fact storage) {
	w.current.calls = maps.Clone(w.current.calls)
	if w.current.calls == nil {
		w.current.calls = map[*ast.CallExpr]storage{}
	}
	w.current.calls[call] = fact
}

func joinCalls(a, b map[*ast.CallExpr]storage) map[*ast.CallExpr]storage {
	out := maps.Clone(a)
	if out == nil {
		out = map[*ast.CallExpr]storage{}
	}
	for call, fact := range b {
		out[call] = joinStorage(out[call], fact)
	}
	return out
}

func sameCalls(a, b map[*ast.CallExpr]storage) bool {
	if len(a) != len(b) {
		return false
	}
	for call, fact := range a {
		other, found := b[call]
		if !found || !sameStorage(fact, other) {
			return false
		}
	}
	return true
}

func bindCallArguments(parent, child *ownershipFlow, call *ast.CallExpr, callee *model.Function) {
	edge := callEdge{call: call, callee: callee}
	for _, param := range callee.Params {
		if param.Ident == nil || param.Ident.Obj == nil {
			continue
		}
		fact := originArgument(parent.s, edge, param.Ident.Obj, parent.current)
		child.current = applyBindingChanges(child.current, bindingChange{param.Ident.Obj, fact})
	}
	if child.s.recv != nil {
		fact := originArgument(parent.s, edge, child.s.recv, parent.current)
		child.current = applyBindingChanges(child.current, bindingChange{child.s.recv, fact})
	}
}

func publishCallState(parent, child *ownershipFlow, callee *model.Function) {
	var changes []bindingChange
	for object := range parent.current.bindings {
		if owns(child.s, object) || paramIndex(callee, object) >= 0 || object == child.s.recv {
			continue
		}
		if fact, found := child.current.bindings[object]; found {
			changes = append(changes, bindingChange{object, fact})
		}
	}
	parent.current = applyBindingChanges(parent.current, changes...)
	parent.current.heap = child.current.heap
}
