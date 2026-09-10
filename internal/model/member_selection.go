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
	classes   map[string]*Class
	functions map[string]*Function
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
	functions := make(map[string]*Function)
	for _, fn := range f.AllFuncs {
		if !fn.IsMethod() && functions[fn.Name] == nil {
			functions[fn.Name] = fn
		}
	}
	collector := &memberSelectionCollector{
		types: memberTypeResolver{classes: classByName, functions: functions},
	}
	collector.scope.resolver = &collector.types
	collector.recorder.resolver = &collector.types
	return collector
}

func (c *memberSelectionCollector) packageTypes(f *File) map[string]string {
	types := map[string]string{}
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

func (c *memberSelectionCollector) collectFunc(decl *ast.FuncDecl, packageTypes map[string]string, names map[string]bool, uses map[MemberKey]bool) {
	types := cloneMemberTypes(packageTypes)
	c.scope.addFuncParameters(decl, types)
	if decl.Body == nil {
		return
	}
	c.scope.collectLocalTypes(decl.Body, types)
	c.collectBody(decl.Body, types, names, uses)
}

func cloneMemberTypes(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, typeName := range source {
		clone[name] = typeName
	}
	return clone
}

func (c *memberScopeCollector) addFuncParameters(decl *ast.FuncDecl, types map[string]string) {
	if decl.Recv != nil {
		addNamedFieldTypes(decl.Recv.List, types)
	}
	if decl.Type == nil || decl.Type.Params == nil {
		return
	}
	addNamedFieldTypes(decl.Type.Params.List, types)
}

func addNamedFieldTypes(fields []*ast.Field, types map[string]string) {
	for _, field := range fields {
		typeName := memberTypeName(field.Type)
		if typeName == "" {
			continue
		}
		for _, name := range field.Names {
			types[name.Name] = typeName
		}
	}
}

func (c *memberScopeCollector) collectLocalTypes(body *ast.BlockStmt, types map[string]string) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeclStmt:
			c.addDeclaration(node, types)
		case *ast.AssignStmt:
			c.addAssignment(node, types)
		}
		return true
	})
}

func (c *memberScopeCollector) addDeclaration(stmt *ast.DeclStmt, types map[string]string) {
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

func (c *memberScopeCollector) addValueSpec(spec *ast.ValueSpec, types map[string]string) {
	declaredType := memberTypeName(spec.Type)
	for index, name := range spec.Names {
		typeName := declaredType
		if typeName == "" && index < len(spec.Values) {
			typeName = c.resolver.expressionType(spec.Values[index], types)
		}
		if typeName != "" && name.Name != "_" {
			types[name.Name] = typeName
		}
	}
}

func (c *memberScopeCollector) addAssignment(stmt *ast.AssignStmt, types map[string]string) {
	for index, lhs := range stmt.Lhs {
		name, ok := lhs.(*ast.Ident)
		if !ok || name.Name == "_" || index >= len(stmt.Rhs) {
			continue
		}
		if typeName := c.resolver.expressionType(stmt.Rhs[index], types); typeName != "" {
			types[name.Name] = typeName
		}
	}
}

func (c *memberSelectionCollector) collectBody(body *ast.BlockStmt, types map[string]string, names map[string]bool, uses map[MemberKey]bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			nestedTypes := cloneMemberTypes(types)
			c.scope.addFuncLiteralParameters(node, nestedTypes)
			c.scope.collectLocalTypes(node.Body, nestedTypes)
			c.collectBody(node.Body, nestedTypes, names, uses)
			return false
		case *ast.SelectorExpr:
			c.recorder.recordSelector(node, types, names, uses)
		case *ast.CompositeLit:
			c.recorder.recordComposite(node, names, uses)
		}
		return true
	})
}

func (c *memberScopeCollector) addFuncLiteralParameters(lit *ast.FuncLit, types map[string]string) {
	if lit.Type == nil || lit.Type.Params == nil {
		return
	}
	addNamedFieldTypes(lit.Type.Params.List, types)
}

func (c *memberUseRecorder) recordSelector(sel *ast.SelectorExpr, types map[string]string, names map[string]bool, uses map[MemberKey]bool) {
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

func (c *memberTypeResolver) expressionType(expr ast.Expr, types map[string]string) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return types[node.Name]
	case *ast.SelectorExpr:
		return c.selectorType(node, types)
	case *ast.CompositeLit:
		return memberTypeName(node.Type)
	case *ast.TypeAssertExpr:
		return memberTypeName(node.Type)
	case *ast.CallExpr:
		return c.callType(node, types)
	default:
		return c.wrappedExpressionType(expr, types)
	}
}

func (c *memberTypeResolver) wrappedExpressionType(expr ast.Expr, types map[string]string) string {
	switch node := expr.(type) {
	case *ast.ParenExpr:
		return c.expressionType(node.X, types)
	case *ast.StarExpr:
		return c.expressionType(node.X, types)
	case *ast.UnaryExpr:
		return c.expressionType(node.X, types)
	case *ast.IndexExpr:
		return c.expressionType(node.X, types)
	case *ast.IndexListExpr:
		return c.expressionType(node.X, types)
	default:
		return ""
	}
}

func (c *memberTypeResolver) selectorType(sel *ast.SelectorExpr, types map[string]string) string {
	baseType := c.expressionType(sel.X, types)
	if baseType == "" {
		return ""
	}
	return c.classMemberType(baseType, sel.Sel.Name)
}

func (c *memberTypeResolver) callType(call *ast.CallExpr, types map[string]string) string {
	if id, ok := call.Fun.(*ast.Ident); ok {
		if id.Name == "new" && len(call.Args) == 1 {
			return memberTypeName(call.Args[0])
		}
		if _, ok := c.classes[id.Name]; ok {
			return id.Name
		}
		if fn := c.functions[id.Name]; fn != nil {
			return functionResultType(fn)
		}
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return c.selectorType(sel, types)
	}
	return ""
}

func (c *memberTypeResolver) classMemberType(typeName, memberName string) string {
	memberType, _, ok := c.lookupMember(typeName, memberName, map[string]bool{})
	if !ok {
		return ""
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

func (c *memberTypeResolver) lookupMember(typeName, memberName string, visiting map[string]bool) (string, []MemberKey, bool) {
	class := c.classes[typeName]
	if class == nil || visiting[typeName] {
		return "", nil, false
	}
	visiting[typeName] = true
	defer delete(visiting, typeName)
	if memberType, ok := directMemberType(class, memberName); ok {
		return memberType, nil, true
	}
	return c.lookupEmbeddedMember(class, typeName, memberName, visiting)
}

func directMemberType(class *Class, memberName string) (string, bool) {
	for _, field := range class.Fields {
		if field.Name == memberName {
			return memberTypeName(field.TypeExpr), true
		}
	}
	for _, method := range class.Methods {
		if method.Name == memberName {
			return functionResultType(method), true
		}
	}
	return "", false
}

func (c *memberTypeResolver) lookupEmbeddedMember(class *Class, typeName, memberName string, visiting map[string]bool) (string, []MemberKey, bool) {
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
	return "", nil, false
}

func functionResultType(fn *Function) string {
	if len(fn.Results) != 1 {
		return ""
	}
	if fn.Results[0].Field == nil {
		return ""
	}
	return memberTypeName(fn.Results[0].Field.Type)
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
		return ""
	}
}
