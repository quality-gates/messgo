package explicitness

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
)

// External parameter pointees can have inline cells without an allocation.
// Intern them before recursive entry abstraction too: otherwise assignments
// between recursive parameters build an unbounded value tree despite a finite
// allocation graph.
func (c *storageSummaryCache) graphStorageEntry(entry state) state {
	entry = compactState(entry)
	type pendingCells struct {
		allocation allocation
		cells      map[string]storage
	}
	var pending []pendingCells
	var intern func(storage) storage
	intern = func(fact storage) storage {
		if len(fact.cells) == 0 {
			return fact
		}
		if len(fact.references) > 0 {
			fact.cells = compactCells(fact.cells, entry)
			return fact
		}
		key := storageInlineKey(fact)
		site := c.inlineSites[key]
		if site == nil {
			site = ast.NewIdent(key)
			c.inlineSites[key] = site
		}
		a := allocation{site: site, context: "@recursive-value", summary: true}
		pending = append(pending, pendingCells{a, fact.cells})
		fact.references, fact.cells = map[reference]bool{{allocation: a}: true}, nil
		return fact
	}
	for object, fact := range entry.bindings {
		entry.bindings[object] = intern(fact)
	}
	// Process existing heap cells and then every newly interned cell map.
	for a, cells := range entry.heap {
		pending = append(pending, pendingCells{a, cells})
		entry.heap[a] = nil
	}
	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]
		cells := map[string]storage{}
		for key, fact := range next.cells {
			cells[key] = intern(fact)
		}
		entry.heap[next.allocation] = joinCells(entry.heap[next.allocation], cells)
	}
	return compactState(entry)
}

func storageInlineKey(fact storage) string {
	var key bytes.Buffer
	if fact.typ != nil {
		_ = format.Node(&key, token.NewFileSet(), fact.typ)
	}
	for _, object := range storageKeys(fact.origins) {
		fmt.Fprintf(&key, "/%p", object)
	}
	return key.String()
}
