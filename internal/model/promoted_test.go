package model

import (
	"testing"
)

func TestPromotedMembersNilAndEmpty(t *testing.T) {
	var c *Class
	f, m := c.PromotedMembers()
	if len(f) != 0 || len(m) != 0 {
		t.Errorf("nil class PromotedMembers() = %v, %v; want empty", f, m)
	}

	c = &Class{}
	f, m = c.PromotedMembers()
	if len(f) != 0 || len(m) != 0 {
		t.Errorf("empty class PromotedMembers() = %v, %v; want empty", f, m)
	}
}

func TestPromotedMembersTransitiveAndInterface(t *testing.T) {
	const src = `package sample

type Closer interface {
	Close() error
}

type Bottom struct {
	baseField int
}

func (b *Bottom) BaseMethod() int { return b.baseField }

type Middle struct {
	Bottom
	midField string
}

func (m *Middle) MidMethod() string { return m.midField }

type Top struct {
	*Middle
	Closer
	topField bool
}

func (t *Top) TopMethod() bool { return t.topField }
`
	f, err := ParseSource("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	var top *Class
	for _, c := range f.Classes {
		if c.Name == "Top" {
			top = c
			break
		}
	}
	if top == nil {
		t.Fatal("Top class not found")
	}

	fields, methods := top.PromotedMembers()
	if !fields["baseField"] || !fields["midField"] {
		t.Errorf("expected promoted fields baseField and midField; got %v", fields)
	}
	if !methods["BaseMethod"] || !methods["MidMethod"] || !methods["Close"] {
		t.Errorf("expected promoted methods BaseMethod, MidMethod, Close; got %v", methods)
	}
	// Direct members of Top should not be marked as promoted
	if fields["topField"] {
		t.Errorf("direct field topField should not be in promoted fields: %v", fields)
	}
	if methods["TopMethod"] {
		t.Errorf("direct method TopMethod should not be in promoted methods: %v", methods)
	}
}

func TestPromotedMembersShadowing(t *testing.T) {
	const src = `package sample

type Inner struct {
	x int
	y int
}

func (i Inner) ShadowedMethod() int { return i.x }
func (i Inner) OtherMethod() int    { return i.y }

type Outer struct {
	Inner
	x int // shadows Inner.x
}

func (o Outer) ShadowedMethod() int { return o.x } // shadows Inner.ShadowedMethod
`
	f, err := ParseSource("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	var outer *Class
	for _, c := range f.Classes {
		if c.Name == "Outer" {
			outer = c
			break
		}
	}
	if outer == nil {
		t.Fatal("Outer class not found")
	}

	fields, methods := outer.PromotedMembers()
	if fields["x"] {
		t.Errorf("Outer.x should shadow Inner.x; got promoted fields %v", fields)
	}
	if !fields["y"] {
		t.Errorf("Inner.y should be promoted; got promoted fields %v", fields)
	}
	if methods["ShadowedMethod"] {
		t.Errorf("Outer.ShadowedMethod should shadow Inner.ShadowedMethod; got promoted methods %v", methods)
	}
	if !methods["OtherMethod"] {
		t.Errorf("Inner.OtherMethod should be promoted; got promoted methods %v", methods)
	}
}

func TestPromotedMembersCyclic(t *testing.T) {
	const src = `package sample

type Node struct {
	*Node
	value int
}
`
	f, err := ParseSource("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}

	node := f.Classes[0]
	// Must terminate cleanly without infinite loop.
	fields, methods := node.PromotedMembers()
	if fields["value"] {
		t.Errorf("direct field value should not be in promoted fields: %v", fields)
	}
	_ = methods
}
