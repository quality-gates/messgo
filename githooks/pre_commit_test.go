//go:build unix

package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPreCommitIgnoresUntrackedFiles(t *testing.T) {
	repo, env := newHookRepo(t)
	writeRepoFile(t, repo, "CHANGELOG.md", "# staged\n")
	runGit(t, repo, "add", "CHANGELOG.md")
	writeRepoFile(t, repo, "complex.go", "package sample\n// BAD_CYCLO\n")

	output, err := runHook(repo, env)
	assertNoHookTempDirs(t, filepath.Join(repo, "tmp"))
	if err != nil {
		t.Fatalf("hook rejected a clean staged tree: %v\n%s", err, output)
	}
	if !strings.Contains(output, "pre-commit: all checks passed") {
		t.Fatalf("hook output does not show success: %s", output)
	}
}

func TestPreCommitUsesStagedContent(t *testing.T) {
	repo, env := newHookRepo(t)
	writeRepoFile(t, repo, "complex.go", "package sample\n")
	runGit(t, repo, "add", "complex.go")
	runGit(t, repo, "commit", "-qm", "add fixture", "--no-verify")

	writeRepoFile(t, repo, "complex.go", "package sample\n// BAD_CYCLO\n")
	runGit(t, repo, "add", "complex.go")
	writeRepoFile(t, repo, "complex.go", "package sample\n")

	output, err := runHook(repo, env)
	assertNoHookTempDirs(t, filepath.Join(repo, "tmp"))
	if err == nil {
		t.Fatalf("hook accepted a staged failing tree after an unstaged fix:\n%s", output)
	}
	if !strings.Contains(output, "pre-commit: functions over cyclomatic complexity 15 found") {
		t.Fatalf("hook did not report the staged failure: %s", output)
	}
}

func TestPreCommitCleansUpOnInterrupt(t *testing.T) {
	repo, env := newHookRepo(t)
	ready := filepath.Join(repo, "ready")
	env = append(env, "HOOK_SLEEP=1", "HOOK_READY="+ready)
	cmd := exec.Command(filepath.Join(repo, "githooks", "pre-commit"))
	cmd.Dir = repo
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, ready)
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("interrupted hook exited successfully")
	}
	assertNoHookTempDirs(t, filepath.Join(repo, "tmp"))
}

func TestPreCommitClearsGitEnvironment(t *testing.T) {
	repo, env := newHookRepo(t)
	tmp := t.TempDir()
	writeRepoFile(t, repo, "CHANGELOG.md", "# staged\n")
	runGit(t, repo, "add", "CHANGELOG.md")
	env = append(env,
		"CHECK_GIT_ENV=1",
		"GIT_DIR="+filepath.Join(repo, ".git"),
		"GIT_WORK_TREE="+repo,
		"TMPDIR="+tmp,
	)

	output, err := runHook(repo, env)
	assertNoHookTempDirs(t, tmp)
	if err != nil {
		t.Fatalf("hook rejected a clean staged tree with hook git variables: %v\n%s", err, output)
	}
}

func TestPreCommitBlocksStagedFailures(t *testing.T) {
	cases := []struct {
		name    string
		marker  string
		message string
	}{
		{name: "go vet", marker: "FAIL_VET", message: "pre-commit: go vet failed"},
		{name: "gocyclo", marker: "BAD_CYCLO", message: "pre-commit: functions over cyclomatic complexity 15 found"},
		{name: "go test", marker: "FAIL_TEST", message: "pre-commit: go test failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, env := newHookRepo(t)
			writeRepoFile(t, repo, "complex.go", "package sample\n")
			runGit(t, repo, "add", "complex.go")
			runGit(t, repo, "commit", "-qm", "add fixture", "--no-verify")

			writeRepoFile(t, repo, "complex.go", "package sample\n// "+tc.marker+"\n")
			runGit(t, repo, "add", "complex.go")
			writeRepoFile(t, repo, "complex.go", "package sample\n")

			output, err := runHook(repo, env)
			assertNoHookTempDirs(t, filepath.Join(repo, "tmp"))
			if err == nil {
				t.Fatalf("hook accepted a staged %s failure after an unstaged fix:\n%s", tc.name, output)
			}
			if !strings.Contains(output, tc.message) {
				t.Fatalf("hook did not report the staged %s failure: %s", tc.name, output)
			}
		})
	}
}

func newHookRepo(t *testing.T) (string, []string) {
	t.Helper()
	repo := t.TempDir()
	bin := filepath.Join(repo, "bin")
	tmp := filepath.Join(repo, "tmp")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeTools(t, bin)
	writeRepoFile(t, repo, "go.mod", "module example\n\ngo 1.26\n")
	writeRepoFile(t, repo, "CHANGELOG.md", "# changelog\n")
	copyHook(t, repo)
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Hook Test")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "initial", "--no-verify")

	env := append([]string{}, os.Environ()...)
	env = append(env, "PATH="+bin+":"+os.Getenv("PATH"))
	env = append(env, "TMPDIR="+tmp)
	return repo, env
}

func assertNoHookTempDirs(t *testing.T, tmp string) {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary hook files remain: %v", entries)
	}
}

func copyHook(t *testing.T, repo string) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	hook, err := os.ReadFile(filepath.Join(filepath.Dir(source), "pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, "githooks")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pre-commit"), hook, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFakeTools(t *testing.T, bin string) {
	t.Helper()
	writeExecutable(t, filepath.Join(bin, "gofmt"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "ineffassign"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "gocyclo"), `#!/bin/sh
if [ "$CHECK_GIT_ENV" = "1" ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "git environment leaked"
fi
if [ -f complex.go ] && grep -q BAD_CYCLO complex.go; then
    echo "18 sample complex.go:2:1"
fi
`)
	writeExecutable(t, filepath.Join(bin, "go"), `#!/bin/sh
case "$1" in
vet)
    if [ "$HOOK_SLEEP" = "1" ]; then
        : > "$HOOK_READY"
        sleep 30
    fi
    if [ -f complex.go ] && grep -q FAIL_VET complex.go; then
        echo "vet failed" >&2
        exit 1
    fi
    ;;
test)
    if [ -f complex.go ] && grep -q FAIL_TEST complex.go; then
        echo "test failed" >&2
        exit 1
    fi
    ;;
build)
    previous=
    for argument in "$@"; do
        if [ "$previous" = "-o" ]; then
            printf '#!/bin/sh\nexit 0\n' > "$argument"
            chmod +x "$argument"
        fi
        previous=$argument
    done
    ;;
esac
exit 0
`)
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeRepoFile(t *testing.T, repo, name, contents string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func runHook(repo string, env []string) (string, error) {
	cmd := exec.Command(filepath.Join(repo, "githooks", "pre-commit"))
	cmd.Dir = repo
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
