package model

import "testing"

func TestFunctionHasGoto(t *testing.T) {
	src := []byte(`package sample

func jump(flag bool) {
	if flag {
		goto done
	}
done:
	return
}

func stay() {
	return
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	if !f.Functions[0].HasGoto() {
		t.Fatalf("%s HasGoto() = false, want true", f.Functions[0].Name)
	}
	if f.Functions[1].HasGoto() {
		t.Fatalf("%s HasGoto() = true, want false", f.Functions[1].Name)
	}
}

func TestFunctionLoopConditionCalls(t *testing.T) {
	src := []byte(`package sample

func scan(items []int) {
	for i := 0; i < len(items); i++ {
		_ = i
	}
	for cap(items) > 0 {
		break
	}
	for _, item := range items {
		_ = item
	}
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	calls := f.Functions[0].LoopConditionCalls(map[string]bool{"len": true, "cap": true})
	if len(calls) != 2 {
		t.Fatalf("LoopConditionCalls count = %d, want 2", len(calls))
	}
	if calls[0].Name != "len" || calls[0].Line != 4 {
		t.Fatalf("first call = %+v, want len at line 4", calls[0])
	}
	if calls[1].Name != "cap" || calls[1].Line != 7 {
		t.Fatalf("second call = %+v, want cap at line 7", calls[1])
	}
}

func TestFunctionCalls(t *testing.T) {
	src := []byte(`package sample

func exit() {
	os.Exit(1)
	syscall.Exit(1)
	println("debug")
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	calls := f.Functions[0].Calls()
	want := []Call{
		{Name: "os.Exit", Line: 4},
		{Name: "syscall.Exit", Line: 5},
		{Name: "println", Line: 6},
	}
	if len(calls) != len(want) {
		t.Fatalf("Calls() = %+v, want %+v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("Calls()[%d] = %+v, want %+v", i, calls[i], want[i])
		}
	}
}

func TestFunctionStatementQueriesReportSourceLines(t *testing.T) {
	src := []byte(`package sample

func inspect(err error, xs []int) {
	if err != nil {
	} else {
		_ = err
	}
	if value = len(xs); value > 0 {
		_ = value
	}
	_ = map[string]int{"a": 1, "a": 2}
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	fn := f.Functions[0]

	if got := fn.EmptyNilCheckBlockLines(); len(got) != 1 || got[0] != 4 {
		t.Fatalf("EmptyNilCheckBlockLines() = %v, want [4]", got)
	}
	if got := fn.ElseBlockLines(); len(got) != 1 || got[0] != 5 {
		t.Fatalf("ElseBlockLines() = %v, want [5]", got)
	}
	assigns := fn.IfAssignmentInitPositions()
	if len(assigns) != 1 || assigns[0].Line != 8 || assigns[0].Column != 5 {
		t.Fatalf("IfAssignmentInitPositions() = %+v, want line 8 column 5", assigns)
	}
	dups := fn.DuplicateLiteralKeys()
	if len(dups) != 1 || dups[0].Display != `"a"` || dups[0].FirstLine != 11 || dups[0].Line != 11 {
		t.Fatalf("DuplicateLiteralKeys() = %+v, want duplicate string key on line 11", dups)
	}
}

func TestFileAndIdentifierQueries(t *testing.T) {
	src := []byte(`package sample

var global int

type item struct {
	used int
}

func use(x int) {
	var local int
	_ = item{used: x}
	local = x
	global = local
	unread := 1
	_ = global
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	vars := f.PackageVars()
	if len(vars) != 1 || vars[0].Name != "global" || vars[0].Line != 3 {
		t.Fatalf("PackageVars() = %+v, want global at line 3", vars)
	}
	mutated := f.MutatedPackageGlobals()
	if !mutated["global"] {
		t.Fatalf("MutatedPackageGlobals() = %v, want global", mutated)
	}
	selected := f.SelectedMemberNames()
	if !selected["used"] {
		t.Fatalf("SelectedMemberNames() = %v, want used", selected)
	}
	fn := f.Functions[0]
	locals := fn.LocalVariables()
	if len(locals) != 2 || locals[0].Name != "local" || locals[0].Line != 10 || locals[1].Name != "unread" || locals[1].Line != 14 {
		t.Fatalf("LocalVariables() = %+v, want local line 10 and unread line 14", locals)
	}
	reads := fn.IdentifierReads()
	if !reads["x"] || !reads["local"] || reads["unread"] {
		t.Fatalf("IdentifierReads() = %v, want x/local read and unread not read", reads)
	}
}

func TestFunctionReceiverQueries(t *testing.T) {
	src := []byte(`package sample

type counter struct {
	value int
	other int
}

func (c *counter) Value() int {
	return c.value
}

func (c *counter) SetValue(v int) {
	c.value = v
}

func (c *counter) Touch() {
	c.value++
	c.SetValue(c.other)
}
`)
	f, err := ParseSource("query.go", src)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	class := f.Classes[0]
	fields := map[string]bool{"value": true, "other": true}
	methods := map[string]int{}
	for i, method := range class.Methods {
		methods[method.Name] = i
	}

	if got := class.Methods[0].AccessorField(fields); got != "value" {
		t.Fatalf("Value AccessorField() = %q, want value", got)
	}
	if got := class.Methods[1].AccessorField(fields); got != "value" {
		t.Fatalf("SetValue AccessorField() = %q, want value", got)
	}
	usedFields, calledMethods := class.Methods[2].ReceiverUses(fields, methods)
	if len(usedFields) != 2 || usedFields[0] != "value" || usedFields[1] != "other" {
		t.Fatalf("ReceiverUses fields = %v, want [value other]", usedFields)
	}
	if len(calledMethods) != 1 || calledMethods[0] != "SetValue" {
		t.Fatalf("ReceiverUses methods = %v, want [SetValue]", calledMethods)
	}
}
