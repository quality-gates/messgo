package model

import (
	"go/ast"
	"slices"
)

// TypeDependencies returns the sorted, distinct non-builtin type names used
// by fields and method signatures. References to the class itself are excluded.
// Names follow TypeReferences; method bodies are not inspected.
func TypeDependencies(c *Class) []string {
	names := map[string]bool{}
	collect := func(expr ast.Expr) {
		for _, name := range TypeReferences(expr) {
			if name != c.Name && !builtinTypeNames[name] {
				names[name] = true
			}
		}
	}
	for _, field := range c.Fields {
		collect(field.TypeExpr)
	}
	for _, method := range c.Methods {
		for _, param := range method.Params {
			collect(param.TypeExpr)
		}
		for _, result := range method.Results {
			collect(result.TypeExpr)
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

var builtinTypeNames = map[string]bool{
	"bool": true, "string": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "uint": true, "uint8": true, "uint16": true,
	"uint32": true, "uint64": true, "uintptr": true, "byte": true, "rune": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
	"error": true, "any": true, "comparable": true,
}

// TypeReferences returns named type references in syntax order, including
// duplicates and builtins. Selectors use the type name without the package
// qualifier, as in the CBO metric. Field names and array lengths are excluded.
// This query uses syntax only; it does not resolve aliases or type parameters.
func TypeReferences(expr ast.Expr) []string {
	if expr == nil {
		return nil
	}
	var names []string
	ast.Inspect(expr, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			names = append(names, n.Name)
			return false
		case *ast.SelectorExpr:
			names = append(names, n.Sel.Name)
			return false
		case *ast.Field:
			names = append(names, TypeReferences(n.Type)...)
			return false
		case *ast.ArrayType:
			names = append(names, TypeReferences(n.Elt)...)
			return false
		}
		return true
	})
	return names
}
