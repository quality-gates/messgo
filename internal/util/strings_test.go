package util

import "testing"

func TestLengthWithoutPrefixesAndSuffixesCountsCharacters(t *testing.T) {
	cases := []struct {
		name               string
		prefixes, suffixes []string
		want               int
	}{
		{"überÄnderungsgrößen", nil, nil, 19},
		{"größeÜbersicht", []string{"größe"}, nil, 9},
		{"zählerGröße", nil, []string{"Größe"}, 6},
	}
	for _, tc := range cases {
		if got := LengthWithoutPrefixesAndSuffixes(tc.name, tc.prefixes, tc.suffixes); got != tc.want {
			t.Errorf("LengthWithoutPrefixesAndSuffixes(%q, %v, %v) = %d, want %d",
				tc.name, tc.prefixes, tc.suffixes, got, tc.want)
		}
	}
}
