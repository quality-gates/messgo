package naming

import (
	"fmt"
	"slices"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func constructorHits(t *testing.T, src string) int {
	t.Helper()
	f, err := model.ParseSource("fixture.go", []byte("package p\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := &ConstructorWithNameAsEnclosingClass{Base: rule.NewBase()}
	vs := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{r}}})
	n := 0
	for _, v := range vs {
		if _, ok := v.Rule.(*ConstructorWithNameAsEnclosingClass); ok {
			n++
		}
	}
	return n
}

func TestConstructorWithNameAsEnclosingClass(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "Error.Error() string pointer",
			src: `
type Error struct{ Msg string }
func (e *Error) Error() string { return e.Msg }
`,
			want: 0,
		},
		{
			name: "Error.Error() string value",
			src: `
type Error struct{ Msg string }
func (e Error) Error() string { return e.Msg }
`,
			want: 0,
		},
		{
			name: "Error.Error() named string result",
			src: `
type Error struct{ Msg string }
func (e *Error) Error() (msg string) { return e.Msg }
`,
			want: 0,
		},
		{
			name: "Widget.Widget() string still reported",
			src: `
type Widget struct{}
func (w *Widget) Widget() string { return "" }
`,
			want: 1,
		},
		{
			name: "String.String() still reported",
			src: `
type String struct{}
func (s *String) String() string { return "" }
`,
			want: 1,
		},
		{
			name: "Error.Error() extra result",
			src: `
type Error struct{ Msg string }
func (e *Error) Error() (string, error) { return e.Msg, nil }
`,
			want: 1,
		},
		{
			name: "Error.Error() with parameter",
			src: `
type Error struct{}
func (e *Error) Error(code int) string { return "" }
`,
			want: 1,
		},
		{
			name: "Error.Error() non-string result",
			src: `
type Error struct{}
func (e *Error) Error() int { return 0 }
`,
			want: 1,
		},
		{
			name: "Error.Error() no results",
			src: `
type Error struct{}
func (e *Error) Error() {}
`,
			want: 1,
		},
		{
			name: "different method name is not this rule",
			src: `
type Error struct{}
func (e *Error) String() string { return "" }
`,
			want: 0,
		},
		{
			name: "free function is not this rule",
			src: `
func Error() string { return "" }
`,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constructorHits(t, tt.src); got != tt.want {
				t.Fatalf("hits = %d, want %d", got, tt.want)
			}
		})
	}
}

// typeNameFindings configures r, analyzes src and returns one
// "line-end class method function args" entry per violation.
func typeNameFindings(t *testing.T, r rule.Rule, props rule.Properties, src string) []string {
	t.Helper()
	if err := r.(rule.Configurable).Configure(props); err != nil {
		t.Fatalf("configure %T: %v", r, err)
	}
	f, err := model.ParseSource("fixture.go", []byte("package p\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got []string
	for _, v := range rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{r}}}) {
		got = append(got, fmt.Sprintf("%d-%d %s %s %s %v", v.BeginLine, v.EndLine, v.Class, v.Method, v.Function, v.Args))
	}
	return got
}

const typeNameSrc = `
type Ab struct{}
type Abc struct{}
type Cd interface {
	M()
}
type Cde interface{ N() }
`

func TestTypeNameRulesReportClassContext(t *testing.T) {
	tests := []struct {
		name    string
		newRule func() rule.Rule
		props   rule.Properties
		want    []string
	}{
		{
			name:    "ShortClassName below minimum",
			newRule: newShortClassName,
			props:   rule.Properties{},
			want:    []string{"3-3 Ab   [Ab 3]", "5-7 Cd   [Cd 3]"},
		},
		{
			name:    "ShortClassName exceptions",
			newRule: newShortClassName,
			props:   rule.Properties{"exceptions": "Ab,Cd"},
			want:    nil,
		},
		{
			name:    "LongClassName above maximum",
			newRule: newLongClassName,
			props:   rule.Properties{"maximum": "2"},
			want:    []string{"4-4 Abc   [Abc 2]", "8-8 Cde   [Cde 2]"},
		},
		{
			name:    "LongClassName subtracted prefix",
			newRule: newLongClassName,
			props:   rule.Properties{"maximum": "2", "subtract-prefixes": "C"},
			want:    []string{"4-4 Abc   [Abc 2]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeNameFindings(t, tt.newRule(), tt.props, typeNameSrc); !slices.Equal(got, tt.want) {
				t.Fatalf("findings = %q, want %q", got, tt.want)
			}
		})
	}
}
