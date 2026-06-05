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
	var greeter *Class
	for _, c := range f.Classes {
		if c.Name == "Greeter" {
			greeter = c
		}
	}
	if greeter == nil {
		t.Fatal("Greeter class not found")
	}
	if !greeter.Exported {
		t.Error("Greeter should be exported")
	}
	if greeter.DocComment == "" {
		t.Error("Greeter should carry its doc comment")
	}
}

func TestParseFieldsAndEmbeds(t *testing.T) {
	f := parseSample(t)
	var greeter *Class
	for _, c := range f.Classes {
		if c.Name == "Greeter" {
			greeter = c
		}
	}
	// Two named fields (Name, private) plus the embedded type.
	if len(greeter.Fields) != 3 {
		t.Fatalf("Fields = %d, want 3", len(greeter.Fields))
	}
	byName := map[string]*Field{}
	for _, fld := range greeter.Fields {
		byName[fld.Name] = fld
	}
	if got := byName["Name"]; got == nil || got.Type != "string" || !got.Exported {
		t.Errorf("Name field = %+v, want exported string", got)
	}
	if got := byName["private"]; got == nil || got.Exported {
		t.Errorf("private field should be unexported, got %+v", got)
	}
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
	if len(iface.Methods) != 1 || iface.Methods[0].Name != "Say" {
		t.Fatalf("interface methods = %+v, want one Say method", iface.Methods)
	}
	say := iface.Methods[0]
	if len(say.Params) != 1 || say.Params[0].Type != "string" {
		t.Errorf("Say params = %+v, want one string param", say.Params)
	}
	if len(say.Results) != 2 {
		t.Errorf("Say results = %d, want 2", len(say.Results))
	}
	if len(iface.Embeds) != 1 || iface.Embeds[0] != "Embedded" {
		t.Errorf("interface Embeds = %v, want [Embedded]", iface.Embeds)
	}
}

func TestParseFunctionsAndMethods(t *testing.T) {
	f := parseSample(t)
	if len(f.Functions) != 1 || f.Functions[0].Name != "Free" {
		t.Fatalf("free Functions = %+v, want one Free", f.Functions)
	}
	free := f.Functions[0]
	if free.IsMethod() {
		t.Error("Free should not be a method")
	}
	if free.NodeType() != TypeFunction {
		t.Errorf("Free NodeType = %q, want function", free.NodeType())
	}
	if len(free.Params) != 2 || len(free.Results) != 1 {
		t.Errorf("Free signature params=%d results=%d, want 2/1", len(free.Params), len(free.Results))
	}

	// Methods are attached to their owning class and excluded from Functions.
	var greeter *Class
	for _, c := range f.Classes {
		if c.Name == "Greeter" {
			greeter = c
		}
	}
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
	if !hello.IsMethod() || hello.Receiver != "Greeter" {
		t.Errorf("Hello receiver = %q (isMethod=%v), want Greeter/true", hello.Receiver, hello.IsMethod())
	}
	if hello.RecvName != "g" {
		t.Errorf("Hello RecvName = %q, want g", hello.RecvName)
	}
	if hello.NodeType() != TypeMethod {
		t.Errorf("Hello NodeType = %q, want method", hello.NodeType())
	}
	if hello.Class != greeter {
		t.Error("Hello.Class should point back to its Greeter class")
	}

	// AllFuncs holds both free functions and methods.
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
