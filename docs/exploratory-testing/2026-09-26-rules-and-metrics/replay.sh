#!/usr/bin/env bash
# Replays the confirmed rules-and-metrics findings from a clean scratch dir.
# Usage: replay.sh <messgo-binary>
set -u
BIN=${1:?messgo binary}
case "$BIN" in /*) ;; *) BIN="$(pwd)/$BIN" ;; esac
W=$(mktemp -d)
cd "$W"
printf 'module example.com/r\n\ngo 1.26\n' > go.mod
mkdir -p b1 b2 b3 b4

cat > b1/a.go <<'GO'
package b1

import "sort"

var Arr [4]byte
var Slice = []int{2, 1}

func MutateGlobals() {
	copy(Arr[:], []byte{1, 2, 3, 4})
	sort.Ints(Slice[1:])
}

func MutateParam(s []int) {
	sort.Ints(s[1:])
}
GO

cat > b2/a.go <<'GO'
package b2

import "fmt"

func CheckIf(a, b bool) {
	if a {
		fmt.Println("duplicate body")
	} else if b {
		fmt.Println("duplicate body")
	}
}

func CheckType(v any) {
	switch v.(type) {
	case int:
		fmt.Println("type duplicate")
	case string:
		fmt.Println("type duplicate")
	}
}
GO

cat > b3/a.go <<'GO'
package b3

import "net/http"

var Statuses = map[int]string{
	http.StatusOK: "OK",
	http.StatusOK: "Duplicate OK",
}
GO

cat > b4/a.go <<'GO'
package b4

type Walker struct{}

func (w *Walker) Walk() {
	w.Walk()
}

func WalkFunc() {
	WalkFunc()
}
GO

cat > b4/rs.xml <<'XML'
<ruleset name="cog">
  <rule ref="codesize/CognitiveComplexity">
    <properties>
      <property name="reportLevel" value="1"/>
    </properties>
  </rule>
</ruleset>
XML

echo "== B1 (#194) slice expression mutations on globals: expect GlobalVariable for Arr and Slice"
"$BIN" ./b1 text design --only GlobalVariable; echo "exit=$?"
echo "== B1 (#194) slice expression mutation on param: expect ImplicitOutput for MutateParam"
"$BIN" ./b1 text explicitness; echo "exit=$?"
echo "== B2 (#195) identical branches in else if and type switch: expect IdenticalBranches for CheckIf and CheckType"
"$BIN" ./b2 text opinionated --only IdenticalBranches; echo "exit=$?"
echo "== B3 (#196) duplicate package-qualified map keys: expect DuplicatedArrayKey for http.StatusOK"
"$BIN" ./b3 text cleancode --only DuplicatedArrayKey; echo "exit=$?"
echo "== B4 (#197) method recursion in CognitiveComplexity: expect CognitiveComplexity for Walk() and WalkFunc()"
"$BIN" ./b4 text ./b4/rs.xml; echo "exit=$?"

rm -rf "$W"
