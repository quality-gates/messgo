package explicitness

import (
	"go/ast"

	"github.com/quality-gates/messgo/internal/model"
)

// Record body edges independently of entry contexts, so new entry graphs on a
// recursive edge are widened even during a later worklist reevaluation.
func (c *storageSummaryCache) recursiveCall(fn *model.Function) bool {
	if c.current == nil {
		return false
	}
	from := c.current.fn.Body
	if c.bodyCalls[from] == nil {
		c.bodyCalls[from] = map[*ast.BlockStmt]bool{}
	}
	c.bodyCalls[from][fn.Body] = true
	return c.bodyReaches(fn.Body, from, map[*ast.BlockStmt]bool{})
}

func (c *storageSummaryCache) bodyReaches(from, target *ast.BlockStmt, seen map[*ast.BlockStmt]bool) bool {
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for next := range c.bodyCalls[from] {
		if c.bodyReaches(next, target, seen) {
			return true
		}
	}
	return false
}

// A recursive entry can represent several concrete input locations at one
// allocation site. Replay to every possible location, preserving old contents
// because a strong update of one abstract node may touch only one original.
func weakReplayStorage(exit, incoming state, inverse storageAllocationMapping) {
	for _, originals := range inverse {
		if len(originals) < 2 {
			continue
		}
		for _, a := range originals {
			if cells, changed := exit.heap[a]; changed {
				exit.heap[a] = joinCells(incoming.heap[a], cells)
			}
		}
	}
}
