package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/quality-gates/messgo/internal/ruleset"
)

func main() {
	dir, err := os.MkdirTemp("", "messgo-ruleset-check-")
	if err != nil {
		fail("create temporary directory: %v", err)
	}
	defer os.RemoveAll(dir)
	if len(os.Args) == 1 {
		checkOverrides(dir)
		checkGraphs(dir)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "graphs" {
		checkGraphs(dir)
		return
	}
	fail("usage: check [graphs]")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func write(path, text string) {
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		fail("write %s: %v", path, err)
	}
}

func checkOverrides(dir string) {
	const inherited = `<rule ref="naming/LongVariable"/>`
	const override2 = `<rule ref="naming/LongVariable"><priority>2</priority></rule>`
	const override3 = `<rule ref="naming/LongVariable"><priority>3</priority></rule>`
	tokens := []string{inherited, override2, override3}
	count := 0
	for length := 1; length <= 5; length++ {
		sequences := 1
		for range length {
			sequences *= len(tokens)
		}
		for encoding := range sequences {
			value := encoding
			var rules []string
			var has2, has3 bool
			for range length {
				choice := value % len(tokens)
				value /= len(tokens)
				rules = append(rules, tokens[choice])
				has2 = has2 || choice == 1
				has3 = has3 || choice == 2
			}
			path := filepath.Join(dir, "overrides.xml")
			write(path, `<ruleset name="test">`+strings.Join(rules, "")+`</ruleset>`)
			sets, err := (&ruleset.Loader{}).Load(path)
			if has2 && has3 {
				if err == nil || !strings.Contains(err.Error(), `conflicting overrides for rule "LongVariable"`) {
					fail("overrides %v: expected conflicting overrides, got %v", rules, err)
				}
			} else {
				if err != nil {
					fail("overrides %v: unexpected error: %v", rules, err)
				}
				priority := 3
				if has2 {
					priority = 2
				}
				if len(sets) != 1 || len(sets[0].Rules) != 1 || sets[0].Rules[0].Name() != "LongVariable" || sets[0].Rules[0].Priority() != priority {
					fail("overrides %v: expected one LongVariable with priority %d, got %#v", rules, priority, sets)
				}
			}
			count++
		}
	}
	fmt.Printf("Verified %d override sequences of length 1 through 5.\n", count)
}

func checkGraphs(dir string) {
	count := 0
	for nodes := 1; nodes <= 3; nodes++ {
		for edges := 0; edges < 1<<(nodes*nodes); edges++ {
			for from := range nodes {
				var refs []string
				for to := range nodes {
					if edges&(1<<(from*nodes+to)) != 0 {
						refs = append(refs, fmt.Sprintf(`<rule ref="node%d.xml"/>`, to))
					}
				}
				refs = append(refs, `<rule ref="naming/LongVariable"/>`)
				write(filepath.Join(dir, fmt.Sprintf("node%d.xml", from)), `<ruleset name="test">`+strings.Join(refs, "")+`</ruleset>`)
			}
			path := filepath.Join(dir, "node0.xml")
			sets, err := (&ruleset.Loader{}).Load(path)
			if reachableCycle(nodes, edges) {
				if err == nil || !strings.Contains(err.Error(), "cyclic ruleset reference") {
					fail("graph nodes=%d edges=%d: expected cycle error, got %v", nodes, edges, err)
				}
			} else if err != nil || len(sets) != 1 || len(sets[0].Rules) != 1 || sets[0].Rules[0].Name() != "LongVariable" {
				fail("graph nodes=%d edges=%d: expected one LongVariable without error, got %v", nodes, edges, err)
			}
			count++
		}
	}
	fmt.Printf("Verified %d directed graphs of 1 through 3 rulesets.\n", count)
}

func reachableCycle(nodes, edges int) bool {
	var visit func(int, uint) bool
	visit = func(node int, path uint) bool {
		if path&(1<<node) != 0 {
			return true
		}
		path |= 1 << node
		for next := range nodes {
			if edges&(1<<(node*nodes+next)) != 0 && visit(next, path) {
				return true
			}
		}
		return false
	}
	return visit(0, 0)
}
