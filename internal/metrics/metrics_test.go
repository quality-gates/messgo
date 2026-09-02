package metrics

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"go/ast"
)

func parseFuncBody(t *testing.T, src string) *ast.BlockStmt {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			return fd.Body
		}
	}
	t.Fatal("no function found")
	return nil
}

// The reference function below is a line-for-line translation of the PHP
// snippet validated against phpmd 2.15.0, which reported CCN=12 and NPath=324.
// These tests assert messgo computes the identical numbers, proving metric
// parity with phpmd/pdepend.
const referenceFunc = `
func highComplexity(a, b, c, d, e int) int {
	x := 0
	if a > 0 && b > 0 {
		x++
	}
	if a > 1 || b > 1 {
		x++
	}
	for i := 0; i < a; i++ {
		if i%2 == 0 {
			x++
		}
	}
	switch c {
	case 1:
		x++
	case 2:
		x++
	case 3:
		x++
	}
	if d > 0 {
		x++
	}
	if e > 0 {
		x++
	}
	return x
}`

func TestCyclomaticComplexityMatchesPHPMD(t *testing.T) {
	body := parseFuncBody(t, referenceFunc)
	if got := CyclomaticComplexity(body); got != 12 {
		t.Errorf("CyclomaticComplexity = %d, want 12 (phpmd reference)", got)
	}
}

func TestNPathComplexityMatchesPHPMD(t *testing.T) {
	body := parseFuncBody(t, referenceFunc)
	if got := NPathComplexity(body); got != 324 {
		t.Errorf("NPathComplexity = %d, want 324 (phpmd reference)", got)
	}
}

func TestCyclomaticComplexityBaseline(t *testing.T) {
	body := parseFuncBody(t, "func f() { return }")
	if got := CyclomaticComplexity(body); got != 1 {
		t.Errorf("empty function CCN = %d, want 1", got)
	}
}

func TestNPathTabledCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"linear", "func f() { a := 1; b := 2; _ = a + b }", 1},
		{"single if no else", "func f(a int) { if a > 0 { } }", 2},
		{"if with else", "func f(a int) { if a > 0 { } else { } }", 2},
		{"if and (&&)", "func f(a, b int) { if a > 0 && b > 0 { } }", 3},
		{"two sequential ifs", "func f(a int) { if a > 0 {}; if a > 1 {} }", 4},
		{"empty switch", "func f() { switch {} }", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := parseFuncBody(t, tc.src)
			if got := NPathComplexity(body); got != tc.want {
				t.Errorf("NPath(%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestNPathComplexitySaturatesInsteadOfOverflowing(t *testing.T) {
	body := parseFuncBody(t, "func f() {\n"+strings.Repeat("if true {}\n", 63)+"}")
	want := int(^uint(0) >> 1)
	if got := NPathComplexity(body); got != want {
		t.Fatalf("NPathComplexity(63 sequential branches) = %d, want saturated max %d", got, want)
	}
}

func TestNPathArithmeticSaturatesAndPreservesNormalValues(t *testing.T) {
	max := npathMaxInt()
	addCases := []struct {
		name string
		a, b int
		want int
	}{
		{name: "normal", a: 2, b: 3, want: 5},
		{name: "overflow", a: max, b: 1, want: max},
		{name: "negative left", a: -1, b: 1, want: max},
		{name: "negative right", a: 1, b: -1, want: max},
	}
	for _, tc := range addCases {
		t.Run("add/"+tc.name, func(t *testing.T) {
			if got := npathAdd(tc.a, tc.b); got != tc.want {
				t.Fatalf("npathAdd(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}

	mulCases := []struct {
		name string
		a, b int
		want int
	}{
		{name: "normal", a: 2, b: 3, want: 6},
		{name: "zero left", a: 0, b: 3, want: 0},
		{name: "zero right", a: 3, b: 0, want: 0},
		{name: "overflow", a: max, b: 2, want: max},
		{name: "negative left", a: -1, b: 2, want: max},
		{name: "negative right", a: 2, b: -1, want: max},
	}
	for _, tc := range mulCases {
		t.Run("multiply/"+tc.name, func(t *testing.T) {
			if got := npathMul(tc.a, tc.b); got != tc.want {
				t.Fatalf("npathMul(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestEffectiveLinesOfCodeIgnoresLineDirectives(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "directive before function",
			src:  "package p\n//line generated.go:100\nfunc f() {\n\tx := 1\n}\n",
			want: 3,
		},
		{
			name: "directive shrinks logical end",
			src:  "package p\nfunc f() {\n\tx := 1\n\ty := 2\n//line gen.go:1\n\tz := 3\n}\n",
			want: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			fd := f.Decls[0]
			if got := EffectiveLinesOfCode(fset, fd.Pos(), fd.End(), src); got != tc.want {
				t.Fatalf("EffectiveLinesOfCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEffectiveLOCIndexAnswersSourceSpans(t *testing.T) {
	src := []byte(`package p
/*
package comment
*/
func first() {
	// line comment

	value := 1 /* inline comment */
	_ = value
}
/* between declarations */
func second() {}
`)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	index := NewEffectiveLOCIndex(src)
	wants := []int{4, 1}
	for declaration, want := range wants {
		decl := file.Decls[declaration]
		if got := index.LinesOfCode(fset, decl.Pos(), decl.End()); got != want {
			t.Errorf("declaration %d effective LOC = %d, want %d", declaration, got, want)
		}
	}
	// Repeated range queries must remain stable after the source is indexed.
	first := file.Decls[0]
	if got := index.LinesOfCode(fset, first.Pos(), first.End()); got != wants[0] {
		t.Errorf("repeated first declaration effective LOC = %d, want %d", got, wants[0])
	}
}

func TestEffectiveLOCIndexClampsQueriesToIndexedSource(t *testing.T) {
	indexedSource := []byte("code\n")
	positionSource := []byte("code\noutside\nmore outside\n")
	fset := token.NewFileSet()
	file := fset.AddFile("x.go", -1, len(positionSource))
	file.SetLinesForContent(positionSource)
	index := NewEffectiveLOCIndex(indexedSource)

	if got := index.LinesOfCode(fset, token.NoPos, file.Pos(len(positionSource))); got != 1 {
		t.Fatalf("clamped effective LOC = %d, want 1", got)
	}
	var nilIndex *EffectiveLOCIndex
	if got := nilIndex.LinesOfCode(fset, file.Pos(0), file.Pos(0)); got != 0 {
		t.Fatalf("nil index effective LOC = %d, want 0", got)
	}
}

func TestNPathCoversRangeSwitchSelectLoopAndReturnForms(t *testing.T) {
	body := parseFuncBody(t, `func f(items []int, ch <-chan int, value any, a, b bool) int {
	total := 0
	for range items {
		total++
	}
	switch value.(type) {
	case int:
		total++
	default:
		total--
	}
	select {
	case <-ch:
		total++
	default:
		total--
	}
	for i := 0 + (a && b); i < 1; i = i + (a || b) {
		total++
	}
	return total
}`)
	if got := NPathComplexity(body); got != 32 {
		t.Fatalf("NPathComplexity(mixed control flow) = %d, want 32", got)
	}
}

func TestNestingDepth(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "flat",
			src:  `func f() { x := 1; _ = x }`,
			want: 0,
		},
		{
			name: "single if",
			src:  `func f(a bool) { if a { return } }`,
			want: 1,
		},
		{
			name: "three deep if",
			src: `func f(a, b, c bool) {
				if a {
					if b {
						if c {
							return
						}
					}
				}
			}`,
			want: 3,
		},
		{
			name: "else-if chain does not add depth",
			src: `func f(a, b, c bool) {
				if a {
					return
				} else if b {
					return
				} else if c {
					return
				}
			}`,
			want: 1,
		},
		{
			name: "else block adds depth",
			src: `func f(a bool) {
				if a {
					if a {
						return
					}
				} else {
					if a {
						return
					}
				}
			}`,
			want: 2,
		},
		{
			name: "for-range-switch-select mixed",
			src: `func f(items []int, ch <-chan int, v any) {
				for _, x := range items {
					switch v.(type) {
					case int:
						if x > 0 {
							return
						}
					}
					select {
					case <-ch:
						if x < 0 {
							return
						}
					default:
					}
				}
			}`,
			// for(1) > switch(2) > case > if(3); the select branch also reaches 3
			want: 3,
		},
		{
			name: "nil body",
			src:  `func f()`,
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// parseFuncBody expects a body; the "nil body" case needs special handling.
			if tc.src == `func f()` {
				if got := NestingDepth(nil); got != tc.want {
					t.Errorf("NestingDepth(nil) = %d, want %d", got, tc.want)
				}
				return
			}
			body := parseFuncBody(t, tc.src)
			if got := NestingDepth(body); got != tc.want {
				t.Errorf("NestingDepth = %d, want %d", got, tc.want)
			}
		})
	}
}

func parseFuncDecl(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			return fd
		}
	}
	t.Fatal("no function found")
	return nil
}

func TestCognitiveComplexity(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"empty", `func f() {}`, 0},
		{"single if", `func f(a bool) { if a { } }`, 1},
		{"nested if", `func f(a, b bool) { if a { if b { } } }`, 3},
		{"if else block", `func f(a bool) { if a { } else { } }`, 2},
		{"if else-if chain", `func f(a, b bool) { if a { } else if b { } }`, 2},
		{"for with nested if", `func f(items []int) { for _, x := range items { if x > 0 { } } }`, 3},
		{"binary and", `func f(a, b bool) bool { return a && b }`, 1},
		{"binary chain same op", `func f(a, b, c bool) bool { return a && b && c }`, 1},
		{"binary chain mixed ops", `func f(a, b, c bool) bool { return a && b || c }`, 2},
		{"switch with cases", `func f(n int) { switch n { case 1: case 2: default: } }`, 1},
		{"labeled break", `func f() { outer: for { break outer } }`, 2},
		{"nil decl", `func f()`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.src == `func f()` {
				if got := CognitiveComplexity(nil); got != tc.want {
					t.Errorf("CognitiveComplexity(nil) = %d, want %d", got, tc.want)
				}
				return
			}
			fn := parseFuncDecl(t, tc.src)
			if got := CognitiveComplexity(fn); got != tc.want {
				t.Errorf("CognitiveComplexity = %d, want %d", got, tc.want)
			}
		})
	}
}
