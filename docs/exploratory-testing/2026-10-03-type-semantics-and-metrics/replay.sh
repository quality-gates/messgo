#!/usr/bin/env bash
# Replays the confirmed type-semantics-and-metrics findings from a clean scratch dir.
# Usage: replay.sh <messgo-binary>
set -u
BIN=${1:?messgo binary}
case "$BIN" in /*) ;; *) BIN="$(pwd)/$BIN" ;; esac
W=$(mktemp -d)
cd "$W"
printf 'module example.com/r\n\ngo 1.26\n' > go.mod
mkdir -p b1 b2 b3 b4

# B1 (#234): CognitiveComplexity ignores nesting inside else blocks
cat > b1/a.go <<'GO'
package b1

func NestedInIf(a, b bool) {
	if a {
		if b {
		}
	}
}

func NestedInElse(a, b bool) {
	if a {
	} else {
		if b {
		}
	}
}
GO

cat > b1/rs.xml <<'XML'
<ruleset name="cog">
  <rule ref="codesize/CognitiveComplexity">
    <properties>
      <property name="reportLevel" value="1"/>
    </properties>
  </rule>
</ruleset>
XML

# B2 (#235): InterfaceMethodSatisfied fails to recognize type aliases (any vs interface{})
cat > b2/a.go <<'GO'
package b2

type handler interface {
	handle(any) bool
}

type worker struct{}

func (w *worker) handle(val interface{}) bool {
	return val != nil
}
GO

# B3 (#236): DuplicatedArrayKey ignores duplicate keys with type conversions
cat > b3/a.go <<'GO'
package b3

import "time"

var Durations = map[time.Duration]string{
	time.Duration(1): "one",
	time.Duration(1): "duplicate one",
}

type StatusCode int

var Codes = map[StatusCode]string{
	StatusCode(200): "ok",
	StatusCode(200): "duplicate ok",
}
GO

# B4 (#237): UnusedPrivateMethod ignores unexported methods on non-struct named types
cat > b4/a.go <<'GO'
package b4

type StatusCode int

func (s StatusCode) unusedHelper() {
}

type HandlerFunc func() error

func (h HandlerFunc) unusedMethod() {
}
GO

echo "== B1 (#234) CognitiveComplexity inside else block: expect NestedInElse complexity 4, got 3"
"$BIN" ./b1 text ./b1/rs.xml; echo "exit=$?"

echo "== B2 (#235) InterfaceMethodSatisfied any vs interface{}: expect exit 0, got exit 2 false positive"
"$BIN" ./b2 text unusedcode; echo "exit=$?"

echo "== B3 (#236) DuplicatedArrayKey with type conversions: expect exit 2, got exit 0 false negative"
"$BIN" ./b3 text cleancode --only DuplicatedArrayKey; echo "exit=$?"

echo "== B4 (#237) UnusedPrivateMethod on non-struct types: expect exit 2, got exit 0 false negative"
"$BIN" ./b4 text unusedcode; echo "exit=$?"

rm -rf "$W"
