package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"testing"
)

// writePackage writes each named source into one temporary package directory.
func writePackage(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var explicitnessDetail = regexp.MustCompile(`has an implicit (?:input|output): (.*)\. (?:Pass|Return) `)

// runExplicitness runs the real CLI with a JSON report over the package and
// returns the exit code plus one "file:line Rule func: detail" entry for each
// violation.
func runExplicitness(t *testing.T, ruleset string, files map[string]string) (int, []string) {
	t.Helper()
	code, out, errOut := runMain(t, writePackage(t, files), "json", ruleset)
	if code != ExitSuccess && code != ExitViolation {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	var report struct {
		Files []struct {
			File       string `json:"file"`
			Violations []struct {
				BeginLine   int    `json:"beginLine"`
				Function    string `json:"function"`
				Class       string `json:"class"`
				Method      string `json:"method"`
				Description string `json:"description"`
				Rule        string `json:"rule"`
			} `json:"violations"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	var got []string
	for _, f := range report.Files {
		for _, v := range f.Violations {
			m := explicitnessDetail.FindStringSubmatch(v.Description)
			if m == nil {
				t.Fatalf("unexpected description %q", v.Description)
			}
			name := v.Function
			if v.Method != "" {
				name = v.Class + "." + v.Method
			}
			got = append(got, fmt.Sprintf("%s:%d %s %s: %s",
				filepath.Base(f.File), v.BeginLine, v.Rule, name, m[1]))
		}
	}
	sort.Strings(got)
	return code, got
}

func assertFindings(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("findings mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestExplicitnessReportsMutablePackageVariableRead(t *testing.T) {
	code, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

var total = 0

func Reset() { total = 0 }

func Current(amount int) int {
	return total + amount
}
`,
	})
	if code != ExitViolation {
		t.Errorf("exit = %d, want %d", code, ExitViolation)
	}
	assertFindings(t, got,
		"a.go:5 ImplicitOutput Reset: package variable total",
		"a.go:8 ImplicitInput Current: package variable total",
	)
}

func TestExplicitnessFollowsPackageVariablesAcrossFiles(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"state.go": `package p

var hits map[string]int
`,
		"use.go": `package p

func Record(key string) {
	hits[key]++
}

func Count(key string) int {
	return hits[key]
}

func Forget(key string) {
	delete(hits, key)
}
`,
	})
	assertFindings(t, got,
		"use.go:4 ImplicitInput Record: package variable hits",
		"use.go:4 ImplicitOutput Record: package variable hits",
		"use.go:8 ImplicitInput Count: package variable hits",
		"use.go:12 ImplicitOutput Forget: package variable hits",
	)
}

func TestExplicitnessIgnoresExplicitData(t *testing.T) {
	code, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import "errors"

var ErrEmpty = errors.New("empty")

type point struct{ x, y int }

const limit = 3

var y int

func bump() { y++ }

func Scale(p point, factor int) (point, error) {
	if factor > limit {
		return point{}, ErrEmpty
	}
	total := 0
	var scaled point
	scaled = point{x: p.x * factor, y: p.y * factor}
	total += scaled.x
	for i := range total {
		total += i
	}
	add := func(n int) { total += n }
	add(1)
	return scaled, nil
}
`,
	})
	if code != ExitViolation {
		t.Errorf("exit = %d, want %d", code, ExitViolation)
	}
	assertFindings(t, got,
		"a.go:13 ImplicitInput bump: package variable y",
		"a.go:13 ImplicitOutput bump: package variable y",
	)
}

func TestExplicitnessReportsStandardLibraryEffects(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import (
	"fmt"
	sys "os"
	"math/rand"
	"strings"
	"time"
)

func Greet(name string) {
	fmt.Println("hi", name)
}

func Home() string {
	return sys.Getenv("HOME")
}

func Stamp() string {
	return time.Now().String()
}

func Roll() int {
	return rand.Intn(6)
}

func Fail(err error) {
	fmt.Fprintln(sys.Stderr, err)
	sys.Exit(1)
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func Safe() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return nil
}

func Pure(s string) string {
	return strings.ToUpper(fmt.Sprintf("%s!", s))
}
`,
	})
	assertFindings(t, got,
		"a.go:12 ImplicitOutput Greet: fmt.Println",
		"a.go:16 ImplicitInput Home: os.Getenv",
		"a.go:20 ImplicitInput Stamp: time.Now",
		"a.go:24 ImplicitInput Roll: math/rand.Intn",
		"a.go:28 ImplicitOutput Fail: os.Stderr",
		"a.go:29 ImplicitOutput Fail: os.Exit",
		"a.go:34 ImplicitOutput Must: panic",
		"a.go:40 ImplicitInput Safe: recover",
	)
}

func TestExplicitnessReportsParametersChannelsAndStreams(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import (
	"fmt"
	"io"
	"sort"
)

type item struct{ n int }

func Fill(dst []int, m map[string]int, p *item) {
	dst[0] = 1
	m["a"] = 2
	p.n++
}

func Sort(xs []int, keys map[string]bool) {
	sort.Ints(xs)
	delete(keys, "a")
}

func Grow(xs []int, v item) []int {
	xs = append(xs, 1)
	v.n = 2
	return xs
}

func Pipe(in <-chan int, out chan<- int) {
	out <- <-in
	for v := range in {
		_ = v
	}
}

func Local() int {
	ch := make(chan int, 1)
	ch <- 1
	return <-ch
}

func Dump(w io.Writer, r io.Reader) error {
	fmt.Fprintln(w, "x")
	_, err := io.Copy(w, r)
	return err
}

type server struct {
	jobs  chan int
	count int
}

func (s *server) Run(n int) {
	s.jobs <- n
	s.count = n
}
`,
	})
	assertFindings(t, got,
		"a.go:12 ImplicitOutput Fill: write through parameter dst",
		"a.go:13 ImplicitOutput Fill: write through parameter m",
		"a.go:14 ImplicitOutput Fill: write through parameter p",
		"a.go:18 ImplicitOutput Sort: write through parameter xs",
		"a.go:19 ImplicitOutput Sort: write through parameter keys",
		"a.go:29 ImplicitOutput Pipe: send on out",
		"a.go:29 ImplicitInput Pipe: receive from in",
		"a.go:42 ImplicitOutput Dump: write to w",
		"a.go:43 ImplicitInput Dump: read from r",
	)
}

func TestExplicitnessReportsWritesToDataThatCopiesShare(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

type ids []int

type bag struct {
	m   map[string]int
	p   *int
	n   int
}

func Named(s ids) { s[0] = 1 }

func MapField(b bag) { b.m["k"] = 1 }

func PointerField(b bag) { *b.p = 1 }

func Delete(b bag) { delete(b.m, "k") }

func Local(b bag) bag {
	b.n = 1
	return b
}

func Fixed(a [2]int) [2]int {
	a[0] = 1
	return a
}
`,
	})
	assertFindings(t, got,
		"a.go:11 ImplicitOutput Named: write through parameter s",
		"a.go:13 ImplicitOutput MapField: write through parameter b",
		"a.go:15 ImplicitOutput PointerField: write through parameter b",
		"a.go:17 ImplicitOutput Delete: write through parameter b",
	)
}

func TestExplicitnessReadsMapLiteralKeys(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

var counter int

func Bump() { counter = 1 }

type pair struct{ counter, n int }

func Keys() map[int]bool { return map[int]bool{counter: true} }

func Fields(n int) pair { return pair{counter: n, n: n} }
`,
	})
	assertFindings(t, got,
		"a.go:5 ImplicitOutput Bump: package variable counter",
		"a.go:9 ImplicitInput Keys: package variable counter",
	)
}

func TestExplicitnessReadsKeysOfNamedMapLiteral(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

var n int

func Bump() { n = 1 }

type names map[int]string

func Label() names { return names{n: "x"} }
`,
	})
	assertFindings(t, got,
		"a.go:5 ImplicitOutput Bump: package variable n",
		"a.go:9 ImplicitInput Label: package variable n",
	)
}

func TestExplicitnessReadsPackageVariableThatASortChanges(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import "sort"

var list = []int{2, 1}

func Order() { sort.Ints(list) }

func First() int { return list[0] }
`,
	})
	assertFindings(t, got,
		"a.go:7 ImplicitInput Order: package variable list",
		"a.go:7 ImplicitOutput Order: package variable list",
		"a.go:9 ImplicitInput First: package variable list",
	)
}

func TestExplicitnessReadsPackageVariableThatACopyChanges(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

var buf = make([]byte, 4)

func Fill(src []byte) { copy(buf, src) }

func Head() byte { return buf[0] }
`,
	})
	assertFindings(t, got,
		"a.go:5 ImplicitInput Fill: package variable buf",
		"a.go:5 ImplicitOutput Fill: package variable buf",
		"a.go:7 ImplicitInput Head: package variable buf",
	)
}

func TestExplicitnessReportsWritesThroughSliceExpressions(t *testing.T) {
	src := `package p

import "sort"

var arr [4]byte

type queue struct{ items []int }

func Fill(src []byte) { copy(arr[:], src) }

func Tail(s []int) { sort.Ints(s[1:]) }

func (q queue) Load(src []int) { copy(q.items[:], src) }
`
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": src})
	assertFindings(t, got,
		"a.go:9 ImplicitInput Fill: package variable arr",
		"a.go:9 ImplicitOutput Fill: package variable arr",
		"a.go:11 ImplicitOutput Tail: write through parameter s",
	)
	_, got = runExplicitness(t, "explicitness-strict", map[string]string{"a.go": src})
	if !slices.Contains(got, "a.go:13 ImplicitOutput queue.Load: receiver field items") {
		t.Fatalf("strict findings missing receiver write: %q", got)
	}
}

func TestExplicitnessReportsReceiveFromChannelThatACallReturns(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import "context"

func Wait(ctx context.Context) { <-ctx.Done() }
`,
	})
	assertFindings(t, got, "a.go:5 ImplicitInput Wait: receive from ctx")
}

func TestExplicitnessReportsProcessNetworkAndTimerEffects(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{
		"a.go": `package p

import (
	"net"
	"os/exec"
	"time"
)

func Run() error { return exec.Command("true").Run() }

func Connect() (net.Conn, error) { return net.Dial("tcp", "localhost:1") }

func Wait() { <-time.After(time.Second) }
`,
	})
	assertFindings(t, got,
		"a.go:9 ImplicitInput Run: os/exec.Command",
		"a.go:9 ImplicitOutput Run: os/exec.Command",
		"a.go:11 ImplicitInput Connect: net.Dial",
		"a.go:11 ImplicitOutput Connect: net.Dial",
		"a.go:13 ImplicitInput Wait: time.After",
	)
}

const cartSource = `package p

type cart struct {
	items []string
	total int
}

func (c *cart) Add(item string) {
	c.items = append(c.items, item)
}

func (c *cart) Count() int {
	return len(c.items)
}

func (c *cart) Reset() {
	c.clear()
}

func (c cart) With(item string) cart {
	c.items = append(c.items, item)
	return c
}

func (c *cart) clear() { c.total = 0 }

func (c cart) Tag(i int, item string) { c.items[i] = item }
`

func TestExplicitnessAllowsReceiverByDefault(t *testing.T) {
	code, got := runExplicitness(t, "explicitness", map[string]string{"a.go": cartSource})
	if code != ExitSuccess {
		t.Errorf("exit = %d, want %d", code, ExitSuccess)
	}
	assertFindings(t, got)
}

func TestExplicitnessStrictReportsReceiverData(t *testing.T) {
	code, got := runExplicitness(t, "explicitness-strict", map[string]string{"a.go": cartSource})
	if code != ExitViolation {
		t.Errorf("exit = %d, want %d", code, ExitViolation)
	}
	assertFindings(t, got,
		"a.go:9 ImplicitInput cart.Add: receiver field items",
		"a.go:9 ImplicitOutput cart.Add: receiver field items",
		"a.go:13 ImplicitInput cart.Count: receiver field items",
		"a.go:17 ImplicitInput cart.Reset: receiver c",
		"a.go:21 ImplicitInput cart.With: receiver field items",
		"a.go:22 ImplicitInput cart.With: receiver c",
		"a.go:25 ImplicitOutput cart.clear: receiver field total",
		"a.go:27 ImplicitOutput cart.Tag: receiver field items",
	)
}
