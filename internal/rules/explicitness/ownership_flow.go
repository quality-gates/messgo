package explicitness

import (
	"go/ast"
	"go/token"
	"maps"
)

// trackOwnership walks the function body in execution order and records the
// storage of each variable after each statement. Branches join their states.
// A loop repeats its body until the states at its start stop changing.
func trackOwnership(s *scope) {
	w := &ownershipFlow{s: s, current: initialState(s), labels: map[string]state{}}
	w.escaped, w.stored = closureTargets(s, w.current)
	var changes []bindingChange
	for object, fact := range w.current {
		changes = append(changes, bindingChange{object, widenStorage(w, object, fact)})
	}
	w.current = applyBindingChanges(w.current, changes...)
	s.ownership = nil
	recordSnapshot(w, s.fn.Body.Pos())
	initial := maps.Clone(w.current)
	for {
		w.s.ownership = w.s.ownership[:1]
		w.current = maps.Clone(initial)
		labels := maps.Clone(w.labels)
		walkBlock(w, s.fn.Body.List)
		if sameLabels(labels, w.labels) {
			return
		}
	}
}

func sameLabels(a, b map[string]state) bool {
	if len(a) != len(b) {
		return false
	}
	for label, values := range a {
		other, ok := b[label]
		if !ok || !sameStates(values, other) {
			return false
		}
	}
	return true
}

// ownershipFlow holds the state of one function traversal.
type ownershipFlow struct {
	s       *scope
	current state
	// escaped holds variables that a pointer or a closure can change at any
	// time.
	escaped map[*ast.Object]bool
	// stored holds the data that a closure can store in a variable.
	stored map[*ast.Object]storage
	// labels holds the joined states at the goto statements of each label.
	labels map[string]state
	// jumps collects the states at branch statements of each enclosing
	// statement that a branch can leave.
	jumps []*[]state
}

func recordSnapshot(w *ownershipFlow, position token.Pos) {
	w.s.ownership = append(w.s.ownership, ownershipSnapshot{position, maps.Clone(w.current)})
}

func walkBlock(w *ownershipFlow, list []ast.Stmt) {
	for _, statement := range list {
		walkStatement(w, statement)
	}
}

func walkStatement(w *ownershipFlow, node ast.Stmt) {
	switch n := node.(type) {
	case *ast.BlockStmt:
		walkBlock(w, n.List)
	case *ast.LabeledStmt:
		if values, ok := w.labels[n.Label.Name]; ok {
			w.current = joinStates(w.current, values)
		}
		recordSnapshot(w, n.Colon)
		walkStatement(w, n.Stmt)
	case *ast.BranchStmt:
		recordJump(w, n)
	default:
		if !walkCompound(w, node) {
			applyStatement(w, node)
		}
	}
}

func walkCompound(w *ownershipFlow, node ast.Stmt) bool {
	switch n := node.(type) {
	case *ast.IfStmt:
		walkIf(w, n)
	case *ast.SwitchStmt:
		walkSwitch(w, n)
	case *ast.TypeSwitchStmt:
		walkTypeSwitch(w, n)
	case *ast.SelectStmt:
		walkSelect(w, n)
	case *ast.ForStmt:
		walkFor(w, n)
	case *ast.RangeStmt:
		walkRange(w, n)
	default:
		return false
	}
	return true
}

func applyStatement(w *ownershipFlow, node ast.Stmt) {
	if node == nil {
		return
	}
	collectEscapes(w, node)
	switch n := node.(type) {
	case *ast.AssignStmt:
		assignBindings(w, n)
	case *ast.DeclStmt:
		declareBindings(w, n)
	case *ast.SendStmt:
		storeStorage(w, n.Chan, storageOf(w.s, n.Value, w.current))
	}
	recordSnapshot(w, node.End())
}

func recordJump(w *ownershipFlow, branch *ast.BranchStmt) {
	if branch.Tok == token.GOTO && branch.Label != nil {
		w.labels[branch.Label.Name] = joinStates(w.labels[branch.Label.Name], w.current)
		return
	}
	for _, collector := range w.jumps {
		*collector = append(*collector, maps.Clone(w.current))
	}
}

func pushJumps(w *ownershipFlow) {
	w.jumps = append(w.jumps, &[]state{})
}

func popJumps(w *ownershipFlow) []state {
	last := w.jumps[len(w.jumps)-1]
	w.jumps = w.jumps[:len(w.jumps)-1]
	return *last
}

func walkIf(w *ownershipFlow, n *ast.IfStmt) {
	if n.Init != nil {
		walkStatement(w, n.Init)
	}
	collectEscapes(w, n.Cond)
	pre := maps.Clone(w.current)
	recordSnapshot(w, n.Body.Lbrace)
	walkBlock(w, n.Body.List)
	then := w.current
	w.current = maps.Clone(pre)
	if n.Else != nil {
		recordSnapshot(w, n.Else.Pos())
		walkStatement(w, n.Else)
	}
	w.current = joinStates(then, w.current)
	recordSnapshot(w, n.End())
}

func walkSwitch(w *ownershipFlow, n *ast.SwitchStmt) {
	if n.Init != nil {
		walkStatement(w, n.Init)
	}
	collectEscapes(w, n.Tag)
	walkClauses(w, n.Body)
	recordSnapshot(w, n.End())
}

func walkTypeSwitch(w *ownershipFlow, n *ast.TypeSwitchStmt) {
	if n.Init != nil {
		walkStatement(w, n.Init)
	}
	collectEscapes(w, n.Assign)
	if assign, ok := n.Assign.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
		fact := storageOf(w.s, assign.Rhs[0], w.current)
		for _, object := range switchObjects(n.Body, assign) {
			w.current = applyBindingChanges(w.current, bindingChange{object, fact})
		}
	}
	recordSnapshot(w, n.Assign.End())
	walkClauses(w, n.Body)
	recordSnapshot(w, n.End())
}

// switchObjects returns the variables that a type switch declares. The parser
// declares one variable for each clause.
func switchObjects(body *ast.BlockStmt, assign *ast.AssignStmt) []*ast.Object {
	var out []*ast.Object
	seen := map[*ast.Object]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && id.Obj != nil && id.Obj.Decl == assign && !seen[id.Obj] {
			seen[id.Obj] = true
			out = append(out, id.Obj)
		}
		return true
	})
	return out
}

// walkClauses walks the clauses of a switch. Each clause starts from the state
// before the switch, or also from the previous clause if that falls through.
// Without a default clause, the switch can also run no clause.
func walkClauses(w *ownershipFlow, body *ast.BlockStmt) {
	pre := maps.Clone(w.current)
	exits := []state{}
	complete := false
	var previous state
	pushJumps(w)
	for _, statement := range body.List {
		clause := statement.(*ast.CaseClause)
		w.current = joinStates(pre, previous)
		for _, expression := range clause.List {
			collectEscapes(w, expression)
		}
		recordSnapshot(w, clause.Colon)
		walkBlock(w, clause.Body)
		exits = append(exits, w.current)
		previous = fallthroughState(clause, w.current)
		complete = complete || clause.List == nil
	}
	exits = append(exits, popJumps(w)...)
	if !complete {
		exits = append(exits, pre)
	}
	w.current = joinStates(exits...)
}

func fallthroughState(clause *ast.CaseClause, current state) state {
	if len(clause.Body) == 0 {
		return nil
	}
	if branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
		return current
	}
	return nil
}

func walkSelect(w *ownershipFlow, n *ast.SelectStmt) {
	pre := maps.Clone(w.current)
	exits := []state{pre}
	pushJumps(w)
	for _, statement := range n.Body.List {
		clause := statement.(*ast.CommClause)
		w.current = maps.Clone(pre)
		applyStatement(w, clause.Comm)
		recordSnapshot(w, clause.Colon)
		walkBlock(w, clause.Body)
		exits = append(exits, w.current)
	}
	exits = append(exits, popJumps(w)...)
	w.current = joinStates(exits...)
	recordSnapshot(w, n.End())
}

func walkFor(w *ownershipFlow, n *ast.ForStmt) {
	start := n.Pos()
	if n.Init != nil {
		walkStatement(w, n.Init)
		start = n.Init.End()
	}
	walkLoop(w, start, func() {
		collectEscapes(w, n.Cond)
		recordSnapshot(w, n.Body.Lbrace)
		walkBlock(w, n.Body.List)
		if n.Post != nil {
			walkStatement(w, n.Post)
		}
	})
	recordSnapshot(w, n.End())
}

func walkRange(w *ownershipFlow, n *ast.RangeStmt) {
	collectEscapes(w, n.X)
	element := rangeStorage(w, n.X)
	walkLoop(w, n.Pos(), func() {
		for _, target := range []ast.Expr{n.Key, n.Value} {
			if target != nil {
				bindTarget(w, target, element, n.Tok)
			}
		}
		recordSnapshot(w, n.Body.Lbrace)
		walkBlock(w, n.Body.List)
	})
	recordSnapshot(w, n.End())
}

// rangeStorage returns the storage of an element of a range. A range over a
// function gets its elements from code that the walker does not see.
func rangeStorage(w *ownershipFlow, x ast.Expr) storage {
	if iterator(w.s, x) {
		return ambientStorage
	}
	return elementOf(storageOf(w.s, x, w.current))
}

func iterator(s *scope, x ast.Expr) bool {
	switch value := ast.Unparen(x).(type) {
	case *ast.FuncLit:
		return true
	case *ast.Ident:
		return value.Obj != nil && value.Obj.Kind == ast.Fun || rangesOverFunction(s, value)
	}
	return false
}

// walkLoop walks a loop body until the state at its start is a fixed point. The
// state at the start joins the state before the loop, the state at the end of
// the body, and the states at its branch statements. Only the snapshots of
// the last pass remain.
func walkLoop(w *ownershipFlow, start token.Pos, iterate func()) {
	entry := maps.Clone(w.current)
	mark := len(w.s.ownership)
	for {
		w.s.ownership = w.s.ownership[:mark]
		w.current = maps.Clone(entry)
		recordSnapshot(w, start)
		pushJumps(w)
		iterate()
		next := joinStates(append([]state{entry, w.current}, popJumps(w)...)...)
		if sameStates(next, entry) {
			break
		}
		entry = next
	}
	w.current = entry
}
