package model

import (
	"go/ast"
	"go/token"
)

// Receiver queries tell how one method uses its own receiver. They find the
// fields that the method selects and the sibling methods that it calls through
// the receiver variable. LackOfCohesionOfMethods (LCOM4) uses this graph for
// each method. Thus these queries match the receiver identifier by name and do
// not resolve types.
//
// File.MemberSelectedForType answers a different question for the unused-member
// rules. It finds each selection of a member on a value of the type, through
// any variable, promotion path, or composite literal in the file. Do not merge
// the two queries.

// AccessorField returns the field wrapped by a trivial getter or setter.
func AccessorField(f *Function, fields map[string]bool) string {
	if f.Body == nil || len(f.Body.List) != 1 {
		return ""
	}
	switch stmt := f.Body.List[0].(type) {
	case *ast.ReturnStmt:
		return returnAccessorField(stmt, f.RecvName, fields)
	case *ast.AssignStmt:
		return assignAccessorField(stmt, f.RecvName, fields)
	}
	return ""
}

func returnAccessorField(stmt *ast.ReturnStmt, recvName string, fields map[string]bool) string {
	if len(stmt.Results) != 1 {
		return ""
	}
	return receiverFieldSelector(stmt.Results[0], recvName, fields)
}

func assignAccessorField(stmt *ast.AssignStmt, recvName string, fields map[string]bool) string {
	if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return ""
	}
	if !isPlainValue(stmt.Rhs[0]) {
		return ""
	}
	return receiverFieldSelector(stmt.Lhs[0], recvName, fields)
}

// ReceiverUses returns fields and sibling methods selected through the
// receiver variable in this function body.
func ReceiverUses(f *Function, fields map[string]bool, methods map[string]int) (usedFields, calledMethods []string) {
	if f.Body == nil || f.RecvName == "" || f.RecvName == "_" {
		return nil, nil
	}
	seen := map[string]bool{}
	ast.Inspect(f.Body, func(n ast.Node) bool {
		name := receiverSelector(n, f.RecvName)
		if name == "" || seen[name] {
			return true
		}
		_, isMethod := methods[name]
		switch {
		case fields[name]:
			seen[name] = true
			usedFields = append(usedFields, name)
		case isMethod:
			seen[name] = true
			calledMethods = append(calledMethods, name)
		}
		return true
	})
	return usedFields, calledMethods
}

func isPlainValue(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.BasicLit:
		return true
	}
	return false
}

func receiverFieldSelector(e ast.Expr, recvName string, fields map[string]bool) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if ok && id.Name == recvName && fields[sel.Sel.Name] {
		return sel.Sel.Name
	}
	return ""
}

func receiverSelector(n ast.Node, recvName string) string {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != recvName {
		return ""
	}
	return sel.Sel.Name
}
