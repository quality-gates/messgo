// Package cli implements the messgo command-line interface, mirroring PHPMD's
// argument layout: `messgo <paths> <format> <ruleset[,...]> [options]`.
package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/quality-gates/messgo/internal/report"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/ruleset"
	"github.com/quality-gates/messgo/internal/runner"
)

// Exit codes match PHPMD.
const (
	ExitSuccess   = 0
	ExitError     = 1
	ExitViolation = 2
)

const version = "0.1.3"

type options struct {
	paths            string
	format           string
	rulesets         string
	minPriority      int
	maxPriority      int
	reportFile       string
	suffixes         string
	exclude          string
	strict           bool
	color            bool
	verbose          bool
	ignoreErrors     bool
	ignoreViolations bool
	ignoreTests      bool
}

// Main runs the CLI and returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return ExitError
	}
	if code, handled := handleInfoFlags(args[0], stdout); handled {
		return code
	}

	opt, positionals, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	if len(positionals) < 3 {
		printUsage(stderr)
		return ExitError
	}
	opt.paths, opt.format, opt.rulesets = positionals[0], positionals[1], positionals[2]
	return run(opt, stdout, stderr)
}

// run executes the analysis pipeline for already-parsed options.
func run(opt options, stdout, stderr io.Writer) int {
	rnd, err := selectRenderer(opt)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	sets, err := loadRuleSets(opt, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	rep, err := runner.Run(runner.Options{
		Paths:       splitList(opt.paths),
		RuleSets:    sets,
		Suffixes:    suffixList(opt.suffixes),
		Exclude:     splitList(opt.exclude),
		IgnoreTests: opt.ignoreTests,
	})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	if err := writeReport(rnd, opt, rep, stdout); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	return exitCodeFor(rep, opt)
}

// handleInfoFlags handles --version/--help, which short-circuit normal runs.
func handleInfoFlags(first string, stdout io.Writer) (code int, handled bool) {
	switch first {
	case "--version":
		fmt.Fprintf(stdout, "messgo %s\n", version)
		return ExitSuccess, true
	case "--help", "-h", "help":
		printUsage(stdout)
		return ExitSuccess, true
	}
	return 0, false
}

// parseArgs parses options and positional arguments, table-driving the flags.
func parseArgs(args []string) (options, []string, error) {
	opt := options{format: "text", maxPriority: 1}
	var positionals []string
	boolFlags := map[string]*bool{
		"--strict":                    &opt.strict,
		"--color":                     &opt.color,
		"--ignore-errors-on-exit":     &opt.ignoreErrors,
		"--ignore-violations-on-exit": &opt.ignoreViolations,
		"--ignore-tests":              &opt.ignoreTests,
	}
	strFlags := map[string]*string{
		"--reportfile": &opt.reportFile,
		"--suffixes":   &opt.suffixes,
		"--exclude":    &opt.exclude,
	}
	intFlags := map[string]*int{
		"--minimumpriority": &opt.minPriority,
		"--maximumpriority": &opt.maxPriority,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--verbose" || a == "-v":
			opt.verbose = true
		case boolFlags[a] != nil:
			*boolFlags[a] = true
		case strFlags[a] != nil:
			i++
			*strFlags[a] = arg(args, i)
		case intFlags[a] != nil:
			i++
			*intFlags[a] = atoi(arg(args, i))
		case strings.HasPrefix(a, "--"):
			return opt, nil, fmt.Errorf("unknown option: %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	return opt, positionals, nil
}

// selectRenderer resolves the requested report format to a renderer.
func selectRenderer(opt options) (report.Renderer, error) {
	if opt.format == "text" && opt.color {
		opt.format = "ansi"
	}
	rnd, ok := report.For(opt.format)
	if !ok {
		return nil, fmt.Errorf("unknown report format %q. Available: %s",
			opt.format, strings.Join(report.Formats(), ", "))
	}
	return rnd, nil
}

// loadRuleSets resolves the requested rulesets, applying priority filters.
func loadRuleSets(opt options, stderr io.Writer) ([]*rule.RuleSet, error) {
	loader := &ruleset.Loader{
		MinPriority: opt.minPriority,
		MaxPriority: opt.maxPriority,
		Warn: func(msg string) {
			if opt.verbose {
				fmt.Fprintln(stderr, "warning:", msg)
			}
		},
	}
	return loader.Load(opt.rulesets)
}

// writeReport renders the report to stdout or the configured report file.
func writeReport(rnd report.Renderer, opt options, rep *report.Report, stdout io.Writer) error {
	out := stdout
	if opt.reportFile != "" {
		f, err := os.Create(opt.reportFile)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	return rnd.Render(out, rep)
}

// exitCodeFor maps the report onto PHPMD's exit-code convention.
func exitCodeFor(rep *report.Report, opt options) int {
	if len(rep.Errors) > 0 && !opt.ignoreErrors {
		return ExitError
	}
	if len(rep.Violations) > 0 && !opt.ignoreViolations {
		return ExitViolation
	}
	return ExitSuccess
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func suffixList(s string) []string {
	parts := splitList(s)
	for i, p := range parts {
		if !strings.HasPrefix(p, ".") {
			parts[i] = "." + p
		}
	}
	return parts
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `messgo %s — a PHPMD-style mess detector for Go

Usage:
  messgo <paths> <format> <ruleset[,...]> [options]

Arguments:
  paths      Comma-separated files or directories to scan.
  format     Report format: %s
  ruleset    Comma-separated built-in rulesets or ruleset XML files.
             Built-in: %s

Options:
  --minimumpriority <n>          Only rules with priority <= n.
  --maximumpriority <n>          Only rules with priority >= n.
  --reportfile <file>            Write the report to a file.
  --suffixes <list>              File extensions to scan (default: go).
  --exclude <list>               Path substrings to exclude.
  --ignore-tests                 Skip *_test.go files.
  --strict                       Also report suppressed violations.
  --color                        Colorize text output.
  --verbose, -v                  Verbose diagnostics.
  --ignore-errors-on-exit        Exit 0 even if parse errors occurred.
  --ignore-violations-on-exit    Exit 0 even if violations were found.
  --version                      Print version.
  --help, -h                     Show this help.

Exit codes: 0 = clean, 1 = error, 2 = violations found.
`, version, strings.Join(report.Formats(), ", "), strings.Join(ruleset.BuiltinNames(), ", "))
}
