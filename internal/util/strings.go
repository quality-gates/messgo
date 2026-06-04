// Package util provides small helpers shared across rules: string handling
// that mirrors PHPMD\Utility\Strings, and Go AST walking utilities.
package util

import (
	"slices"
	"strings"
)

// SplitToList splits a comma-separated property value, trimming entries and
// dropping empties (PHPMD\Utility\Strings::splitToList).
func SplitToList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// LengthWithoutPrefixesAndSuffixes returns the length of name after removing
// the first matching suffix and first matching prefix
// (PHPMD\Utility\Strings::lengthWithoutPrefixesAndSuffixes).
func LengthWithoutPrefixesAndSuffixes(name string, prefixes, suffixes []string) int {
	length := len(name)
	for _, suffix := range suffixes {
		if suffix != "" && strings.HasSuffix(name, suffix) {
			length -= len(suffix)
			break
		}
	}
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(name, prefix) {
			length -= len(prefix)
			break
		}
	}
	return length
}

// Contains reports whether list contains s.
func Contains(list []string, s string) bool {
	return slices.Contains(list, s)
}
