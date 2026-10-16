package util

import (
	"go/ast"
	"go/token"
	"path"
	"strconv"
)

// MutatedGlobalNames returns the set of package-level variable names that are
// mutated somewhere across the given files (the files of a single package).
// A variable is "mutated" if it is reassigned (`=`, `+=`, ...), incremented or
// decremented, has a field/element written through it (`g.f = x`, `g[k] = v`),
// is changed by delete, clear, copy, close or an in-place sort (`sort.Ints(g)`),
// is sent to (`g <- x`), or has its address taken (`&g`). These are the
// targets that Mutations reports. Short variable declarations (`:=`) introduce locals and
// are ignored, as are the variables' own initializers (which are declarations,
// not assignments). The result is intersected with the names
// actually declared as package-level vars, so locals never appear.
//
// Detection is AST-based and deliberately errs toward visibility: a local that
// shadows a global name (via `var`) may cause the global to be reported, which
// is preferable to hiding a genuinely mutable global.
func MutatedGlobalNames(files []*ast.File) map[string]bool {
	globals, topSpecs := topLevelVarNames(files)
	mutated := map[string]bool{}
	if len(globals) == 0 {
		return mutated
	}
	for _, f := range files {
		collectMutations(f, globals, topSpecs, mutated)
	}
	return mutated
}

// collectMutations records, into mutated, every name in globals that is mutated
// anywhere in f.
func collectMutations(f *ast.File, globals map[string]bool, topSpecs map[any]bool, mutated map[string]bool) {
	mark := func(e ast.Expr) {
		id := RootIdent(e)
		if id == nil || !globals[id.Name] {
			return
		}
		if id.Obj != nil && !topSpecs[id.Obj.Decl] {
			return
		}
		mutated[id.Name] = true
	}
	ast.Inspect(f, func(n ast.Node) bool {
		for _, m := range Mutations(f, n) {
			mark(m.Target)
		}
		return true
	})
}

// QualifiedName returns the import path and name of a package member, for
// example "net/http.Get", or "" if sel is not a member of a package that f
// imports.
func QualifiedName(f *ast.File, sel *ast.SelectorExpr) string {
	qualifier, ok := sel.X.(*ast.Ident)
	if !ok || qualifier.Obj != nil {
		return ""
	}
	pkg := ImportedPath(f, qualifier.Name)
	if pkg == "" {
		return ""
	}
	return pkg + "." + sel.Sel.Name
}

// ImportedPath returns the path of the package that f imports as name, or ""
// if f does not import a package with that name.
func ImportedPath(f *ast.File, name string) string {
	if f == nil {
		return ""
	}
	for _, spec := range f.Imports {
		importPath, ok := importPath(spec)
		if ok && importMatchesQualifier(spec, importPath, name) {
			return importPath
		}
	}
	return ""
}

func importMatchesQualifier(spec *ast.ImportSpec, importPath, qualifierName string) bool {
	localName := path.Base(importPath)
	if spec.Name != nil {
		localName = spec.Name.Name
	}
	return localName != "_" && localName != "." && localName == qualifierName
}

func importPath(spec *ast.ImportSpec) (string, bool) {
	if spec == nil || spec.Path == nil {
		return "", false
	}
	value, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// topLevelVarNames collects the names declared in package-level `var`
// declarations across the files and the set of top-level ValueSpec nodes.
// The blank identifier is skipped.
func topLevelVarNames(files []*ast.File) (map[string]bool, map[any]bool) {
	names := map[string]bool{}
	specs := map[any]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			collectGenDeclVars(decl, names, specs)
		}
	}
	return names, specs
}

func collectGenDeclVars(decl ast.Decl, names map[string]bool, specs map[any]bool) {
	gd, ok := decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return
	}
	for _, spec := range gd.Specs {
		collectValueSpecVars(spec, names, specs)
	}
}

func collectValueSpecVars(spec ast.Spec, names map[string]bool, specs map[any]bool) {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	specs[vs] = true
	for _, name := range vs.Names {
		if name.Name != "_" {
			names[name.Name] = true
		}
	}
}

// RootIdent peels selector, index, slice, star and paren wrappers off an lvalue
// to its leading identifier: x, x.f, x[i], x[i:j], *x, (x) all reduce to x. A
// slice expression shares the storage of its operand, so a write through
// x[i:j] changes x. Returns nil if the expression is not rooted at an
// identifier.
func RootIdent(e ast.Expr) *ast.Ident {
	for {
		switch t := e.(type) {
		case *ast.Ident:
			return t
		case *ast.SelectorExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.SliceExpr:
			e = t.X
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		default:
			return nil
		}
	}
}

// TypeAliasNames maps each type-alias name declared across the given files
// (`type alias = original`) to the local type name it aliases. Aliases of
// types from other packages, and of unnamed types such as struct literals,
// are omitted: there is no local named type for them to denote.
func TypeAliasNames(files []*ast.File) map[string]string {
	aliases := map[string]string{}
	for _, f := range files {
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
				collectTypeAliases(gen, aliases)
			}
		}
	}
	return aliases
}

func collectTypeAliases(gen *ast.GenDecl, aliases map[string]string) {
	for _, spec := range gen.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || !ts.Assign.IsValid() {
			continue
		}
		if target := aliasTargetName(ts.Type); target != "" && target != ts.Name.Name {
			aliases[ts.Name.Name] = target
		}
	}
}

// aliasTargetName unwraps pointers and generic instantiations to the bare
// identifier naming an alias target.
func aliasTargetName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return aliasTargetName(t.X)
	case *ast.ParenExpr:
		return aliasTargetName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return aliasTargetName(t.X)
	case *ast.IndexListExpr:
		return aliasTargetName(t.X)
	}
	return ""
}

// UnwrapTypeInstance strips parentheses and generic instantiation from a type
// expression, so `(M[string, int])` becomes `M`.
func UnwrapTypeInstance(e ast.Expr) ast.Expr {
	for {
		switch t := e.(type) {
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		default:
			return e
		}
	}
}

// ResolveTypeAlias follows a chain of type aliases to the name of the defined
// type they denote. Names that are not aliases are returned unchanged, and a
// cyclic chain resolves to the name it started from.
func ResolveTypeAlias(aliases map[string]string, name string) string {
	seen := map[string]bool{}
	for {
		target, ok := aliases[name]
		if !ok || seen[name] {
			return name
		}
		seen[name] = true
		name = target
	}
}
