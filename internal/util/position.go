package util

import "go/token"

// PhysicalPosition returns the position of p in the scanned file.
// A line directive does not adjust the result.
func PhysicalPosition(fset *token.FileSet, p token.Pos) token.Position {
	return fset.PositionFor(p, false)
}
