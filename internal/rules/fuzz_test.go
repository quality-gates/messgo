package rules_test

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/ruleset"
)

func FuzzAnalyze(f *testing.F) {
	f.Add([]byte(`package main
func main() {
	println("hello")
}
`))
	f.Add([]byte(`package p
type Foo struct {
	x int
}
func (f *Foo) Bar(a int) bool {
	if a > 0 {
		return true
	}
	return false
}
`))
	f.Add([]byte(`package p
func f() {
	for i := 0; i < 10; i++ {
		if i == 5 {
			break
		}
	}
}
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := model.ParseSource("fuzz.go", data)
		if err != nil {
			return
		}

		loader := &ruleset.Loader{}
		sets, err := loader.Load("codesize,naming,unusedcode,design,cleancode,controversial")
		if err != nil {
			t.Fatalf("failed to load rulesets: %v", err)
		}

		_ = rule.Analyze(file, sets)
	})
}
