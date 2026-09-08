package naming

import (
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
