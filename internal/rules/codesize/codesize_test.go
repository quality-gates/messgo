package codesize

import (
	"fmt"
	"strings"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

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
