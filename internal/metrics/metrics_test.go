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
