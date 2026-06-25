package rule

import (
	"sort"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

// recordingFuncRule records every function it is applied to, so we can assert
// the engine's unified function dispatch covers free functions, methods, and
// interface methods exactly once each.
type recordingFuncRule struct {
	*Base
	seen []string
}

func (r *recordingFuncRule) ApplyFunc(c *Context, fn *model.Function) {
	r.seen = append(r.seen, fn.Name)
}

func TestFuncRuleDispatchCoversAllFunctions(t *testing.T) {
	src := `package fixture

func Free() {}

type Greeter struct{}

func (g Greeter) Hello() {}

type Speaker interface {
	Say()
}
`
	f, err := model.ParseSource("fixture.go", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := &recordingFuncRule{Base: NewBase()}
	set := &RuleSet{Name: "test", Rules: []Rule{r}}

	Analyze(f, []*RuleSet{set})

	got := append([]string(nil), r.seen...)
	sort.Strings(got)
	want := []string{"Free", "Hello", "Say"}
	if len(got) != len(want) {
		t.Fatalf("ApplyFunc saw %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ApplyFunc saw %v, want %v", got, want)
		}
	}
}
