package cli

import "testing"

func TestExplicitnessImportedEmbeddingMayHideLocalMethod(t *testing.T) {
	_, got := runExplicitness(t, "explicitness", map[string]string{"a.go": `package p
import "bytes"
type Deep struct{}
func (Deep) Write(p []byte) (int, error) { p[0] = 1; return len(p), nil }
type Mid struct{ Deep }
type Root struct{ Mid; bytes.Buffer }
func use(r *Root, p []byte) { _, _ = r.Write(p) }
`})
	assertFindings(t, got,
		"a.go:4 ImplicitOutput Deep.Write: write through parameter p",
		"a.go:7 UnknownEffect use: unresolved call")
}
