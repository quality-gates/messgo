//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReportFileParentDirPermissions(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)

	path := writeFixture(t, "package p\nfunc f(a int) int { return a }\n")
	dir := filepath.Join(t.TempDir(), "reports")
	code, _, errOut := runMain(t, path, "text", "codesize", "--reportfile", filepath.Join(dir, "messgo.txt"))
	if code != ExitSuccess {
		t.Fatalf("exit = %d, want %d (stderr %q)", code, ExitSuccess, errOut)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("created dir mode = %o, want 755", got)
	}
}
