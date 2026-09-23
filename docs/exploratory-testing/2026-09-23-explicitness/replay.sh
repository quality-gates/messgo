#!/usr/bin/env bash
# Replays the confirmed explicitness findings from a clean scratch dir.
# Usage: replay.sh <messgo-binary>
set -u
BIN=${1:?messgo binary}
W=$(mktemp -d)
cd "$W"
printf 'module example.com/r\n\ngo 1.26\n' > go.mod
mkdir -p b1 b2 b3 o1

cat > b1/a.go <<'GO'
package b1

import "sort"

var list = []int{2, 1}

func Order() { sort.Ints(list) }

func First() int { return list[0] }

var buf = make([]byte, 4)

func Fill(src []byte) { copy(buf, src) }

func Head() byte { return buf[0] }
GO

cat > b2/a.go <<'GO'
package b2

import "context"

func Wait(ctx context.Context) { <-ctx.Done() }

func Control(done <-chan struct{}) { <-done }
GO

cat > b3/a.go <<'GO'
package b3

var n int

func Bump() { n = 1 }

type names map[int]string

func Label() names { return names{n: "x"} }

func Control() map[int]string { return map[int]string{n: "x"} }
GO

cat > o1/a.go <<'GO'
package o1

import "slices"

// Sorted copies s before it sorts, so the caller's slice does not change.
func Sorted(s []string) []string {
	s = append([]string(nil), s...)
	slices.Sort(s)
	return s
}
GO

echo "== B1 (#179) copy / in-place sort: expect GlobalVariable for list and buf"
"$BIN" ./b1 text design; echo "exit=$?"
echo "== B1 (#179) expect ImplicitInput for First (list) and Head (buf)"
"$BIN" ./b1 text explicitness; echo "exit=$?"
echo "== B2 (#180) <-ctx.Done(): expect ImplicitInput Wait and Control"
"$BIN" ./b2 text explicitness; echo "exit=$?"
echo "== B3 (#181) named map literal key: expect ImplicitInput Label and Control"
"$BIN" ./b3 text explicitness; echo "exit=$?"
echo "== O1 copy-on-write parameter (documented limit, still reported)"
"$BIN" ./o1 text explicitness; echo "exit=$?"
rm -rf "$W"
