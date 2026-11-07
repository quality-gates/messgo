package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
)

func TestDiscoverSourcesSelection(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a.go": "package p\n", "a_test.go": "package p\n", "ignore.go": "//go:build ignore\n\npackage p\n",
		"gen/b.go": "package p\n", "vendor/v.go": "package p\n", "note.txt": "text\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(dir, "...")
	got, err := DiscoverSources(DiscoveryOptions{Paths: []string{root, filepath.Join(dir, "a.go")}, IgnoreTests: true, ExcludePatterns: []string{"*/GEN/*"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.go")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverSources = %v, want %v", got, want)
	}
	got, err = DiscoverSources(DiscoveryOptions{Paths: []string{filepath.Join(dir, "ignore.go"), filepath.Join(dir, "a_test.go")}})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{filepath.Join(dir, "a_test.go"), filepath.Join(dir, "ignore.go")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit sources = %v, want %v", got, want)
	}
	if runtime.GOOS == "windows" {
		return
	}
	alias := filepath.Join(dir, "alias.go")
	if err := os.Symlink(filepath.Join(dir, "a.go"), alias); err != nil {
		t.Fatal(err)
	}
	got, err = DiscoverSources(DiscoveryOptions{Paths: []string{filepath.Join(dir, "a.go"), alias}})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{filepath.Join(dir, "a.go")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
}

func TestDiscoveryFilterCompilesPatternsOnce(t *testing.T) {
	calls := 0
	filter := newDiscoveryFilter(DiscoveryOptions{ExcludePatterns: []string{"*/gen/*", "*/vendor/*"}}, func(pattern string) *regexp.Regexp {
		calls++
		return ignorePatternRegexp(pattern)
	})
	for _, tc := range []struct {
		path string
		want bool
	}{{"pkg/a.go", true}, {"pkg/gen/b.go", false}, {"pkg/vendor/c.go", false}} {
		if got := filter.shouldIncludeFile(tc.path); got != tc.want {
			t.Fatalf("include %q = %v, want %v", tc.path, got, tc.want)
		}
	}
	if calls != 2 {
		t.Fatalf("compiled %d patterns, want 2", calls)
	}
}

func BenchmarkDiscoverSourcesPatterns(b *testing.B) {
	dir := b.TempDir()
	for i := 0; i < 100; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%03d.go", i)), []byte("package p\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	options := DiscoveryOptions{Paths: []string{dir}, ExcludePatterns: []string{"*/generated/*", "*/vendor/*", "*/other/*"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		files, err := DiscoverSources(options)
		if err != nil || len(files) != 100 {
			b.Fatalf("DiscoverSources = %d files, %v", len(files), err)
		}
	}
}
