// Package runner orchestrates discovery, parsing and analysis of Go sources.
package runner

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quality-gates/messgo/internal/model"
	"github.com/quality-gates/messgo/internal/report"
	"github.com/quality-gates/messgo/internal/rule"
	"github.com/quality-gates/messgo/internal/util"
)

// Options configures a run.
type Options struct {
	Paths       []string // files or directories to scan
	RuleSets    []*rule.RuleSet
	Suffixes    []string // file extensions to include (default ".go")
	Exclude     []string // path substrings to skip
	IgnoreTests bool     // skip *_test.go files
}

// Run discovers files, parses and analyzes them, and returns a Report.
func Run(opts Options) (*report.Report, error) {
	if len(opts.Suffixes) == 0 {
		opts.Suffixes = []string{".go"}
	}
	files, err := discover(opts)
	if err != nil {
		return nil, err
	}
	rep := &report.Report{}
	parsed := parseFiles(files, rep)
	annotatePackages(parsed)
	for _, file := range parsed {
		vs := rule.Analyze(file, opts.RuleSets)
		rep.Violations = append(rep.Violations, vs...)
	}
	rule.SortViolations(rep.Violations)
	return rep, nil
}

// parseFiles parses every path, recording parse failures on the report and
// returning the files that parsed successfully.
func parseFiles(files []string, rep *report.Report) []*model.File {
	var parsed []*model.File
	for _, path := range files {
		file, err := model.Parse(path)
		if err != nil {
			rep.Errors = append(rep.Errors, report.ProcessingError{File: path, Message: err.Error()})
			continue
		}
		parsed = append(parsed, file)
	}
	return parsed
}

// annotatePackages groups files by directory (a Go package lives in one
// directory) and records, on every file, the set of package-level variables
// mutated anywhere in that package — enabling cross-file analysis.
func annotatePackages(parsed []*model.File) {
	byDir := map[string][]*model.File{}
	for _, f := range parsed {
		dir := filepath.Dir(f.Path)
		byDir[dir] = append(byDir[dir], f)
	}
	for _, group := range byDir {
		asts := make([]*ast.File, len(group))
		for i, f := range group {
			asts[i] = f.Syntax
		}
		mutated := util.MutatedGlobalNames(asts)
		for _, f := range group {
			f.MutatedGlobals = mutated
		}
	}
}

func walkDirFunc(opts Options, add func(string)) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasSuffix(path, opts.Suffixes) {
			return nil
		}
		if opts.IgnoreTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if isExcluded(path, opts.Exclude) {
			return nil
		}
		add(path)
		return nil
	}
}

func discover(opts Options) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		abs, _ := filepath.Abs(p)
		if seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, p)
	}
	for _, p := range opts.Paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, walkDirFunc(opts, add))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case "vendor", "node_modules", ".git":
		return true
	}
	return false
}

func hasSuffix(path string, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}

func isExcluded(path string, exclude []string) bool {
	for _, e := range exclude {
		if e != "" && strings.Contains(path, e) {
			return true
		}
	}
	return false
}
