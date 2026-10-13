package util

import (
	"go/ast"
	"go/token"
	"maps"
)

// Sharing tells which parts of a value of a type can refer to storage that
// other values share. Outer is true if the value itself refers to shared
// storage, as a slice, a map or a pointer does. Reachable is true if data
// that the value holds can refer to shared storage.
type Sharing struct {
	Outer, Reachable bool
}

func (s Sharing) any() bool { return s.Outer || s.Reachable }

var shared = Sharing{Outer: true, Reachable: true}

// basicTypes are the predeclared types whose values hold no references.
var basicTypes = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"byte": true, "rune": true, "float32": true, "float64": true, "complex64": true, "complex128": true,
}

// IsBasicType reports whether name is a predeclared type whose values hold no
// references.
func IsBasicType(name string) bool { return basicTypes[name] }

// TypeSharing returns the sharing of a value of typ. A type that the analysis
// cannot resolve, as a type parameter or an imported type, may share any
// storage. definitions holds the named types of the package.
func TypeSharing(typ ast.Expr, definitions map[string]ast.Expr) Sharing {
	return typeSharing(typ, map[ast.Expr]bool{}, definitions)
}

func typeSharing(typ ast.Expr, seen map[ast.Expr]bool, definitions map[string]ast.Expr) Sharing {
	typ = UnwrapTypeInstance(ast.Unparen(typ))
	if typ == nil {
		return Sharing{}
	}
	if seen[typ] {
		return shared
	}
	seen[typ] = true
	switch value := typ.(type) {
	case *ast.ArrayType:
		return Sharing{Outer: value.Len == nil, Reachable: reachableType(value.Elt, seen, definitions)}
	case *ast.MapType:
		return mapSharing(value, seen, definitions)
	case *ast.ChanType:
		return Sharing{Outer: true, Reachable: reachableType(value.Value, seen, definitions)}
	case *ast.StructType:
		return Sharing{Reachable: fieldsShare(value.Fields, seen, definitions)}
	case *ast.FuncType:
		return Sharing{}
	case *ast.Ident:
		return namedSharing(value, seen, definitions)
	}
	return shared
}

func mapSharing(typ *ast.MapType, seen map[ast.Expr]bool, definitions map[string]ast.Expr) Sharing {
	return Sharing{Outer: true, Reachable: reachableType(typ.Key, seen, definitions) || reachableType(typ.Value, seen, definitions)}
}

func reachableType(typ ast.Expr, seen map[ast.Expr]bool, definitions map[string]ast.Expr) bool {
	return typeSharing(typ, maps.Clone(seen), definitions).any()
}

func fieldsShare(fields *ast.FieldList, seen map[ast.Expr]bool, definitions map[string]ast.Expr) bool {
	for _, field := range fields.List {
		if reachableType(field.Type, seen, definitions) {
			return true
		}
	}
	return false
}

func namedSharing(id *ast.Ident, seen map[ast.Expr]bool, definitions map[string]ast.Expr) Sharing {
	if id.Obj != nil {
		if spec, ok := id.Obj.Decl.(*ast.TypeSpec); ok {
			return typeSharing(spec.Type, seen, definitions)
		}
		return shared
	}
	if definition := definitions[id.Name]; definition != nil {
		return typeSharing(definition, seen, definitions)
	}
	if basicTypes[id.Name] {
		return Sharing{}
	}
	return shared
}

// SharedGlobalNames returns the package variables whose contents a function
// cannot assume to stay the same while it runs. A variable is shared if the
// package changes it, if other packages can change it because it is
// exported, or if data that it refers to escapes into other data, a call, or
// a result, where code can change it.
func SharedGlobalNames(files []*ast.File, definitions map[string]ast.Expr) map[string]bool {
	out := MutatedGlobalNames(files)
	globals := globalSharings(files, definitions)
	for name := range globals {
		if ast.IsExported(name) {
			out[name] = true
		}
	}
	_, specs := topLevelVarNames(files)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			for _, use := range aliasingUses(n) {
				if name := aliasedGlobal(use, globals, specs); name != "" {
					out[name] = true
				}
			}
			return true
		})
	}
	return out
}

// globalSharings returns the sharing of each package variable, from its type
// or from the literal that initializes it.
func globalSharings(files []*ast.File, definitions map[string]ast.Expr) map[string]Sharing {
	out := map[string]Sharing{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					out[name.Name] = specSharing(value, i, definitions)
				}
			}
		}
	}
	return out
}

func specSharing(spec *ast.ValueSpec, index int, definitions map[string]ast.Expr) Sharing {
	if spec.Type != nil {
		return TypeSharing(spec.Type, definitions)
	}
	if len(spec.Values) != len(spec.Names) {
		return shared
	}
	switch value := ast.Unparen(spec.Values[index]).(type) {
	case *ast.CompositeLit:
		if value.Type != nil {
			return TypeSharing(value.Type, definitions)
		}
	case *ast.BasicLit:
		return Sharing{}
	}
	return shared
}

// use is an expression whose value other code gets. deep is true if the
// other code gets the elements of the value. receiver is true if the other
// code is a method, which can also get the address of the value.
type use struct {
	expr     ast.Expr
	deep     bool
	receiver bool
}

func aliasingUses(n ast.Node) []use {
	switch s := n.(type) {
	case *ast.AssignStmt:
		return uses(s.Rhs)
	case *ast.ValueSpec:
		return uses(s.Values)
	case *ast.ReturnStmt:
		return uses(s.Results)
	case *ast.SendStmt:
		return uses([]ast.Expr{s.Value})
	case *ast.CompositeLit:
		return elementUses(s.Elts)
	case *ast.CallExpr:
		return callUses(s)
	case *ast.RangeStmt:
		if s.Value != nil {
			return []use{{expr: s.X, deep: true}}
		}
	}
	return nil
}

func uses(exprs []ast.Expr) []use {
	out := make([]use, 0, len(exprs))
	for _, expr := range exprs {
		out = append(out, use{expr: expr})
	}
	return out
}

func elementUses(elements []ast.Expr) []use {
	out := make([]use, 0, len(elements))
	for _, element := range elements {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			element = pair.Value
		}
		out = append(out, use{expr: element})
	}
	return out
}

// callUses returns the arguments of a call, and the receiver of a method.
// len and cap only read the size of their argument.
func callUses(call *ast.CallExpr) []use {
	if name, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && name.Obj == nil && (name.Name == "len" || name.Name == "cap") {
		return nil
	}
	out := uses(call.Args)
	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		out = append(out, use{expr: selector.X, receiver: true})
	}
	return out
}

// aliasedGlobal returns the name of the package variable that u lets other
// code change, or "". A use of an element or a field of a variable shares
// data only if the elements or the fields refer to shared storage.
func aliasedGlobal(u use, globals map[string]Sharing, specs map[any]bool) string {
	deep := u.deep
	expr := u.expr
	for {
		switch value := expr.(type) {
		case *ast.ParenExpr:
			expr = value.X
			continue
		case *ast.SliceExpr:
			expr = value.X
			continue
		case *ast.SelectorExpr:
			expr, deep = value.X, true
			continue
		case *ast.IndexExpr:
			expr, deep = value.X, true
			continue
		case *ast.StarExpr:
			expr, deep = value.X, true
			continue
		case *ast.Ident:
			return globalName(value, deep && !u.receiver, u.receiver, globals, specs)
		}
		return ""
	}
}

func globalName(id *ast.Ident, deep, receiver bool, globals map[string]Sharing, specs map[any]bool) string {
	sharing, global := globals[id.Name]
	if !global || id.Obj != nil && !specs[id.Obj.Decl] {
		return ""
	}
	if receiver || sharing.Reachable || !deep && sharing.Outer {
		return id.Name
	}
	return ""
}
