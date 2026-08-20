package design

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

func TestBuiltinTypeNamesIncludeGoContainersAndFunctionTypes(t *testing.T) {
	for _, name := range []string{"map", "chan", "func"} {
		if !builtinTypes[name] {
			t.Errorf("builtinTypes[%q] = false, want true", name)
		}
	}
	for input, want := range map[string]string{
		"map[string]int": "map",
		"chan int":       "chan",
		"func":           "func",
	} {
		if got := baseTypeName(input); got != want {
			t.Errorf("baseTypeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCouplingMeasureIgnoresBuiltinContainerTypes(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample

type Box struct {
	values map[string]int
	signal chan int
	callback func() error
	custom Other
}

type Other struct{}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	rule := newCouplingBetweenObjects().(*CouplingBetweenObjects)
	measurement, ok := rule.measure(nil, f.Classes[0])
	if !ok {
		t.Fatal("CouplingBetweenObjects.measure() returned ok = false")
	}
	if measurement.Value != 1 {
		t.Fatalf("CouplingBetweenObjects.measure() = %d, want only custom type counted", measurement.Value)
	}
}
