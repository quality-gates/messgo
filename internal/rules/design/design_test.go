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

func TestCouplingMeasureSkipsSelfType(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample

type S struct{}

func (S) Clone() S { return S{} }
`))
	if err != nil {
		t.Fatal(err)
	}
	rule := newCouplingBetweenObjects().(*CouplingBetweenObjects)
	measurement, ok := rule.measure(nil, f.Classes[0])
	if !ok {
		t.Fatal("measure returned ok = false")
	}
	if measurement.Value != 0 {
		t.Fatalf("self type counted as coupling: %d", measurement.Value)
	}
}

func TestCouplingMeasureNamedTypesInDecoratedForms(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample

type Key struct{}
type Value struct{}
type S struct {
	ptr *Key
	signal chan Value
}

func (S) Use(items ...Key) {}
`))
	if err != nil {
		t.Fatal(err)
	}
	var class *model.Class
	for _, c := range f.Classes {
		if c.Name == "S" {
			class = c
		}
	}
	rule := newCouplingBetweenObjects().(*CouplingBetweenObjects)
	measurement, ok := rule.measure(nil, class)
	if !ok {
		t.Fatal("measure returned ok = false")
	}
	if measurement.Value != 2 {
		t.Fatalf("decorated named types coupling = %d, want 2", measurement.Value)
	}
}

func TestCouplingMeasureCountsMapTypeArguments(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample

type Key struct{}
type Value struct{}
type S struct {
	items map[Key]Value
}
`))
	if err != nil {
		t.Fatal(err)
	}
	var class *model.Class
	for _, c := range f.Classes {
		if c.Name == "S" {
			class = c
		}
	}
	if class == nil {
		t.Fatal("class S not found")
	}
	rule := newCouplingBetweenObjects().(*CouplingBetweenObjects)
	measurement, ok := rule.measure(nil, class)
	if !ok {
		t.Fatal("measure returned ok = false")
	}
	if measurement.Value != 2 {
		t.Fatalf("map[Key]Value coupling = %d, want 2", measurement.Value)
	}
}
