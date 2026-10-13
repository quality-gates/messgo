package cli

import "testing"

func TestExplicitnessSharesAddressedFieldsAndElements(t *testing.T) {
	tests := []struct {
		name, source, finding string
	}{
		{"independent field holdout", `package p

type Box struct{ Items []int }

func Change(items []int) {
    box := &Box{}
    field := &box.Items
    *field = items
    box.Items[0] = 9
}
`, "a.go:9 ImplicitOutput Change: write through parameter box"},
		{"reverse field", `package p
type Box struct{ Items []int }
func Change(items []int) {
 box := &Box{}
 field := &box.Items
 box.Items = items
 (*field)[0] = 9
}
`, "a.go:7 ImplicitOutput Change: write through parameter field"},
		{"element", `package p
func Change(items []int) {
 local := [][]int{nil}
 element := &local[0]
 *element = items
 local[0][0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter local"},
		{"reverse element", `package p
func Change(items []int) {
 local := [][]int{nil}
 element := &local[0]
 local[0] = items
 (*element)[0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter element"},
		{"value field", `package p
type Box struct{ Items []int }
func Change(items []int) {
 box := Box{}
 field := &box.Items
 *field = items
 box.Items[0] = 9
}
`, "a.go:7 ImplicitOutput Change: write through parameter box"},
		{"value array", `package p
func Change(items []int) {
 local := [1][]int{}
 element := &local[0]
 *element = items
 local[0][0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter local"},
		{"value replacement", `package p
type Box struct{ Items []int }
func Change(items []int) {
 box := Box{}
 field := &box.Items
 box = Box{Items: items}
 (*field)[0] = 9
}
`, "a.go:7 ImplicitOutput Change: write through parameter field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, got := runExplicitness(t, "explicitness", map[string]string{"a.go": test.source})
			if code != ExitViolation {
				t.Errorf("exit = %d, want %d", code, ExitViolation)
			}
			assertFindings(t, got, test.finding)
		})
	}
}

func TestExplicitnessKeepsAddressedPrivateSlotsQuiet(t *testing.T) {
	tests := []struct{ name, source string }{
		{"field quiet twin", `package p
type Box struct{ Items []int }
func Calculate(items []int) []int {
 box := &Box{}
 field := &box.Items
 *field = []int{0}
 box.Items[0] = 9
 return box.Items
}
`},
		{"reverse field quiet twin", `package p
type Box struct{ Items []int }
func Calculate(items []int) []int {
 box := &Box{}
 field := &box.Items
 box.Items = []int{0}
 (*field)[0] = 9
 return *field
}
`},
		{"element quiet twin", `package p
func Calculate(items []int) []int {
 local := [][]int{nil}
 element := &local[0]
 *element = []int{0}
 local[0][0] = 9
 return local[0]
}
`},
		{"reverse element quiet twin", `package p
func Calculate(items []int) []int {
 local := [][]int{nil}
 element := &local[0]
 local[0] = []int{0}
 (*element)[0] = 9
 return *element
}
`},
		{"field private replacement", `package p
type Box struct{ Items []int }
func Calculate(items []int) []int {
 box := &Box{Items: items}
 field := &box.Items
 *field = []int{0}
 box.Items[0] = 9
 return box.Items
}
`},
		{"element private replacement", `package p
func Calculate(items []int) []int {
 local := [][]int{items}
 element := &local[0]
 *element = []int{0}
 local[0][0] = 9
 return local[0]
}
`},
		{"sibling field", `package p
type Box struct{ Left, Right []int }
func Calculate(items []int) []int {
 box := &Box{Right: []int{0}}
 field := &box.Left
 *field = items
 box.Right[0] = 9
 return box.Right
}
`},
		{"sibling element", `package p
func Calculate(items []int) []int {
 local := [][]int{nil, {0}}
 element := &local[0]
 *element = items
 local[1][0] = 9
 return local[1]
}
`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, got := runExplicitness(t, "explicitness", map[string]string{"a.go": test.source})
			if code != ExitSuccess {
				t.Errorf("exit = %d, want %d", code, ExitSuccess)
			}
			assertFindings(t, got)
		})
	}
}

func TestExplicitnessPreservesAddressedSlotsAcrossHelpers(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct{ Items []int }
func Put(slot *[]int, items []int) { *slot = items }
func Address(box *Box) *[]int { return &box.Items }
func Change(items []int) {
 box := &Box{}
 Put(&box.Items, items)
 box.Items[0] = 9
}
func ChangeReturned(items []int) {
 box := &Box{}
 *Address(box) = items
 box.Items[0] = 9
}
func Calculate(items []int) []int {
 box := &Box{Items: items}
 Put(Address(box), []int{0})
 box.Items[0] = 9
 return box.Items
}
`})
	assertFindings(t, got,
		"a.go:3 ImplicitOutput Put: write through parameter slot",
		"a.go:8 ImplicitOutput Change: write through parameter box",
		"a.go:13 ImplicitOutput ChangeReturned: write through parameter box",
	)
}

func TestExplicitnessPreservesAddressedNestedSlotsAndCallContexts(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct{ Items []int }
type Nested struct{ Inner Box }
func Fresh() *[]int { box := Box{Items: []int{0}}; return &box.Items }
func Address(box *Box) *[]int { return &box.Items }
func Change(items []int) {
 box := &Nested{}
 field := &box.Inner.Items
 *field = items
 box.Inner.Items[0] = 9
}
func ChangeCaller(box *Box, items []int) { *Address(box) = items }
func Conditional(items []int, replace bool) {
 box := &Box{Items: items}
 field := &box.Items
 if replace { *field = []int{0} }
 box.Items[0] = 9
}
func Calculate(items []int) int {
 left := Fresh()
 right := Fresh()
 *left = items
 (*right)[0] = 9
 return (*right)[0]
}
`})
	assertFindings(t, got,
		"a.go:10 ImplicitOutput Change: write through parameter box",
		"a.go:12 ImplicitOutput ChangeCaller: write through call result",
		"a.go:17 ImplicitOutput Conditional: write through parameter box",
	)
}

func TestExplicitnessEvaluatesCallsBeforeTakingElementAddresses(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Same(items [][]int) [][]int { return items }
func Fresh() [][]int { return [][]int{nil} }
func Change(items []int) {
 local := [][]int{nil}
 element := &Same(local)[0]
 *element = items
 local[0][0] = 9
}
func ChangeCaller(items [][]int, replacement []int) { *(&Same(items)[0]) = replacement }
func ChangeNested(items [][]int) {
 element := &Same(items)[0]
 (*element)[0] = 9
}
func Calculate() []int {
 element := &Fresh()[0]
 *element = []int{0}
 (*element)[0] = 9
 return *element
}
`})
	assertFindings(t, got,
		"a.go:8 ImplicitOutput Change: write through parameter local",
		"a.go:10 ImplicitOutput ChangeCaller: write through call result",
		"a.go:13 ImplicitOutput ChangeNested: write through parameter element",
	)
}
