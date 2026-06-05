// Package model wraps the Go standard-library AST into phpmd-style code
// artifacts (Class, Interface, Method, Function, Field, Parameter). Rules are
// written against these artifacts, mirroring how PHPMD rules operate on
// pdepend's ASTClass / ASTMethod / ASTFunction nodes.
package model

import (
	"go/ast"
	"go/token"
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
}

func (f *Function) IsMethod() bool { return f.Receiver != "" }

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
