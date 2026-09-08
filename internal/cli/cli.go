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
	"github.com/quality-gates/messgo/internal/version"
)

// Exit codes match PHPMD.
const (
	ExitSuccess         = 0
	ExitError           = 1
	ExitViolation       = 2
	ExitProcessingError = 3
)

type options struct {
	paths            string
	format           string
	rulesets         string
	minPriority      int
	maxPriority      int
	reportFile       string
	suffixes         string
	exclude          string
	ruleFilter       ruleFilter
	strict           bool
	color            bool
	verbose          bool
	ignoreErrors     bool
	ignoreViolations bool
	ignoreTests      bool
}

// ruleFilter selects a subset of rules by name: --enable/--only (whitelist)
// and --disable (blacklist), each a comma-separated list.
type ruleFilter struct {
	enable  string
	disable string
}

// Main runs the CLI and returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return ExitError
	}
	if code, handled := handleInfoFlags(args, stdout); handled {
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
	if err := requireNonEmptyLists(opt); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
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
	applyRuleFilters(opt, sets, stderr)
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

// applyRuleFilters narrows the loaded rule sets by name and warns when the
// filter leaves nothing selected or names rules that are not loaded.
func applyRuleFilters(opt options, sets []*rule.RuleSet, stderr io.Writer) {
	enable, disable := splitList(opt.ruleFilter.enable), splitList(opt.ruleFilter.disable)
	if len(enable) == 0 && len(disable) == 0 {
		return
	}
	res := ruleset.ApplyRuleFilter(sets, enable, disable)
	if res.Remaining == 0 {
		fmt.Fprintln(stderr, "warning: no rules selected (check --enable/--only/--disable)")
	}
	if opt.verbose {
		for _, name := range res.Unmatched {
			fmt.Fprintf(stderr, "warning: no rule named %q (check --enable/--only/--disable)\n", name)
		}
	}
}

// handleInfoFlags handles --version/--help, which short-circuit normal runs
// wherever they appear in the argument list, before any other validation.
func handleInfoFlags(args []string, stdout io.Writer) (code int, handled bool) {
	for _, a := range args {
		switch a {
		case "--version":
			fmt.Fprintf(stdout, "messgo %s\n", version.Version)
			return ExitSuccess, true
		case "--help", "-h":
			printUsage(stdout)
			return ExitSuccess, true
		}
	}
	// The bare `help` command is only honoured as the first argument, where it
	// cannot be mistaken for a path.
	if args[0] == "help" {
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
		"--verbose":                   &opt.verbose,
		"-v":                          &opt.verbose,
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
		"--enable":     &opt.ruleFilter.enable,
		"--only":       &opt.ruleFilter.enable, // alias for --enable
		"--disable":    &opt.ruleFilter.disable,
	}
	intFlags := map[string]*int{
		"--minimumpriority": &opt.minPriority,
		"--maximumpriority": &opt.maxPriority,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case boolFlags[a] != nil:
			*boolFlags[a] = true
		case strFlags[a] != nil || intFlags[a] != nil:
			if err := valueFlag(args, &i, a, valueTarget{str: strFlags[a], pri: intFlags[a]}); err != nil {
				return opt, nil, err
			}
		case strings.HasPrefix(a, "--"):
			return opt, nil, fmt.Errorf("unknown option: %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) > 3 {
		return opt, nil, fmt.Errorf("unexpected argument %q", positionals[3])
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
		return ExitProcessingError
	}
	if len(rep.Violations) > 0 && !opt.ignoreViolations {
		return ExitViolation
	}
	return ExitSuccess
}

// valueTarget is where a value-taking flag stores its value: either a string
// field or an integer priority field.
type valueTarget struct {
	str *string
	pri *int
}

// valueFlag consumes the value token following flag and stores it in t.
func valueFlag(args []string, i *int, flag string, t valueTarget) error {
	v, err := flagValue(args, i, flag)
	if err != nil {
		return err
	}
	if t.str != nil {
		*t.str = v
		return nil
	}
	n, err := nonNegativeInt(v)
	if err != nil {
		return fmt.Errorf("invalid value for %s: %q", flag, v)
	}
	*t.pri = n
	return nil
}

// flagValue consumes the value token following a value-taking flag. A missing
// token, or one that looks like another option, is a usage error.
func flagValue(args []string, i *int, flag string) (string, error) {
	next := *i + 1
	if next >= len(args) || strings.HasPrefix(args[next], "--") {
		return "", fmt.Errorf("%s requires a value", flag)
	}
	*i = next
	return args[next], nil
}

// nonNegativeInt parses a priority value: a whole number >= 0.
func nonNegativeInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%q is not a non-negative integer", s)
	}
	return n, nil
}

func requireNonEmptyLists(opt options) error {
	if len(splitList(opt.paths)) == 0 {
		return fmt.Errorf("no paths given")
	}
	if len(splitList(opt.rulesets)) == 0 {
		return fmt.Errorf("no rulesets given")
	}
	return nil
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
  --exclude <list>               Path substrings to exclude (./ prefixes cleaned).
  --enable, --only <list>        Run only these rules (comma-separated names).
  --disable <list>               Skip these rules (comma-separated names).
  --ignore-tests                 Skip *_test.go files.
  --strict                       Also report suppressed violations.
  --color                        Colorize text output.
  --verbose, -v                  Verbose diagnostics.
  --ignore-errors-on-exit        Exit 0 even if parse errors occurred.
  --ignore-violations-on-exit    Exit 0 even if violations were found.
  --version                      Print version.
  --help, -h                     Show this help.

Exit codes: 0 = clean, 1 = error, 2 = violations found, 3 = processing error.
`, version.Version, strings.Join(report.Formats(), ", "), strings.Join(ruleset.BuiltinNames(), ", "))
}
