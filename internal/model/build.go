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
	b := &fileBuilder{
		f:       f,
		classes: map[string]*Class{},
		ifaces:  map[string]*Interface{},
	}
	b.build()
	return f, nil
}

type fileBuilder struct {
	f       *File
	classes map[string]*Class
	ifaces  map[string]*Interface
}

func (b *fileBuilder) line(p token.Pos) int { return b.f.Fset.Position(p).Line }

func (b *fileBuilder) build() {
	b.collectTypes()
	b.collectFuncs()
}

func (b *fileBuilder) collectTypes() {
	for _, decl := range b.f.Syntax.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			b.collectTypeSpec(ts, gen.Doc)
		}
	}
}

func (b *fileBuilder) collectTypeSpec(ts *ast.TypeSpec, docGroup *ast.CommentGroup) {
	doc := docText(ts.Doc, docGroup)
	switch t := ts.Type.(type) {
	case *ast.StructType:
		c := &Class{
			Name:       ts.Name.Name,
			Line:       b.line(ts.Pos()),
			EndLine:    b.line(ts.End()),
			Exported:   ts.Name.IsExported(),
			File:       b.f,
			Spec:       ts,
			Struct:     t,
			DocComment: doc,
		}
		b.collectFields(c, t)
		b.classes[c.Name] = c
		b.f.Classes = append(b.f.Classes, c)
	case *ast.InterfaceType:
		i := &Interface{
			Name:       ts.Name.Name,
			Line:       b.line(ts.Pos()),
			EndLine:    b.line(ts.End()),
			Exported:   ts.Name.IsExported(),
			File:       b.f,
			Spec:       ts,
			Iface:      t,
			DocComment: doc,
		}
		b.collectInterfaceMethods(i, t)
		b.ifaces[i.Name] = i
		b.f.Interfaces = append(b.f.Interfaces, i)
	}
}

func (b *fileBuilder) collectFuncs() {
	for _, decl := range b.f.Syntax.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fn := b.buildFunc(fd)
		b.f.AllFuncs = append(b.f.AllFuncs, fn)
		if !fn.IsMethod() {
			b.f.Functions = append(b.f.Functions, fn)
			continue
		}
		if c := b.classes[fn.Receiver]; c != nil {
			c.Methods = append(c.Methods, fn)
			fn.Class = c
		}
	}
}

func (b *fileBuilder) collectFields(c *Class, st *ast.StructType) {
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
				Line:     b.line(fld.Pos()),
				Exported: ast.IsExported(name),
			})
			continue
		}
		for _, n := range fld.Names {
			c.Fields = append(c.Fields, &Field{
				Name:     n.Name,
				Type:     typeStr,
				Line:     b.line(n.Pos()),
				Exported: n.IsExported(),
				Ident:    n,
			})
		}
	}
}

func (b *fileBuilder) collectInterfaceMethods(i *Interface, it *ast.InterfaceType) {
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
				Receiver: i.Name,
				Line:     b.line(n.Pos()),
				EndLine:  b.line(m.End()),
				Exported: n.IsExported(),
				File:     b.f,
			}
			if ft != nil {
				fn.Params = b.params(ft.Params)
				fn.Results = b.params(ft.Results)
			}
			i.Methods = append(i.Methods, fn)
		}
	}
}

func (b *fileBuilder) buildFunc(fd *ast.FuncDecl) *Function {
	fn := &Function{
		Name:       fd.Name.Name,
		Line:       b.line(fd.Pos()),
		EndLine:    b.line(fd.End()),
		Exported:   fd.Name.IsExported(),
		Decl:       fd,
		Body:       fd.Body,
		File:       b.f,
		DocComment: docText(fd.Doc),
	}
	if fd.Type != nil {
		fn.Params = b.params(fd.Type.Params)
		fn.Results = b.params(fd.Type.Results)
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

func (b *fileBuilder) params(fl *ast.FieldList) []*Parameter {
	if fl == nil {
		return nil
	}
	var out []*Parameter
	for _, fld := range fl.List {
		typeStr := exprString(fld.Type)
		if len(fld.Names) == 0 {
			out = append(out, &Parameter{Type: typeStr, Line: b.line(fld.Pos()), Field: fld})
			continue
		}
		for _, n := range fld.Names {
			out = append(out, &Parameter{
				Name:  n.Name,
				Type:  typeStr,
				Line:  b.line(n.Pos()),
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
	case *ast.IndexListExpr:
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
		return arrayTypeString(t)
	case *ast.ChanType:
		return chanTypeString(t)
	case *ast.ParenExpr:
		return exprString(t.X)
	default:
		return exprStringOther(t)
	}
}

func arrayTypeString(t *ast.ArrayType) string {
	return "[" + exprString(t.Len) + "]" + exprString(t.Elt)
}

func chanTypeString(t *ast.ChanType) string {
	switch t.Dir {
	case ast.SEND:
		return "chan<- " + exprString(t.Value)
	case ast.RECV:
		return "<-chan " + exprString(t.Value)
	default:
		return "chan " + exprString(t.Value)
	}
}

func exprStringOther(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.BasicLit:
		return t.Value
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	case *ast.Ellipsis:
		return "..." + exprString(t.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	case *ast.IndexExpr, *ast.IndexListExpr:
		return indexTypeString(t)
	}
	return ""
}

func indexTypeString(e ast.Expr) string {
	switch t := e.(type) {
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
