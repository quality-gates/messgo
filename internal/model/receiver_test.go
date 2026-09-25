package model

import (
	"fmt"
	"testing"
)

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

	if got := AccessorField(class.Methods[0], fields); got != "value" {
		t.Fatalf("Value AccessorField() = %q, want value", got)
	}
	if got := AccessorField(class.Methods[1], fields); got != "value" {
		t.Fatalf("SetValue AccessorField() = %q, want value", got)
	}
	if got := AccessorField(class.Methods[2], fields); got != "" {
		t.Fatalf("multi-statement AccessorField() = %q, want empty", got)
	}
	if got := AccessorField(&Function{}, fields); got != "" {
		t.Fatalf("nil body AccessorField() = %q, want empty", got)
	}
	if u, c := ReceiverUses(&Function{}, fields, methods); u != nil || c != nil {
		t.Fatalf("nil body ReceiverUses = %v, %v, want nil, nil", u, c)
	}
	if u, c := ReceiverUses(&Function{RecvName: "_", Body: class.Methods[2].Body}, fields, methods); u != nil || c != nil {
		t.Fatalf("blank receiver ReceiverUses = %v, %v, want nil, nil", u, c)
	}
	if u, c := ReceiverUses(&Function{RecvName: "", Body: class.Methods[2].Body}, fields, methods); u != nil || c != nil {
		t.Fatalf("empty receiver ReceiverUses = %v, %v, want nil, nil", u, c)
	}
}

func TestReceiverUsesCollectsFieldsAndSiblingCalls(t *testing.T) {
	f, err := ParseSource("query.go", []byte(`package sample

type counter struct {
	value int
	other int
}

func (c *counter) SetValue(v int) {
	c.value = v
}

func (c *counter) Touch() {
	c.value++
	c.SetValue(c.other)
	c.value--
}
`))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	fields := map[string]bool{"value": true, "other": true}
	methods := map[string]int{"SetValue": 0, "Touch": 1}
	used, called := ReceiverUses(f.Classes[0].Methods[1], fields, methods)
	if got := fmt.Sprint(used, called); got != "[value other] [SetValue]" {
		t.Fatalf("ReceiverUses(Touch) = %s, want [value other] [SetValue]", got)
	}
}
