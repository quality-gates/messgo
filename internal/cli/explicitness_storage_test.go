package cli

import "testing"

func TestExplicitnessDistinguishesArrayAndSliceFieldsOfValueParameter(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Holder struct { Counts [2]int; Items []int }
func Scalar(h Holder) {
 h.Counts[0]++
}
func Shared(h Holder) {
 h.Items[0]++
}
`})
	assertFindings(t, got, "a.go:7 ImplicitOutput Shared: write through parameter h")
}

func TestExplicitnessDistinguishesNestedArrayAndSliceStructElements(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Inner struct { Count int; Items []int }
type Holder struct { Boxes [2]Inner; Rows []Inner }
func Scalar(h Holder) {
 h.Boxes[0].Count++
}
func SharedArrayMember(h Holder) {
 h.Boxes[0].Items[0]++
}
func SharedSliceElement(h Holder) {
 h.Rows[0].Count++
}
`})
	assertFindings(t, got,
		"a.go:8 ImplicitOutput SharedArrayMember: write through parameter h",
		"a.go:11 ImplicitOutput SharedSliceElement: write through parameter h",
	)
}

func TestExplicitnessCopiesDereferencedPackageStructWithoutSharingScalars(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p
type Holder struct { Count int; Counts [2]int; Items []int }
var original = &Holder{Items: []int{1}}
var shared = *original
var literal = *(&Holder{Items: []int{1}})
`,
		"use.go": `package p
func FromName() {
 value := shared
 value.Count++
 value.Counts[0]++
 value.Items[0]++
}
func FromLiteral() {
 value := literal
 value.Count++
 value.Counts[0]++
 value.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"use.go:3 ImplicitInput FromName: package variable shared",
		"use.go:6 ImplicitOutput FromName: write through parameter value",
		"use.go:9 ImplicitInput FromLiteral: package variable literal",
		"use.go:12 ImplicitOutput FromLiteral: write through parameter value",
	)
}

func TestExplicitnessInfersCopiedPackageGlobalSelectionTypes(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p
type Holder struct { Count int; Items []int }
var array = [1]Holder{{Items: []int{1}}}
var fromArray = array[0]
type Wrapper struct { Value Holder }
var wrapper = Wrapper{Value: Holder{Items: []int{1}}}
var fromField = wrapper.Value
var pointers = [1]*Holder{&Holder{Items: []int{1}}}
var fromPointerArray = *pointers[0]
var typed Holder = array[0]
`,
		"use.go": `package p
func FromArray() {
 value := fromArray
 value.Count++
 value.Items[0]++
}
func FromField() {
 value := fromField
 value.Count++
 value.Items[0]++
}
func FromPointerArray() {
 value := fromPointerArray
 value.Count++
 value.Items[0]++
}
func FromTyped() {
 value := typed
 value.Count++
 value.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"use.go:3 ImplicitInput FromArray: package variable fromArray",
		"use.go:5 ImplicitOutput FromArray: write through parameter value",
		"use.go:8 ImplicitInput FromField: package variable fromField",
		"use.go:10 ImplicitOutput FromField: write through parameter value",
		"use.go:13 ImplicitInput FromPointerArray: package variable fromPointerArray",
		"use.go:15 ImplicitOutput FromPointerArray: write through parameter value",
		"use.go:18 ImplicitInput FromTyped: package variable typed",
		"use.go:20 ImplicitOutput FromTyped: write through parameter value",
	)
}

func TestExplicitnessInfersNestedPackageValueSelections(t *testing.T) {
	for _, selection := range []string{"wrapper.Values[0]", "pointer.Value", "slice[0]", `entries["one"]`} {
		t.Run(selection, func(t *testing.T) {
			source := `package p
type Holder struct { Count int; Items []int }
type Wrapper struct { Value Holder; Values [1]Holder }
var fixed = [1]Holder{{Items: []int{1}}}
var slice = []Holder{{Items: []int{1}}}
var entries = map[string]Holder{"one": {Items: []int{1}}}
var wrapper = Wrapper{Value: fixed[0], Values: fixed}
var pointer = &wrapper
var shared = ` + selection + `
func Copy() {
 value := shared
 value.Count++
 value.Items[0]++
}
`
			_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": source})
			assertFindings(t, got,
				"a.go:11 ImplicitInput Copy: package variable shared",
				"a.go:13 ImplicitOutput Copy: write through parameter value",
			)
		})
	}
}

func TestExplicitnessInfersEmbeddedPackageValueSelection(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"embedded.go": `package p
type Holder struct { Count int; Items []int }
type Outer struct { Holder }
var outer = Outer{Holder: Holder{Items: []int{1}}}
var shared = outer.Holder
func Copy() {
 value := shared
 value.Count++
 value.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"embedded.go:7 ImplicitInput Copy: package variable shared",
		"embedded.go:9 ImplicitOutput Copy: write through parameter value",
	)
}

func TestExplicitnessKeepsCopiedStructScalarsPrivateAndSliceFieldsShared(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p
type Holder struct { Count int; Items []int }
var shared = Holder{Items: []int{1}}
func Get() Holder { return shared }
`,
		"use.go": `package p
func Direct() {
 copy := shared
 copy.Count++
 copy.Items[0]++
}
func ViaGetter() {
 copy := Get()
 copy.Count++
 copy.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"state.go:4 ImplicitInput Get: package variable shared",
		"use.go:3 ImplicitInput Direct: package variable shared",
		"use.go:5 ImplicitOutput Direct: write through parameter copy",
		"use.go:8 ImplicitInput ViaGetter: package variable shared",
		"use.go:10 ImplicitOutput ViaGetter: write through parameter copy",
	)
}

func TestExplicitnessKeepsCopiedConstructorGlobalScalarsPrivate(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p
type Holder struct { Count int; Items []int }
func Build() Holder { return Holder{Items: []int{1}} }
var shared = Build()
`,
		"use.go": `package p
func Copy() {
 value := shared
 value.Count++
 value.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"use.go:3 ImplicitInput Copy: package variable shared",
		"use.go:5 ImplicitOutput Copy: write through parameter value",
	)
}

func TestExplicitnessKeepsCopiedPackageAliasScalarsPrivate(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p
type Holder struct { Count int; Items []int }
var shared = Holder{Items: []int{1}}
var alias = shared
`,
		"use.go": `package p
func Copy() {
 value := alias
 value.Count++
 value.Items[0]++
}
`,
	})
	assertFindings(t, got,
		"use.go:3 ImplicitInput Copy: package variable alias",
		"use.go:5 ImplicitOutput Copy: write through parameter value",
	)
}

func TestExplicitnessSharesLateStorageAcrossExistingAliases(t *testing.T) {
	tests := []struct {
		name, source, finding string
	}{
		{"pointer", `package p
type Box struct { Items []int }
func Change(items []int) {
 box := &Box{}
 alias := box
 box.Items = items
 alias.Items[0] = 9
}
`, "a.go:7 ImplicitOutput Change: write through parameter alias"},
		{"reverse pointer", `package p
type Box struct { Items []int }
func Change(items []int) {
 box := &Box{}
 alias := box
 alias.Items = items
 box.Items[0] = 9
}
`, "a.go:7 ImplicitOutput Change: write through parameter box"},
		{"map", `package p
func Change(items []int) {
 box := map[string][]int{}
 alias := box
 alias["value"] = items
 box["value"][0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter box"},
		{"slice", `package p
func Change(items []int) {
 box := make([][]int, 1)
 alias := box
 box[0] = items
 alias[0][0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter alias"},
		{"copy", `package p
func Change(items [][]int) {
 box := make([][]int, 1)
 alias := box
 copy(box, items)
 alias[0][0] = 9
}
`, "a.go:6 ImplicitOutput Change: write through parameter alias"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": test.source})
			assertFindings(t, got, test.finding)
		})
	}
}

func TestExplicitnessReplacesPrivateStorageWithoutOldOrigins(t *testing.T) {
	tests := []struct{ name, source string }{
		{"pointer", `package p
type Box struct { Items []int }
func Calculate(items []int) []int {
 box := &Box{}
 alias := box
 box.Items = items
 alias.Items = []int{0}
 box.Items[0] = 9
 return box.Items
}
`},
		{"map", `package p
func Calculate(items []int) []int {
 box := map[string][]int{}
 alias := box
 box["value"] = items
 alias["value"] = []int{0}
 box["value"][0] = 9
 return box["value"]
}
`},
		{"slice", `package p
func Calculate(items []int) []int {
 box := make([][]int, 1)
 alias := box
 box[0] = items
 alias[0] = []int{0}
 box[0][0] = 9
 return box[0]
}
`},
		{"literal false", `package p
func Calculate(items []int) []int {
 box := make([][]int, 1)
 box[0] = []int{0}
 if false { box[0] = items }
 box[0][0] = 9
 return box[0]
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

func TestExplicitnessExecutesHelperStoresAndKeepsAllocationsSeparate(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct { Items []int }
func Put(box *Box, items []int) { box.Items = items }
func Fresh() *Box { return &Box{Items: []int{0}} }
func Wrap() *Box { return Fresh() }
func Change(items []int) {
 box := Fresh()
 alias := box
 Put(box, items)
 alias.Items[0] = 9
}
func Calculate(items []int) []int {
 left := Wrap()
 right := Wrap()
 Put(left, items)
 right.Items[0] = 9
 return right.Items
}
func Replace(items []int) []int {
 box := Fresh()
 Put(box, items)
 Put(box, []int{0})
 box.Items[0] = 9
 return box.Items
}
`})
	assertFindings(t, got,
		"a.go:3 ImplicitOutput Put: write through parameter box",
		"a.go:10 ImplicitOutput Change: write through parameter alias",
	)
}

func TestExplicitnessKeepsDeferredSettersOutOfTheirDefiner(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Calculate(items []int) []int {
 local := []int{0}
 setter := func() { local = items }
 _ = setter
 local[0] = 9
 return local
}
func Change(items []int) {
 local := []int{0}
 setter := func() { local = items }
 setter()
 local[0] = 9
}
func Dead(items []int) []int {
 local := []int{0}
 if false { local = items }
 for false { println(items) }
 local[0] = 9
 return local
}
`})
	assertFindings(t, got,
		"a.go:4 ImplicitOutput <lambda>: captured variable local",
		"a.go:11 ImplicitOutput <lambda>: captured variable local",
		"a.go:13 ImplicitOutput Change: write through parameter local",
	)
}

func TestExplicitnessReadsMutableCapturedStorageThroughAliases(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
import "slices"
func Shared(items []int) func() int {
 return func() int { return items[0] }
}
func Sorted() func() int {
 items := []int{3, 1}
 return func() int {
  before := items[0]
  slices.Sort(items)
  return before
 }
}
func Aliased() func() int {
 items := []int{3, 1}
 alias := items
 return func() int {
  before := items[0]
  alias[0] = 9
  return before
 }
}
func Fixed(value int) func() int {
 items := []int{value}
 return func() int { return items[0] }
}
`})
	assertFindings(t, got,
		"a.go:4 ImplicitInput <lambda>: captured variable items",
		"a.go:9 ImplicitInput <lambda>: captured variable items",
		"a.go:10 ImplicitOutput <lambda>: captured variable items",
		"a.go:18 ImplicitInput <lambda>: captured variable items",
		"a.go:19 ImplicitOutput <lambda>: captured variable alias",
	)
}

func TestExplicitnessUsesTheEarliestTransferredEffectLine(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Loud(value int) { println(value) }
func Relay(value int) {
 Loud(value)
 println(value + 1)
}
func Recursive(value int) {
 if value > 0 { Recursive(value - 1) } else { println(value) }
}
`})
	assertFindings(t, got,
		"a.go:2 ImplicitOutput Loud: println",
		"a.go:4 ImplicitOutput Relay: println",
		"a.go:8 ImplicitOutput Recursive: println",
	)
}

func TestExplicitnessPreservesReturnedHolderAndFieldAliases(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct { Items []int }
func Same(box *Box) *Box { return box }
func Items(box *Box) []int { return box.Items }
func Change(items []int) {
 box := &Box{}
 box.Items = items
 Same(box).Items[0] = 9
}
func ChangeField(items []int) {
 box := &Box{}
 box.Items = items
 Items(box)[0] = 9
}
func Calculate() int {
 value := 0
 alias := &value
 *alias = 9
 return value
}
`})
	assertFindings(t, got,
		"a.go:8 ImplicitOutput Change: write through call result",
		"a.go:13 ImplicitOutput ChangeField: write through call result",
	)
}

func TestExplicitnessUpdatesPrivateCopiesAndAppendedCells(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Calculate(items [][]int) []int {
 box := make([][]int, 1)
 copy(box, items)
 box[0] = []int{0}
 box[0][0] = 9
 return box[0]
}
func Change(items []int) {
 box := make([][]int, 0)
 box = append(box, items)
 box[0][0] = 9
}
func Conditional(items []int, replace bool) {
 box := make([][]int, 1)
 box[0] = items
 if replace { box[0] = []int{0} }
 box[0][0] = 9
}
`})
	assertFindings(t, got,
		"a.go:12 ImplicitOutput Change: write through parameter box",
		"a.go:18 ImplicitOutput Conditional: write through parameter box",
	)
}

func TestExplicitnessReadsCapturedNestedStorageAndValueArrays(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct { Items []int }
func Nested() func() int {
 box := &Box{Items: []int{1}}
 alias := box.Items
 return func() int {
  before := box.Items[0]
  alias[0] = 9
  return before
 }
}
func Array() func() int {
 items := [1]int{1}
 return func() int {
  before := items[0]
  items[0] = 9
  return before
 }
}
`})
	assertFindings(t, got,
		"a.go:7 ImplicitInput <lambda>: captured variable box",
		"a.go:8 ImplicitOutput <lambda>: captured variable alias",
		"a.go:15 ImplicitInput <lambda>: captured variable items",
		"a.go:16 ImplicitOutput <lambda>: captured variable items",
	)
}

func TestExplicitnessTracksAddressableBindingsAndPointerOutParameters(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Get(src []int, dst *[]int) { *dst = src }
func Change(items []int) {
 target := []int{0}
 alias := &target
 *alias = items
 target[0] = 9
}
func ChangeThroughHelper(items []int) {
 var target []int
 Get(items, &target)
 target[0] = 9
}
func Fresh() *[]int { value := []int{0}; return &value }
func Calculate(items []int) int {
 left := Fresh()
 right := Fresh()
 *left = items
 (*right)[0] = 9
 return (*right)[0]
}
func Captured() func() int {
 value := 0
 alias := &value
 return func() int {
  before := value
  *alias = 9
  return before
 }
}
`})
	assertFindings(t, got,
		"a.go:2 ImplicitOutput Get: write through parameter dst",
		"a.go:7 ImplicitOutput Change: write through parameter target",
		"a.go:12 ImplicitOutput ChangeThroughHelper: write through parameter target",
		"a.go:26 ImplicitInput <lambda>: captured variable value",
		"a.go:27 ImplicitOutput <lambda>: captured variable alias",
	)
}

func TestExplicitnessKeepsExactFieldsAndIndexesSeparate(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct { Left, Right []int }
func Calculate(items []int) []int {
 box := &Box{Left: items, Right: []int{0}}
 alias := box
 alias.Left = []int{0}
 box.Left[0] = 9
 box.Right[0] = 9
 return box.Left
}
func ChangeField(items []int) {
 box := &Box{Left: items, Right: []int{0}}
 box.Right[0] = 9
 box.Left[0] = 9
}
func ChangeIndex(items []int) {
 box := [][]int{items, {0}}
 box[1][0] = 9
 box[0][0] = 9
}
`})
	assertFindings(t, got,
		"a.go:14 ImplicitOutput ChangeField: write through parameter box",
		"a.go:19 ImplicitOutput ChangeIndex: write through parameter box",
	)
}

func TestExplicitnessExecutesCallbackStorageInsertion(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Box struct { Items []int }
func Put(box *Box, items []int) { box.Items = items }
func Apply(box *Box, items []int, put func(*Box, []int)) { put(box, items) }
func Change(items []int) {
 box := &Box{}
 alias := box
 Apply(box, items, Put)
 alias.Items[0] = 9
}
func ChangeLiteral(items []int) {
 box := &Box{}
 alias := box
 Apply(box, items, func(target *Box, value []int) { target.Items = value })
 alias.Items[0] = 9
}
`})
	assertFindings(t, got,
		"a.go:3 ImplicitOutput Put: write through parameter box",
		"a.go:4 UnknownEffect Apply: unresolved call",
		"a.go:9 ImplicitOutput Change: write through parameter alias",
		"a.go:14 ImplicitOutput <lambda>: write through parameter target",
		"a.go:15 ImplicitOutput ChangeLiteral: write through parameter alias",
	)
}
