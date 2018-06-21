/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import "bytes"

func hexToCharCode(shex cmapHexString) CharCode {
	val := CharCode(0)

	for _, v := range shex.b {
		val <<= 8
		val |= CharCode(v)
	}

	return val
}

func hexToString(shex cmapHexString) string {
	var buf bytes.Buffer

	// Assumes unicode in format <HHLL> with 2 bytes HH and LL representing a rune.
	for i := 0; i < len(shex.b)-1; i += 2 {
		b1 := uint64(shex.b[i])
		b2 := uint64(shex.b[i+1])
		r := rune((b1 << 8) | b2)

		buf.WriteRune(r)
	}

	return buf.String()
}
