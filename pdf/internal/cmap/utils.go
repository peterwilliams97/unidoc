/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"errors"
	"unicode/utf16"

	"github.com/unidoc/unidoc/common"
)

// hexToCharCode returns the integer that is encoded in `shex` as a big-endian hex value
func hexToCharCode(shex cmapHexString) CharCode {
	val := CharCode(0)
	for _, v := range shex.b {
		val <<= 8
		val |= CharCode(v)
	}
	return val
}

// hexToString returns the unicode string that is UTF-16BE encoded in `shex`.
// 9.10.3 ToUnicode CMaps (page 293)
// • It shall use the beginbfchar, endbfchar, beginbfrange, and endbfrange operators to define the
// mapping from character codes to Unicode character sequences expressed in UTF-16BE encoding.
func hexToString(shex cmapHexString) string {

	return string(utf16ToRunes(shex))
}

// hexToString decodeds the UTF-16BE encoded string `shex` to unicode runes.
func utf16ToRunes(shex cmapHexString) []rune {
	if len(shex.b) == 1 {
		return []rune{rune(shex.b[0])}
	}
	b := shex.b
	if len(b)%2 != 0 {
		b = append(b, 0)
		err := errors.New("odd number of bytes")
		common.Log.Debug("ERROR: hexToRunes. padding shex=%#v to %+v err=%v", shex, b, err)
		panic(err)
	}
	n := len(b) / 2
	chars := make([]uint16, n)
	for i := 0; i < n; i++ {
		chars[i] = uint16(b[i])<<2 + uint16(b[i+1])
	}
	runes := utf16.Decode(chars)
	// fmt.Printf("hexToRunes: %#v->%#v->%#v\n", shex.b, chars, runes)
	return runes
}

// codeSize returns the number of bytes needed to represent `code`
//  1 for 0x0      ...0xFF
//  2 for 0x100    ...0xFFFF
//  3 for 0x10000  ...0xFFFFFF
//  4 for 0x1000000...0xFFFFFFFF
func codeSize(code CharCode) int {
	for i := 3; i >= 0; i-- {
		if code>>CharCode(i*8) != 0 {
			return i + 1
		}
	}
	return 1
}
