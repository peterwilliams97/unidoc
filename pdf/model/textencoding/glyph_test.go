/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package textencoding

import (
	"fmt"
	"testing"
	"unicode"
)

// TestGlypRune tests that glyphlistGlyphToRuneMap and glyphlistRuneToGlyphMap match
func TestGlypRune(t *testing.T) {
	for r, g := range glyphlistRuneToGlyphMap {
		r2, ok := glyphlistGlyphToRuneMap[g]
		if !ok {
			t.Errorf("rune→glyph→rune mismatch: %s → %q → %s", rs(r), g, rs(r2))
		}
	}

	for g, r := range glyphlistGlyphToRuneMap {
		g2, ok := glyphlistRuneToGlyphMap[r]
		if !ok {
			t.Errorf("glyph→rune→glyph mismatch: %q → %s → %q", g, rs(r), g2)
		}
	}
}

func rs(r rune) string {
	c := "unprintable"
	if unicode.IsPrint(r) {
		c = fmt.Sprintf("%c", r)
	}
	return fmt.Sprintf(`'\u%04x' (%s)`, r, c)
}
