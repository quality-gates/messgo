package design

import (
	"testing"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/rule"
)

func TestCouplingNestedTypeDependenciesThroughAnalyze(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample
 const arrayLength = 3
 type Service struct {
  callback func(named Input, rest ...Extra) (result Output)
  nested struct { field [arrayLength]Element }
  contract interface { Method(param Request) Response }
  self *Service
  builtins map[string]chan int
 }
 func (Service) Process(Input) Output { panic("fixture") }
 `))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		maximum string
		want    int
	}{{"6", 1}, {"7", 0}} {
		t.Run(tc.maximum, func(t *testing.T) {
			threshold := newCouplingBetweenObjects()
			if err := threshold.(rule.Configurable).Configure(rule.Properties{"maximum": tc.maximum}); err != nil {
				t.Fatal(err)
			}
			violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{threshold}}})
			if len(violations) != tc.want {
				t.Fatalf("CBO violations at maximum %s = %d, want %d (six dependencies)", tc.maximum, len(violations), tc.want)
			}
		})
	}
}

func TestCouplingBuiltinTypesThroughAnalyze(t *testing.T) {
	f, err := model.ParseSource("builtins.go", []byte(`package sample
 type Service struct {
  boolean bool
  text string
  integer int
  signed8 int8
  signed16 int16
  signed32 int32
  signed64 int64
  unsigned uint
  unsigned8 uint8
  unsigned16 uint16
  unsigned32 uint32
  unsigned64 uint64
  pointer uintptr
  octet byte
  character rune
  real32 float32
  real64 float64
  complex64 complex64
  complex128 complex128
  failure error
  anything any
  constraint interface { comparable }
  empty struct{}
  contract interface{}
  container map[string]chan int
  callback func() error
 }
 `))
	if err != nil {
		t.Fatal(err)
	}
	threshold := newCouplingBetweenObjects()
	if err := threshold.(rule.Configurable).Configure(rule.Properties{"maximum": "1"}); err != nil {
		t.Fatal(err)
	}
	if violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{threshold}}}); len(violations) != 0 {
		t.Fatalf("builtin-only class has CBO findings: %v", violations)
	}
}

func TestCouplingBetweenObjectsThresholdReportsThroughAnalyze(t *testing.T) {
	f, err := model.ParseSource("coupling.go", []byte(`package sample

type Other struct{}
type Service struct { dependency Other }
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	threshold := newCouplingBetweenObjects()
	if err := threshold.(rule.Configurable).Configure(rule.Properties{"maximum": "1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{threshold}}})
	if len(violations) != 1 {
		t.Fatalf("CouplingBetweenObjects violations = %d, want 1", len(violations))
	}
}

func TestLackOfCohesionThresholdReportsThroughAnalyze(t *testing.T) {
	f, err := model.ParseSource("cohesion.go", []byte(`package sample

type Service struct {
	first  int
	second int
}

func (s *Service) AddFirst(value int) {
	if value > 0 { s.first += value }
}
func (s *Service) AddSecond(value int) {
	if value > 0 { s.second += value }
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	threshold := newLackOfCohesionOfMethods()
	if err := threshold.(rule.Configurable).Configure(rule.Properties{"maximum": "1"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	violations := rule.Analyze(f, []*rule.RuleSet{{Rules: []rule.Rule{threshold}}})
	if len(violations) != 1 {
		t.Fatalf("LackOfCohesionOfMethods violations = %d, want 1", len(violations))
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

func TestTypeReferencesInGenericFields(t *testing.T) {
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
		f, err := model.ParseSource("generic.go", []byte("package sample; type Service struct { field "+tc.input+" }"))
		if err != nil {
			t.Fatal(err)
		}
		got := model.TypeReferences(f.Classes[0].Fields[0].TypeExpr)
		if len(got) != len(tc.want) {
			t.Fatalf("TypeReferences(%q) = %v, want %v", tc.input, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("TypeReferences(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
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
