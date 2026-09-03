package model

import (
	"go/ast"
	"go/token"
	"testing"
)

const queryHelperFixture = `package sample

func outer(captured int) {
	index := 0
	closure := func() {
		short := 1
		for key := range []int{1} {
			_ = key
		}
		for index = range []int{1} {
		}
	}
	_ = closure
}
`

func parseQueryHelperFixture(t *testing.T) *Function {
	t.Helper()
	f, err := ParseSource("helpers.go", []byte(queryHelperFixture))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	return f.Functions[0]
}

type declarationForms struct {
	shortIdent    *ast.Ident
	shortDecl     ast.Node
	defineRange   *ast.RangeStmt
	assignedRange *ast.RangeStmt
}

func findDeclarationForms(body *ast.BlockStmt) declarationForms {
	var forms declarationForms
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			findShortDeclaration(node, &forms)
		case *ast.RangeStmt:
			findRangeDeclarations(node, &forms)
		}
		return true
	})
	return forms
}

func findShortDeclaration(node *ast.AssignStmt, forms *declarationForms) {
	if node.Tok != token.DEFINE || forms.shortIdent != nil {
		return
	}
	for _, lhs := range node.Lhs {
		id, ok := lhs.(*ast.Ident)
		if ok && id.Name == "short" {
			forms.shortIdent, forms.shortDecl = id, node
		}
	}
}

func findRangeDeclarations(node *ast.RangeStmt, forms *declarationForms) {
	if node.Tok == token.DEFINE && forms.defineRange == nil {
		forms.defineRange = node
	}
	if node.Tok == token.ASSIGN {
		forms.assignedRange = node
	}
}

func TestIdentifierQueryHelpersRejectUnboundAndWrongScopeObjects(t *testing.T) {
	if IdentifierRead(&Function{}, &ast.Ident{Name: "missing"}) {
		t.Fatal("IdentifierRead on a function without a body = true, want false")
	}
	if isDeclaredBy(nil, nil) {
		t.Fatal("isDeclaredBy(nil, nil) = true, want false")
	}
	if isReadIdentifier(&ast.Ident{Name: "_"}, nil) {
		t.Fatal("isReadIdentifier(_, nil) = true, want false")
	}
	writeTarget := &ast.Ident{Name: "writeTarget"}
	if isReadIdentifier(writeTarget, map[*ast.Ident]bool{writeTarget: true}) {
		t.Fatal("isReadIdentifier() treated a write target as a read")
	}
}

func TestRangeBindingHelpersRespectDeclarationForms(t *testing.T) {
	fn := parseQueryHelperFixture(t)
	forms := findDeclarationForms(fn.Body)
	if forms.shortIdent == nil || forms.shortDecl == nil || forms.defineRange == nil || forms.assignedRange == nil {
		t.Fatal("test fixture did not expose declaration forms")
	}
	if isDeclaredBy(forms.shortIdent, &ast.EmptyStmt{}) {
		t.Fatal("isDeclaredBy() matched a declaration to the wrong node")
	}
	if isRangeDeclaredBy(forms.assignedRange.Key.(*ast.Ident), forms.assignedRange) {
		t.Fatal("isRangeDeclaredBy() treated an assignment range as a declaration")
	}
	if isRangeDeclaredBy(forms.defineRange.Key.(*ast.Ident), &ast.RangeStmt{Tok: token.DEFINE, Key: &ast.Ident{Name: "other"}}) {
		t.Fatal("isRangeDeclaredBy() matched an identifier from a different range")
	}
	if isRangeDeclaredBy(fn.Params[0].Ident, forms.defineRange) {
		t.Fatal("isRangeDeclaredBy() treated a parameter object as a range declaration")
	}
}
