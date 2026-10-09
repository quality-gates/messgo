// Package controlflow decomposes Go control-flow statements into structured
// models so metrics and rules need not walk the raw AST shapes themselves.
//
// Go's AST stores an if / else if / else chain as a linked list in
// ast.IfStmt.Else: an else-if is a nested *ast.IfStmt and a final else is an
// *ast.BlockStmt. This package unrolls that list once. It lives below
// internal/model, rather than in it, because internal/metrics consumes it and
// internal/model already imports internal/metrics.
package controlflow

import (
	"go/ast"
	"go/token"
)

// IfClause is one guarded arm of an if chain: the head if or a chained else-if.
type IfClause struct {
	Init ast.Stmt // optional initializer, nil when absent
	Cond ast.Expr
	Body *ast.BlockStmt
	Pos  token.Pos // position of the if keyword
}

// IfChain is an if statement with all of its chained else-if clauses and the
// optional trailing else block.
type IfChain struct {
	Clauses []IfClause     // the head first, then each else-if in order
	Else    *ast.BlockStmt // trailing else block, nil when absent
	ElsePos token.Pos      // opening brace of Else, token.NoPos when absent
}

// NewIfChain unrolls the chain headed by head. Passing a chained else-if
// yields the tail of its chain starting at that clause.
func NewIfChain(head *ast.IfStmt) IfChain {
	var c IfChain
	for cur := head; cur != nil; {
		c.Clauses = append(c.Clauses, IfClause{Init: cur.Init, Cond: cur.Cond, Body: cur.Body, Pos: cur.If})
		switch e := cur.Else.(type) {
		case *ast.IfStmt:
			cur = e
		case *ast.BlockStmt:
			c.Else, c.ElsePos = e, e.Lbrace
			cur = nil
		default:
			cur = nil
		}
	}
	return c
}

// IfChains returns every if chain under root in source order, one per chain
// head. A chained else-if is part of its head's chain and never starts a chain
// of its own; an if nested inside any clause body or the trailing else does.
func IfChains(root ast.Node) []IfChain {
	if root == nil {
		return nil
	}
	var chains []IfChain
	var visit func(ast.Node) bool
	inspect := func(n ast.Node) { // n is nil only for an absent Init
		if n != nil {
			ast.Inspect(n, visit)
		}
	}
	visit = func(n ast.Node) bool {
		head, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		chain := NewIfChain(head)
		chains = append(chains, chain)
		for _, cl := range chain.Clauses {
			inspect(cl.Init)
			inspect(cl.Cond)
			inspect(cl.Body)
		}
		if chain.Else != nil {
			inspect(chain.Else)
		}
		return false
	}
	ast.Inspect(root, visit)
	return chains
}
