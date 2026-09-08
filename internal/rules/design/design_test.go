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

func TestCouplingMeasureGenericTypes(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"List[pkg.Entry]", []string{"List", "Entry"}},
		{"pkg1.List[pkg2.Entry]", []string{"List", "Entry"}},
		{"List[int]", []string{"List", "int"}},
		{"Pair[Key, Value]", []string{"Pair", "Key", "Value"}},
		{"Map[string, List[CustomType]]", []string{"Map", "string", "List", "CustomType"}},
		{"Container[T, comparable]", []string{"Container", "T", "comparable"}},
		{"[]List[Entry]", []string{"List", "Entry"}},
		{"*pkg.Box[pkg2.Item]", []string{"Box", "Item"}},
		{"map[string]List[Entry]", []string{"string", "List", "Entry"}},
		{"chan List[Entry]", []string{"List", "Entry"}},
		{"List[*Entry]", []string{"List", "Entry"}},
		{"List[[]Entry]", []string{"List", "Entry"}},
		{"Pair[map[string]Entry, chan Value]", []string{"Pair", "string", "Entry", "Value"}},
	}

	for _, tc := range tests {
		got := namedTypesIn(tc.input)
		if len(got) != len(tc.want) {
			t.Fatalf("namedTypesIn(%q) len = %d (%v), want len = %d (%v)", tc.input, len(got), got, len(tc.want), tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("namedTypesIn(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestCouplingMeasureGenericFieldsMethodsAndResults(t *testing.T) {
	src := `package sample

type Entry struct{}
type Result[T any] struct{}
type Box[T any] struct{}
type Service struct {
	cache Box[Entry]
	ints  Box[int]
	strs  Box[string]
}

func (Service) Process(items Box[pkg.Entry], flag bool) Result[Entry] {
	return Result[Entry]{}
}

func (Service) Status() (Result[int], error) {
	return Result[int]{}, nil
}
`
	f, err := model.ParseSource("coupling.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	var serviceClass *model.Class
	for _, c := range f.Classes {
		if c.Name == "Service" {
			serviceClass = c
			break
		}
	}
	if serviceClass == nil {
		t.Fatal("Service class not found")
	}

	rule := newCouplingBetweenObjects().(*CouplingBetweenObjects)
	measurement, ok := rule.measure(nil, serviceClass)
	if !ok {
		t.Fatal("measure returned ok = false")
	}
	// Service couples to Box, Entry, Result (3 types).
	// Builtins (int, string, bool, error) must not be counted.
	// Multiple instantiations of Box and Result should be counted once each.
	if measurement.Value != 3 {
		t.Errorf("Service coupling = %d, want 3 (Box, Entry, Result)", measurement.Value)
	}
}
