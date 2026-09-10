// Package model wraps the Go standard-library AST into phpmd-style code
// artifacts (Class, Interface, Method, Function, Field, Parameter). Rules are
// written against these artifacts, mirroring how PHPMD rules operate on
// pdepend's ASTClass / ASTMethod / ASTFunction nodes.
package model

import (
	"go/ast"
	"go/token"
	"maps"
	"sync"

	"github.com/quality-gates/messgo/internal/metrics"
)

// NodeType identifies the kind of artifact, used for rule message rendering
// (the PHPMD "{0}" placeholder is typically the artifact type: "class",
// "method", "function", "interface").
type NodeType string

const (
	TypeClass     NodeType = "class"
	TypeInterface NodeType = "interface"
	TypeTrait     NodeType = "trait"
	TypeMethod    NodeType = "method"
	TypeFunction  NodeType = "function"
)

// File is a parsed Go source file plus all artifacts discovered within it.
type File struct {
	Path       string
	Fset       *token.FileSet
	Syntax     *ast.File
	Src        []byte
	Package    string
	Classes    []*Class
	Interfaces []*Interface
	Functions  []*Function
	// AllFuncs includes both free functions and methods, in source order.
	AllFuncs []*Function
	// MutatedGlobals holds the package-level variable names that are mutated
	// anywhere in this file's package. It is populated by the runner once all
	// of a package's files are parsed, enabling cross-file analysis. It is nil
	// when a file is analyzed in isolation (rules then fall back to single-file
	// analysis).
	MutatedGlobals map[string]bool
	// PackageClasses holds all structs from every file in this file's package.
	// It is populated by the runner after parsing, enabling cross-file embedding
	// analysis. When nil (file analyzed in isolation), rules fall back to
	// this file's own Classes.
	PackageClasses []*Class
	// PackageInterfaces holds all interface types from every file in this
	// file's package. It is populated by the runner after parsing, enabling
	// cross-file interface-satisfaction analysis. When nil (file analyzed in
	// isolation), rules fall back to this file's own Interfaces.
	PackageInterfaces []*Interface
	// PackageMembers holds all selected member names across every file in
	// this file's package. It is populated by the runner after parsing,
	// enabling cross-file unused member analysis. When nil, rules fall back
	// to this file's own selected members.
	PackageMembers map[string]bool

	analysis fileAnalysisCache
}

type fileAnalysisCache struct {
	selectedMembersOnce sync.Once
	selectedMembers     map[string]bool
	ifaceMethodsOnce    sync.Once
	ifaceMethods        map[string]bool
	ifaceMethodSigs     map[string][]*Function
	effectiveLOCOnce    sync.Once
	effectiveLOC        *metrics.EffectiveLOCIndex
}

// SelectedMemberNames returns a snapshot of field or method names selected or
// used in struct literals anywhere in this file.
func (f *File) SelectedMemberNames() map[string]bool {
	f.collectSelectedMemberNames()
	return maps.Clone(f.analysis.selectedMembers)
}

// PackageMemberNames returns a snapshot of field or method names selected
// anywhere across this file's package (or this file if analyzed in isolation).
func (f *File) PackageMemberNames() map[string]bool {
	if f.PackageMembers != nil {
		return maps.Clone(f.PackageMembers)
	}
	return f.SelectedMemberNames()
}

// MemberSelected reports whether name is selected or used in a struct literal
// anywhere in this file (or in this file's package if PackageMembers is
// populated). The file-wide AST scan runs once.
func (f *File) MemberSelected(name string) bool {
	if f.PackageMembers != nil {
		return f.PackageMembers[name]
	}
	f.collectSelectedMemberNames()
	return f.analysis.selectedMembers[name]
}

func (f *File) collectSelectedMemberNames() {
	f.analysis.selectedMembersOnce.Do(func() {
		f.analysis.selectedMembers = map[string]bool{}
		classes := f.Classes
		if f.PackageClasses != nil {
			classes = f.PackageClasses
		}
		ast.Inspect(f.Syntax, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.SelectorExpr:
				f.analysis.selectedMembers[e.Sel.Name] = true
			case *ast.CompositeLit:
				collectCompositeMemberNames(e, f.analysis.selectedMembers, classes)
			}
			return true
		})
	})
}

// InterfaceMethodNames returns the set of method names declared by any
// interface in this file's package (or this file if analyzed in isolation),
// including methods inherited through embedded interfaces resolved within the
// same package. The set is computed at most once per file.
func (f *File) InterfaceMethodNames() map[string]bool {
	f.collectInterfaceMethods()
	return f.analysis.ifaceMethods
}

// collectInterfaceMethods builds, at most once per file, the set of method
// names declared by any interface in this file's package, plus the declared
// methods grouped by name for signature matching.
func (f *File) collectInterfaceMethods() {
	f.analysis.ifaceMethodsOnce.Do(func() {
		ifaces := f.Interfaces
		if f.PackageInterfaces != nil {
			ifaces = f.PackageInterfaces
		}
		byName := make(map[string]*Interface, len(ifaces))
		for _, i := range ifaces {
			byName[i.Name] = i
		}
		visited := map[string]bool{}
		names := map[string]bool{}
		sigs := map[string][]*Function{}
		var visit func(i *Interface)
		visit = func(i *Interface) {
			if visited[i.Name] {
				return
			}
			visited[i.Name] = true
			for _, m := range i.Methods {
				names[m.Name] = true
				sigs[m.Name] = append(sigs[m.Name], m)
			}
			for _, e := range i.Embeds {
				if emb := byName[e]; emb != nil {
					visit(emb)
				}
			}
		}
		for _, i := range ifaces {
			visit(i)
		}
		f.analysis.ifaceMethods = names
		f.analysis.ifaceMethodSigs = sigs
	})
}

// InterfaceMethodSatisfied reports whether fn (a concrete method) matches a
// same-named method declared by some interface in this file's package (or this
// file if analyzed in isolation), i.e. the method could satisfy that
// interface's method set. Matching requires identical parameter and result
// types, in order; a method whose signature differs from every same-named
// interface method satisfies nothing. Computed at most once per file.
func (f *File) InterfaceMethodSatisfied(fn *Function) bool {
	f.collectInterfaceMethods()
	for _, im := range f.analysis.ifaceMethodSigs[fn.Name] {
		if sameSignature(im, fn) {
			return true
		}
	}
	return false
}

func sameSignature(a, b *Function) bool {
	if len(a.Params) != len(b.Params) || len(a.Results) != len(b.Results) {
		return false
	}
	for i := range a.Params {
		if a.Params[i].Type != b.Params[i].Type {
			return false
		}
	}
	for i := range a.Results {
		if a.Results[i].Type != b.Results[i].Type {
			return false
		}
	}
	return true
}

// EffectiveLinesOfCode returns the number of code-bearing physical source
// lines in the requested span. The source is indexed at most once per file.
func (f *File) EffectiveLinesOfCode(start, end token.Pos) int {
	f.analysis.effectiveLOCOnce.Do(func() {
		f.analysis.effectiveLOC = metrics.NewEffectiveLOCIndex(f.Src)
	})
	return f.analysis.effectiveLOC.LinesOfCode(f.Fset, start, end)
}

func collectCompositeMemberNames(lit *ast.CompositeLit, set map[string]bool, classes []*Class) {
	switch lit.Type.(type) {
	case *ast.MapType, *ast.ArrayType:
		return
	}
	if name, ok := unkeyedStructName(lit); ok {
		markStructFields(name, set, classes)
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if ok {
			set[id.Name] = true
		}
	}
}

func unkeyedStructName(lit *ast.CompositeLit) (string, bool) {
	if len(lit.Elts) == 0 {
		return "", false
	}
	for _, elt := range lit.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			return "", false
		}
	}
	return namedTypeName(lit.Type)
}

func namedTypeName(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.ParenExpr:
		return namedTypeName(t.X)
	case *ast.StarExpr:
		return namedTypeName(t.X)
	case *ast.IndexExpr:
		return namedTypeName(t.X)
	case *ast.IndexListExpr:
		return namedTypeName(t.X)
	default:
		return "", false
	}
}

func markStructFields(name string, set map[string]bool, classes []*Class) {
	for _, class := range classes {
		if class.Name != name {
			continue
		}
		for _, field := range class.Fields {
			set[field.Name] = true
		}
		return
	}
}

// Parameter is a formal parameter of a function or method.
type Parameter struct {
	Name     string
	Type     string
	Line     int
	Field    *ast.Field
	Ident    *ast.Ident
	Promoted bool // reserved; Go has no constructor promotion
}

// Field is a struct field (the analog of a PHP class property).
type Field struct {
	Name     string
	Type     string
	Line     int
	Exported bool
	Static   bool // package-level var attached as a "static" field (unused for structs)
	Ident    *ast.Ident
}

// Function represents a free function OR a method (when Receiver != "").
// PHPMD distinguishes ASTMethod from ASTFunction; we keep one struct and use
// IsMethod()/Receiver to tell them apart, which keeps rule code uniform.
type Function struct {
	Name       string
	Receiver   string // empty for free functions; type name (without *) for methods
	RecvName   string // receiver variable name, e.g. "f" in (f *Foo)
	Params     []*Parameter
	Results    []*Parameter
	Line       int
	EndLine    int
	Exported   bool
	Decl       *ast.FuncDecl
	Body       *ast.BlockStmt
	File       *File
	Class      *Class // owning class for a method, if resolved
	DocComment string

	identifierReads identifierReadCache
}

func (f *Function) IsMethod() bool { return f.Receiver != "" }

type identifierReadCache struct {
	once       sync.Once
	names      map[string]bool
	objects    map[*ast.Object]bool
	unresolved map[string]bool
}

func functionIdentifierReadFacts(f *Function) *identifierReadCache {
	cache := &f.identifierReads
	cache.once.Do(func() {
		facts := scanIdentifierReads(f.Body)
		cache.names = facts.names
		cache.objects = facts.objects
		cache.unresolved = facts.unresolved
	})
	return cache
}

func (f *Function) NodeType() NodeType {
	if f.IsMethod() {
		return TypeMethod
	}
	return TypeFunction
}

// Class is a named struct type plus its associated methods (analog of a PHP
// class). A type defined as `type T struct{...}` becomes a Class; methods with
// receiver T are attached.
type Class struct {
	Name       string
	Line       int
	EndLine    int
	Exported   bool
	Fields     []*Field
	Methods    []*Function
	File       *File
	Spec       *ast.TypeSpec
	Struct     *ast.StructType
	DocComment string
	// Embeds holds embedded type names (the closest Go analog to parents).
	Embeds []string
}

func (c *Class) NodeType() NodeType { return TypeClass }

// Interface is a named interface type (analog of a PHP interface).
type Interface struct {
	Name       string
	Line       int
	EndLine    int
	Exported   bool
	Methods    []*Function
	File       *File
	Spec       *ast.TypeSpec
	Iface      *ast.InterfaceType
	DocComment string
	Embeds     []string
}

func (i *Interface) NodeType() NodeType { return TypeInterface }
