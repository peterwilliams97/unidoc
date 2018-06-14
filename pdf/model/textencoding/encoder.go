/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package textencoding

import . "github.com/unidoc/unidoc/pdf/core"

type TextEncoder interface {
	// !@#$ This is copied between implmentations
	// Convert a raw utf8 string (series of runes) to an encoded string (series of bytes) to be used
	// in PDF.
	Encode(raw string) string

	// Conversion between character code and glyph name.
	// The bool return flag is true if there was a match, and false otherwise.
	CharcodeToGlyph(code uint16) (string, bool)

	// Conversion between glyph name and character code.
	// The bool return flag is true if there was a match, and false otherwise.
	GlyphToCharcode(glyph string) (uint16, bool)

	// Convert rune to character code.
	// The bool return flag is true if there was a match, and false otherwise.
	RuneToCharcode(val rune) (uint16, bool)

	// Convert character code to rune.
	// The bool return flag is true if there was a match, and false otherwise.
	CharcodeToRune(charcode uint16) (rune, bool)

	// Convert rune to glyph name.
	// The bool return flag is true if there was a match, and false otherwise.
	RuneToGlyph(val rune) (string, bool)

	// Convert glyph to rune.
	// The bool return flag is true if there was a match, and false otherwise.
	GlyphToRune(glyph string) (rune, bool)

	ToPdfObject() PdfObject
}

// Convenience functions

// Encode
func doEncode(enc TextEncoder, raw string) string {
	encoded := []byte{}
	for _, rune := range raw {
		code, found := enc.RuneToCharcode(rune)
		if !found {
			continue
		}
		encoded = append(encoded, byte(code))
	}
	return string(encoded)
}

// Convert rune to character code.
// The bool return flag is true if there was a match, and false otherwise.
func doRuneToCharcode(enc TextEncoder, val rune) (uint16, bool) {
	g, ok := enc.RuneToGlyph(val)
	if !ok {
		return 0, false
	}
	return enc.GlyphToCharcode(g)
}

func doCharcodeToRune(enc TextEncoder, code uint16) (rune, bool) {
	g, ok := enc.CharcodeToGlyph(code)
	if !ok {
		return 0, false
	}
	return enc.GlyphToRune(g)
}
