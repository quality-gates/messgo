package cli

import "testing"

func TestExplicitnessLocalOriginLabel(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
type Holder struct{ Items []int }
var shared = Holder{Items: []int{1}}
func FromName() {
 value := shared
 value.Items[0]++
}
`})
	assertFindings(t, got,
		"a.go:5 ImplicitInput FromName: package variable shared",
		"a.go:6 ImplicitOutput FromName: write through local value",
	)
}

func TestExplicitnessOpaqueLocalOriginLabel(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
func Change() {
 value := unknown()
 value.Items[0]++
}
`})
	assertFindings(t, got,
		"a.go:3 UnknownEffect Change: unresolved call",
		"a.go:4 ImplicitOutput Change: write through local value")
}

func TestExplicitnessReceiverAliasPolicy(t *testing.T) {
	for _, name := range []string{"explicitness", "explicitness-strict"} {
		t.Run(name, func(t *testing.T) {
			_, got := runExplicitness(t, name, map[string]string{"a.go": `package p
type T struct{ x int; items []int }
func (t *T) recvAlias() {
 r := t
 r.x = 1
}
func (t T) valueAlias() {
 r := t
 r.x = 1
 r.items[0]++
}
func (t *T) rebound() {
 r := t
 r = &T{}
 r.x = 1
}
`})
			if name == "explicitness" {
				assertFindings(t, got,
					"a.go:4 UnknownEffect T.recvAlias: receiver effect",
					"a.go:8 UnknownEffect T.valueAlias: receiver effect",
					"a.go:13 UnknownEffect T.rebound: receiver effect")
			} else {
				assertFindings(t, got,
					"a.go:4 ImplicitInput T.recvAlias: receiver t",
					"a.go:5 ImplicitOutput T.recvAlias: receiver field x",
					"a.go:8 ImplicitInput T.valueAlias: receiver t",
					"a.go:10 ImplicitOutput T.valueAlias: receiver field items",
					"a.go:13 ImplicitInput T.rebound: receiver t")
			}
		})
	}
}
