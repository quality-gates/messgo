package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/quality-gates/messgo/internal/util"
)

// Parse reads and parses a Go source file into a File model.
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
	// aliases maps type-alias names in this file to the type they denote, so
	// a method on an alias receiver attaches to the aliased type.
	aliases map[string]string
}

func (b *fileBuilder) build() {
	b.aliases = util.TypeAliasNames([]*ast.File{b.f.Syntax})
	b.collectTypes()
	b.collectFuncs()
	b.f.Constants = collectConstants(b.f)
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
	switch t := ast.Unparen(ts.Type).(type) { // type S (struct{...})
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
