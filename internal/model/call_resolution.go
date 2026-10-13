package model

import "go/ast"

// ResolveCall resolves lexical free functions and concrete receiver methods.
// Interface dispatch and function-valued variables require effect analysis.
func ResolveCall(fn *Function, call *ast.CallExpr) *Function {
	collector := newMemberSelectionCollector(fn.File)
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		if fun.Obj != nil && fun.Obj.Kind != ast.Fun {
			return nil
		}
		return collector.types.functions[fun.Name]
	case *ast.SelectorExpr:
		types := map[string]memberVarType{}
		if id, ok := ast.Unparen(fun.X).(*ast.Ident); ok {
			types[id.Name] = lexicalReceiverType(id, &collector.types)
		}
		name := collector.types.expressionType(fun.X, types)
		method := lookupMethod(collector.types.classes, collector.types.interfaces, name, fun.Sel.Name, map[string]bool{})
		if method != nil && method.Body != nil {
			return method
		}
	}
	return nil
}

func lexicalReceiverType(id *ast.Ident, resolver *memberTypeResolver) memberVarType {
	if id.Obj == nil {
		return memberVarType{}
	}
	switch decl := id.Obj.Decl.(type) {
	case *ast.Field:
		return memberVarTypeOf(decl.Type)
	case *ast.ValueSpec:
		if decl.Type != nil {
			return memberVarTypeOf(decl.Type)
		}
		if len(decl.Values) == 1 {
			return resolver.resolvedType(decl.Values[0], nil)
		}
	case *ast.AssignStmt:
		if len(decl.Rhs) == 1 {
			return resolver.resolvedType(decl.Rhs[0], nil)
		}
	}
	return memberVarType{}
}
