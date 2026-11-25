package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/quality-gates/messgo/internal/util"
)

type memberSelectionCollector struct {
	types    memberTypeResolver
	scope    memberScopeCollector
	recorder memberUseRecorder
}

type memberTypeResolver struct {
	classes    map[string]*Class
	interfaces map[string]*Interface
	functions  map[string]*Function
	typeDefs   map[string]ast.Expr
}

type memberScopeCollector struct {
	resolver *memberTypeResolver
}

type memberUseRecorder struct {
	resolver *memberTypeResolver
}

func collectSelectedMemberUses(f *File) map[MemberKey]bool {
	uses := map[MemberKey]bool{}
	collector := newMemberSelectionCollector(f)
	packageTypes := collector.packageTypes(f)

	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			collector.collectFunc(node, packageTypes, uses)
			return false
		case *ast.SelectorExpr:
			collector.recorder.recordSelector(node, packageTypes, uses)
		case *ast.CompositeLit:
			collector.recorder.recordComposite(node, uses)
		}
		return true
	})
	return uses
}

func newMemberSelectionCollector(f *File) *memberSelectionCollector {
	classes := f.Classes
	if f.PackageClasses != nil {
		classes = f.PackageClasses
	}
	classByName := make(map[string]*Class, len(classes))
	for _, class := range classes {
		classByName[class.Name] = class
	}
	ifaces := f.Interfaces
	if f.PackageInterfaces != nil {
		ifaces = f.PackageInterfaces
	}
	ifaceByName := make(map[string]*Interface, len(ifaces))
	for _, iface := range ifaces {
		ifaceByName[iface.Name] = iface
	}
	indexMemberAliases(f, classByName, ifaceByName)
	funcs := f.AllFuncs
	if f.PackageFunctions != nil {
		funcs = f.PackageFunctions
	}
	functions := make(map[string]*Function, len(funcs))
	for _, fn := range funcs {
		if !fn.IsMethod() && functions[fn.Name] == nil {
			functions[fn.Name] = fn
		}
	}
	typeDefs := memberTypeDefs(f, classes)
	collector := &memberSelectionCollector{
		types: memberTypeResolver{
			classes:    classByName,
			interfaces: ifaceByName,
			functions:  functions,
			typeDefs:   typeDefs,
		},
	}
	collector.scope.resolver = &collector.types
	collector.recorder.resolver = &collector.types
	return collector
}

func indexMemberAliases(f *File, classByName map[string]*Class, ifaceByName map[string]*Interface) {
	aliases := f.PackageTypeAliases
	if aliases == nil {
		aliases = util.TypeAliasNames([]*ast.File{f.Syntax})
	}
	for alias := range aliases {
		target := util.ResolveTypeAlias(aliases, alias)
		if class := classByName[target]; class != nil {
			classByName[alias] = class
		}
		if iface := ifaceByName[target]; iface != nil {
			ifaceByName[alias] = iface
		}
	}
}

func memberTypeDefs(f *File, classes []*Class) map[string]ast.Expr {
	if f.PackageTypeDefs != nil {
		return f.PackageTypeDefs
	}
	return collectPackageTypeDefs(f, classes)
}

func (c *memberSelectionCollector) packageTypes(f *File) map[string]memberVarType {
	types := map[string]memberVarType{}
	for _, decl := range f.Syntax.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if ok {
				c.scope.addValueSpec(value, types)
			}
		}
	}
	return types
}

func (c *memberSelectionCollector) collectFunc(decl *ast.FuncDecl, packageTypes map[string]memberVarType, uses map[MemberKey]bool) {
	types := cloneMemberTypes(packageTypes)
	c.scope.addFuncParameters(decl, types)
	if decl.Body == nil {
		return
	}
	c.scope.collectLocalTypes(decl.Body, types)
	c.collectBody(decl.Body, types, uses)
}

func cloneMemberTypes(source map[string]memberVarType) map[string]memberVarType {
	clone := make(map[string]memberVarType, len(source))
	for name, typeName := range source {
		clone[name] = typeName
	}
	return clone
}

func (c *memberScopeCollector) addFuncParameters(decl *ast.FuncDecl, types map[string]memberVarType) {
	if decl.Recv != nil {
		addNamedFieldTypes(decl.Recv.List, types)
	}
	if decl.Type == nil || decl.Type.Params == nil {
		return
	}
	addNamedFieldTypes(decl.Type.Params.List, types)
}

func addNamedFieldTypes(fields []*ast.Field, types map[string]memberVarType) {
	for _, field := range fields {
		fieldType := memberVarTypeOf(field.Type)
		if fieldType.empty() {
			continue
		}
		for _, name := range field.Names {
			types[name.Name] = fieldType
		}
	}
}

func (c *memberScopeCollector) collectLocalTypes(body *ast.BlockStmt, types map[string]memberVarType) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeclStmt:
			c.addDeclaration(node, types)
		case *ast.AssignStmt:
			c.addAssignment(node, types)
		case *ast.RangeStmt:
			c.addRange(node, types)
		}
		return true
	})
}

func (c *memberScopeCollector) addDeclaration(stmt *ast.DeclStmt, types map[string]memberVarType) {
	gen, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if ok {
			c.addValueSpec(value, types)
		}
	}
}

func (c *memberScopeCollector) addValueSpec(spec *ast.ValueSpec, types map[string]memberVarType) {
	declaredType := memberVarTypeOf(spec.Type)
	valueTypes := c.valueSpecRhsTypes(spec, declaredType, types)
	for index, name := range spec.Names {
		if name.Name == "_" || index >= len(valueTypes) {
			continue
		}
		if !valueTypes[index].empty() {
			types[name.Name] = valueTypes[index]
		}
	}
}

func (c *memberScopeCollector) valueSpecRhsTypes(spec *ast.ValueSpec, declaredType memberVarType, types map[string]memberVarType) []memberVarType {
	if !declaredType.empty() {
		result := make([]memberVarType, len(spec.Names))
		for i := range result {
			result[i] = declaredType
		}
		return result
	}
	if len(spec.Values) == 1 && len(spec.Names) > 1 {
		if call, ok := unwrapParen(spec.Values[0]).(*ast.CallExpr); ok {
			return c.resolver.callResultTypes(call, types)
		}
	}
	result := make([]memberVarType, len(spec.Values))
	for i, v := range spec.Values {
		result[i] = c.resolver.resolvedType(v, types)
	}
	return result
}

func (c *memberScopeCollector) addAssignment(stmt *ast.AssignStmt, types map[string]memberVarType) {
	rhsTypes := c.assignmentRhsTypes(stmt, types)
	for index, lhs := range stmt.Lhs {
		name, ok := lhs.(*ast.Ident)
		if !ok || name.Name == "_" || index >= len(rhsTypes) {
			continue
		}
		if !rhsTypes[index].empty() {
			types[name.Name] = rhsTypes[index]
		}
	}
}

func (c *memberScopeCollector) assignmentRhsTypes(stmt *ast.AssignStmt, types map[string]memberVarType) []memberVarType {
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		if call, ok := unwrapParen(stmt.Rhs[0]).(*ast.CallExpr); ok {
			return c.resolver.callResultTypes(call, types)
		}
	}
	result := make([]memberVarType, len(stmt.Rhs))
	for i, rhs := range stmt.Rhs {
		result[i] = c.resolver.resolvedType(rhs, types)
	}
	return result
}

// addRange binds the iteration variables of a range statement. Which variable
// carries the container's member-bearing type depends on the container: a map
// yields key then value, a sequence yields index then element, and a channel
// yields the element in the first variable.
func (c *memberScopeCollector) addRange(stmt *ast.RangeStmt, types map[string]memberVarType) {
	subject := c.resolver.resolvedType(stmt.X, types)
	switch subject.kind {
	case containerMap:
		bindRangeVar(stmt.Key, memberVarType{name: subject.key}, types)
		bindRangeVar(stmt.Value, elementVarType(subject), types)
	case containerSequence:
		bindRangeVar(stmt.Value, elementVarType(subject), types)
	case containerChannel:
		bindRangeVar(stmt.Key, elementVarType(subject), types)
	default:
		bindRangeVar(rangeElementVar(stmt), elementVarType(subject), types)
	}
}

// rangeElementVar picks the variable that holds the element of a container of
// unknown shape, preferring the value variable when the statement has one.
func rangeElementVar(stmt *ast.RangeStmt) ast.Expr {
	if stmt.Value != nil {
		return stmt.Value
	}
	return stmt.Key
}

func bindRangeVar(target ast.Expr, varType memberVarType, types map[string]memberVarType) {
	id, ok := target.(*ast.Ident)
	if !ok || id.Name == "_" || varType.empty() {
		return
	}
	types[id.Name] = varType
}

func (c *memberSelectionCollector) collectBody(body ast.Node, types map[string]memberVarType, uses map[MemberKey]bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			nestedTypes := cloneMemberTypes(types)
			c.scope.addFuncLiteralParameters(node, nestedTypes)
			c.scope.collectLocalTypes(node.Body, nestedTypes)
			c.collectBody(node.Body, nestedTypes, uses)
			return false
		case *ast.TypeSwitchStmt:
			c.collectTypeSwitch(node, types, uses)
			return false
		case *ast.SelectorExpr:
			c.recorder.recordSelector(node, types, uses)
		case *ast.CompositeLit:
			c.recorder.recordComposite(node, uses)
		}
		return true
	})
}

// collectTypeSwitch binds the switch variable of `switch v := x.(type)` to the
// case type inside each single-type clause; in any other clause v keeps the
// type of x, which is unknown here, so it is unbound.
func (c *memberSelectionCollector) collectTypeSwitch(stmt *ast.TypeSwitchStmt, types map[string]memberVarType, uses map[MemberKey]bool) {
	if stmt.Init != nil {
		c.collectBody(stmt.Init, types, uses)
	}
	c.collectBody(stmt.Assign, types, uses)
	bound := typeSwitchVar(stmt)
	for _, clause := range stmt.Body.List {
		cc := clause.(*ast.CaseClause)
		c.collectBody(cc, clauseTypes(types, bound, cc), uses)
	}
}

func clauseTypes(types map[string]memberVarType, bound string, cc *ast.CaseClause) map[string]memberVarType {
	scoped := cloneMemberTypes(types)
	delete(scoped, bound)
	if len(cc.List) == 1 {
		scoped[bound] = memberVarTypeOf(cc.List[0])
	}
	return scoped
}

func typeSwitchVar(stmt *ast.TypeSwitchStmt) string {
	if assign, ok := stmt.Assign.(*ast.AssignStmt); ok {
		return assign.Lhs[0].(*ast.Ident).Name
	}
	return ""
}

func (c *memberScopeCollector) addFuncLiteralParameters(lit *ast.FuncLit, types map[string]memberVarType) {
	if lit.Type == nil || lit.Type.Params == nil {
		return
	}
	addNamedFieldTypes(lit.Type.Params.List, types)
}

func (c *memberUseRecorder) recordSelector(sel *ast.SelectorExpr, types map[string]memberVarType, uses map[MemberKey]bool) {
	name := sel.Sel.Name
	base := c.resolver.resolvedType(sel.X, types)
	if base.empty() {
		return
	}
	typeName := base.name
	uses[MemberKey{Type: typeName, Name: name}] = true
	for _, key := range c.resolver.promotedMemberPathTyped(base, name) {
		uses[key] = true
	}
}

func (c *memberUseRecorder) recordComposite(lit *ast.CompositeLit, uses map[MemberKey]bool) {
	c.recordCompositeWithType(lit, lit.Type, uses)
}

func (c *memberUseRecorder) recordChildElement(expr ast.Expr, expectedType ast.Expr, uses map[MemberKey]bool) {
	if childLit, ok := expr.(*ast.CompositeLit); ok && childLit.Type == nil {
		c.recordCompositeWithType(childLit, expectedType, uses)
	}
}

func (c *memberUseRecorder) recordCompositeWithType(lit *ast.CompositeLit, typeExpr ast.Expr, uses map[MemberKey]bool) {
	if c.recordCollection(lit, typeExpr, uses) {
		return
	}
	typeName := memberTypeName(typeExpr)
	if recordKeyedMembers(lit, typeName, uses) {
		return
	}
	if len(lit.Elts) == 0 || typeName == "" {
		return
	}
	c.recordUnkeyedMembers(typeName, lit, uses)
}

func (c *memberUseRecorder) recordCollection(lit *ast.CompositeLit, typeExpr ast.Expr, uses map[MemberKey]bool) bool {
	switch t := c.underlyingType(typeExpr).(type) {
	case *ast.ArrayType:
		c.recordArrayElements(lit, t.Elt, uses)
		return true
	case *ast.MapType:
		c.recordMapElements(lit, t.Key, t.Value, uses)
		return true
	default:
		return false
	}
}

func (c *memberUseRecorder) underlyingType(expr ast.Expr) ast.Expr {
	for {
		id, ok := expr.(*ast.Ident)
		if !ok {
			return expr
		}
		def, ok := c.resolver.typeDefs[id.Name]
		if !ok || def == expr {
			return expr
		}
		expr = def
	}
}

func collectPackageTypeDefs(f *File, classes []*Class) map[string]ast.Expr {
	defs := make(map[string]ast.Expr)
	collectFromFile(f.Syntax, defs)
	for _, class := range classes {
		if class.File != nil {
			collectFromFile(class.File.Syntax, defs)
		}
	}
	return defs
}

func collectFromFile(syntax *ast.File, defs map[string]ast.Expr) {
	if syntax == nil {
		return
	}
	for _, decl := range syntax.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Type != nil {
					defs[ts.Name.Name] = ts.Type
				}
			}
		}
	}
}

func (c *memberUseRecorder) recordArrayElements(lit *ast.CompositeLit, eltType ast.Expr, uses map[MemberKey]bool) {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		c.recordChildElement(elt, eltType, uses)
	}
}

func (c *memberUseRecorder) recordMapElements(lit *ast.CompositeLit, keyType, valType ast.Expr, uses map[MemberKey]bool) {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			c.recordChildElement(kv.Key, keyType, uses)
			c.recordChildElement(kv.Value, valType, uses)
		}
	}
}

func recordKeyedMembers(lit *ast.CompositeLit, typeName string, uses map[MemberKey]bool) bool {
	keyed := false
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyed = true
		recordKeyedMember(kv, typeName, uses)
	}
	return keyed
}

func recordKeyedMember(kv *ast.KeyValueExpr, typeName string, uses map[MemberKey]bool) {
	id, ok := kv.Key.(*ast.Ident)
	if !ok {
		return
	}
	if typeName != "" {
		uses[MemberKey{Type: typeName, Name: id.Name}] = true
	}
}

func (c *memberUseRecorder) recordUnkeyedMembers(typeName string, lit *ast.CompositeLit, uses map[MemberKey]bool) {
	class := c.resolver.classes[typeName]
	if class == nil {
		return
	}
	for _, field := range class.Fields {
		uses[MemberKey{Type: typeName, Name: field.Name}] = true
	}
}

// expressionType is the member-bearing type name of an expression, discarding
// the container shape that resolvedType carries.
func (c *memberTypeResolver) expressionType(expr ast.Expr, types map[string]memberVarType) string {
	return c.resolvedType(expr, types).name
}

func (c *memberTypeResolver) resolvedType(expr ast.Expr, types map[string]memberVarType) memberVarType {
	switch node := expr.(type) {
	case *ast.Ident:
		return c.identType(node, types)
	case *ast.SelectorExpr:
		return c.selectorType(node, types)
	case *ast.CompositeLit:
		return memberVarTypeOf(node.Type)
	case *ast.TypeAssertExpr:
		return memberVarTypeOf(node.Type)
	case *ast.CallExpr:
		return c.callType(node, types)
	default:
		return c.wrappedExpressionType(expr, types)
	}
}

// identType resolves a variable to its bound type. An identifier with no
// binding that names a package struct is the receiver of a method expression
// such as T.m or (*T).m, so it resolves to that struct.
func (c *memberTypeResolver) identType(id *ast.Ident, types map[string]memberVarType) memberVarType {
	if bound, ok := types[id.Name]; ok {
		return bound
	}
	if _, ok := c.classes[id.Name]; ok {
		return memberVarType{name: id.Name}
	}
	if _, ok := c.typeDefs[id.Name]; ok {
		return memberVarType{name: id.Name}
	}
	return memberVarType{}
}

func (c *memberTypeResolver) wrappedExpressionType(expr ast.Expr, types map[string]memberVarType) memberVarType {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return c.resolvedType(node.X, types)
	case *ast.StarExpr:
		return c.resolvedType(node.X, types)
	case *ast.UnaryExpr:
		return c.resolvedType(node.X, types)
	case *ast.SliceExpr:
		return c.resolvedType(node.X, types)
	case *ast.IndexExpr:
		return elementType(c.resolvedType(node.X, types))
	case *ast.IndexListExpr:
		return elementType(c.resolvedType(node.X, types))
	default:
		return memberVarType{}
	}
}

// elementType drops the container shape: indexing a container yields an element,
// which is no longer that container.
func elementType(container memberVarType) memberVarType {
	return memberVarType{name: container.name, args: container.args}
}

// elementVarType is the type of an element of a container type, keeping any
// generic instantiation of the element.
func elementVarType(container memberVarType) memberVarType {
	return memberVarType{name: container.name, args: container.args}
}

func (c *memberTypeResolver) selectorType(sel *ast.SelectorExpr, types map[string]memberVarType) memberVarType {
	baseType := c.resolvedType(sel.X, types)
	if baseType.empty() {
		return memberVarType{}
	}
	return c.classMemberTypeTyped(baseType, sel.Sel.Name)
}

func (c *memberTypeResolver) callType(call *ast.CallExpr, types map[string]memberVarType) memberVarType {
	results := c.callResultTypes(call, types)
	if len(results) == 1 {
		return results[0]
	}
	return memberVarType{}
}

func (c *memberTypeResolver) callResultTypes(call *ast.CallExpr, types map[string]memberVarType) []memberVarType {
	if call == nil {
		return nil
	}
	fun, typeArgs := genericCallFun(unwrapParen(call.Fun))
	if id, ok := fun.(*ast.Ident); ok {
		if _, ok := c.typeDefs[id.Name]; ok {
			return []memberVarType{conversionType(id, typeArgs)}
		}
		return resolveIdentCall(c, id, call.Args, typeArgs, types)
	}
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		return resolveSelectorCall(c, sel, types)
	}
	return nil
}

// genericCallFun unwraps an explicit generic instantiation in call position,
// Id[T](...) / pkg.Id[T](...), returning the underlying function expression
// and the explicit type arguments. A plain call returns no type arguments.
func genericCallFun(fun ast.Expr) (ast.Expr, string) {
	switch node := unwrapParen(fun).(type) {
	case *ast.IndexExpr:
		if underlying := unwrapParen(node.X); isIdentOrSelector(underlying) {
			return underlying, memberVarTypeOf(node.Index).canonical()
		}
	case *ast.IndexListExpr:
		if underlying := unwrapParen(node.X); isIdentOrSelector(underlying) {
			return underlying, genericMemberVarType("", node.Indices, typeBinding{}).args
		}
	}
	return fun, ""
}

func isIdentOrSelector(expr ast.Expr) bool {
	switch unwrapParen(expr).(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return true
	default:
		return false
	}
}

func conversionType(id *ast.Ident, typeArgs string) memberVarType {
	if typeArgs != "" {
		return memberVarType{name: id.Name, args: typeArgs}
	}
	return memberVarTypeOf(id)
}

func resolveIdentCall(c *memberTypeResolver, id *ast.Ident, args []ast.Expr, typeArgs string, types map[string]memberVarType) []memberVarType {
	if id.Name == "new" && len(args) == 1 {
		if argType := memberVarTypeOf(args[0]); !argType.empty() {
			return []memberVarType{argType}
		}
		return nil
	}
	if _, ok := c.classes[id.Name]; ok {
		return []memberVarType{{name: id.Name, args: typeArgs}}
	}
	if fn := c.functions[id.Name]; fn != nil {
		subst := genericCallSubst(c, fn, args, typeArgs, types)
		return functionResultTypesWithSubst(fn, subst)
	}
	return nil
}

// genericCallSubst binds a generic function's type parameters for one call:
// from the explicit type arguments when present, otherwise inferred by
// matching each formal parameter type against the corresponding argument.
func genericCallSubst(c *memberTypeResolver, fn *Function, args []ast.Expr, typeArgs string, types map[string]memberVarType) typeBinding {
	names := fn.TypeParams
	if len(names) == 0 {
		return typeBinding{}
	}
	if typeArgs != "" {
		return typeBinding{names: names, args: typeArgs}
	}
	inferred := ""
	for _, name := range names {
		candidate, conflicted := inferredTypeArgFor(c, fn, args, types, name)
		if conflicted {
			candidate = ""
		}
		if inferred != "" {
			inferred += ";"
		}
		inferred += candidate
	}
	return typeBinding{names: names, args: inferred}
}

func inferredTypeArgFor(c *memberTypeResolver, fn *Function, args []ast.Expr, types map[string]memberVarType, name string) (string, bool) {
	candidate := ""
	for i, param := range fn.Params {
		if i >= len(args) {
			break
		}
		pattern := paramTypeExpr(param)
		if pattern == nil {
			continue
		}
		actual := c.resolvedType(args[i], types)
		if actual.empty() {
			continue
		}
		found, conflicted := inferredTypeArg(pattern, actual, name)
		if conflicted {
			return "", true
		}
		if found == "" {
			continue
		}
		if candidate == "" {
			candidate = found
		} else if candidate != found {
			return "", true
		}
	}
	return candidate, false
}

func paramTypeExpr(param *Parameter) ast.Expr {
	if param == nil {
		return nil
	}
	if param.TypeExpr != nil {
		return param.TypeExpr
	}
	if param.Field != nil {
		return param.Field.Type
	}
	return nil
}

// inferredTypeArg extracts, from one formal parameter pattern matched
// against an actual argument type, the type argument implied for the named
// type parameter. It is read-only: it returns the candidate as a canonical
// type string, or conflicted when the pattern implies two different types.
func inferredTypeArg(pattern ast.Expr, actual memberVarType, name string) (string, bool) {
	switch node := unwrapParen(pattern).(type) {
	case *ast.Ident:
		if node.Name == name {
			return actual.canonical(), false
		}
	case *ast.StarExpr:
		return inferredTypeArg(node.X, actual, name)
	case *ast.IndexExpr:
		return inferredGenericArg(memberTypeName(node.X), []ast.Expr{node.Index}, actual, name)
	case *ast.IndexListExpr:
		return inferredGenericArg(memberTypeName(node.X), node.Indices, actual, name)
	default:
		return inferredNonGenericArg(node, actual, name)
	}
	return "", false
}

func inferredNonGenericArg(pattern ast.Expr, actual memberVarType, name string) (string, bool) {
	switch node := pattern.(type) {
	case *ast.ArrayType:
		return inferredContainerArg(node.Elt, actual, name, containerSequence)
	case *ast.Ellipsis:
		return inferredVariadicArg(node.Elt, actual, name)
	case *ast.ChanType:
		return inferredContainerArg(node.Value, actual, name, containerChannel)
	case *ast.MapType:
		return inferredMapArg(node, actual, name)
	}
	return "", false
}

func inferredContainerArg(elt ast.Expr, actual memberVarType, name string, kind containerKind) (string, bool) {
	if actual.kind != kind {
		return "", false
	}
	return inferredTypeArg(elt, elementVarType(actual), name)
}

func inferredVariadicArg(elt ast.Expr, actual memberVarType, name string) (string, bool) {
	if actual.kind == containerSequence {
		return inferredTypeArg(elt, elementVarType(actual), name)
	}
	return inferredTypeArg(elt, actual, name)
}

func inferredMapArg(node *ast.MapType, actual memberVarType, name string) (string, bool) {
	if actual.kind != containerMap {
		return "", false
	}
	keyArg, keyConflict := inferredTypeArg(node.Key, memberVarType{name: actual.key}, name)
	if keyConflict {
		return "", true
	}
	valueArg, valueConflict := inferredTypeArg(node.Value, elementVarType(actual), name)
	if valueConflict || (keyArg != "" && valueArg != "" && keyArg != valueArg) {
		return "", true
	}
	if keyArg != "" {
		return keyArg, false
	}
	return valueArg, false
}

func inferredGenericArg(base string, patterns []ast.Expr, actual memberVarType, name string) (string, bool) {
	if actual.name != base {
		return "", false
	}
	parts := strings.Split(actual.args, ";")
	if len(parts) != len(patterns) {
		return "", false
	}
	candidate := ""
	for i, pattern := range patterns {
		found, conflicted := inferredTypeArg(pattern, memberVarTypeFromString(parts[i]), name)
		if conflicted {
			return "", true
		}
		if found == "" {
			continue
		}
		if candidate == "" {
			candidate = found
		} else if candidate != found {
			return "", true
		}
	}
	return candidate, false
}

func resolveSelectorCall(r *memberTypeResolver, sel *ast.SelectorExpr, types map[string]memberVarType) []memberVarType {
	baseType := r.resolvedType(sel.X, types)
	if baseType.empty() {
		return nil
	}
	method, declaring := lookupMethodTyped(r.classes, r.interfaces, baseType, sel.Sel.Name, map[string]bool{})
	if method == nil {
		return nil
	}
	return functionResultTypesWithSubst(method, bindingForType(r.classes, r.interfaces, declaring))
}

type selectedMember struct {
	memberType memberVarType
	method     *Function
	path       []MemberKey
	// declaring is the instantiated type that declares the member, so a
	// generic method's results can be substituted by its callers.
	declaring memberVarType
}

type embeddedMemberSearch struct {
	typeName string
	// typ is the instantiated type being searched; when empty, typeName alone
	// is the type. typ.args carries the generic arguments for typ.name.
	typ      memberVarType
	path     []MemberKey
	multiple bool
}

// effectiveType returns the instantiated type for the search node, falling
// back to the plain type name when no instantiation was recorded.
func (n embeddedMemberSearch) effectiveType() memberVarType {
	if n.typ.name != "" {
		return n.typ
	}
	return memberVarType{name: n.typeName}
}

type selectionState int

const (
	selectionAbsent selectionState = iota
	selectionFound
	selectionAmbiguous
)

type selectionLevel struct {
	selected selectedMember
	state    selectionState
	next     []embeddedMemberSearch
}

func lookupMethod(classes map[string]*Class, ifaces map[string]*Interface, typeName, methodName string, visiting map[string]bool) *Function {
	selected, ok := selectMember(classes, ifaces, typeName, methodName, visiting)
	if !ok {
		return nil
	}
	return selected.method
}

func lookupMethodTyped(classes map[string]*Class, ifaces map[string]*Interface, typ memberVarType, methodName string, visiting map[string]bool) (*Function, memberVarType) {
	selected, ok := selectMemberTyped(classes, ifaces, typ, methodName, visiting)
	if !ok {
		return nil, memberVarType{}
	}
	return selected.method, selected.declaring
}

func selectMember(classes map[string]*Class, ifaces map[string]*Interface, typeName, memberName string, visiting map[string]bool) (selectedMember, bool) {
	return selectMemberTyped(classes, ifaces, memberVarType{name: typeName}, memberName, visiting)
}

func selectMemberTyped(classes map[string]*Class, ifaces map[string]*Interface, typ memberVarType, memberName string, visiting map[string]bool) (selectedMember, bool) {
	typ = canonicalTypedMember(classes, ifaces, typ)
	queue := []embeddedMemberSearch{{typeName: typ.name, typ: typ}}
	for len(queue) > 0 {
		level := selectMemberLevel(classes, ifaces, queue, memberName, visiting)
		switch level.state {
		case selectionAmbiguous:
			return selectedMember{}, false
		case selectionFound:
			return level.selected, true
		}
		queue = level.next
	}
	return selectedMember{}, false
}

func selectMemberLevel(classes map[string]*Class, ifaces map[string]*Interface, queue []embeddedMemberSearch, memberName string, visiting map[string]bool) selectionLevel {
	var level selectionLevel
	nextIndex := map[string]int{}
	for _, node := range queue {
		if visiting[node.typeName] {
			continue
		}
		visiting[node.typeName] = true
		// An unmodelled embedding may declare this member at this depth.
		// No candidate at this level or below can be selected with certainty.
		if classes[node.typeName] == nil && ifaces[node.typeName] == nil {
			return selectionLevel{state: selectionAmbiguous}
		}
		if member, ok := directSelection(classes, ifaces, node, memberName); ok {
			if level.state == selectionFound || node.multiple {
				return selectionLevel{state: selectionAmbiguous}
			}
			level.selected, level.state = member, selectionFound
			continue
		}
		level.next = appendEmbeddedSearches(classes, ifaces, node, level.next, nextIndex)
	}
	return level
}

func directSelection(classes map[string]*Class, ifaces map[string]*Interface, node embeddedMemberSearch, memberName string) (selectedMember, bool) {
	declaring := node.effectiveType()
	subst := bindingForType(classes, ifaces, declaring)
	if class := classes[node.typeName]; class != nil {
		for _, field := range class.Fields {
			if field.Name == memberName {
				return selectedMember{memberType: memberVarTypeOfSubst(field.TypeExpr, subst), path: memberPath(node, memberName), declaring: declaring}, true
			}
		}
		for _, method := range class.Methods {
			if method.Name == memberName {
				return selectedMember{memberType: functionResultTypeWithSubst(method, subst), method: method, path: memberPath(node, memberName), declaring: declaring}, true
			}
		}
	}
	if method := interfaceMethod(ifaces, node.typeName, memberName, map[string]bool{}); method != nil {
		return selectedMember{memberType: functionResultTypeWithSubst(method, subst), method: method, path: memberPath(node, memberName), declaring: declaring}, true
	}
	return selectedMember{}, false
}

func interfaceMethod(ifaces map[string]*Interface, typeName, methodName string, visiting map[string]bool) *Function {
	iface := ifaces[typeName]
	if iface == nil || visiting[typeName] {
		return nil
	}
	visiting[typeName] = true
	for _, method := range iface.Methods {
		if method.Name == methodName {
			return method
		}
	}
	for _, embed := range iface.Embeds {
		if method := interfaceMethod(ifaces, embed, methodName, visiting); method != nil {
			return method
		}
	}
	return nil
}

func memberPath(node embeddedMemberSearch, name string) []MemberKey {
	path := make([]MemberKey, len(node.path), len(node.path)+1)
	copy(path, node.path)
	return append(path, MemberKey{Type: node.typeName, Name: name})
}

func appendEmbeddedSearches(classes map[string]*Class, ifaces map[string]*Interface, node embeddedMemberSearch, next []embeddedMemberSearch, index map[string]int) []embeddedMemberSearch {
	class := classes[node.typeName]
	if class == nil {
		return next
	}
	subst := bindingForType(classes, ifaces, node.effectiveType())
	for _, field := range class.Fields {
		if field.Ident != nil {
			continue
		}
		typ := canonicalTypedMember(classes, ifaces, memberVarTypeOfSubst(field.TypeExpr, subst))
		typeName := typ.name
		if pos, ok := index[typeName]; ok {
			next[pos].multiple = true
			continue
		}
		index[typeName] = len(next)
		next = append(next, embeddedMemberSearch{typeName: typeName, typ: typ, path: memberPath(node, field.Name), multiple: node.multiple})
	}
	return next
}

func canonicalMemberType(classes map[string]*Class, ifaces map[string]*Interface, name string) string {
	if class := classes[name]; class != nil {
		return class.Name
	}
	if iface := ifaces[name]; iface != nil {
		return iface.Name
	}
	return name
}

// canonicalTypedMember rewrites an instantiated type's name to the canonical
// declaring type (resolving generic aliases), keeping its type arguments.
func canonicalTypedMember(classes map[string]*Class, ifaces map[string]*Interface, typ memberVarType) memberVarType {
	if typ.name == "" {
		return typ
	}
	return memberVarType{name: canonicalMemberType(classes, ifaces, typ.name), key: typ.key, kind: typ.kind, args: typ.args}
}

// bindingForType binds the type parameters of the named type in typ to
// typ's type arguments, e.g. Box[Ttt] binds Box's T to Ttt. The names come
// from the type's precomputed TypeParams, so no per-lookup extraction or
// map allocation is needed.
func bindingForType(classes map[string]*Class, ifaces map[string]*Interface, typ memberVarType) typeBinding {
	if class := classes[typ.name]; class != nil {
		return typeBinding{names: class.TypeParams, args: typ.args}
	}
	if iface := ifaces[typ.name]; iface != nil {
		return typeBinding{names: iface.TypeParams, args: typ.args}
	}
	return typeBinding{}
}

func (c *memberTypeResolver) classMemberTypeTyped(typ memberVarType, memberName string) memberVarType {
	memberType, _, ok := c.lookupMemberTyped(typ, memberName, map[string]bool{})
	if !ok {
		return memberVarType{}
	}
	return memberType
}

func (c *memberTypeResolver) promotedMemberPathTyped(typ memberVarType, memberName string) []MemberKey {
	_, path, ok := c.lookupMemberTyped(typ, memberName, map[string]bool{})
	if !ok {
		return nil
	}
	return path
}

func (c *memberTypeResolver) lookupMemberTyped(typ memberVarType, memberName string, visiting map[string]bool) (memberVarType, []MemberKey, bool) {
	selected, ok := selectMemberTyped(c.classes, c.interfaces, typ, memberName, visiting)
	return selected.memberType, selected.path, ok
}

func functionResultTypes(fn *Function) []memberVarType {
	return functionResultTypesWithSubst(fn, typeBinding{})
}

func functionResultTypesWithSubst(fn *Function, subst typeBinding) []memberVarType {
	if fn == nil || len(fn.Results) == 0 {
		return nil
	}
	results := make([]memberVarType, len(fn.Results))
	for i, res := range fn.Results {
		if res.Field != nil {
			results[i] = memberVarTypeOfSubst(res.Field.Type, subst)
		}
	}
	return results
}

func functionResultType(fn *Function) memberVarType {
	return functionResultTypeWithSubst(fn, typeBinding{})
}

func functionResultTypeWithSubst(fn *Function, subst typeBinding) memberVarType {
	results := functionResultTypesWithSubst(fn, subst)
	if len(results) == 1 {
		return results[0]
	}
	return memberVarType{}
}

func unwrapParen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func memberTypeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		return exprString(node)
	case *ast.ParenExpr:
		return memberTypeName(node.X)
	case *ast.StarExpr:
		return memberTypeName(node.X)
	case *ast.IndexExpr:
		return memberTypeName(node.X)
	case *ast.IndexListExpr:
		return memberTypeName(node.X)
	default:
		return containerTypeName(expr)
	}
}

// containerKind is the shape of a container type, which decides how a range
// statement over it binds its iteration variables.
type containerKind int

const (
	containerNone containerKind = iota
	containerSequence
	containerMap
	containerChannel
)

// memberVarType is the member-bearing type of an expression. For a container it
// records the element type in name, plus the shape and, for maps, the key type,
// so callers can tell a map key from a map value. For a generic instantiation
// it records the type arguments in args, in declared type-parameter order, so
// a type parameter in a field or result type can be substituted on access.
type memberVarType struct {
	name string
	key  string
	kind containerKind
	// args encodes generic type arguments as canonical type strings joined
	// by ';', in declared type-parameter order. Strings (not slices) keep
	// member lookup cheap for the explicitness summaries.
	args string
}

func (t memberVarType) empty() bool {
	return t.name == "" && t.key == ""
}

func sameMemberVarType(a, b memberVarType) bool {
	return a.name == b.name && a.key == b.key && a.kind == b.kind && a.args == b.args
}

// canonical returns a type string for t that memberVarTypeOf can read back,
// e.g. "[]Box[Ttt]" or "map[string]Ttt".
func (t memberVarType) canonical() string {
	switch t.kind {
	case containerSequence:
		return "[]" + encodeGenericType(t.name, t.args)
	case containerMap:
		return "map[" + t.key + "]" + encodeGenericType(t.name, t.args)
	case containerChannel:
		return "chan " + encodeGenericType(t.name, t.args)
	default:
		return encodeGenericType(t.name, t.args)
	}
}

func encodeGenericType(name, args string) string {
	if args == "" {
		return name
	}
	return name + "[" + strings.ReplaceAll(args, ";", ",") + "]"
}

// memberVarTypeFromString parses a canonical type string into a member type.
func memberVarTypeFromString(text string) memberVarType {
	if text == "" {
		return memberVarType{}
	}
	expr, err := parser.ParseExpr(text)
	if err != nil {
		return memberVarType{}
	}
	return memberVarTypeOf(expr)
}

// typeBinding binds type parameter names to concrete types, in declared
// order. It is a pair of parallel slices rather than a map so that member
// lookup does not allocate a map on every selection.
type typeBinding struct {
	names []string
	args  string
}

func (b typeBinding) lookup(name string) (memberVarType, bool) {
	if b.args == "" {
		return memberVarType{}, false
	}
	parts := strings.Split(b.args, ";")
	for i, param := range b.names {
		if param == name && i < len(parts) && parts[i] != "" {
			return memberVarTypeFromString(parts[i]), true
		}
	}
	return memberVarType{}, false
}

func memberVarTypeOf(expr ast.Expr) memberVarType {
	return memberVarTypeOfSubst(expr, typeBinding{})
}

// memberVarTypeOfSubst renders a type expression, substituting any type
// parameter bound in subst with its bound type. An empty subst renders the
// declared type as written.
func memberVarTypeOfSubst(expr ast.Expr, subst typeBinding) memberVarType {
	switch node := expr.(type) {
	case *ast.Ident:
		if bound, ok := subst.lookup(node.Name); ok {
			return bound
		}
		return memberVarType{name: node.Name}
	case *ast.SelectorExpr:
		return memberVarType{name: exprString(node)}
	case *ast.ParenExpr:
		return memberVarTypeOfSubst(node.X, subst)
	case *ast.StarExpr:
		return memberVarTypeOfSubst(node.X, subst)
	default:
		return memberVarTypeOfSubstOther(node, subst)
	}
}

func memberVarTypeOfSubstOther(expr ast.Expr, subst typeBinding) memberVarType {
	switch node := expr.(type) {
	case *ast.IndexExpr:
		return genericMemberVarType(memberTypeName(node.X), []ast.Expr{node.Index}, subst)
	case *ast.IndexListExpr:
		return genericMemberVarType(memberTypeName(node.X), node.Indices, subst)
	case *ast.ArrayType:
		return containerMemberVarType(memberVarTypeOfSubst(node.Elt, subst), containerSequence)
	case *ast.ChanType:
		return containerMemberVarType(memberVarTypeOfSubst(node.Value, subst), containerChannel)
	case *ast.MapType:
		return mapMemberVarType(node, subst)
	case *ast.Ellipsis:
		return containerMemberVarType(memberVarTypeOfSubst(node.Elt, subst), containerSequence)
	default:
		return memberVarType{name: containerTypeName(expr)}
	}
}

func mapMemberVarType(node *ast.MapType, subst typeBinding) memberVarType {
	value := memberVarTypeOfSubst(node.Value, subst)
	key := memberVarTypeOfSubst(node.Key, subst)
	return memberVarType{name: value.name, key: key.name, args: value.args, kind: containerMap}
}

func genericMemberVarType(base string, indices []ast.Expr, subst typeBinding) memberVarType {
	args := ""
	for _, index := range indices {
		if args != "" {
			args += ";"
		}
		args += memberVarTypeOfSubst(index, subst).canonical()
	}
	return memberVarType{name: base, args: args}
}

func containerMemberVarType(elt memberVarType, kind containerKind) memberVarType {
	return memberVarType{name: elt.name, args: elt.args, kind: kind}
}

// containerShape reports the container shape of a type expression and, for a
// map, the member-bearing type name of its key.
func containerShape(expr ast.Expr) (containerKind, string) {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return containerShape(node.X)
	case *ast.StarExpr:
		return containerShape(node.X)
	case *ast.ArrayType:
		return containerSequence, ""
	case *ast.ChanType:
		return containerChannel, ""
	case *ast.MapType:
		return containerMap, memberTypeName(node.Key)
	default:
		return containerNone, ""
	}
}

func containerTypeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.ArrayType:
		return memberTypeName(node.Elt)
	case *ast.ChanType:
		return memberTypeName(node.Value)
	case *ast.MapType:
		return memberTypeName(node.Value)
	default:
		return ""
	}
}
