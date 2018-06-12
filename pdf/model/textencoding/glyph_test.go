/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package textencoding

import (
	"testing"
)

// TestGlypRune tests that glyphlistGlyphToRuneMap and glyphlistRuneToGlyphMap match
func TestGlypRune(t *testing.T) {
	for r, g := range glyphlistRuneToGlyphMap {
		r2, ok := glyphlistGlyphToRuneMap[g]
		if !ok {
			t.Errorf("rune->glyph->rune mismatch: %c (0x%04X) -> %q %c (0x%04X)", r, r, g, r2, r2)
		}
	}

	for g, r := range glyphlistGlyphToRuneMap {
		g2, ok := glyphlistRuneToGlyphMap[r]
		if !ok {
			t.Errorf("glyph->rune-glyph mismatch: %q -> %c (0x%04X) %q", g, r, r, g2)
		}
	}
}
