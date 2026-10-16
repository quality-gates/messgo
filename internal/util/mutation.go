package util

import (
	"go/ast"
	"go/token"
)

// MutationKind is the syntactic form of an operation that changes a variable
// or the data that it refers to.
type MutationKind int

const (
	// MutationAssign replaces the target without reading it: a plain
	// assignment (x = v) or a range clause that assigns to existing variables.
	MutationAssign MutationKind = iota
	// MutationUpdate reads the target and then replaces it: an operator
	// assignment (x += v), an increment or a decrement.
	MutationUpdate
	// MutationAddress takes the address of the target (&x), so code that gets
	// the address can change it.
	MutationAddress
	// MutationSend sends on the channel that the target is.
	MutationSend
	// MutationCall is a builtin or a library call that changes the data that
	// its first argument refers to.
	MutationCall
)

// Mutation is one target that an operation changes.
type Mutation struct {
	Target ast.Expr
	Kind   MutationKind
	// Reads is true if the operation also reads the root of Target, as
	// x += 1 and copy(dst, src) do.
	Reads bool
	// Shared is true if the operation changes the data that Target refers to
	// rather than Target itself, as delete(m, k) and ch <- v do.
	Shared bool
}

// Mutations returns the targets that node n changes. f is the file of n; it
// resolves the import names of library calls. A short variable declaration
// (:=) introduces variables and does not change any, and a write to the blank
// identifier changes nothing.
func Mutations(f *ast.File, n ast.Node) []Mutation {
	var out mutations
	switch s := n.(type) {
	case *ast.AssignStmt:
		out.assign(s)
	case *ast.IncDecStmt:
		out.add(s.X, MutationUpdate, true)
	case *ast.RangeStmt:
		out.rangeAssign(s)
	case *ast.UnaryExpr:
		if s.Op == token.AND {
			out.add(s.X, MutationAddress, false)
		}
	case *ast.SendStmt:
		out.add(s.Chan, MutationSend, true)
	case *ast.CallExpr:
		if m, ok := MutatingCall(f, s); ok {
			out = append(out, m)
		}
	}
	return out
}

type mutations []Mutation

func (out *mutations) add(target ast.Expr, kind MutationKind, reads bool) {
	if target != nil && !isBlank(target) {
		*out = append(*out, Mutation{Target: target, Kind: kind, Reads: reads, Shared: kind == MutationSend})
	}
}

func (out *mutations) assign(s *ast.AssignStmt) {
	if s.Tok == token.DEFINE {
		return
	}
	for _, lhs := range s.Lhs {
		out.add(lhs, assignKind(s.Tok), s.Tok != token.ASSIGN)
	}
}

func (out *mutations) rangeAssign(s *ast.RangeStmt) {
	if s.Tok == token.ASSIGN {
		out.add(s.Key, MutationAssign, false)
		out.add(s.Value, MutationAssign, false)
	}
}

func assignKind(tok token.Token) MutationKind {
	if tok == token.ASSIGN {
		return MutationAssign
	}
	return MutationUpdate
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
}

// InPlaceChanges are standard library functions that change the elements of
// their first argument in place: sorts, and the slices helpers that shift or
// overwrite elements in the backing array even when the result is discarded.
var InPlaceChanges = map[string]bool{
	"sort.Slice": true, "sort.SliceStable": true, "sort.Sort": true,
	"sort.Stable": true, "sort.Strings": true, "sort.Ints": true,
	"sort.Float64s": true, "slices.Sort": true, "slices.SortFunc": true,
	"slices.SortStableFunc": true, "slices.Reverse": true,
	"slices.Delete": true, "slices.DeleteFunc": true, "slices.Compact": true,
	"slices.CompactFunc": true, "slices.Replace": true, "slices.Insert": true,
}

// changingBuiltins are the builtin functions that change the data that their
// first argument refers to. The value is true if the builtin also reads that
// data.
var changingBuiltins = map[string]bool{
	"delete": false, "clear": false, "copy": true, "close": true,
}

// MutatingCall reports the first argument of a call that changes the data
// that the argument refers to. These calls are delete, clear, copy, close, an
// append to a two-index slice of the argument, and an in-place library
// change. MutatingCall removes parentheses and explicit instantiation from the
// callee. A call to a declaration that shadows a builtin or an import name is
// not a mutation.
func MutatingCall(f *ast.File, call *ast.CallExpr) (Mutation, bool) {
	if len(call.Args) == 0 {
		return Mutation{}, false
	}
	m := Mutation{Target: call.Args[0], Kind: MutationCall, Reads: true, Shared: true}
	switch fun := UnwrapTypeInstance(call.Fun).(type) {
	case *ast.Ident:
		if fun.Obj != nil {
			return Mutation{}, false
		}
		reads, ok := changingBuiltins[fun.Name]
		if ok {
			m.Reads = reads
		}
		return m, ok || resliceAppend(fun, call)
	case *ast.SelectorExpr:
		return m, InPlaceChanges[QualifiedName(f, fun)]
	}
	return Mutation{}, false
}

// resliceAppend reports whether call appends to a two-index slice of other
// data, as append(xs[:1], 2) does. The slice keeps the capacity of the data,
// so the append writes into the array of the data.
func resliceAppend(fun *ast.Ident, call *ast.CallExpr) bool {
	if fun.Name != "append" || len(call.Args) < 2 {
		return false
	}
	slice, ok := ast.Unparen(call.Args[0]).(*ast.SliceExpr)
	return ok && !slice.Slice3
}
