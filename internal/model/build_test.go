package model

import "testing"

const sampleSrc = `package sample

// Greeter greets.
type Greeter struct {
	Name    string
	private int
	Embedded
}

type Embedded struct{}

// Speaker speaks.
type Speaker interface {
	Embedded
	Say(msg string) (string, error)
}

func Free(a int, b string) bool { return true }

// Hello greets by name.
func (g *Greeter) Hello(loud bool) string { return g.Name }

func (g Greeter) value() {}
`

func parseSample(t *testing.T) *File {
	t.Helper()
	f, err := ParseSource("sample.go", []byte(sampleSrc))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	return f
}

func TestParseSourceErrorsOnInvalidGo(t *testing.T) {
	if _, err := ParseSource("bad.go", []byte("package p\nfunc (")); err == nil {
		t.Fatal("expected parse error for invalid source, got nil")
	}
}

func TestParsePackageAndClasses(t *testing.T) {
	f := parseSample(t)
	if f.Package != "sample" {
		t.Errorf("Package = %q, want %q", f.Package, "sample")
	}
	if len(f.Classes) != 2 {
		t.Fatalf("Classes = %d, want 2", len(f.Classes))
	}
	greeter := findGreeter(t, f)
	if !greeter.Exported {
		t.Error("Greeter should be exported")
	}
	if greeter.DocComment == "" {
		t.Error("Greeter should carry its doc comment")
	}
	assertSpan(t, "Greeter", greeter.Line, greeter.EndLine, greeter.File, 4, 8, f)
}

func TestParseFieldsAndEmbeds(t *testing.T) {
	f := parseSample(t)
	greeter := findGreeter(t, f)
	if len(greeter.Fields) != 3 {
		t.Fatalf("Fields = %d, want 3", len(greeter.Fields))
	}
	byName := map[string]*Field{}
	for _, fld := range greeter.Fields {
		byName[fld.Name] = fld
	}
	assertFieldMeta(t, byName["Name"], "string", true, 5, true)
	assertFieldMeta(t, byName["private"], "int", false, 6, true)
	assertFieldMeta(t, byName["Embedded"], "Embedded", true, 7, false)
	if len(greeter.Embeds) != 1 || greeter.Embeds[0] != "Embedded" {
		t.Errorf("Embeds = %v, want [Embedded]", greeter.Embeds)
	}
}

func TestParseInterfaceMethods(t *testing.T) {
	f := parseSample(t)
	if len(f.Interfaces) != 1 {
		t.Fatalf("Interfaces = %d, want 1", len(f.Interfaces))
	}
	iface := f.Interfaces[0]
	if iface.Name != "Speaker" {
		t.Errorf("interface name = %q, want Speaker", iface.Name)
	}
	assertSpan(t, "Speaker", iface.Line, iface.EndLine, iface.File, 13, 16, f)
	if len(iface.Methods) != 1 || iface.Methods[0].Name != "Say" {
		t.Fatalf("interface methods = %+v, want one Say method", iface.Methods)
	}
	say := iface.Methods[0]
	if !say.IsMethod() || say.NodeType() != TypeMethod {
		t.Errorf("Say isMethod=%v NodeType=%q", say.IsMethod(), say.NodeType())
	}
	assertSpan(t, "Say", say.Line, say.EndLine, say.File, 15, 15, f)
	if len(say.Params) != 1 || len(say.Results) != 2 {
		t.Fatalf("Say params=%d results=%d", len(say.Params), len(say.Results))
	}
	assertParamMeta(t, say.Params[0], "string", 15, true)
	assertParamMeta(t, say.Results[0], "string", 15, false)
	assertParamMeta(t, say.Results[1], "error", 15, false)
	if len(iface.Embeds) != 1 || iface.Embeds[0] != "Embedded" {
		t.Errorf("interface Embeds = %v, want [Embedded]", iface.Embeds)
	}
}

func TestExprStringPreservesArrayLenAndChanDir(t *testing.T) {
	f, err := ParseSource("types.go", []byte(`package sample
func f(a [3]int, b []int, c <-chan int, d chan<- int, e chan int) {}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(f.Functions[0].Params))
	for i, p := range f.Functions[0].Params {
		got[i] = p.Type
	}
	want := []string{"[3]int", "[]int", "<-chan int", "chan<- int", "chan int"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("param %d type = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseFreeFunction(t *testing.T) {
	f := parseSample(t)
	if len(f.Functions) != 1 || f.Functions[0].Name != "Free" {
		t.Fatalf("free Functions = %+v, want one Free", f.Functions)
	}
	free := f.Functions[0]
	if free.IsMethod() || free.NodeType() != TypeFunction {
		t.Errorf("Free isMethod=%v NodeType=%q", free.IsMethod(), free.NodeType())
	}
	if len(free.Params) != 2 || len(free.Results) != 1 {
		t.Fatalf("Free signature params=%d results=%d", len(free.Params), len(free.Results))
	}
	assertSpan(t, "Free", free.Line, free.EndLine, free.File, 18, 18, f)
	assertParamMeta(t, free.Params[0], "int", 18, true)
	assertParamMeta(t, free.Params[1], "string", 18, true)
	assertParamMeta(t, free.Results[0], "bool", 18, false)
}

// findGreeter returns the Greeter class parsed from the sample source.
func findGreeter(t *testing.T, f *File) *Class {
	t.Helper()
	for _, c := range f.Classes {
		if c.Name == "Greeter" {
			return c
		}
	}
	t.Fatal("Greeter class not found")
	return nil
}

func TestParseGreeterMethods(t *testing.T) {
	f := parseSample(t)
	greeter := findGreeter(t, f)
	if len(greeter.Methods) != 2 {
		t.Fatalf("Greeter methods = %d, want 2", len(greeter.Methods))
	}
	var hello *Function
	for _, m := range greeter.Methods {
		if m.Name == "Hello" {
			hello = m
		}
	}
	if hello == nil {
		t.Fatal("Hello method not found")
	}
	assertSpan(t, "Hello", hello.Line, hello.EndLine, hello.File, 21, 21, f)
	if !hello.IsMethod() || hello.Receiver != "Greeter" || hello.RecvName != "g" || hello.NodeType() != TypeMethod {
		t.Errorf("Hello isMethod=%v Recv=%q RecvName=%q NodeType=%q", hello.IsMethod(), hello.Receiver, hello.RecvName, hello.NodeType())
	}
	if hello.Class != greeter {
		t.Error("Hello.Class should point back to its Greeter class")
	}
}

func assertSpan(t *testing.T, name string, line, endLine int, file *File, wantLine, wantEnd int, wantFile *File) {
	t.Helper()
	if line != wantLine || endLine != wantEnd || file != wantFile {
		t.Errorf("%s span = line %d end %d file %v, want %d/%d/%v", name, line, endLine, file, wantLine, wantEnd, wantFile)
	}
}

func assertFieldMeta(t *testing.T, fld *Field, wantType string, wantExported bool, wantLine int, wantIdent bool) {
	t.Helper()
	if fld == nil {
		t.Fatal("field is nil")
	}
	if fld.Type != wantType || fld.Exported != wantExported || fld.Line != wantLine || (fld.Ident != nil) != wantIdent {
		t.Errorf("field %s meta = %+v", fld.Name, fld)
	}
}

func assertParamMeta(t *testing.T, p *Parameter, wantType string, wantLine int, wantIdent bool) {
	t.Helper()
	if p == nil {
		t.Fatal("param is nil")
	}
	if p.Type != wantType || p.Line != wantLine || p.Field == nil || (p.Ident != nil) != wantIdent {
		t.Errorf("param %s meta = %+v", p.Name, p)
	}
}

func TestParseAllFuncsIncludesFunctionsAndMethods(t *testing.T) {
	f := parseSample(t)
	if len(f.AllFuncs) != 3 {
		t.Errorf("AllFuncs = %d, want 3 (Free, Hello, value)", len(f.AllFuncs))
	}
}

func TestExprStringRendersCommonTypes(t *testing.T) {
	src := `package p
func f(a *int, b []string, c map[string]int, e chan int, d ...bool) {}
`
	f, err := ParseSource("x.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	params := f.Functions[0].Params
	want := []string{"*int", "[]string", "map[string]int", "chan int", "...bool"}
	if len(params) != len(want) {
		t.Fatalf("params = %d, want %d", len(params), len(want))
	}
	for i, w := range want {
		if params[i].Type != w {
			t.Errorf("param %d type = %q, want %q", i, params[i].Type, w)
		}
	}
}
