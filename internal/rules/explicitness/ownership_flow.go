package explicitness

import (
	"go/ast"
	"go/token"
	"maps"
)

func trackOwnership(s *scope, summaries *storageSummaryCache) {
	w := &ownershipFlow{s: s, current: initialState(s), labels: map[string]state{}, summaries: summaries}
	s.ownership = nil
	recordSnapshot(w, s.fn.Body.Pos())
	initial := cloneState(w.current)
	for {
		w.s.ownership = w.s.ownership[:1]
		w.current = cloneState(initial)
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

type ownershipFlow struct {
	s         *scope
	summaries *storageSummaryCache
	current   state
	result    storage
	labels    map[string]state
	jumps     []*[]state
}

func recordSnapshot(w *ownershipFlow, position token.Pos) {
	w.s.ownership = append(w.s.ownership, ownershipSnapshot{position, cloneState(w.current)})
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
	case *ast.ReturnStmt:
		w.result = joinStorage(w.result, resultStorage(w.s, n, w.current))
	}
	recordSnapshot(w, node.End())
}

func recordJump(w *ownershipFlow, branch *ast.BranchStmt) {
	if branch.Tok == token.GOTO && branch.Label != nil {
		w.labels[branch.Label.Name] = compactState(joinStates(w.labels[branch.Label.Name], w.current))
		return
	}
	for _, collector := range w.jumps {
		*collector = append(*collector, cloneState(w.current))
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
	if value, known := literalBool(n.Cond); known {
		if value {
			recordSnapshot(w, n.Body.Lbrace)
			walkBlock(w, n.Body.List)
		} else if n.Else != nil {
			walkStatement(w, n.Else)
		}
		recordSnapshot(w, n.End())
		return
	}
	pre := cloneState(w.current)
	recordSnapshot(w, n.Body.Lbrace)
	walkBlock(w, n.Body.List)
	then := w.current
	w.current = cloneState(pre)
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

func walkClauses(w *ownershipFlow, body *ast.BlockStmt) {
	pre := cloneState(w.current)
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
		return state{}
	}
	if branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt); ok && branch.Tok == token.FALLTHROUGH {
		return current
	}
	return state{}
}

func walkSelect(w *ownershipFlow, n *ast.SelectStmt) {
	pre := cloneState(w.current)
	exits := []state{pre}
	pushJumps(w)
	for _, statement := range n.Body.List {
		clause := statement.(*ast.CommClause)
		w.current = cloneState(pre)
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
	if value, known := literalBool(n.Cond); known && !value {
		collectEscapes(w, n.Cond)
		recordSnapshot(w, n.End())
		return
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
	markNodeMutation(w, n)
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

func walkLoop(w *ownershipFlow, start token.Pos, iterate func()) {
	entry := cloneState(w.current)
	mark := len(w.s.ownership)
	for {
		w.s.ownership = w.s.ownership[:mark]
		w.current = cloneState(entry)
		recordSnapshot(w, start)
		pushJumps(w)
		iterate()
		next := compactState(joinStates(append([]state{entry, w.current}, popJumps(w)...)...))
		if sameStates(next, entry) {
			break
		}
		entry = next
	}
	w.current = entry
}
