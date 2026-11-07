package cli

import (
	"slices"
	"testing"
)

func TestExplicitnessPromotedSelectors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		want   []string
	}{
		{
			name: "shallower method wins over earlier deeper method",
			source: `package p
type a struct{}
func (a) apply(s []int) {}
type b struct{ a }
type c struct{}
func (c) apply(s []int) { s[0] = 1 }
type d struct{ b; c }
func use(x d, s []int) { x.apply(s) }
`,
			want: []string{"a.go:6 ImplicitOutput c.apply: write through parameter s", "a.go:8 ImplicitOutput use: write through parameter s"},
		},
		{
			name: "direct function field shadows promoted method",
			source: `package p
type X struct{}
func (X) Apply(s []int) {}
type Y struct { X; Apply func([]int) }
func use(y Y, s []int) { y.Apply(s) }
`,
			want: []string{"a.go:5 UnknownEffect use: unresolved call"},
		},
		{
			name: "promoted function field shadows deeper method",
			source: `package p
type X struct{}
func (X) Apply(s []int) {}
type Inner struct { X }
type Callback struct { Apply func([]int) }
type Y struct { Inner; Callback }
func use(y Y, s []int) { y.Apply(s) }
`,
			want: []string{"a.go:7 UnknownEffect use: unresolved call"},
		},
		{
			name: "shallower method shadows deeper function field",
			source: `package p
type Callback struct { Apply func([]int) }
type Inner struct { Callback }
type Writer struct{}
func (Writer) Apply(s []int) { s[0] = 1 }
type Y struct { Inner; Writer }
func use(y Y, s []int) { y.Apply(s) }
`,
			want: []string{"a.go:5 ImplicitOutput Writer.Apply: write through parameter s", "a.go:7 ImplicitOutput use: write through parameter s"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": tc.source})
			assertFindings(t, got, tc.want...)
		})
	}
}

func TestUnusedCodeUsesTheShallowestPromotedMethod(t *testing.T) {
	path := writeFixture(t, `package p
type a struct{}
func (a) hidden() int { return 1 }
type b struct{ a }
type c struct{}
func (c) hidden() int { return 2 }
type d struct {
 b
 c
}
func use(x d) int { return x.hidden() }
`)
	code, out, stderr := runMain(t, path, "json", "unusedcode")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	violations := violationsFor(t, out, path)
	methods := linesFor(violations, "UnusedPrivateMethod")
	fields := linesFor(violations, "UnusedPrivateField")
	if !slices.Contains(methods, 3) || slices.Contains(methods, 6) {
		t.Errorf("unused method lines = %v, want unselected line 3 and not selected line 6", methods)
	}
	if !slices.Contains(fields, 8) || slices.Contains(fields, 9) {
		t.Errorf("unused field lines = %v, want unselected line 8 and not selected line 9", fields)
	}
}
