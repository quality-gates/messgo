package cli

import (
	"path/filepath"
	"testing"
)

func TestExplicitnessCrossFunctionCompletion(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "explicitness-cross-function", "extract.go")
	code, findings, errOut := runAddressChild(t, "explicitness", "", path)
	if code != ExitViolation || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	assertFindings(t, findings,
		"ImplicitOutput ParseSource: write through local b",
		"ImplicitOutput ParseSource: write through parameter src",
		"UnknownEffect ParseSource: receiver effect",
		"UnknownEffect ParseSource: unresolved call",
		"UnknownEffect fileBuilder.build: receiver effect",
		"UnknownEffect fileBuilder.build: unresolved call",
		"UnknownEffect fileBuilder.collectTypeSpec: receiver effect",
		"UnknownEffect fileBuilder.collectTypeSpec: unresolved call",
		"UnknownEffect fileBuilder.collectTypes: receiver effect",
		"UnknownEffect fileBuilder.collectTypes: unresolved call",
	)
	code, findings, errOut = runAddressChild(t, "go", "", path)
	if code != ExitSuccess || len(findings) != 0 || errOut != "" {
		t.Fatalf("go control: exit = %d, findings = %q, stderr = %q", code, findings, errOut)
	}
}

func TestExplicitnessCyclicStorageEffects(t *testing.T) {
	const source = `package p

type node struct { next *node; items []int }
var global int

func build(items []int, flag bool) {
 root := &node{}
 for i := 0; i < 2; i++ {
  link(root, items, flag)
 }
 root.next.items[0] = 1
}
func link(root *node, items []int, flag bool) {
 if flag { root.next = root } else { root.next = &node{next: root} }
 root.next.items = items
}
func jump(items []int, flag bool) {
 root := &node{}
again:
 link(root, items, flag)
 if flag { goto again }
 root.next.items[0] = 1
}
func first(p *int, flag bool) {
 if flag { second(p, flag) } else { *p = 1 }
}
func second(p *int, flag bool) {
 if flag { first(p, flag) } else { global = 1 }
}
func callback(f func(), flag bool) {
 if flag { f() } else { global = 2 }
}
func knownCallback(flag bool) { callback(func(){ global = 3 }, flag) }
func unknownCallback(flag bool) { callback(missing, flag) }
func pure(x int) int { return x + 1 }
`
	code, findings, errOut := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	assertFindings(t, findings,
		"ImplicitOutput build: write through parameter items",
		"ImplicitOutput link: write through parameter root",
		"ImplicitOutput jump: write through parameter items",
		"ImplicitOutput first: write through parameter p",
		"ImplicitOutput first: package variable global",
		"ImplicitOutput second: write through parameter p",
		"ImplicitOutput second: package variable global",
		"ImplicitOutput callback: package variable global",
		"UnknownEffect callback: unresolved call",
		"ImplicitOutput knownCallback: package variable global",
		"ImplicitOutput <lambda>: package variable global",
		"ImplicitOutput unknownCallback: package variable global",
		"UnknownEffect unknownCallback: unresolved call",
	)
}
