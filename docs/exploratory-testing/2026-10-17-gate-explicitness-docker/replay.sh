#!/bin/sh
# Replay the 2026-10-17 exploratory findings against a messgo binary.
# Usage: replay.sh /absolute/path/to/messgo
set -u

BIN=${1:?usage: replay.sh /path/to/messgo}
ROOT=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO=$(CDPATH= cd -- "$ROOT/../../.." && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/messgo-et-replay.XXXXXX")
export GOMAXPROCS=${GOMAXPROCS:-1}

cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

run3() {
	name=$1
	shift
	echo "===== $name ====="
	i=1
	while [ "$i" -le 3 ]; do
		echo "-- run $i --"
		"$@"
		echo "exit=$?"
		i=$((i + 1))
	done
}

crash_once() {
	dir=$WORK/crash
	rm -rf "$dir"
	mkdir -p "$dir"
	cat > "$dir/p.go" << 'EOF'
package p

func f() int {
	var s T
	g(&s)
	return s.f
}
EOF
	"$BIN" "$dir/p.go" text explicitness > "$dir/out" 2> "$dir/err"
	echo "exit=$?"
	if grep -q "stack overflow" "$dir/err"; then
		echo "stack overflow"
	else
		echo "NO stack overflow"
		head -5 "$dir/err"
	fi
	echo "stdout_bytes=$(wc -c < "$dir/out" | tr -d ' ')"
}

receiver_once() {
	dir=$WORK/receiver
	rm -rf "$dir"
	mkdir -p "$dir"
	cat > "$dir/p.go" << 'EOF'
package p

type Counter struct{ n int }

func (c *Counter) Inc() { c.n++ }

func (c *Counter) N() int { return c.n }

func (c Counter) Read() int { return c.n }

func (c *Counter) Pure(x int) int { return x + 1 }
EOF
	echo "-- explicitness --"
	"$BIN" "$dir/p.go" text explicitness
	echo "exit=$?"
	echo "-- explicitness-strict --"
	"$BIN" "$dir/p.go" text explicitness-strict
	echo "exit=$?"
}

done_once() {
	dir=$WORK/done
	rm -rf "$dir"
	mkdir -p "$dir"
	cat > "$dir/p.go" << 'EOF'
package p

import "context"

func retDone(ctx context.Context) <-chan struct{} { return ctx.Done() }

func retErr(ctx context.Context) error { return ctx.Err() }

func recv(ctx context.Context) { <-ctx.Done() }
EOF
	"$BIN" "$dir/p.go" text explicitness
	echo "exit=$?"
}

alias_once() {
	dir=$WORK/alias
	rm -rf "$dir"
	mkdir -p "$dir"
	cat > "$dir/p.go" << 'EOF'
package p

func alias(p *int) {
	q := p
	*q = 1
}

func direct(p *int) { *p = 1 }
EOF
	"$BIN" "$dir/p.go" text explicitness
	echo "exit=$?"
}

slow_once() {
	src=$REPO/internal/model/build.go
	[ -f "$src" ] || fail "missing $src"
	dir=$WORK/slow
	rm -rf "$dir"
	mkdir -p "$dir"
	python3 - "$src" "$dir/extract.go" << 'PY'
import pathlib, re, sys
src, dest = sys.argv[1:]
text = pathlib.Path(src).read_text()
parts = re.split(r"\nfunc ", text)
header, funcs = parts[0], parts[1:]
wanted = ("ParseSource(", "(b *fileBuilder) build()", "(b *fileBuilder) collectTypes()", "(b *fileBuilder) collectTypeSpec(")
picked = []
for func in funcs:
    sig = func.split("{", 1)[0]
    if any(w in sig for w in wanted):
        picked.append(func)
if len(picked) != 4:
    raise SystemExit(f"expected 4 functions, found {len(picked)}")
pathlib.Path(dest).write_text(header + "".join("\nfunc " + f for f in picked))
PY
	echo "extract_lines=$(wc -l < "$dir/extract.go" | tr -d ' ')"
	python3 - "$BIN" "$dir/extract.go" << 'PY'
import subprocess, sys, os
bin, path = sys.argv[1:]
env = os.environ.copy()
env["GOMAXPROCS"] = "1"
try:
    p = subprocess.run([bin, path, "text", "explicitness"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=8, env=env)
except subprocess.TimeoutExpired:
    print("TIMEOUT >8s")
    raise SystemExit(0)
err = p.stderr.decode(errors="replace")
print(f"finished exit={p.returncode} overflow={'stack overflow' in err}")
PY
	"$BIN" "$dir/extract.go" text go > "$dir/go.out"
	echo "go_ruleset_exit=$? go_lines=$(wc -l < "$dir/go.out" | tr -d ' ')"
}

echo "binary: $BIN"
"$BIN" --version
echo "repo: $REPO"
echo

run3 "Bug 1 stack overflow" crash_once
run3 "Bug 2 receiver policy" receiver_once
run3 "Bug 3 ctx.Done" done_once
run3 "Bug 4 local alias named as parameter" alias_once
run3 "Bug 5 cross-function explicitness does not finish" slow_once
