package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Golden findings were captured through the CLI at 113de14b, before storage
// summaries. Report timestamps and ordering are deliberately not compared.
func TestExplicitnessStorageSummaryPreservesFindings(t *testing.T) {
	for _, test := range []struct{ name, path string }{
		{"extract", filepath.Join("..", "..", "testdata", "explicitness-cross-function", "extract.go")},
		{"model", filepath.Join("..", "model")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Main([]string{test.path, "json", "explicitness", "--ignore-tests"}, &out, &stderr)
			if code != ExitViolation || stderr.Len() != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
			}
			got := storageReportFindings(t, out.Bytes())
			want, err := os.ReadFile(filepath.Join("testdata", "explicitness-summary-"+test.name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("CLI findings changed\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func storageReportFindings(t *testing.T, data []byte) string {
	t.Helper()
	var report struct {
		Files []struct {
			File       string
			Violations []struct {
				BeginLine, EndLine                         int
				Rule, Function, Class, Method, Description string
			}
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	var findings []string
	for _, file := range report.Files {
		for _, v := range file.Violations {
			name := v.Function
			if v.Method != "" {
				name = v.Class + "." + v.Method
			}
			findings = append(findings, fmt.Sprintf("%s:%d:%d %s %s: %s", filepath.Base(file.File), v.BeginLine, v.EndLine, v.Rule, name, v.Description))
		}
	}
	sort.Strings(findings)
	return strings.Join(findings, "\n") + "\n"
}

func TestExplicitnessStorageSummaryKeepsRecursiveValuesAndPaths(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         []string
	}{
		{"inline values", `package p
type Box struct { Items []int; Next *Box }
func a(x,y *Box,n int) { y.Items=x.Items; if n>0 { b(x,y,n-1) } }
func b(x,y *Box,n int) { x.Next=y; if n>0 { a(y,x,n-1) } }
`, []string{"ImplicitOutput a: write through parameter x", "ImplicitOutput a: write through parameter y", "ImplicitOutput b: write through parameter x", "ImplicitOutput b: write through parameter y"}},
		{"private projected paths", `package p
type S []S
func f(x *S,n int) { if n>0 { f(&(*x)[0],n-1) }; *x=nil }
func Run() { x:=S{S{}}; f(&x,1) }
`, []string{"ImplicitOutput f: write through parameter x"}},
		{"projected heap writes", `package p
type S []S
func f(x *S,items S,n int) { if n>0 { f(&(*x)[0],items,n-1) }; *x=items }
func Run(items S) { x:=S{S{}}; f(&x,items,1); x[0][0]=nil }
`, []string{"ImplicitOutput f: write through parameter x", "ImplicitOutput Run: write through parameter items"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, findings, stderr := runAddressChild(t, "explicitness", test.source, "")
			if code != ExitViolation || stderr != "" {
				t.Fatalf("exit = %d, stderr = %q", code, stderr)
			}
			assertFindings(t, findings, test.want...)
		})
	}
}

func TestExplicitnessStorageSummaryKeepsNamedResultTypes(t *testing.T) {
	const source = `package p
type S struct { B []int }
func zero() (s S) { return }
func Zero() { x := zero(); x.B[0] = 1 }
`
	code, findings, stderr := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertFindings(t, findings, "ImplicitOutput Zero: write through local x")
}

func TestExplicitnessStorageSummaryKeepsRecursiveEntries(t *testing.T) {
	const source = `package p
type Box struct { Next *Box; Items []int }
func a(x *Box,n int,items []int) { x.Next=&Box{Next:x,Items:items}; if n>0 { b(x.Next,n-1,items) } }
func b(x *Box,n int,items []int) { x.Next=&Box{Next:x,Items:items}; if n>0 { a(x.Next,n-1,items) } }
func Run(items []int) { x:=&Box{}; a(x,2,items); x.Next.Next.Items[0]++ }
`
	code, findings, stderr := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertFindings(t, findings, "ImplicitOutput a: write through parameter x", "ImplicitOutput b: write through parameter x", "ImplicitOutput Run: write through parameter items")
}

func TestExplicitnessStorageSummaryKeepsAddressedCaptures(t *testing.T) {
	const source = `package p
func set(slot *[]int, items []int) { *slot = items }
func callbacks(f func()) { f(); f() }
func Captured(items []int) {
 var x []int
 set(&x, items)
 callbacks(func(){ x[0] = 1 })
}
func Private(items []int) {
 var x []int
 set(&x, []int{0})
 callbacks(func(){ x[0] = 1 })
}
`
	code, findings, stderr := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertFindings(t, findings,
		"ImplicitOutput set: write through parameter slot",
		"UnknownEffect callbacks: unresolved call",
		"ImplicitOutput Captured: write through parameter items",
		"ImplicitOutput <lambda>: captured variable x",
		"ImplicitOutput <lambda>: captured variable x",
	)
}

func TestExplicitnessStorageSummaryKeepsIndirectRecursionContext(t *testing.T) {
	const source = `package p
func A(slot *[]int, items []int) { if len(items)>1 { B(slot,items[1:]) }; *slot = items }
func B(slot *[]int, items []int) { f := A; f(slot,items) }
func Run(items []int) {
 x := items
 x = nil
 p := &x
 A(p,items)
 x = nil
 B(p,items)
 x[0] = 1
}
`
	code, findings, stderr := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertFindings(t, findings,
		"ImplicitOutput A: write through parameter slot",
		"ImplicitOutput B: write through parameter slot",
		"ImplicitOutput Run: write through parameter items",
	)
}

func TestExplicitnessStorageSummaryInstantiatesAllocations(t *testing.T) {
	const source = `package p
type Box struct { Items []int; Next *Box }
func box(items []int) *Box { return &Box{Items: items} }
func set(b *Box, items []int) { b.Items = items }
func Change(items []int) {
 a, b := box(nil), box(nil)
 set(a, items)
 b.Items[0] = 1
 a.Items[0] = 1
}
func Quiet(items []int) {
 a, b := box(nil), box(nil)
 set(a, items)
 b.Items[0] = 1
}
func Alias(items []int) {
 a := box(nil)
 b := a
 set(a, items)
 b.Items[0] = 1
}
func node(b *Box, items []int) { b.Next = &Box{Next: b.Next, Items: items} }
func Loop(items []int) {
 b := &Box{}
 for i:=0;i<2;i++ { node(b,items) }
 b.Next.Next.Items[0]=1
}
func allocate(n int) *Box { x := &Box{}; if n > 0 { x.Next = allocate(n-1) }; return x }
func RecursiveInstances(items []int) {
 a, b := allocate(2), allocate(2)
 a.Items = items
 b.Items[0]++
}
func allocateA(n int) *Box { x := &Box{}; if n > 0 { x.Next = allocateB(n-1) }; return x }
func allocateB(n int) *Box { x := &Box{}; if n > 0 { x.Next = allocateA(n-1) }; return x }
func MutualInstances(items []int) {
 a, b := allocateA(2), allocateA(2)
 a.Items = items
 b.Items[0]++
}
`
	code, findings, stderr := runAddressChild(t, "explicitness", source, "")
	if code != ExitViolation || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertFindings(t, findings,
		"ImplicitOutput set: write through parameter b",
		"ImplicitOutput Change: write through parameter items",
		"ImplicitOutput Alias: write through parameter items",
		"ImplicitOutput node: write through parameter b",
		"ImplicitOutput Loop: write through local b",
		"ImplicitOutput RecursiveInstances: write through local b",
		"ImplicitOutput MutualInstances: write through local b",
	)
}

func TestExplicitnessCallFanoutCompletes(t *testing.T) {
	// Measure the shipping binary, not race/coverage instrumentation or compiler
	// time. The remaining summary tests exercise Main under that instrumentation.
	binary := filepath.Join(t.TempDir(), "messgo")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/messgo")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	for _, test := range []struct {
		name, header, params, args, prolog, leaf string
		want                                     []string
	}{
		{"parameter", "type T struct { n int }", "t *T", "t", "", "t.n++", nil},
		{"heap", "type T struct { Items []int }; func Run(items []int) { t := &T{}; f0(t,items); t.Items[0] = 1 }", "t *T, items []int", "t,items", "", "t.Items = items", []string{"ImplicitOutput Run: write through parameter items"}},
		{"addressed local", "type T struct { n int }", "t *T", "&x", "t.n++; var x T;", "t.n++", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source strings.Builder
			fmt.Fprintf(&source, "package p\n%s\n", test.header)
			for i := 0; i < 16; i++ {
				fmt.Fprintf(&source, "func f%d(%s) { %s f%d(%s); f%d(%s) }\n", i, test.params, test.prolog, i+1, test.args, i+1, test.args)
			}
			fmt.Fprintf(&source, "func f16(%s) { %s }\n", test.params, test.leaf)
			findings := storageFanoutFindings(t, binary, source.String())
			want := test.want
			for i := 0; i <= 16; i++ {
				want = append(want, fmt.Sprintf("ImplicitOutput f%d: write through parameter t", i))
			}
			assertFindings(t, findings, want...)
		})
	}
	for _, graph := range []struct {
		name, prefixes string
		last           int
	}{{"diamond", "ab", 12}, {"ring", "f", 15}} {
		t.Run(graph.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "explicitness-summary-"+graph.name+".go"))
			if err != nil {
				t.Fatal(err)
			}
			findings := storageFanoutFindings(t, binary, string(source))
			var want []string
			if graph.name == "diamond" {
				want = append(want, "ImplicitOutput Run: write through parameter t")
			}
			for _, prefix := range graph.prefixes {
				for i := 0; i <= graph.last; i++ {
					want = append(want, fmt.Sprintf("ImplicitOutput %c%d: write through parameter t", prefix, i))
				}
			}
			assertFindings(t, findings, want...)
		})
	}
}

func storageFanoutFindings(t *testing.T, binary, source string) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fan.go")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, path, "json", "explicitness")
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	out, err := cmd.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != ExitViolation || stderr.Len() != 0 {
		t.Fatalf("CLI: %v\nstdout: %s\nstderr: %s", err, out, stderr.String())
	}
	// Bound work separately from scheduling delays on shared CI hosts. The
	// context above also enforces an 8s wall-clock completion deadline.
	cpu := cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
	if cpu > time.Second {
		t.Fatalf("call graph used %s CPU, want under 1s", cpu)
	}
	t.Logf("call graph: %s wall, %s CPU", time.Since(start), cpu)
	return explicitnessFindings(t, string(out))
}
