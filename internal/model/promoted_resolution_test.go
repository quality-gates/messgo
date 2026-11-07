package model

import (
	"go/ast"
	"testing"
)

type promotedResolutionCase struct {
	name, source, receiver string
	used, unused           []MemberKey
}

func TestPromotedCallResolution(t *testing.T) {
	for _, tc := range []promotedResolutionCase{
		{
			name: "pointer and generic embedding",
			source: `package p
type Worker[T any] struct{}
func (*Worker[T]) Run() {}
type Middle[T any] struct{ *Worker[T] }
type Root struct{ Middle[int] }
func probe(r Root) { r.Run() }
`,
			receiver: "Worker",
			used:     []MemberKey{{Type: "Root", Name: "Middle"}, {Type: "Middle", Name: "Worker"}, {Type: "Worker", Name: "Run"}},
		},
		{
			name: "ambiguous siblings",
			source: `package p
type Left struct{}
func (Left) Run() {}
type Right struct{}
func (Right) Run() {}
type Root struct{ Left; Right }
func probe(r Root) { r.Run() }
`,
			unused: []MemberKey{{Type: "Root", Name: "Left"}, {Type: "Root", Name: "Right"}, {Type: "Left", Name: "Run"}, {Type: "Right", Name: "Run"}},
		},
		{
			name: "ambiguous diamond",
			source: `package p
type Base struct{}
func (Base) Run() {}
type Left struct{ *Base }
type Right struct{ *Base }
type Root struct{ Left; Right }
func probe(r Root) { r.Run() }
`,
			unused: []MemberKey{{Type: "Root", Name: "Left"}, {Type: "Root", Name: "Right"}, {Type: "Base", Name: "Run"}},
		},
		{
			name: "ambiguous diamond above the method",
			source: `package p
type Base struct{}
func (Base) Run() {}
type Shared struct{ Base }
type Left struct{ Shared }
type Right struct{ Shared }
type Root struct{ Left; Right }
func probe(r Root) { r.Run() }
`,
			unused: []MemberKey{{Type: "Root", Name: "Left"}, {Type: "Root", Name: "Right"}, {Type: "Shared", Name: "Base"}, {Type: "Base", Name: "Run"}},
		},
		{
			name: "shared embedded type has a unique shallower path",
			source: `package p
type Base struct{}
func (Base) Run() {}
type Shared struct{ Base }
type Left struct{ Shared }
type Right struct{ Base }
type Root struct{ Left; Right }
func probe(r Root) { r.Run() }
`,
			receiver: "Base",
			used:     []MemberKey{{Type: "Root", Name: "Right"}, {Type: "Right", Name: "Base"}, {Type: "Base", Name: "Run"}},
			unused:   []MemberKey{{Type: "Root", Name: "Left"}, {Type: "Left", Name: "Shared"}},
		},
		{
			name: "previously visited embedded type does not hide a sibling",
			source: `package p
type Shared struct{}
type Other struct{}
func (Other) Run() {}
type Middle struct{ Shared; Other }
type Root struct{ Shared; Middle }
func probe(r Root) { r.Run() }
`,
			receiver: "Other",
			used:     []MemberKey{{Type: "Root", Name: "Middle"}, {Type: "Middle", Name: "Other"}, {Type: "Other", Name: "Run"}},
			unused:   []MemberKey{{Type: "Root", Name: "Shared"}, {Type: "Middle", Name: "Shared"}},
		},
		{
			name: "duplicate embedded type does not hide a later field",
			source: `package p
type Shared struct{}
type Writer struct{}
func (Writer) Run() {}
type Left struct{ Shared }
type Right struct{ Shared; Writer }
type Root struct{ Left; Right }
func probe(r Root) { r.Run() }
`,
			receiver: "Writer",
			used:     []MemberKey{{Type: "Root", Name: "Right"}, {Type: "Right", Name: "Writer"}, {Type: "Writer", Name: "Run"}},
			unused:   []MemberKey{{Type: "Root", Name: "Left"}, {Type: "Left", Name: "Shared"}, {Type: "Right", Name: "Shared"}},
		},
		{
			name: "external embedding does not hide a deeper local method",
			source: `package p
import "io"
type Worker struct{}
func (Worker) Run() {}
type Branch struct{ Worker }
type Root struct{ Branch; io.Reader }
func probe(r Root) { r.Run() }
`,
			receiver: "Worker",
			used:     []MemberKey{{Type: "Root", Name: "Branch"}, {Type: "Branch", Name: "Worker"}, {Type: "Worker", Name: "Run"}},
			unused:   []MemberKey{{Type: "Root", Name: "Reader"}},
		},
		{
			name: "embedded interface competes with a concrete method",
			source: `package p
type Action interface{ Run() }
type Other struct{}
func (Other) Run() {}
type Root struct{ Other; Action }
func probe(r Root) { r.Run() }
`,
			unused: []MemberKey{{Type: "Root", Name: "Other"}, {Type: "Root", Name: "Action"}, {Type: "Other", Name: "Run"}},
		},
		{
			name: "recursive pointer with absent member",
			source: `package p
type Root struct{ *Root }
func probe(r Root) { r.Run() }
`,
		},
		{
			name: "embedded interface dispatch remains unknown",
			source: `package p
type Action interface { Run() }
type Root struct { Action }
func probe(r Root) { r.Run() }
`,
			used: []MemberKey{{Type: "Root", Name: "Action"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertPromotedResolution(t, tc)
		})
	}
}

func assertPromotedResolution(t *testing.T, tc promotedResolutionCase) {
	t.Helper()
	f, err := ParseSource("probe.go", []byte(tc.source))
	if err != nil {
		t.Fatal(err)
	}
	got := resolveProbeCall(t, f)
	if tc.receiver == "" && got != nil {
		t.Errorf("resolved %s.%s, want unknown", got.Receiver, got.Name)
	} else if tc.receiver != "" && (got == nil || got.Receiver != tc.receiver) {
		t.Errorf("resolved %v, want method of %s", got, tc.receiver)
	}
	for _, key := range tc.used {
		if !f.MemberSelectedForType(key.Type, key.Name) {
			t.Errorf("selected member %v not recorded", key)
		}
	}
	for _, key := range tc.unused {
		if f.MemberSelectedForType(key.Type, key.Name) {
			t.Errorf("unselected member %v recorded as selected", key)
		}
	}
}

func resolveProbeCall(t *testing.T, f *File) *Function {
	t.Helper()
	var probe *Function
	for _, fn := range f.Functions {
		if fn.Name == "probe" {
			probe = fn
		}
	}
	if probe == nil {
		t.Fatal("missing probe")
	}
	var call *ast.CallExpr
	ast.Inspect(probe.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			call = c
		}
		return true
	})
	if call == nil {
		t.Fatal("missing call")
	}
	return ResolveCall(probe, call)
}
