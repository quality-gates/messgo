package explicitness

import (
	"go/ast"
	"go/token"

	"github.com/quality-gates/messgo/internal/util"
)

// writeVisitor finds writes to data outside the function: package variables,
// data that a reference parameter refers to, and in strict mode data that a
// pointer receiver refers to.
type writeVisitor struct {
	*scope
	*result
	// roots are identifiers that the function only writes, so they are not
	// reads.
	roots map[*ast.Ident]bool
}

func (w *writeVisitor) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.AssignStmt:
		w.visitAssign(n)
	case *ast.IncDecStmt:
		w.write(n.X, false, false)
	case *ast.RangeStmt:
		w.visitRange(n)
	case *ast.CallExpr:
		w.visitCall(n)
	}
	return true
}

// visitAssign records the writes of an assignment. A plain assignment does
// not read its targets. An operator assignment (x += y) reads them too.
func (w *writeVisitor) visitAssign(a *ast.AssignStmt) {
	if a.Tok == token.DEFINE {
		return
	}
	for _, lhs := range a.Lhs {
		w.write(lhs, a.Tok == token.ASSIGN, false)
	}
}

// visitRange records a range that assigns to existing variables.
func (w *writeVisitor) visitRange(r *ast.RangeStmt) {
	if r.Tok == token.ASSIGN {
		w.write(r.Key, true, false)
		w.write(r.Value, true, false)
	}
}

// visitCall records a builtin or a library call that changes the data that its
// first argument refers to.
func (w *writeVisitor) visitCall(call *ast.CallExpr) {
	if len(call.Args) == 0 {
		return
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if only, ok := builtinWrites[fun.Name]; ok && fun.Obj == nil {
			w.write(call.Args[0], only, true)
		}
		if resliceAppend(fun, call) {
			w.write(call.Args[0], false, true)
		}
	case *ast.SelectorExpr:
		if util.InPlaceChanges[w.qualified(fun)] {
			w.write(call.Args[0], false, true)
		}
	}
}

// resliceAppend reports whether call appends to a two-index slice of other
// data, as append(xs[:1], 2) does. The slice keeps the capacity of the data,
// so the append writes into the array of the data.
func resliceAppend(fun *ast.Ident, call *ast.CallExpr) bool {
	if fun.Name != "append" || fun.Obj != nil || len(call.Args) < 2 {
		return false
	}
	slice, ok := ast.Unparen(call.Args[0]).(*ast.SliceExpr)
	return ok && !slice.Slice3
}

// write records a write to the variable at the root of target. If only is
// true, the root is not also a read. If shared is true, the write changes the
// data that target refers to, as delete(m, k) does.
func (w *writeVisitor) write(target ast.Expr, only, shared bool) {
	id := util.RootIdent(target)
	if id == nil {
		w.writeCallResult(target, shared)
		return
	}
	if only {
		w.roots[id] = true
	}
	what := w.describe(target, id, shared)
	if what == "" {
		return
	}
	e := w.effect(id, what)
	e.deep = deepWrite(target)
	if captured(w.scope, id) {
		e.binding, e.bindingAccess = id.Obj, target == ast.Expr(id)
		w.outputs = append(w.outputs, e)
		return
	}
	w.outputs = append(w.outputs, originEffects(e, storageOf(w.scope, writtenContainer(target, shared), w.ownership.stateAt(target.Pos())))...)
}

func (w *writeVisitor) writeCallResult(target ast.Expr, shared bool) {
	call := callRoot(target)
	if call == nil || (target == ast.Expr(call) && !shared) {
		return
	}
	fact := storageOf(w.scope, writtenContainer(target, shared), w.ownership.stateAt(call.Pos()))
	deep := deepWrite(target)
	if !fact.outer && !(deep && fact.reachable) {
		return
	}
	e := w.effect(target, "write through call result")
	e.deep = deep
	w.outputs = append(w.outputs, originEffects(e, fact)...)
}

func callRoot(target ast.Expr) *ast.CallExpr {
	for {
		switch t := target.(type) {
		case *ast.CallExpr:
			return t
		case *ast.SelectorExpr:
			target = t.X
		case *ast.IndexExpr:
			target = t.X
		case *ast.SliceExpr:
			target = t.X
		case *ast.UnaryExpr:
			target = t.X
		case *ast.StarExpr:
			target = t.X
		case *ast.ParenExpr:
			target = t.X
		default:
			return nil
		}
	}
}

func (w *writeVisitor) describe(target ast.Expr, id *ast.Ident, builtin bool) string {
	direct := target == ast.Expr(id)
	shared := builtin || reachesShared(target)
	fact := w.ownership.stateAt(target.Pos()).bindings[id.Obj]
	switch {
	case w.isGlobal(id):
		return "package variable " + id.Name
	case w.strict && w.isReceiver(id) && w.receiverWrite(direct, shared):
		return receiverPart(id, receiverField(target, id))
	case captured(w.scope, id):
		return "captured variable " + id.Name
	case !sharedWrite(w.scope, target, writtenContainer(target, builtin), direct, shared, fact):
	case w.isReceiver(id):
		return "write through receiver " + id.Name
	default:
		return "write through parameter " + id.Name
	}
	return ""
}

func captured(s *scope, id *ast.Ident) bool {
	if id.Name == "_" || id.Obj == nil || id.Obj.Kind != ast.Var {
		return false
	}
	pos := id.Obj.Pos()
	owned := pos >= s.fn.Decl.Type.Pos() && pos < s.fn.Body.End()
	return !owned && s.external(id) && !s.isReceiver(id) && !s.isGlobal(id)
}

func sharedWrite(s *scope, target, container ast.Expr, direct, shared bool, fact storage) bool {
	return !direct && storageOf(s, container, s.ownership.stateAt(target.Pos())).outer || shared && storageOf(s, container, s.ownership.stateAt(target.Pos())).outer
}

func writtenContainer(target ast.Expr, builtin bool) ast.Expr {
	if builtin {
		return target
	}
	switch t := ast.Unparen(target).(type) {
	case *ast.IndexExpr:
		return t.X
	case *ast.StarExpr:
		return t.X
	case *ast.SelectorExpr:
		return t.X
	}
	return target
}

func deepWrite(target ast.Expr) bool {
	depth := 0
	ast.Inspect(target, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.IndexExpr, *ast.StarExpr, *ast.SelectorExpr:
			depth++
		}
		return true
	})
	return depth > 1
}

// receiverWrite reports whether a write to the receiver changes data of the
// caller. A write to a field of a value receiver changes only a copy.
func (w *writeVisitor) receiverWrite(direct, shared bool) bool {
	return shared || (w.pointerRcv && !direct)
}

// reachesShared reports whether target is an element of a map or a slice, or
// the value that a pointer refers to. A copy of the root of target still
// refers to that data. The syntax tree does not show if the operand of an
// index is an array, thus an array element is also shared.
func reachesShared(target ast.Expr) bool {
	for {
		switch t := target.(type) {
		case *ast.IndexExpr, *ast.StarExpr:
			return true
		case *ast.SelectorExpr:
			target = t.X
		case *ast.ParenExpr:
			target = t.X
		default:
			return false
		}
	}
}

// receiverField returns the name of the field of root that target selects, or
// "" if target does not select a field of root.
func receiverField(target ast.Expr, root *ast.Ident) string {
	field := ""
	ast.Inspect(target, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.X == ast.Expr(root) {
			field = sel.Sel.Name
		}
		return field == ""
	})
	return field
}

// readVisitor finds reads of package variables that the package changes, and
// in strict mode reads of the receiver. A package variable that no code
// changes is a constant, thus it is not an implicit input. A call to a method
// of the receiver is also a read of the receiver, because the method gets the
// receiver.
type readVisitor struct {
	*scope
	*result
	writeRoots map[*ast.Ident]bool
	// fields maps an identifier to the field that it selects.
	fields map[*ast.Ident]string
	// calls are selectors that a call calls. They select a method, not a
	// field.
	calls    map[*ast.SelectorExpr]bool
	contents map[*ast.Ident]bool
}

// visit walks in source order, thus a call or a selector is found before the
// identifier in it.
func (r *readVisitor) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.CallExpr:
		r.visitReadCall(n)
	case *ast.SelectorExpr:
		// The selected name is a field or a method, not a variable.
		if id, ok := n.X.(*ast.Ident); ok && !r.calls[n] {
			r.fields[id] = n.Sel.Name
			r.readContent(n.X)
		}
		ast.Inspect(n.X, r.visit)
		return false
	case *ast.IndexExpr:
		r.readContent(n.X)
	case *ast.StarExpr:
		r.readContent(n.X)
	case *ast.CompositeLit:
		r.visitComposite(n)
		return false
	case *ast.Ident:
		r.read(n)
	}
	return true
}

func (r *readVisitor) visitReadCall(call *ast.CallExpr) {
	if target := mutationArgument(r.scope, call); target != nil {
		r.readContent(target)
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		r.calls[selector] = true
	}
}

func (r *readVisitor) readContent(expression ast.Expr) {
	if id := util.RootIdent(expression); id != nil {
		r.contents[id] = true
	}
}

// visitComposite skips the field names of a struct literal, because they are
// not variables. The keys of a map literal are expressions, thus they can be
// reads.
func (r *readVisitor) visitComposite(lit *ast.CompositeLit) {
	var packageMapTypes map[string]bool
	if r.fn.File != nil {
		packageMapTypes = r.fn.File.PackageMapTypes
	}
	isMap := isMapType(lit.Type, packageMapTypes)
	if lit.Type != nil {
		ast.Inspect(lit.Type, r.visit)
	}
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok && !isMap && isIdent(kv.Key) {
			elt = kv.Value
		}
		ast.Inspect(elt, r.visit)
	}
}

// isMapType reports whether t is a map type or the name of one, looking
// through generic instantiations and chains of type names. The parser
// resolves same-file type objects; the package index covers unresolved names
// declared in other files.
func isMapType(t ast.Expr, packageMapTypes map[string]bool) bool {
	seen := map[*ast.TypeSpec]bool{}
	for {
		t = util.UnwrapTypeInstance(t)
		id, ok := t.(*ast.Ident)
		if !ok {
			_, ok = t.(*ast.MapType)
			return ok
		}
		if id.Obj == nil {
			return packageMapTypes[id.Name]
		}
		spec, ok := id.Obj.Decl.(*ast.TypeSpec)
		if !ok || seen[spec] {
			return false
		}
		seen[spec] = true
		t = spec.Type
	}
}

func isIdent(e ast.Expr) bool {
	_, ok := e.(*ast.Ident)
	return ok
}

func (r *readVisitor) read(id *ast.Ident) {
	switch {
	case r.writeRoots[id]:
	case r.isGlobal(id) && r.sharedGlobals[id.Name]:
		r.inputs = append(r.inputs, r.effect(id, "package variable "+id.Name))
	case r.strict && r.isReceiver(id):
		r.inputs = append(r.inputs, r.effect(id, receiverPart(id, r.fields[id])))
	case captured(r.scope, id):
		r.readCapture(id)
	}
}

func (r *readVisitor) readCapture(id *ast.Ident) {
	if !capturedMutable(r.scope, id, r.contents[id]) {
		return
	}
	e := r.effect(id, "captured variable "+id.Name)
	e.binding, e.bindingAccess = id.Obj, true
	if r.contents[id] && !r.mutations.mutatedObjects[id.Obj] {
		e.bindingAccess, e.storageRead, e.deep = false, true, true
	}
	r.inputs = append(r.inputs, e)
}

// flowVisitor finds data that goes through channels, streams, panics and the
// standard library functions that read or change the environment.
type flowVisitor struct {
	*scope
	*result
}

func (f *flowVisitor) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.SendStmt:
		f.flow(&f.outputs, n.Chan, "send on ")
	case *ast.UnaryExpr:
		if n.Op == token.ARROW {
			f.flow(&f.inputs, n.X, "receive from ")
		}
	case *ast.RangeStmt:
		f.visitRange(n)
	case *ast.CallExpr:
		f.visitCall(n)
	case *ast.SelectorExpr:
		f.visitSelector(n)
	}
	return true
}

// visitRange records a range over a channel parameter, which is a receive.
func (f *flowVisitor) visitRange(r *ast.RangeStmt) {
	if id, ok := ast.Unparen(r.X).(*ast.Ident); ok && f.params.isChan(id) {
		f.flow(&f.inputs, id, "receive from ")
	}
}

// visitCall records a panic, a recover, and a standard library call that
// writes to or reads from a stream. The panic builtin sends data out of the
// function, and the recover builtin gets it back.
func (f *flowVisitor) visitCall(call *ast.CallExpr) {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		f.visitBuiltin(fun, call)
	case *ast.SelectorExpr:
		name := f.qualified(fun)
		if i, ok := streamWriters[name]; ok && i < len(call.Args) {
			f.flow(&f.outputs, call.Args[i], "write to ")
		}
		if i, ok := streamReaders[name]; ok && i < len(call.Args) {
			f.flow(&f.inputs, call.Args[i], "read from ")
		}
	}
}

func (f *flowVisitor) visitBuiltin(fun *ast.Ident, call *ast.CallExpr) {
	switch {
	case fun.Obj != nil:
	case fun.Name == "panic":
		f.outputs = append(f.outputs, f.effect(call, "panic"))
	case fun.Name == "recover":
		f.inputs = append(f.inputs, f.effect(call, "recover"))
	case fun.Name == "print" || fun.Name == "println":
		f.outputs = append(f.outputs, f.effect(call, fun.Name))
	}
}

// visitSelector records a standard library function or variable that reads
// or changes the environment, for example os.Getenv or os.Stdout.
func (f *flowVisitor) visitSelector(sel *ast.SelectorExpr) {
	name := f.qualified(sel)
	if ambientInputs[name] {
		f.inputs = append(f.inputs, f.effect(sel, name))
	}
	if ambientOutputs[name] {
		f.outputs = append(f.outputs, f.effect(sel, name))
	}
}

func (f *flowVisitor) flow(list *[]effect, e ast.Expr, prefix string) {
	if call, ok := e.(*ast.CallExpr); ok {
		e = call.Fun
	}
	id := flowRoot(f.scope, e)
	switch {
	case id == nil:
	case f.strict && f.isReceiver(id):
		*list = append(*list, f.effect(e, receiverPart(id, "")))
	case id.Obj == nil || f.isGlobal(id):
		*list = append(*list, f.effect(e, prefix+id.Name))
	case captured(f.scope, id):
		fact := f.effect(e, prefix+id.Name)
		fact.binding, fact.deep = id.Obj, true
		*list = append(*list, fact)
	default:
		*list = append(*list, f.storedFlow(e, prefix+id.Name)...)
	}
}

func flowRoot(s *scope, expression ast.Expr) *ast.Ident {
	id := util.RootIdent(expression)
	if id == nil || id.Obj == nil && !s.external(id) {
		return nil
	}
	return id
}

func (f *flowVisitor) storedFlow(e ast.Expr, what string) []effect {
	storage := storageOf(f.scope, e, f.ownership.stateAt(e.Pos()))
	if !storage.outer && !storage.reachable {
		return nil
	}
	fact := f.effect(e, what)
	fact.deep = true
	return originEffects(fact, storage)
}
