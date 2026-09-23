// Package explicitness implements rules that find the implicit inputs and the
// implicit outputs of a function. An explicit input is an argument. An explicit
// output is a return value. All other data that goes into or comes out of a
// function is implicit, and makes the function an action (see "Grokking
// Simplicity", chapter 3). These rules are not part of the default `go`
// ruleset.
package explicitness

import (
	"go/ast"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func init() {
	rule.Register("messgo\\Rule\\ImplicitInput", newImplicitInput)
	rule.Register("messgo\\Rule\\ImplicitOutput", newImplicitOutput)
}

// receiverProperty makes the rules also report the receiver of a method.
const receiverProperty = "include-receiver"

// ImplicitInput flags data that goes into a function from a source that is not
// an argument.
type ImplicitInput struct {
	*rule.Base
	includeReceiver bool
}

func newImplicitInput() rule.Rule { return &ImplicitInput{Base: rule.NewBase(receiverProperty)} }

func (r *ImplicitInput) Configure(props rule.Properties) error {
	r.includeReceiver = props.Bool(receiverProperty, false)
	return nil
}

func (r *ImplicitInput) ApplyFunc(c *rule.Context, fn *model.Function) {
	report(c, fn, scan(c, fn, r.includeReceiver).inputs)
}

// ImplicitOutput flags data that comes out of a function by a path that is not
// a return value.
type ImplicitOutput struct {
	*rule.Base
	includeReceiver bool
}

func newImplicitOutput() rule.Rule { return &ImplicitOutput{Base: rule.NewBase(receiverProperty)} }

func (r *ImplicitOutput) Configure(props rule.Properties) error {
	r.includeReceiver = props.Bool(receiverProperty, false)
	return nil
}

func (r *ImplicitOutput) ApplyFunc(c *rule.Context, fn *model.Function) {
	report(c, fn, scan(c, fn, r.includeReceiver).outputs)
}

// effect is one implicit input or output at a source line.
type effect struct {
	line int
	what string
}

// result holds the implicit inputs and outputs of one function.
type result struct {
	inputs  []effect
	outputs []effect
}

// report records one violation for each different effect, at its first line.
// Each description comes from one visitor only, and each visitor walks in
// source order, thus the first effect has the first line.
func report(c *rule.Context, fn *model.Function, effects []effect) {
	seen := map[string]bool{}
	for _, e := range effects {
		if seen[e.what] {
			continue
		}
		seen[e.what] = true
		c.ReportFuncAt(fn, e.line, e.line, string(fn.NodeType()), fn.Name, e.what)
	}
}

// scan finds the effects of the function body. Each visitor finds a different
// kind of effect. The write visitor runs first, because the read visitor must
// know which identifiers are only written.
func scan(c *rule.Context, fn *model.Function, strict bool) *result {
	res := &result{}
	if fn.Body == nil {
		return res
	}
	s := newScope(c, fn, strict)
	writes := &writeVisitor{scope: s, result: res, roots: map[*ast.Ident]bool{}}
	ast.Inspect(fn.Body, writes.visit)
	reads := &readVisitor{
		scope:      s,
		result:     res,
		writeRoots: writes.roots,
		fields:     map[*ast.Ident]string{},
		calls:      map[*ast.SelectorExpr]bool{},
	}
	ast.Inspect(fn.Body, reads.visit)
	ast.Inspect(fn.Body, (&flowVisitor{scope: s, result: res}).visit)
	return res
}

// scope tells which identifiers in a function body refer to data outside the
// function.
type scope struct {
	fn         *model.Function
	mutated    map[string]bool
	params     paramTypes
	recv       *ast.Object
	pointerRcv bool
	// strict is true when the receiver is not exempt. Then a read of the
	// receiver is an input, and a write through a pointer receiver is an
	// output.
	strict bool
}

func newScope(c *rule.Context, fn *model.Function, strict bool) *scope {
	s := &scope{
		fn:      fn,
		mutated: c.File.MutatedPackageGlobals(),
		params:  paramTypes{},
		strict:  strict,
	}
	for _, p := range fn.Params {
		if p.Ident != nil && p.Ident.Obj != nil {
			s.params[p.Ident.Obj] = p.Field.Type
		}
	}
	s.recv, s.pointerRcv = receiverObject(fn.Decl)
	return s
}

// receiverObject returns the object of the named receiver of decl, or nil, and
// whether the receiver is a pointer.
func receiverObject(decl *ast.FuncDecl) (*ast.Object, bool) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return nil, false
	}
	field := decl.Recv.List[0]
	_, pointer := field.Type.(*ast.StarExpr)
	return field.Names[0].Obj, pointer
}

// isGlobal reports whether id is a package variable. An identifier that the
// parser cannot resolve is declared in a different file of the package.
func (s *scope) isGlobal(id *ast.Ident) bool {
	if id.Obj == nil {
		return s.mutated[id.Name]
	}
	if _, ok := id.Obj.Decl.(*ast.ValueSpec); !ok || id.Obj.Kind != ast.Var {
		return false
	}
	pos := id.Obj.Pos()
	return pos < s.fn.Decl.Pos() || pos >= s.fn.Decl.End()
}

// paramTypes maps each named parameter to its type expression.
type paramTypes map[*ast.Object]ast.Expr

// isReference reports whether id is a parameter that refers to data of the
// caller: a pointer, a slice, a map or a variadic parameter.
func (p paramTypes) isReference(id *ast.Ident) bool {
	if id.Obj == nil {
		return false
	}
	switch t := p[id.Obj].(type) {
	case *ast.StarExpr, *ast.MapType, *ast.Ellipsis:
		return true
	case *ast.ArrayType:
		return t.Len == nil
	}
	return false
}

// canShare reports whether id is a parameter that can refer to data of the
// caller through an element or a field. Only a fixed-size array is a full
// copy.
func (p paramTypes) canShare(id *ast.Ident) bool {
	if id.Obj == nil {
		return false
	}
	t, ok := p[id.Obj]
	array, fixed := t.(*ast.ArrayType)
	return ok && !(fixed && array.Len != nil)
}

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

func (s *scope) effect(n ast.Node, what string) effect {
	return effect{line: s.fn.File.Fset.Position(n.Pos()).Line, what: what}
}

// receiverPart describes the receiver, or the receiver field if field is not
// "".
func receiverPart(id *ast.Ident, field string) string {
	if field == "" {
		return "receiver " + id.Name
	}
	return "receiver field " + field
}
