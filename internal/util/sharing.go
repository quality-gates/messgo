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
	globals := GlobalSharings(files, definitions)
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

// GlobalSharings describes the value semantics of package variables. A
// struct can have private outer storage while still holding shared fields.
func GlobalSharings(files []*ast.File, definitions map[string]ast.Expr) map[string]Sharing {
	resolver := globalSharingResolver{
		definitions:  definitions,
		results:      packageFunctionResults(files),
		declarations: globalDeclarations(files),
		types:        GlobalValueTypes(files, definitions),
		resolved:     map[string]Sharing{},
	}
	for name := range resolver.declarations {
		resolver.resolve(name, map[string]bool{})
	}
	return resolver.resolved
}

type globalDeclaration struct {
	spec  *ast.ValueSpec
	index int
}

func globalDeclarations(files []*ast.File) map[string]globalDeclaration {
	declarations := map[string]globalDeclaration{}
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					declarations[name.Name] = globalDeclaration{value, i}
				}
			}
		}
	}
	return declarations
}

type globalSharingResolver struct {
	definitions  map[string]ast.Expr
	results      map[string]ast.Expr
	declarations map[string]globalDeclaration
	types        map[string]ast.Expr
	resolved     map[string]Sharing
}

func (r *globalSharingResolver) resolve(name string, seen map[string]bool) Sharing {
	if result, known := r.resolved[name]; known {
		return result
	}
	decl, known := r.declarations[name]
	if !known || seen[name] {
		return shared
	}
	seen[name] = true
	var result Sharing
	if typ := r.types[name]; typ != nil {
		result = TypeSharing(typ, r.definitions)
	} else {
		result = specSharing(decl.spec, decl.index, r.definitions, r.results, func(alias string) Sharing {
			return r.resolve(alias, seen)
		})
	}
	delete(seen, name)
	r.resolved[name] = result
	return result
}

// GlobalValueTypes follows declarations whose value type is known from
// syntax, including aliases and dereferences of package pointers.
func GlobalValueTypes(files []*ast.File, definitions map[string]ast.Expr) map[string]ast.Expr {
	resolver := globalTypeResolver{
		declarations: globalDeclarations(files),
		results:      packageFunctionResults(files),
		definitions:  definitions,
		resolved:     map[string]ast.Expr{},
	}
	for name := range resolver.declarations {
		resolver.resolve(name, map[string]bool{})
	}
	return resolver.resolved
}

type globalTypeResolver struct {
	declarations map[string]globalDeclaration
	results      map[string]ast.Expr
	definitions  map[string]ast.Expr
	resolved     map[string]ast.Expr
}

func (r *globalTypeResolver) resolve(name string, seen map[string]bool) ast.Expr {
	if typ := r.resolved[name]; typ != nil {
		return typ
	}
	decl, known := r.declarations[name]
	if !known || seen[name] {
		return nil
	}
	seen[name] = true
	typ := decl.spec.Type
	if typ == nil && len(decl.spec.Values) == len(decl.spec.Names) {
		typ = r.expressionType(decl.spec.Values[decl.index], seen)
	}
	delete(seen, name)
	if typ != nil {
		r.resolved[name] = typ
	}
	return typ
}

func (r *globalTypeResolver) expressionType(expression ast.Expr, seen map[string]bool) ast.Expr {
	switch value := ast.Unparen(expression).(type) {
	case *ast.CompositeLit:
		return value.Type
	case *ast.Ident:
		return r.resolve(value.Name, seen)
	case *ast.UnaryExpr:
		return r.addressType(value, seen)
	case *ast.StarExpr:
		return r.dereferenceType(value, seen)
	case *ast.CallExpr:
		return r.callType(value)
	case *ast.IndexExpr:
		return r.indexType(value.X, seen)
	case *ast.SelectorExpr:
		return r.fieldType(value.X, value.Sel.Name, seen)
	}
	return nil
}

func (r *globalTypeResolver) addressType(value *ast.UnaryExpr, seen map[string]bool) ast.Expr {
	if value.Op != token.AND {
		return nil
	}
	if typ := r.expressionType(value.X, seen); typ != nil {
		return &ast.StarExpr{X: typ}
	}
	return nil
}

func (r *globalTypeResolver) dereferenceType(value *ast.StarExpr, seen map[string]bool) ast.Expr {
	if pointer, ok := r.expressionType(value.X, seen).(*ast.StarExpr); ok {
		return pointer.X
	}
	return nil
}

func (r *globalTypeResolver) indexType(container ast.Expr, seen map[string]bool) ast.Expr {
	typ := r.typeShape(r.expressionType(container, seen))
	if pointer, ok := typ.(*ast.StarExpr); ok {
		typ = r.typeShape(pointer.X)
	}
	switch value := typ.(type) {
	case *ast.ArrayType:
		return value.Elt
	case *ast.MapType:
		return value.Value
	}
	return nil
}

func (r *globalTypeResolver) fieldType(container ast.Expr, name string, seen map[string]bool) ast.Expr {
	typ := r.typeShape(r.expressionType(container, seen))
	if pointer, ok := typ.(*ast.StarExpr); ok {
		typ = r.typeShape(pointer.X)
	}
	structure, ok := typ.(*ast.StructType)
	if !ok {
		return nil
	}
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 && embeddedFieldName(field.Type) == name {
			return field.Type
		}
		for _, candidate := range field.Names {
			if candidate.Name == name {
				return field.Type
			}
		}
	}
	return nil
}

func embeddedFieldName(typ ast.Expr) string {
	typ = ast.Unparen(typ)
	if pointer, ok := typ.(*ast.StarExpr); ok {
		typ = ast.Unparen(pointer.X)
	}
	switch value := typ.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	}
	return ""
}

func (r *globalTypeResolver) typeShape(typ ast.Expr) ast.Expr {
	seen := map[ast.Expr]bool{}
	for typ != nil && !seen[typ] {
		seen[typ] = true
		id, named := UnwrapTypeInstance(ast.Unparen(typ)).(*ast.Ident)
		if !named {
			return typ
		}
		if id.Obj != nil {
			if spec, ok := id.Obj.Decl.(*ast.TypeSpec); ok {
				typ = spec.Type
				continue
			}
		}
		typ = r.definitions[id.Name]
	}
	return typ
}

func (r *globalTypeResolver) callType(value *ast.CallExpr) ast.Expr {
	name, ok := ast.Unparen(value.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	if name.Name == "new" && len(value.Args) == 1 {
		return &ast.StarExpr{X: value.Args[0]}
	}
	return r.results[name.Name]
}

func packageFunctionResults(files []*ast.File) map[string]ast.Expr {
	results := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}
			field := fn.Type.Results.List[0]
			if len(field.Names) <= 1 {
				results[fn.Name.Name] = field.Type
			}
		}
	}
	return results
}

func specSharing(spec *ast.ValueSpec, index int, definitions map[string]ast.Expr, results map[string]ast.Expr, alias func(string) Sharing) Sharing {
	if spec.Type != nil {
		return TypeSharing(spec.Type, definitions)
	}
	if len(spec.Values) != len(spec.Names) {
		return shared
	}
	return expressionSharing(spec.Values[index], definitions, results, alias)
}

func expressionSharing(expression ast.Expr, definitions map[string]ast.Expr, results map[string]ast.Expr, alias func(string) Sharing) Sharing {
	switch value := ast.Unparen(expression).(type) {
	case *ast.CompositeLit:
		if value.Type != nil {
			return TypeSharing(value.Type, definitions)
		}
	case *ast.BasicLit:
		return Sharing{}
	case *ast.Ident:
		return identifierSharing(value, alias)
	case *ast.CallExpr:
		return callResultSharing(value, definitions, results)
	}
	return shared
}

func identifierSharing(value *ast.Ident, alias func(string) Sharing) Sharing {
	if value.Name == "true" || value.Name == "false" || value.Name == "nil" {
		return Sharing{}
	}
	return alias(value.Name)
}

func callResultSharing(call *ast.CallExpr, definitions map[string]ast.Expr, results map[string]ast.Expr) Sharing {
	if fun, ok := ast.Unparen(call.Fun).(*ast.Ident); ok {
		if typ := results[fun.Name]; typ != nil {
			return TypeSharing(typ, definitions)
		}
	}
	return shared
}

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
