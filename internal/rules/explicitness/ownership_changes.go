package explicitness

import (
	"go/ast"
	"go/token"
	"maps"

	"github.com/quality-gates/messgo/internal/util"
)

type bindingChange struct {
	object *ast.Object
	fact   storage
}

func applyBindingChanges(current state, changes ...bindingChange) state {
	next := maps.Clone(current)
	for _, change := range changes {
		next[change.object] = change.fact
	}
	return next
}

// widenStorage returns the storage of object after code that may run at any time
// changes it. A closure can assign the variable, or store data in it.
func widenStorage(w *ownershipFlow, object *ast.Object, fact storage) storage {
	if w.escaped[object] {
		fact = exposed(fact)
	}
	if stored, ok := w.stored[object]; ok {
		fact = joinStorage(fact, stored)
	}
	return fact
}

func assignBindings(w *ownershipFlow, a *ast.AssignStmt) {
	facts := make([]storage, len(a.Lhs))
	for i := range a.Lhs {
		facts[i] = storageOf(w.s, a.Rhs[min(i, len(a.Rhs)-1)], w.current)
		if len(a.Rhs) != len(a.Lhs) {
			facts[i] = storageOf(w.s, a.Rhs[0], w.current)
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
		fact = storageOf(s, spec.Values[index], current)
	case len(spec.Values) == 1:
		fact = storageOf(s, spec.Values[0], current)
	}
	if spec.Type != nil {
		fact.typ = spec.Type
	}
	return fact
}

// bindTarget records an assignment of storage fact to target. A variable gets
// the storage. An element, a field or an indirection stores the data in the
// variable at its root.
func bindTarget(w *ownershipFlow, target ast.Expr, fact storage, tok token.Token) {
	id, named := ast.Unparen(target).(*ast.Ident)
	if !named {
		storeStorage(w, target, fact)
		return
	}
	if id.Obj == nil || id.Name == "_" || w.s.isGlobal(id) {
		return
	}
	prior, tracked := w.current[id.Obj]
	if tok != token.ASSIGN && tok != token.DEFINE && tracked {
		fact = joinStorage(prior, fact)
	}
	if fact.typ == nil {
		fact.typ = prior.typ
	}
	w.current = applyBindingChanges(w.current, bindingChange{id.Obj, widenStorage(w, id.Obj, fact)})
}

// storeStorage records that the data that target refers to now also refers to fact.
func storeStorage(w *ownershipFlow, target ast.Expr, fact storage) {
	root := util.RootIdent(target)
	if root == nil || root.Obj == nil {
		return
	}
	if prior, tracked := w.current[root.Obj]; tracked {
		w.current = applyBindingChanges(w.current, bindingChange{root.Obj, joinStorage(prior, contained(fact))})
	}
}

// collectEscapes records the expressions in node that let other code change a
// variable: an address of a variable, and a copy into a slice. An address
// that a call gets lets the callee store the other arguments in the variable.
// Any other address lets any code change the variable later.
func collectEscapes(w *ownershipFlow, node ast.Node) {
	if node == nil {
		return
	}
	arguments := map[ast.Expr]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returnedAddresses(n, arguments)
		case *ast.CallExpr:
			collectCallEscapes(w, n, arguments)
		case *ast.UnaryExpr:
			if n.Op == token.AND && !arguments[n] {
				escapeStorage(w, n.X)
			}
		}
		return true
	})
}

// returnedAddresses marks the addresses that ret returns. The caller gets the
// variable, and the function can no longer change it.
func returnedAddresses(ret *ast.ReturnStmt, arguments map[ast.Expr]bool) {
	for _, result := range ret.Results {
		if unary, ok := ast.Unparen(result).(*ast.UnaryExpr); ok && unary.Op == token.AND {
			arguments[unary] = true
		}
	}
}

// collectCallEscapes records what a call can store in variables. copy stores the
// elements of its source in its destination. A method of a variable, and a
// function that gets the address of a variable, can store the arguments in
// that variable.
func collectCallEscapes(w *ownershipFlow, call *ast.CallExpr, arguments map[ast.Expr]bool) {
	if builtinName(call) == "copy" && len(call.Args) == 2 {
		storeStorage(w, call.Args[0], elementOf(storageOf(w.s, call.Args[1], w.current)))
	}
	incoming := argumentsStorage(w.s, call.Args, w.current)
	if root := calledReceiver(w, call); root != nil {
		w.current = applyBindingChanges(w.current, bindingChange{root, addressed(w.current[root], incoming)})
	}
	for _, arg := range call.Args {
		unary, ok := ast.Unparen(arg).(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		arguments[unary] = true
		if root := trackedRoot(w, unary.X); root != nil {
			w.current = applyBindingChanges(w.current, bindingChange{root, addressed(w.current[root], incoming)})
		}
	}
}

func calledReceiver(w *ownershipFlow, call *ast.CallExpr) *ast.Object {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || w.s.qualified(selector) != "" {
		return nil
	}
	return trackedRoot(w, selector.X)
}

func escapeStorage(w *ownershipFlow, target ast.Expr) {
	if root := trackedRoot(w, target); root != nil {
		w.escaped[root] = true
		w.current = applyBindingChanges(w.current, bindingChange{root, exposed(w.current[root])})
	}
}

func trackedRoot(w *ownershipFlow, target ast.Expr) *ast.Object {
	root := util.RootIdent(target)
	if root == nil || root.Obj == nil {
		return nil
	}
	if _, tracked := w.current[root.Obj]; !tracked {
		return nil
	}
	return root.Obj
}

// addressed returns the storage of a variable after code that has its address
// stores incoming in it. A variable of a value type keeps its own storage, and
// only the data that it refers to can change.
func addressed(fact, incoming storage) storage {
	if valueType(fact.typ) {
		return joinStorage(fact, contained(incoming))
	}
	return joinStorage(fact, incoming)
}

func exposed(fact storage) storage {
	return addressed(fact, ambientStorage)
}

func valueType(typ ast.Expr) bool {
	if typ == nil {
		return false
	}
	return !typeStorage(typ, nil).outer
}

// closureTargets returns the variables of the function that a function
// literal in it can change. The literal can run at any later time. An
// assignment of a variable, or an address of it, can change it to any data. A
// store into an element or a field stores the assigned data.
func closureTargets(s *scope, current state) (map[*ast.Object]bool, map[*ast.Object]storage) {
	escaped := map[*ast.Object]bool{}
	stored := map[*ast.Object]storage{}
	ast.Inspect(s.fn.Body, func(n ast.Node) bool {
		literal, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(literal.Body, func(m ast.Node) bool {
			for _, change := range changes(m) {
				object := capturedTarget(s, current, literal, change.target)
				if object == nil {
					continue
				}
				if _, variable := ast.Unparen(change.target).(*ast.Ident); variable || change.value == nil {
					escaped[object] = true
					continue
				}
				stored[object] = joinStorage(stored[object], contained(storageOf(s, change.value, state{})))
			}
			return true
		})
		return false
	})
	return escaped, stored
}

func capturedTarget(s *scope, current state, literal *ast.FuncLit, target ast.Expr) *ast.Object {
	if target == nil {
		return nil
	}
	root := util.RootIdent(target)
	if root == nil || root.Obj == nil || !outside(literal, root.Obj) {
		return nil
	}
	if owns(s, root.Obj) || hasObject(current, root.Obj) {
		return root.Obj
	}
	return nil
}

func hasObject(current state, object *ast.Object) bool {
	_, ok := current[object]
	return ok
}

func outside(node ast.Node, object *ast.Object) bool {
	return object.Pos() < node.Pos() || object.Pos() >= node.End()
}

// change is a target that a statement changes, and the data that it stores,
// or nil if the data is unknown.
type change struct {
	target, value ast.Expr
}

func changes(node ast.Node) []change {
	var out []change
	switch n := node.(type) {
	case *ast.AssignStmt:
		if n.Tok == token.DEFINE {
			return nil
		}
		for i, target := range n.Lhs {
			out = append(out, change{target, assignedValue(n, i)})
		}
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN {
			out = append(out, change{n.Key, nil}, change{n.Value, nil})
		}
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			out = append(out, change{n.X, nil})
		}
	}
	return out
}

func assignedValue(a *ast.AssignStmt, index int) ast.Expr {
	if len(a.Lhs) != len(a.Rhs) {
		return nil
	}
	return a.Rhs[index]
}
