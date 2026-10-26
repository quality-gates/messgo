// Package explicitness implements rules that find the implicit inputs and the
// implicit outputs of a function. An explicit input is an argument. An explicit
// output is a return value. All other data that goes into or comes out of a
// function is implicit, and makes the function an action (see "Grokking
// Simplicity", chapter 3). These rules are not part of the default `go`
// ruleset.
package explicitness

import (
	"cmp"
	"go/ast"
	"slices"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

func init() {
	rule.Register("messgo\\Rule\\ImplicitInput", newImplicitInput)
	rule.Register("messgo\\Rule\\ImplicitOutput", newImplicitOutput)
	rule.Register("messgo\\Rule\\UnknownEffect", newUnknownEffect)
}

// receiverProperty makes the rules also report the receiver of a method.
const receiverProperty = "include-receiver"

// ImplicitInput flags data that goes into a function from a source that is not
// an argument.
type ImplicitInput struct {
	*rule.Base
	includeReceiver bool
	summaries       storageSummaryCache
}

func newImplicitInput() rule.Rule { return &ImplicitInput{Base: rule.NewBase(receiverProperty)} }

func (r *ImplicitInput) Configure(props rule.Properties) error {
	r.includeReceiver = props.Bool(receiverProperty, false)
	return nil
}

func (r *ImplicitInput) ApplyFunc(c *rule.Context, fn *model.Function) {
	reportFunctions(c, moduleOwner(fn), r.includeReceiver, true, r.summaries.forFile(c.File))
}

func (r *ImplicitInput) ApplyFile(c *rule.Context) { applyModule(c, r.ApplyFunc) }

// ImplicitOutput flags data that comes out of a function by a path that is not
// a return value.
type ImplicitOutput struct {
	*rule.Base
	includeReceiver bool
	summaries       storageSummaryCache
}

func newImplicitOutput() rule.Rule { return &ImplicitOutput{Base: rule.NewBase(receiverProperty)} }

func (r *ImplicitOutput) Configure(props rule.Properties) error {
	r.includeReceiver = props.Bool(receiverProperty, false)
	return nil
}

func (r *ImplicitOutput) ApplyFunc(c *rule.Context, fn *model.Function) {
	reportFunctions(c, moduleOwner(fn), r.includeReceiver, false, r.summaries.forFile(c.File))
}

func (r *ImplicitOutput) ApplyFile(c *rule.Context) { applyModule(c, r.ApplyFunc) }

// effect is one implicit input or output at a source line.
type effect struct {
	line          int
	what          string
	binding       *ast.Object
	deep          bool
	callable      bool
	callableArgs  []ast.Expr
	bindingAccess bool
	storageRead   bool
}

// result holds the implicit inputs and outputs of one function.
type result struct {
	inputs  []effect
	outputs []effect
	unknown []effect
}

func report(c *rule.Context, fn *model.Function, effects []effect) {
	ordered := slices.Clone(effects)
	slices.SortStableFunc(ordered, func(a, b effect) int { return cmp.Compare(a.line, b.line) })
	seen := map[string]bool{}
	for _, e := range ordered {
		if seen[e.what] {
			continue
		}
		seen[e.what] = true
		c.ReportFuncAt(fn, e.line, e.line, string(fn.NodeType()), fn.Name, e.what)
	}
}

func reportFunctions(c *rule.Context, fn *model.Function, strict, input bool, summaries *storageSummaryCache) {
	graph := analyze(c, fn, strict, summaries)
	for _, callable := range graph.owned(fn) {
		effects := graph.nodes[callable].facts.outputs
		if input {
			effects = graph.nodes[callable].facts.inputs
		}
		report(c, callable, reportableEffects(callable, effects, !strict))
	}
}

func inspectExecuted(body *ast.BlockStmt, visit func(ast.Node) bool) {
	inspectReachable(body, false, visit)
}

func inspectExecutedCallables(body *ast.BlockStmt, visit func(ast.Node) bool) {
	inspectReachable(body, true, visit)
}

func inspectReachable(node ast.Node, callables bool, visit func(ast.Node) bool) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case nil:
			return true
		case *ast.FuncLit:
			if callables {
				visit(n)
			}
			return false
		}
		if inspectControl(n, callables, visit) {
			return false
		}
		return visit(n)
	})
}

func inspectControl(node ast.Node, callables bool, visit func(ast.Node) bool) bool {
	switch n := node.(type) {
	case *ast.IfStmt:
		if visit(n) {
			inspectReachable(n.Init, callables, visit)
			inspectReachable(n.Cond, callables, visit)
			inspectChoice(n.Cond, n.Body, n.Else, callables, visit)
		}
	case *ast.ForStmt:
		if visit(n) {
			inspectReachable(n.Init, callables, visit)
			inspectReachable(n.Cond, callables, visit)
			if value, known := literalBool(n.Cond); !known || value {
				inspectReachable(n.Body, callables, visit)
				inspectReachable(n.Post, callables, visit)
			}
		}
	default:
		return false
	}
	return true
}

func inspectChoice(condition ast.Expr, positive, negative ast.Node, callables bool, visit func(ast.Node) bool) {
	value, known := literalBool(condition)
	if !known || value {
		inspectReachable(positive, callables, visit)
	}
	if !known || !value {
		inspectReachable(negative, callables, visit)
	}
}

func literalBool(expr ast.Expr) (bool, bool) {
	id, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok || id.Obj != nil {
		return false, false
	}
	switch id.Name {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// scan finds the effects of the function body. Each visitor finds a different
// kind of effect. The write visitor runs first, because the read visitor must
// know which identifiers are only written.
func scanScope(s *scope) *result {
	fn := s.fn
	res := &result{}
	if fn.Body == nil {
		return res
	}
	writes := &writeVisitor{scope: s, result: res, roots: map[*ast.Ident]bool{}}
	inspectExecuted(fn.Body, writes.visit)
	reads := &readVisitor{
		scope:      s,
		result:     res,
		writeRoots: writes.roots,
		fields:     map[*ast.Ident]string{},
		calls:      map[*ast.SelectorExpr]bool{},
		contents:   map[*ast.Ident]bool{},
	}
	inspectExecuted(fn.Body, reads.visit)
	inspectExecuted(fn.Body, (&flowVisitor{scope: s, result: res}).visit)
	return res
}

// scope tells which identifiers in a function body refer to data outside the
// function.
type scope struct {
	fn            *model.Function
	sharedGlobals map[string]bool
	params        paramTypes
	ownership     ownershipSnapshots
	captures      state
	invocation    *ast.CallExpr
	context       string
	recv          *ast.Object
	pointerRcv    bool
	mutations     *storageMutations
	// strict is true when the receiver is not exempt. Then a read of the
	// receiver is an input, and a write through a pointer receiver is an
	// output.
	strict bool
}

func untrackedScope(fn *model.Function, strict bool, mutations *storageMutations) *scope {
	s := &scope{
		fn:            fn,
		sharedGlobals: sharedPackageGlobals(fn.File),
		params:        paramTypes{},
		strict:        strict,
		mutations:     mutations,
	}
	for _, p := range fn.Params {
		if p.Ident != nil && p.Ident.Obj != nil {
			s.params[p.Ident.Obj] = p.Field.Type
		}
	}
	s.recv, s.pointerRcv = receiverObject(fn.Decl)
	return s
}

func sharedPackageGlobals(f *model.File) map[string]bool {
	if f.SharedGlobals != nil {
		return f.SharedGlobals
	}
	return util.SharedGlobalNames([]*ast.File{f.Syntax}, f.PackageTypeDefs)
}

// receiverObject returns the object of the named receiver of decl, or nil, and
// whether the receiver is a pointer.
func receiverObject(decl *ast.FuncDecl) (*ast.Object, bool) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return nil, false
	}
	field := decl.Recv.List[0]
	_, pointer := ast.Unparen(field.Type).(*ast.StarExpr)
	return field.Names[0].Obj, pointer
}

func (s *scope) isGlobal(id *ast.Ident) bool {
	if id.Obj == nil {
		return s.sharedGlobals[id.Name]
	}
	if _, ok := id.Obj.Decl.(*ast.ValueSpec); !ok || id.Obj.Kind != ast.Var {
		return false
	}
	pos := id.Obj.Pos()
	return (pos < s.fn.Decl.Pos() || pos >= s.fn.Decl.End()) && s.fn.File.Syntax.Scope.Lookup(id.Name) == id.Obj
}

// paramTypes maps each named parameter to its type expression.
type paramTypes map[*ast.Object]ast.Expr

// isChan reports whether id is a channel parameter.
func (p paramTypes) isChan(id *ast.Ident) bool {
	if id.Obj == nil {
		return false
	}
	_, ok := p[id.Obj].(*ast.ChanType)
	return ok
}

// isReceiver reports whether id is the receiver of the method.
func (s *scope) isReceiver(id *ast.Ident) bool {
	return id.Obj != nil && id.Obj == s.recv
}

// external reports whether id is a variable that the function body does not
// declare. The receiver is external only in strict mode. An imported package
// name is not a variable.
func (s *scope) external(id *ast.Ident) bool {
	if id.Obj == nil {
		return model.ImportedPackagePath(s.fn.File, id) == ""
	}
	pos := id.Obj.Pos()
	local := pos >= s.fn.Body.Pos() && pos < s.fn.Body.End()
	return !local && (s.strict || id.Obj != s.recv)
}

// qualified returns the import path and name of a package member, for example
// "net/http.Get", or "" if sel is not a package member.
func (s *scope) qualified(sel *ast.SelectorExpr) string {
	qualifier, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	pkg := model.ImportedPackagePath(s.fn.File, qualifier)
	if pkg == "" {
		return ""
	}
	return pkg + "." + sel.Sel.Name
}

// syntax returns the syntax tree of the file of the function, or nil.
func (s *scope) syntax() *ast.File {
	if s.fn.File == nil {
		return nil
	}
	return s.fn.File.Syntax
}

func (s *scope) effect(n ast.Node, what string) effect {
	return effect{line: model.PhysicalSpan(s.fn.File, n.Pos(), n.Pos()).Line, what: what}
}

// receiverPart describes the receiver, or the receiver field if field is not
// "".
func receiverPart(id *ast.Ident, field string) string {
	if field == "" {
		return "receiver " + id.Name
	}
	return "receiver field " + field
}

// excludedReceiverEffect is the non-strict stand-in for a receiver read or a
// caller-visible receiver write. The effect stays unknown so a clean report
// does not certify the method as a calculation.
const excludedReceiverEffect = "receiver effect"

// parameterWrite prefixes the description of a write through the storage a
// parameter refers to.
const parameterWrite = "write through parameter "

func noteExcludedReceiver(s *scope, res *result, at ast.Node) {
	if s.strict || at == nil {
		return
	}
	res.unknown = append(res.unknown, s.effect(at, excludedReceiverEffect))
}
