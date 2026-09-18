package codesize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func analyzeConfiguredRule(t *testing.T, src string, r rule.Rule, props rule.Properties) []*rule.Violation {
	t.Helper()
	configurable, ok := r.(rule.Configurable)
	if !ok {
		t.Fatalf("%T should implement rule.Configurable", r)
	}
	if err := configurable.Configure(props); err != nil {
		t.Fatalf("configure %T: %v", r, err)
	}
	f, err := model.ParseSource("fixture.go", []byte("package fixture\n"+src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{r}}})
}

func TestCyclomaticComplexityFlagsByDefault(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
func complexFunc(x int) {
	if x > 0 {
		println(x)
	}
}
`, newCyclomaticComplexity(), rule.Properties{"reportLevel": "2"})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
}

func TestCyclomaticComplexityShowMethodsComplexityFalseSuppresses(t *testing.T) {
	violations := analyzeConfiguredRule(t, `
func complexFunc(x int) {
	if x > 0 {
		println(x)
	}
}
`, newCyclomaticComplexity(), rule.Properties{"reportLevel": "2", "showMethodsComplexity": "false"})
	if len(violations) != 0 {
		t.Fatalf("expected showMethodsComplexity=false to suppress the violation, got %d", len(violations))
	}
}

func BenchmarkWhitespaceAwareLOC(b *testing.B) {
	for _, classes := range []int{100, 200, 400} {
		b.Run(fmt.Sprintf("classes_%d", classes), func(b *testing.B) {
			methodRule := newLongMethod().(*LongMethod)
			classRule := newLongClass().(*LongClass)
			props := rule.Properties{
				"minimum":           "1000000",
				"ignore-whitespace": "true",
			}
			if err := methodRule.Configure(props); err != nil {
				b.Fatal(err)
			}
			if err := classRule.Configure(props); err != nil {
				b.Fatal(err)
			}
			sets := []*rule.RuleSet{{Rules: []rule.Rule{methodRule, classRule}}}
			b.ResetTimer()
			for b.Loop() {
				file := whitespaceAwareLOCFile(b, classes)
				if violations := rule.Analyze(file, sets); len(violations) != 0 {
					b.Fatalf("violations = %d, want 0", len(violations))
				}
			}
		})
	}
}

func whitespaceAwareLOCFile(tb testing.TB, classes int) *model.File {
	tb.Helper()
	var src strings.Builder
	src.WriteString("package sample\n")
	for index := range classes {
		fmt.Fprintf(&src, "type type%d struct {\n\t// field\n\n\tvalue int\n}\n", index)
		fmt.Fprintf(&src, "func (type%d) method%d() {\n\t// body\n\n\t_ = %d\n}\n", index, index, index)
	}
	file, err := model.ParseSource("codesize.go", []byte(src.String()))
	if err != nil {
		tb.Fatalf("ParseSource: %v", err)
	}
	return file
}
