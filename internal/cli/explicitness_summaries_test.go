package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	)
}

func TestExplicitnessCallFanoutCompletes(t *testing.T) {
	for _, test := range []struct {
		name, header, params, args, leaf string
		want                             []string
	}{
		{"parameter", "type T struct { n int }", "t *T", "t", "t.n++", nil},
		{"heap", "type T struct { Items []int }; func Run(items []int) { t := &T{}; f0(t,items); t.Items[0] = 1 }", "t *T, items []int", "t,items", "t.Items = items", []string{"ImplicitOutput Run: write through parameter items"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source strings.Builder
			fmt.Fprintf(&source, "package p\n%s\n", test.header)
			for i := 0; i < 16; i++ {
				fmt.Fprintf(&source, "func f%d(%s) { f%d(%s); f%d(%s) }\n", i, test.params, i+1, test.args, i+1, test.args)
			}
			fmt.Fprintf(&source, "func f16(%s) { %s }\n", test.params, test.leaf)
			start := time.Now()
			code, findings, stderr := runAddressChild(t, "explicitness", source.String(), "")
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("depth-16 fan-out took %s, want under 1s", elapsed)
			}
			if code != ExitViolation || stderr != "" {
				t.Fatalf("exit = %d, stderr = %q", code, stderr)
			}
			want := test.want
			for i := 0; i <= 16; i++ {
				want = append(want, fmt.Sprintf("ImplicitOutput f%d: write through parameter t", i))
			}
			assertFindings(t, findings, want...)
		})
	}
}
