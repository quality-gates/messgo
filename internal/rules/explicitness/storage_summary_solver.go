package explicitness

import (
	"maps"

	"github.com/quality-gates/messgo/internal/model"
)

// Summaries are entered before walking their bodies. Back edges consume a
// provisional summary and register a dependency; a worklist then propagates
// changes to a fixed point, without enumerating active-ancestor subsets.
func (c *storageSummaryCache) summarize(fn *model.Function, entry state, strict bool) (*storageSummary, bool) {
	summary := c.lookup(fn.Body, entry, strict)
	if summary == nil {
		summary = &storageSummary{fn: fn, entry: entry, strict: strict, context: storageSummaryContext,
			callees: map[*storageSummary]bool{}, callers: map[*storageSummary]bool{}}
		c.entries[fn.Body] = append(c.entries[fn.Body], summary)
	}
	c.link(c.current, summary)
	recursive := storageSummaryReaches(summary, c.current, map[*storageSummary]bool{})
	if summary.computing {
		return provisionalStorageSummary(entry), true
	}
	if !summary.initialized {
		c.evaluate(summary)
	}
	if c.current == nil {
		c.solve()
	}
	return summary, recursive
}

func provisionalStorageSummary(entry state) *storageSummary {
	return &storageSummary{entry: entry, exit: entry, result: ambientStorage,
		mutations: newStorageMutations(), context: storageSummaryContext}
}

func (c *storageSummaryCache) link(caller, callee *storageSummary) {
	if caller != nil {
		caller.callees[callee] = true
		callee.callers[caller] = true
	}
}

func storageSummaryReaches(from, target *storageSummary, seen map[*storageSummary]bool) bool {
	if target == nil || seen[from] {
		return false
	}
	if from == target {
		return true
	}
	seen[from] = true
	for next := range from.callees {
		if storageSummaryReaches(next, target, seen) {
			return true
		}
	}
	return false
}

func (c *storageSummaryCache) evaluate(summary *storageSummary) {
	previous := c.current
	c.current, summary.computing = summary, true
	s := untrackedScope(summary.fn, summary.strict, newStorageMutations())
	flow := &ownershipFlow{s: s, summaries: c, labels: map[string]state{}}
	next := computeStorageSummary(flow, summary.fn, summary.entry)
	c.current, summary.computing = previous, false
	changed := !summary.initialized || !sameStorageSummary(summary, next)
	summary.exit, summary.result, summary.mutations = next.exit, next.result, next.mutations
	summary.initialized = true
	if changed {
		for caller := range summary.callers {
			if !caller.computing && !c.queued[caller] {
				c.pending = append(c.pending, caller)
				c.queued[caller] = true
			}
		}
	}
}

func (c *storageSummaryCache) solve() {
	for len(c.pending) > 0 {
		next := c.pending[0]
		c.pending = c.pending[1:]
		delete(c.queued, next)
		c.evaluate(next)
	}
}

func sameStorageSummary(a, b *storageSummary) bool {
	return sameStorageEntry(a.exit, b.exit) && sameStorage(a.result, b.result) &&
		sameStorageType(a.result.typ, b.result.typ) &&
		maps.Equal(a.mutations.mutatedObjects, b.mutations.mutatedObjects) &&
		maps.Equal(a.mutations.mutatedContents, b.mutations.mutatedContents) &&
		maps.Equal(a.mutations.mutatedAllocations, b.mutations.mutatedAllocations)
}
