package model

import (
	"go/ast"
	"go/token"
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
}

type memberScopeCollector struct {
	resolver *memberTypeResolver
}

type memberUseRecorder struct {
	resolver *memberTypeResolver
}

func collectSelectedMemberUses(f *File) (map[string]bool, map[MemberKey]bool) {
	names := map[string]bool{}
	uses := map[MemberKey]bool{}
	collector := newMemberSelectionCollector(f)
	packageTypes := collector.packageTypes(f)

	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			collector.collectFunc(node, packageTypes, names, uses)
			return false
		case *ast.SelectorExpr:
			collector.recorder.recordSelector(node, packageTypes, names, uses)
		case *ast.CompositeLit:
			collector.recorder.recordComposite(node, names, uses)
		}
		return true
	})
	return names, uses
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
	collector := &memberSelectionCollector{
		types: memberTypeResolver{
			classes:    classByName,
			interfaces: ifaceByName,
			functions:  functions,
		},
	}
	collector.scope.resolver = &collector.types
	collector.recorder.resolver = &collector.types
	return collector
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

func (c *memberSelectionCollector) collectFunc(decl *ast.FuncDecl, packageTypes map[string]memberVarType, names map[string]bool, uses map[MemberKey]bool) {
	types := cloneMemberTypes(packageTypes)
	c.scope.addFuncParameters(decl, types)
	if decl.Body == nil {
		return
	}
	c.scope.collectLocalTypes(decl.Body, types)
	c.collectBody(decl.Body, types, names, uses)
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
		bindRangeVar(stmt.Value, memberVarType{name: subject.name}, types)
	case containerSequence:
		bindRangeVar(stmt.Value, memberVarType{name: subject.name}, types)
	case containerChannel:
		bindRangeVar(stmt.Key, memberVarType{name: subject.name}, types)
	default:
		bindRangeVar(rangeElementVar(stmt), memberVarType{name: subject.name}, types)
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

func (c *memberSelectionCollector) collectBody(body ast.Node, types map[string]memberVarType, names map[string]bool, uses map[MemberKey]bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			nestedTypes := cloneMemberTypes(types)
			c.scope.addFuncLiteralParameters(node, nestedTypes)
			c.scope.collectLocalTypes(node.Body, nestedTypes)
			c.collectBody(node.Body, nestedTypes, names, uses)
			return false
		case *ast.TypeSwitchStmt:
			c.collectTypeSwitch(node, types, names, uses)
			return false
		case *ast.SelectorExpr:
			c.recorder.recordSelector(node, types, names, uses)
		case *ast.CompositeLit:
			c.recorder.recordComposite(node, names, uses)
		}
		return true
	})
}

// collectTypeSwitch binds the switch variable of `switch v := x.(type)` to the
// case type inside each single-type clause; in any other clause v keeps the
// type of x, which is unknown here, so it is unbound.
func (c *memberSelectionCollector) collectTypeSwitch(stmt *ast.TypeSwitchStmt, types map[string]memberVarType, names map[string]bool, uses map[MemberKey]bool) {
	if stmt.Init != nil {
		c.collectBody(stmt.Init, types, names, uses)
	}
	c.collectBody(stmt.Assign, types, names, uses)
	bound := typeSwitchVar(stmt)
	for _, clause := range stmt.Body.List {
		cc := clause.(*ast.CaseClause)
		c.collectBody(cc, clauseTypes(types, bound, cc), names, uses)
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

func (c *memberUseRecorder) recordSelector(sel *ast.SelectorExpr, types map[string]memberVarType, names map[string]bool, uses map[MemberKey]bool) {
	name := sel.Sel.Name
	names[name] = true
	if typeName := c.resolver.expressionType(sel.X, types); typeName != "" {
		uses[MemberKey{Type: typeName, Name: name}] = true
		for _, key := range c.resolver.promotedMemberPath(typeName, name) {
			uses[key] = true
		}
	}
}

func (c *memberUseRecorder) recordComposite(lit *ast.CompositeLit, names map[string]bool, uses map[MemberKey]bool) {
	if isCollectionLiteral(lit) {
		return
	}
	typeName := memberTypeName(lit.Type)
	if recordKeyedMembers(lit, typeName, names, uses) {
		return
	}
	if len(lit.Elts) == 0 || typeName == "" {
		return
	}
	c.recordUnkeyedMembers(typeName, lit, names, uses)
}

func isCollectionLiteral(lit *ast.CompositeLit) bool {
	switch lit.Type.(type) {
	case *ast.MapType, *ast.ArrayType:
		return true
	default:
		return false
	}
}

func recordKeyedMembers(lit *ast.CompositeLit, typeName string, names map[string]bool, uses map[MemberKey]bool) bool {
	keyed := false
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyed = true
		recordKeyedMember(kv, typeName, names, uses)
	}
	return keyed
}

func recordKeyedMember(kv *ast.KeyValueExpr, typeName string, names map[string]bool, uses map[MemberKey]bool) {
	id, ok := kv.Key.(*ast.Ident)
	if !ok {
		return
	}
	names[id.Name] = true
	if typeName != "" {
		uses[MemberKey{Type: typeName, Name: id.Name}] = true
	}
}

func (c *memberUseRecorder) recordUnkeyedMembers(typeName string, lit *ast.CompositeLit, names map[string]bool, uses map[MemberKey]bool) {
	class := c.resolver.classes[typeName]
	if class == nil {
		return
	}
	for _, field := range class.Fields {
		names[field.Name] = true
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
		return types[node.Name]
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
	return memberVarType{name: container.name}
}

func (c *memberTypeResolver) selectorType(sel *ast.SelectorExpr, types map[string]memberVarType) memberVarType {
	baseType := c.expressionType(sel.X, types)
	if baseType == "" {
		return memberVarType{}
	}
	return c.classMemberType(baseType, sel.Sel.Name)
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
	fun := unwrapParen(call.Fun)
	if id, ok := fun.(*ast.Ident); ok {
		return resolveIdentCall(c.classes, c.functions, id, call.Args)
	}
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		return resolveSelectorCall(c, sel, types)
	}
	return nil
}

func resolveIdentCall(classes map[string]*Class, functions map[string]*Function, id *ast.Ident, args []ast.Expr) []memberVarType {
	if id.Name == "new" && len(args) == 1 {
		if argType := memberVarTypeOf(args[0]); !argType.empty() {
			return []memberVarType{argType}
		}
		return nil
	}
	if _, ok := classes[id.Name]; ok {
		return []memberVarType{{name: id.Name}}
	}
	if fn := functions[id.Name]; fn != nil {
		return functionResultTypes(fn)
	}
	return nil
}

func resolveSelectorCall(r *memberTypeResolver, sel *ast.SelectorExpr, types map[string]memberVarType) []memberVarType {
	baseType := r.expressionType(sel.X, types)
	if baseType == "" {
		return nil
	}
	method := lookupMethod(r.classes, r.interfaces, baseType, sel.Sel.Name, map[string]bool{})
	if method == nil {
		return nil
	}
	return functionResultTypes(method)
}

func lookupMethod(classes map[string]*Class, ifaces map[string]*Interface, typeName, methodName string, visiting map[string]bool) *Function {
	if visiting[typeName] {
		return nil
	}
	visiting[typeName] = true
	defer delete(visiting, typeName)

	if class := classes[typeName]; class != nil {
		if method := classMethod(classes, ifaces, class, methodName, visiting); method != nil {
			return method
		}
	}
	if iface := ifaces[typeName]; iface != nil {
		if method := interfaceMethod(classes, ifaces, iface, methodName, visiting); method != nil {
			return method
		}
	}
	return nil
}

func classMethod(classes map[string]*Class, ifaces map[string]*Interface, class *Class, methodName string, visiting map[string]bool) *Function {
	for _, method := range class.Methods {
		if method.Name == methodName {
			return method
		}
	}
	for _, field := range class.Fields {
		if field.Ident != nil {
			continue
		}
		embeddedType := memberTypeName(field.TypeExpr)
		if method := lookupMethod(classes, ifaces, embeddedType, methodName, visiting); method != nil {
			return method
		}
	}
	return nil
}

func interfaceMethod(classes map[string]*Class, ifaces map[string]*Interface, iface *Interface, methodName string, visiting map[string]bool) *Function {
	for _, method := range iface.Methods {
		if method.Name == methodName {
			return method
		}
	}
	for _, embed := range iface.Embeds {
		if method := lookupMethod(classes, ifaces, embed, methodName, visiting); method != nil {
			return method
		}
	}
	return nil
}

func (c *memberTypeResolver) classMemberType(typeName, memberName string) memberVarType {
	memberType, _, ok := c.lookupMember(typeName, memberName, map[string]bool{})
	if !ok {
		return memberVarType{}
	}
	return memberType
}

func (c *memberTypeResolver) promotedMemberPath(typeName, memberName string) []MemberKey {
	_, path, ok := c.lookupMember(typeName, memberName, map[string]bool{})
	if !ok {
		return nil
	}
	return path
}

func (c *memberTypeResolver) lookupMember(typeName, memberName string, visiting map[string]bool) (memberVarType, []MemberKey, bool) {
	class := c.classes[typeName]
	if class == nil || visiting[typeName] {
		return memberVarType{}, nil, false
	}
	visiting[typeName] = true
	defer delete(visiting, typeName)
	if memberType, ok := directMemberType(class, memberName); ok {
		return memberType, []MemberKey{{Type: typeName, Name: memberName}}, true
	}
	return c.lookupEmbeddedMember(class, typeName, memberName, visiting)
}

func directMemberType(class *Class, memberName string) (memberVarType, bool) {
	for _, field := range class.Fields {
		if field.Name == memberName {
			return memberVarTypeOf(field.TypeExpr), true
		}
	}
	for _, method := range class.Methods {
		if method.Name == memberName {
			return functionResultType(method), true
		}
	}
	return memberVarType{}, false
}

func (c *memberTypeResolver) lookupEmbeddedMember(class *Class, typeName, memberName string, visiting map[string]bool) (memberVarType, []MemberKey, bool) {
	for _, field := range class.Fields {
		if field.Ident != nil {
			continue
		}
		embeddedType := memberTypeName(field.TypeExpr)
		memberType, path, ok := c.lookupMember(embeddedType, memberName, visiting)
		if !ok {
			continue
		}
		path = append([]MemberKey{{Type: typeName, Name: field.Name}}, path...)
		return memberType, path, true
	}
	return memberVarType{}, nil, false
}

func functionResultTypes(fn *Function) []memberVarType {
	if fn == nil || len(fn.Results) == 0 {
		return nil
	}
	results := make([]memberVarType, len(fn.Results))
	for i, res := range fn.Results {
		if res.Field != nil {
			results[i] = memberVarTypeOf(res.Field.Type)
		}
	}
	return results
}

func functionResultType(fn *Function) memberVarType {
	results := functionResultTypes(fn)
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
// so callers can tell a map key from a map value.
type memberVarType struct {
	name string
	key  string
	kind containerKind
}

func (t memberVarType) empty() bool {
	return t.name == "" && t.key == ""
}

func memberVarTypeOf(expr ast.Expr) memberVarType {
	kind, key := containerShape(expr)
	return memberVarType{name: memberTypeName(expr), key: key, kind: kind}
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
