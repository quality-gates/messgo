package model

import (
	"go/ast"
	"go/constant"
	"go/token"

	"github.com/quality-gates/messgo/internal/util"
)

// Call describes a function call found inside an artifact.
type Call struct {
	Name        string
	Selector    string
	PackagePath string
	Line        int
}

// SourcePosition identifies a position in the original source file.
type SourcePosition struct {
	Line   int
	Column int
}

// DuplicateLiteralKey describes a repeated constant key in a composite literal.
type DuplicateLiteralKey struct {
	Display   string
	FirstLine int
	Line      int
}

// PackageVar describes a package-level variable declaration.
type PackageVar struct {
	Name string
	Line int
}

// HasGoto reports whether the function body contains a goto statement.
func HasGoto(f *Function) bool {
	if f.Body == nil {
		return false
	}
	found := false
	ast.Inspect(f.Body, func(n ast.Node) bool {
		if b, ok := n.(*ast.BranchStmt); ok && b.Tok == token.GOTO {
			found = true
			return false
		}
		return true
	})
	return found
}

// Calls returns every call expression in this function body.
func Calls(f *Function) []Call {
	if f == nil || f.Body == nil {
		return nil
	}
	var out []Call
	ast.Inspect(f.Body, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			out = append(out, callDetails(f.File, ce))
		}
		return true
	})
	return out
}

func callDetails(file *File, ce *ast.CallExpr) Call {
	call := Call{Name: calleeName(ce.Fun), Line: file.Fset.Position(ce.Pos()).Line}
	if selector, ok := calledSelector(ce.Fun); ok {
		call.Selector = selector.Sel.Name
		call.PackagePath = ImportedPackagePath(file, packageQualifier(selector.X))
	}
	return call
}

func calledSelector(expr ast.Expr) (*ast.SelectorExpr, bool) {
	for {
		switch node := expr.(type) {
		case *ast.ParenExpr:
			expr = node.X
		case *ast.SelectorExpr:
			return node, true
		default:
			return nil, false
		}
	}
}

func packageQualifier(expr ast.Expr) *ast.Ident {
	for {
		switch node := expr.(type) {
		case *ast.ParenExpr:
			expr = node.X
		case *ast.Ident:
			return node
		default:
			return nil
		}
	}
}

// ImportedPackagePath returns the import path that qualifier names in file, or
// "" if qualifier is not an imported package name.
func ImportedPackagePath(file *File, qualifier *ast.Ident) string {
	if !resolvableImportQualifier(file, qualifier) {
		return ""
	}
	return util.ImportedPath(file.Syntax, qualifier.Name)
}

func resolvableImportQualifier(file *File, qualifier *ast.Ident) bool {
	return file != nil && file.Syntax != nil && qualifier != nil && qualifier.Obj == nil
}

// LoopConditionCalls returns calls to selected names found in for-loop
// conditions, reported at the loop's line.
func LoopConditionCalls(f *Function, names map[string]bool) []Call {
	if f == nil || f.Body == nil {
		return nil
	}
	var out []Call
	ast.Inspect(f.Body, func(n ast.Node) bool {
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return true
		}
		ast.Inspect(fs.Cond, func(cn ast.Node) bool {
			ce, ok := cn.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(ce.Fun)
			if names[name] {
				out = append(out, Call{Name: name, Line: f.File.Fset.Position(fs.Pos()).Line})
			}
			return true
		})
		return true
	})
	return out
}

// EmptyNilCheckBlockLines returns lines for empty if-blocks whose condition
// compares any operand with nil using !=.
func EmptyNilCheckBlockLines(f *Function) []int {
	if f == nil || f.Body == nil {
		return nil
	}
	var lines []int
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok || ifs.Body == nil || len(ifs.Body.List) != 0 {
			return true
		}
		if conditionChecksNil(ifs.Cond) {
			lines = append(lines, f.File.Fset.Position(ifs.Pos()).Line)
		}
		return true
	})
	return lines
}

// ElseBlockLines returns the source lines of else blocks, excluding else-if
// chains.
func ElseBlockLines(f *Function) []int {
	if f == nil || f.Body == nil {
		return nil
	}
	var lines []int
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if _, isBlock := ifs.Else.(*ast.BlockStmt); isBlock {
			lines = append(lines, f.File.Fset.Position(ifs.Else.Pos()).Line)
		}
		return true
	})
	return lines
}

// IfAssignmentInitPositions returns positions for plain assignment initializers
// in if statements. Short declarations are intentionally excluded.
func IfAssignmentInitPositions(f *Function) []SourcePosition {
	if f == nil || f.Body == nil {
		return nil
	}
	var positions []SourcePosition
	ast.Inspect(f.Body, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := ifs.Init.(*ast.AssignStmt)
		if ok && assign.Tok == token.ASSIGN {
			pos := f.File.Fset.Position(assign.Pos())
			positions = append(positions, SourcePosition{Line: pos.Line, Column: pos.Column})
		}
		return true
	})
	return positions
}

// DuplicateLiteralKeys returns duplicate constant keys in composite literals.
func DuplicateLiteralKeys(f *Function) []DuplicateLiteralKey {
	if f == nil || f.Body == nil {
		return nil
	}
	return duplicateLiteralKeys(f.Body, f.File.Fset)
}

// PackageDuplicateLiteralKeys returns duplicate constant keys in package-level
// composite literals.
func (f *File) PackageDuplicateLiteralKeys() []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	for _, decl := range f.Syntax.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		out = append(out, duplicateLiteralKeys(gd, f.Fset)...)
	}
	return out
}

func duplicateLiteralKeys(root ast.Node, fset *token.FileSet) []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	ast.Inspect(root, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		out = append(out, duplicateKeysInLiteral(cl, fset)...)
		return true
	})
	return out
}

func duplicateKeysInLiteral(cl *ast.CompositeLit, fset *token.FileSet) []DuplicateLiteralKey {
	var out []DuplicateLiteralKey
	seen := map[string]int{}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := literalKey(kv.Key)
		if !ok {
			continue
		}
		line := fset.Position(kv.Key.Pos()).Line
		if first, dup := seen[key]; dup {
			out = append(out, DuplicateLiteralKey{
				Display:   displayKey(kv.Key),
				FirstLine: first,
				Line:      line,
			})
			continue
		}
		seen[key] = line
	}
	return out
}

// PackageVars returns package-level variables declared in the file, skipping
// the blank identifier.
func (f *File) PackageVars() []PackageVar {
	var out []PackageVar
	for _, decl := range f.Syntax.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.Name == "_" {
					continue
				}
				out = append(out, PackageVar{Name: name.Name, Line: f.Fset.Position(name.Pos()).Line})
			}
		}
	}
	return out
}

// MutatedPackageGlobals returns package-level variable names mutated in this
// file unless the runner has already populated cross-file mutation data.
func (f *File) MutatedPackageGlobals() map[string]bool {
	if f.MutatedGlobals != nil {
		return f.MutatedGlobals
	}
	return util.MutatedGlobalNames([]*ast.File{f.Syntax})
}

func conditionChecksNil(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || be.Op != token.NEQ {
			return true
		}
		if isNilIdent(be.X) || isNilIdent(be.Y) {
			found = true
		}
		return true
	})
	return found
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func literalKey(e ast.Expr) (string, bool) {
	switch k := e.(type) {
	case *ast.ParenExpr:
		return literalKey(k.X)
	case *ast.UnaryExpr:
		return unaryLitKey(k)
	case *ast.BasicLit:
		return basicLitKey(k.Value, k.Kind)
	case *ast.Ident:
		return "ident:" + k.Name, true
	}
	return "", false
}

func basicLitKey(val string, kind token.Token) (string, bool) {
	v := constant.MakeFromLiteral(val, kind, 0)
	if v.Kind() == constant.Unknown {
		return kind.String() + ":" + val, true
	}
	return v.Kind().String() + ":" + v.ExactString(), true
}

func unaryLitKey(u *ast.UnaryExpr) (string, bool) {
	if u.Op != token.SUB && u.Op != token.ADD {
		return "", false
	}
	lit, ok := u.X.(*ast.BasicLit)
	if !ok {
		return "", false
	}
	return basicLitKey(u.Op.String()+lit.Value, lit.Kind)
}

func displayKey(e ast.Expr) string {
	switch k := e.(type) {
	case *ast.ParenExpr:
		return displayKey(k.X)
	case *ast.UnaryExpr:
		if k.Op == token.SUB || k.Op == token.ADD {
			return k.Op.String() + displayKey(k.X)
		}
	case *ast.BasicLit:
		return k.Value
	case *ast.Ident:
		return k.Name
	}
	return ""
}

func calleeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		x := calleeName(t.X)
		if x == "" {
			return t.Sel.Name
		}
		return x + "." + t.Sel.Name
	case *ast.ParenExpr:
		return calleeName(t.X)
	default:
		return ""
	}
}
