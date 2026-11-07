package runner

import (
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DiscoveryOptions selects source files without parsing or analyzing them.
type DiscoveryOptions struct {
	Paths           []string
	Suffixes        []string
	Exclude         []string
	ExcludePatterns []string
	IgnoreTests     bool
}

type discoveryFilter struct {
	options  DiscoveryOptions
	patterns []*regexp.Regexp
}

func walkDirFunc(root string, filter discoveryFilter, add func(string)) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipFile(d.Name()) {
			return nil
		}
		if !inBuildContext(path) || !filter.shouldIncludeFile(path) {
			return nil
		}
		add(path)
		return nil
	}
}

// inBuildContext reports whether a Go file found by a directory walk belongs to
// the default build context, honouring //go:build lines and GOOS/GOARCH
// filename suffixes. Non-Go files and files whose header cannot be read are
// kept so that suffix filters and parse error reporting still apply to them.
func inBuildContext(path string) bool {
	if filepath.Ext(path) != ".go" {
		return true
	}
	match, err := build.Default.MatchFile(filepath.Dir(path), filepath.Base(path))
	return match || err != nil
}

func newDiscoveryFilter(opts DiscoveryOptions, compile func(string) *regexp.Regexp) discoveryFilter {
	if len(opts.Suffixes) == 0 {
		opts.Suffixes = []string{".go"}
	}
	filter := discoveryFilter{options: opts}
	for _, pattern := range opts.ExcludePatterns {
		if re := compile(pattern); re != nil {
			filter.patterns = append(filter.patterns, re)
		}
	}
	return filter
}

// DiscoverSources returns sorted, deduplicated paths selected by the run's discovery rules.
func DiscoverSources(opts DiscoveryOptions) ([]string, error) {
	filter := newDiscoveryFilter(opts, ignorePatternRegexp)
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = normalizePath(p)
		identity := pathIdentity(p)
		if seen[identity] {
			return
		}
		seen[identity] = true
		out = append(out, p)
	}
	for _, p := range opts.Paths {
		root := discoveryRoot(p)
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !filter.shouldIncludeFile(root) {
				continue
			}
			add(root)
			continue
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		root = normalizePath(root)
		err = filepath.WalkDir(root, walkDirFunc(root, filter, add))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func pathIdentity(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return normalizePath(resolved)
	}
	return normalizePath(abs)
}

func discoveryRoot(p string) string {
	if filepath.Base(p) != "..." {
		return p
	}
	return filepath.Dir(p)
}

func (f discoveryFilter) shouldIncludeFile(path string) bool {
	if !hasSuffix(path, f.options.Suffixes) || f.options.IgnoreTests && strings.HasSuffix(path, "_test.go") || isExcluded(path, f.options.Exclude) {
		return false
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	for _, re := range f.patterns {
		if matchIgnorePath(re, path) {
			return false
		}
	}
	return true
}

func ignorePatternRegexp(pattern string) *regexp.Regexp {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	quoted := regexp.QuoteMeta(slashPath(pattern))
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	return regexp.MustCompile("(?i)^(?:" + quoted + ")")
}

func matchIgnorePath(re *regexp.Regexp, path string) bool {
	path = slashPath(path)
	for {
		if re.MatchString(path) || re.MatchString("/"+path) {
			return true
		}
		slash := strings.Index(path, "/")
		if slash < 0 {
			return false
		}
		path = path[slash+1:]
	}
}

func slashPath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func shouldSkipDir(name string) bool {
	if name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "testdata":
		return true
	}
	return false
}

func shouldSkipFile(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
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
	path = normalizePath(path)
	for _, e := range exclude {
		if e != "" && strings.Contains(path, normalizePath(e)) {
			return true
		}
	}
	return false
}

func normalizePath(p string) string {
	return filepath.Clean(p)
}
