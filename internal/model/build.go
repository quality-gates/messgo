package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// Parse reads and parses a Go source file into a File model.
func Parse(path string) (*File, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(path, src)
}

// ParseSource parses Go source bytes into a File model.
func ParseSource(path string, src []byte) (*File, error) {
	fset := token.NewFileSet()
	syntax, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return nil, err
	}
	f := &File{
		Path:    path,
		Fset:    fset,
		Syntax:  syntax,
		Src:     src,
		Package: syntax.Name.Name,
	}
	f.build()
	return f, nil
}

func (f *File) line(p token.Pos) int { return f.Fset.Position(p).Line }

func (f *File) build() {
	classes := map[string]*Class{}
	ifaces := map[string]*Interface{}

	f.collectTypes(classes, ifaces)
	f.collectFuncs(classes)
}

func (f *File) collectTypes(classes map[string]*Class, ifaces map[string]*Interface) {
	for _, decl := range f.Syntax.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			f.collectTypeSpec(ts, gen.Doc, classes, ifaces)
		}
	}
}

func (f *File) collectTypeSpec(ts *ast.TypeSpec, docGroup *ast.CommentGroup, classes map[string]*Class, ifaces map[string]*Interface) {
	doc := docText(ts.Doc, docGroup)
	switch t := ts.Type.(type) {
	case *ast.StructType:
		c := &Class{
			Name:       ts.Name.Name,
			Line:       f.line(ts.Pos()),
			EndLine:    f.line(ts.End()),
			Exported:   ts.Name.IsExported(),
			File:       f,
			Spec:       ts,
			Struct:     t,
			DocComment: doc,
		}
		f.collectFields(c, t)
		classes[c.Name] = c
		f.Classes = append(f.Classes, c)
	case *ast.InterfaceType:
		i := &Interface{
			Name:       ts.Name.Name,
			Line:       f.line(ts.Pos()),
			EndLine:    f.line(ts.End()),
			Exported:   ts.Name.IsExported(),
			File:       f,
			Spec:       ts,
			Iface:      t,
			DocComment: doc,
		}
		f.collectInterfaceMethods(i, t)
		ifaces[i.Name] = i
		f.Interfaces = append(f.Interfaces, i)
	}
}

func (f *File) collectFuncs(classes map[string]*Class) {
	for _, decl := range f.Syntax.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fn := f.buildFunc(fd)
		f.AllFuncs = append(f.AllFuncs, fn)
		if !fn.IsMethod() {
			f.Functions = append(f.Functions, fn)
			continue
		}
		if c := classes[fn.Receiver]; c != nil {
			c.Methods = append(c.Methods, fn)
			fn.Class = c
		}
	}
}

func (f *File) collectFields(c *Class, st *ast.StructType) {
	if st.Fields == nil {
		return
	}
	for _, fld := range st.Fields.List {
		typeStr := exprString(fld.Type)
		if len(fld.Names) == 0 {
			// Embedded field: the type name is the field name.
			name := embeddedName(fld.Type)
			c.Embeds = append(c.Embeds, name)
			c.Fields = append(c.Fields, &Field{
				Name:     name,
				Type:     typeStr,
				Line:     f.line(fld.Pos()),
				Exported: ast.IsExported(name),
			})
			continue
		}
		for _, n := range fld.Names {
			c.Fields = append(c.Fields, &Field{
				Name:     n.Name,
				Type:     typeStr,
				Line:     f.line(n.Pos()),
				Exported: n.IsExported(),
				Ident:    n,
			})
		}
	}
}

func (f *File) collectInterfaceMethods(i *Interface, it *ast.InterfaceType) {
	if it.Methods == nil {
		return
	}
	for _, m := range it.Methods.List {
		if len(m.Names) == 0 {
			i.Embeds = append(i.Embeds, exprString(m.Type))
			continue
		}
		ft, _ := m.Type.(*ast.FuncType)
		for _, n := range m.Names {
			fn := &Function{
				Name:     n.Name,
				Line:     f.line(n.Pos()),
				EndLine:  f.line(m.End()),
				Exported: n.IsExported(),
				File:     f,
			}
			if ft != nil {
				fn.Params = f.params(ft.Params)
				fn.Results = f.params(ft.Results)
			}
			i.Methods = append(i.Methods, fn)
		}
	}
}

func (f *File) buildFunc(fd *ast.FuncDecl) *Function {
	fn := &Function{
		Name:       fd.Name.Name,
		Line:       f.line(fd.Pos()),
		EndLine:    f.line(fd.End()),
		Exported:   fd.Name.IsExported(),
		Decl:       fd,
		Body:       fd.Body,
		File:       f,
		DocComment: docText(fd.Doc),
	}
	if fd.Type != nil {
		fn.Params = f.params(fd.Type.Params)
		fn.Results = f.params(fd.Type.Results)
	}
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		r := fd.Recv.List[0]
		fn.Receiver = receiverTypeName(r.Type)
		if len(r.Names) > 0 {
			fn.RecvName = r.Names[0].Name
		}
	}
	return fn
}

func (f *File) params(fl *ast.FieldList) []*Parameter {
	if fl == nil {
		return nil
	}
	var out []*Parameter
	for _, fld := range fl.List {
		typeStr := exprString(fld.Type)
		if len(fld.Names) == 0 {
			out = append(out, &Parameter{Type: typeStr, Line: f.line(fld.Pos()), Field: fld})
			continue
		}
		for _, n := range fld.Names {
			out = append(out, &Parameter{
				Name:  n.Name,
				Type:  typeStr,
				Line:  f.line(n.Pos()),
				Field: fld,
				Ident: n,
			})
		}
	}
	return out
}

func receiverTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver Foo[T]
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	}
	return ""
}

func embeddedName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return embeddedName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(t.X)
	}
	return exprString(e)
}

func docText(groups ...*ast.CommentGroup) string {
	for _, g := range groups {
		if g != nil {
			return g.Text()
		}
	}
	return ""
}

// exprString renders a type expression to a compact string. It avoids
// go/printer to keep things dependency-free and stable.
func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	case *ast.Ellipsis:
		return "..." + exprString(t.Elt)
	case *ast.ChanType:
		return "chan " + exprString(t.Value)
	case *ast.ParenExpr:
		return exprString(t.X)
	default:
		return exprStringOther(t)
	}
}

func exprStringOther(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	case *ast.IndexExpr:
		return exprString(t.X) + "[" + exprString(t.Index) + "]"
	case *ast.IndexListExpr:
		parts := make([]string, len(t.Indices))
		for i, ix := range t.Indices {
			parts[i] = exprString(ix)
		}
		return exprString(t.X) + "[" + strings.Join(parts, ",") + "]"
	}
	return ""
}
