package explicitness

import (
	"go/ast"
	"go/token"

	"github.com/quality-gates/messgo/internal/util"
)

type bindingChange struct {
	object *ast.Object
	fact   storage
}

func applyBindingChanges(current state, changes ...bindingChange) state {
	next := cloneState(current)
	if next.bindings == nil {
		next.bindings = map[*ast.Object]storage{}
	}
	for _, change := range changes {
		next.bindings[change.object] = change.fact
		next = registerStorage(next, change.fact)
		next = writeBindingCell(next, change.object, change.fact)
	}
	return next
}

func assignBindings(w *ownershipFlow, a *ast.AssignStmt) {
	facts := make([]storage, len(a.Lhs))
	for i := range a.Lhs {
		facts[i] = valueStorage(w.s, a.Rhs[min(i, len(a.Rhs)-1)], w.current)
		if len(a.Rhs) != len(a.Lhs) {
			facts[i] = valueStorage(w.s, a.Rhs[0], w.current)
		}
	}
	for i, target := range a.Lhs {
		bindTarget(w, target, facts[i], a.Tok)
	}
}

func declareBindings(w *ownershipFlow, d *ast.DeclStmt) {
	decl, ok := d.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		value := spec.(*ast.ValueSpec)
		for i, name := range value.Names {
			fact := specStorage(w.s, value, i, w.current)
			bindTarget(w, name, fact, token.DEFINE)
		}
	}
}

func specStorage(s *scope, spec *ast.ValueSpec, index int, current state) storage {
	fact := storage{}
	switch {
	case len(spec.Values) == len(spec.Names):
		fact = valueStorage(s, spec.Values[index], current)
	case len(spec.Values) == 1:
		fact = valueStorage(s, spec.Values[0], current)
	}
	if spec.Type != nil {
		fact.typ = spec.Type
	}
	return fact
}

func bindTarget(w *ownershipFlow, target ast.Expr, fact storage, tok token.Token) {
	id, named := ast.Unparen(target).(*ast.Ident)
	if !named {
		storeStorage(w, target, fact)
		return
	}
	if id.Obj == nil || id.Name == "_" || w.s.isGlobal(id) {
		return
	}
	prior, tracked := w.current.bindings[id.Obj]
	if tok != token.ASSIGN && tok != token.DEFINE && tracked {
		fact = joinStorage(prior, fact)
	}
	if fact.typ == nil {
		fact.typ = prior.typ
	}
	w.current = applyBindingChanges(w.current, bindingChange{id.Obj, fact})
}

func storeStorage(w *ownershipFlow, target ast.Expr, fact storage) {
	storeCell(w, target, fact)
}

func collectEscapes(w *ownershipFlow, node ast.Node) {
	if node == nil {
		return
	}
	arguments := map[ast.Expr]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		markNodeMutation(w, n)
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returnedAddresses(n, arguments)
		case *ast.UnaryExpr:
			collectEscapes(w, n.X)
			recordAddress(w, n)
			return false
		case *ast.CallExpr:
			collectEscapes(w, n.Fun)
			for _, arg := range n.Args {
				collectEscapes(w, arg)
			}
			collectCallEscapes(w, n, arguments)
			markCallMutation(w, n)
			return false
		}
		return true
	})
}

func returnedAddresses(ret *ast.ReturnStmt, arguments map[ast.Expr]bool) {
	for _, result := range ret.Results {
		if unary, ok := ast.Unparen(result).(*ast.UnaryExpr); ok && unary.Op == token.AND {
			arguments[unary] = true
		}
	}
}

func collectCallEscapes(w *ownershipFlow, call *ast.CallExpr, arguments map[ast.Expr]bool) {
	if builtinName(call) == "copy" && len(call.Args) == 2 {
		storeElement(w, call.Args[0], selectedCell(valueStorage(w.s, call.Args[1], w.current), "*", w.current, true))
	}
	if executeStorageCall(w, call) {
		return
	}
	before := w.current
	if root := calledReceiver(w, call); root != nil {
		w.current = escapeAddressed(w, before, root, call.Args)
	}
	for _, arg := range call.Args {
		unary, ok := ast.Unparen(arg).(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		arguments[unary] = true
		if root := trackedRoot(w, unary.X); root != nil {
			w.current = escapeAddressed(w, before, root, call.Args)
		}
	}
}

// escapeAddressed records that an unresolved callee may overwrite root.
// The address argument is a pointer to root, not a value stored into it;
// joining that pointer installs a $value cycle and selectedCell never returns.
func escapeAddressed(w *ownershipFlow, before state, root *ast.Object, args []ast.Expr) state {
	fact := w.current.bindings[root]
	incoming := argumentsExceptRoot(w.s, args, root, before)
	return applyBindingChanges(w.current, bindingChange{root, addressed(fact, incoming)})
}

func argumentsExceptRoot(s *scope, args []ast.Expr, root *ast.Object, current state) storage {
	fact := storage{}
	for _, arg := range args {
		if addressOfRoot(arg, root) {
			continue
		}
		fact = joinStorage(fact, valueStorage(s, arg, current))
	}
	return fact
}

func addressOfRoot(arg ast.Expr, root *ast.Object) bool {
	unary, ok := ast.Unparen(arg).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return false
	}
	id := util.RootIdent(unary.X)
	return id != nil && id.Obj == root
}

func calledReceiver(w *ownershipFlow, call *ast.CallExpr) *ast.Object {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || w.s.qualified(selector) != "" {
		return nil
	}
	return trackedRoot(w, selector.X)
}

func trackedRoot(w *ownershipFlow, target ast.Expr) *ast.Object {
	root := util.RootIdent(target)
	if root == nil || root.Obj == nil {
		return nil
	}
	if _, tracked := w.current.bindings[root.Obj]; !tracked {
		return nil
	}
	return root.Obj
}

func addressed(fact, incoming storage) storage {
	if valueType(fact.typ) {
		return joinStorage(fact, contained(incoming))
	}
	return joinStorage(fact, incoming)
}

func valueType(typ ast.Expr) bool {
	if typ == nil {
		return false
	}
	return !typeStorage(typ, nil).outer
}
