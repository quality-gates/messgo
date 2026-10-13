package explicitness

import (
	"go/ast"
	"maps"

	"github.com/quality-gates/messgo/internal/model"
)

func functionStorage(s *scope, expression ast.Expr) storage {
	if literal, defined := ast.Unparen(expression).(*ast.FuncLit); defined {
		callee := literalFunction(s.fn.File, literal)
		return storage{targets: map[*ast.BlockStmt]*model.Function{callee.Body: callee}}
	}
	if callee := model.ResolveCall(s.fn, &ast.CallExpr{Fun: expression}); callee != nil && callee.Body != nil {
		return storage{targets: map[*ast.BlockStmt]*model.Function{callee.Body: callee}}
	}
	return storage{}
}

func storageTargets(s *scope, expression ast.Expr, current state) map[*ast.BlockStmt]*model.Function {
	if fact := valueStorage(s, expression, current); len(fact.targets) > 0 {
		return fact.targets
	}
	return functionStorage(s, expression).targets
}

func joinedTargets(a, b map[*ast.BlockStmt]*model.Function) map[*ast.BlockStmt]*model.Function {
	out := maps.Clone(a)
	if out == nil {
		out = map[*ast.BlockStmt]*model.Function{}
	}
	maps.Copy(out, b)
	return out
}

func sameTargets(a, b map[*ast.BlockStmt]*model.Function) bool {
	if len(a) != len(b) {
		return false
	}
	for body := range a {
		if _, found := b[body]; !found {
			return false
		}
	}
	return true
}
