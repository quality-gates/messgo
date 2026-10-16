package cli

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// runMutationRules runs the real CLI with the mutable-global and explicitness
// rules over the package and returns the exit code plus one "Rule function"
// entry for each violation.
func runMutationRules(t *testing.T, src string) (int, []string) {
	t.Helper()
	dir := writePackage(t, map[string]string{"a.go": src})
	code, out, errOut := runMain(t, dir, "json", "design,explicitness",
		"--only", "GlobalVariable,ImplicitInput,ImplicitOutput")
	if code != ExitSuccess && code != ExitViolation {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	var report struct {
		Files []struct {
			Violations []struct {
				Function string `json:"function"`
				Rule     string `json:"rule"`
			} `json:"violations"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	var got []string
	for _, f := range report.Files {
		for _, v := range f.Violations {
			got = append(got, strings.TrimSpace(v.Rule+" "+v.Function))
		}
	}
	sort.Strings(got)
	return code, got
}

// TestMutationClassificationAgreesAcrossRules checks that GlobalVariable,
// ImplicitInput and ImplicitOutput classify the same calls as mutations: a
// builtin, a builtin in parentheses, an aliased library call, a library call
// in parentheses, a channel close, and a local no-op that shadows a builtin.
func TestMutationClassificationAgreesAcrossRules(t *testing.T) {
	mutated := []string{
		"GlobalVariable",
		"ImplicitInput Fill",
		"ImplicitOutput Fill",
		"ImplicitInput Read",
	}
	cases := []struct {
		name string
		src  string
		code int
		want []string
	}{
		{"builtin", `package p

var xs []int

func Fill(src []int) { copy(xs, src) }

func Read() int { return xs[0] }
`, ExitViolation, mutated},
		{"parenthesized builtin", `package p

var xs []int

func Fill(src []int) { (copy)(xs, src) }

func Read() int { return xs[0] }
`, ExitViolation, mutated},
		{"aliased library call", `package p

import s "sort"

var xs []int

func Fill(src []int) { s.Ints(xs) }

func Read() int { return xs[0] }
`, ExitViolation, mutated},
		{"parenthesized library call", `package p

import "slices"

var xs []int

func Fill(src []int) { (slices.Sort)(xs) }

func Read() int { return xs[0] }
`, ExitViolation, mutated},
		{"channel close", `package p

var xs chan int

func Fill(src []int) { close(xs) }

func Read() int { return <-xs }
`, ExitViolation, []string{
			"GlobalVariable",
			"ImplicitInput Fill",
			"ImplicitOutput Fill",
			"ImplicitInput Read",
			"ImplicitInput Read",
		}},
		{"shadowed no-op", `package p

var xs []int

func copy(dst, src []int) {}

func Fill(src []int) { copy(xs, src) }

func Read() int { return xs[0] }
`, ExitViolation, []string{
			// Passing xs to any call shares it, but the call changes nothing.
			"ImplicitInput Fill",
			"ImplicitInput Read",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, got := runMutationRules(t, tc.src)
			if code != tc.code {
				t.Errorf("exit = %d, want %d", code, tc.code)
			}
			assertFindings(t, got, tc.want...)
		})
	}
}

// TestExplicitnessReadsCapturedVariableThatARangeAssigns checks that a range
// clause that assigns to a captured variable makes a read of that variable an
// implicit input, as a plain assignment does.
func TestExplicitnessReadsCapturedVariableThatARangeAssigns(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

func Ranged(xs []int) (func(), func() int) {
	x := 0
	set := func() {
		for x = range xs {
		}
	}
	get := func() int { return x }
	return set, get
}

func Assigned(xs []int) (func(), func() int) {
	x := 0
	set := func() { x = len(xs) }
	get := func() int { return x }
	return set, get
}
`,
	})
	assertFindings(t, got,
		"a.go:6 ImplicitOutput <lambda>: captured variable x",
		"a.go:9 ImplicitInput <lambda>: captured variable x",
		"a.go:15 ImplicitOutput <lambda>: captured variable x",
		"a.go:16 ImplicitInput <lambda>: captured variable x",
	)
}
