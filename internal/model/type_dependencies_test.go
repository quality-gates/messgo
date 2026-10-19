package model_test

import (
	"go/parser"
	"reflect"
	"slices"
	"testing"

	"github.com/quality-gates/messgo/internal/model"
)

func TestClassTypeDependencies(t *testing.T) {
	f, err := model.ParseSource("dependencies.go", []byte(`package sample
 type Service struct {
  *Embedded
  first, second map[Key][]*pkg.Value
  self *Service
  callback func(bool, string, int, int8, int16, int32, int64,
   uint, uint8, uint16, uint32, uint64, uintptr, byte, rune,
   float32, float64, complex64, complex128, any) error
  constraint interface { comparable }
 }
 func (Service) Process(a, b Input, rest ...Extra) (result Output, err error) { panic("fixture") }
 func (Service) Clone() *Service { var local BodyOnly; _ = local; return nil }
 func (Service) Unnamed(Input) (Output, error) { panic("fixture") }
 func unrelated(Outside) {}
 `))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Embedded", "Extra", "Input", "Key", "Output", "Value"}
	if got := model.TypeDependencies(f.Classes[0]); !reflect.DeepEqual(got, want) {
		t.Fatalf("TypeDependencies = %v, want %v", got, want)
	}
	for _, fn := range f.AllFuncs {
		for _, p := range slices.Concat(fn.Params, fn.Results) {
			if p.TypeExpr == nil {
				t.Fatalf("%s parameter %q has no type expression", fn.Name, p.Name)
			}
		}
	}
}

func TestTypeReferences(t *testing.T) {
	expr, err := parser.ParseExpr(`struct {
  field *pkg.Box[Key, []Value]
  array [size.Count]Element
  channel <-chan map[MapKey]chan<- MapValue
  callback func(argument Input, rest ...Extra) (result Output)
  nested interface { Method(param Request) Response; Embedded }
  parenthesized (Other)
  builtin int
 }`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Box", "Key", "Value", "Element", "MapKey", "MapValue", "Input", "Extra", "Output", "Request", "Response", "Embedded", "Other", "int"}
	if got := model.TypeReferences(expr); !reflect.DeepEqual(got, want) {
		t.Fatalf("TypeReferences = %v, want %v", got, want)
	}
	if got := model.TypeReferences(nil); len(got) != 0 {
		t.Fatalf("TypeReferences(nil) = %v, want no references", got)
	}
}
