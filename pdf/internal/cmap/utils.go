/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"sort"
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
	n := len(b) >> 1
	chars := make([]uint16, n)
	for i := 0; i < n; i++ {
		chars[i] = uint16(b[i<<1])<<8 + uint16(b[i<<1+1])
	}
	runes := utf16.Decode(chars)
	return runes
}

type ByteRange struct {
	lo    CharCode
	hi    CharCode
	codes map[CharCode]string
}

// isPrefixFree returns true if `codes` is prefix free
// https://en.wikipedia.org/wiki/Prefix_code

type CMapResult struct {
	nbits   int
	hash    string
	cmap    *CMap
	data    []byte
	maxCode CharCode
}

type CMapExtreme struct {
	nbits       int
	hashes      []string
	maxCode     CharCode
	maxNumCodes int
}

func (e *CMapExtreme) update(r CMapResult) (changed bool) {
	if r.maxCode > e.maxCode {
		e.maxCode = r.maxCode
		changed = true
	}
	if len(r.cmap.codeToUnicode) > e.maxNumCodes {
		e.maxNumCodes = len(r.cmap.codeToUnicode)
	}
	e.hashes = append(e.hashes, r.hash)
	return
}

var (
	cmapLog    = openLog("cmap.log")
	CurrentPDF = ""
	extremes8  = CMapExtreme{}
	extremes16 = CMapExtreme{}
	fileHashes = map[string][]string{}
	hashFiles  = map[string][]string{}
	hashCmap   = map[string]CMapResult{}
)

func openLog(filename string) *os.File {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		panic(err)
	}
	return f
}

func SetPDFFile(filename string) {
	CurrentPDF = filename
	fmt.Fprintln(cmapLog, "==========================================")
	fmt.Fprintf(cmapLog, "CurrentPDF=%q\n", CurrentPDF)
}

func logCMap(cmap *CMap, data []byte, nbits int) {
	return
	if CurrentPDF == "" {
		panic("No CurrentPDF")
	}
	r := analyze(cmap, data, nbits)
	_, duplicate := hashCmap[r.hash]
	fileHashes[CurrentPDF] = append(fileHashes[CurrentPDF], r.hash)
	hashFiles[r.hash] = append(hashFiles[r.hash], CurrentPDF)
	hashCmap[r.hash] = r
	var extremes *CMapExtreme
	if nbits == 8 {
		extremes = &extremes8
	} else {
		extremes = &extremes16
	}
	changed := extremes.update(r)

	fmt.Fprintln(cmapLog, "------------------------------------------")
	fmt.Fprintf(cmapLog, "%d: nbits=%d %s\n", len(hashFiles), nbits, cmap.String())
	if duplicate {
		return
	}

	fmt.Fprintf(cmapLog, "%s\n", string(data))
	if changed {
		fmt.Fprintf(cmapLog, "New record %2d: maxCode=0x%04x maxNumCodes=%5d %q\n",
			nbits, extremes.maxCode, extremes.maxNumCodes, CurrentPDF)
		fmt.Fprintf(os.Stderr, "New record %2d: maxCode=0x%04x maxNumCodes=%5d %q\n",
			nbits, extremes.maxCode, extremes.maxNumCodes, CurrentPDF)
	}
}

func analyze(cmap *CMap, data []byte, nbits int) (r CMapResult) {
	r.nbits = nbits
	r.cmap = cmap
	r.data = data
	r.hash = hashData(data)
	for code := range cmap.codeToUnicode {
		if code > r.maxCode {
			r.maxCode = code
		}
	}
	return
}

func hashData(data []byte) string {
	arr := sha1.Sum(data)
	slc := arr[:]
	return string(slc)
}

// https://en.wikipedia.org/wiki/Prefix_code
func (cmap *CMap) codespacePrefixFree() bool {
	order, numSpace := byNumBytes(cmap.codespaces)
	for i := 1; i < len(order); i++ {
		n0, n1 := order[i-1], order[i]
		codespaces0, codespaces1 := numSpace[n0], numSpace[n1]
		for _, cs0 := range codespaces0 {
			for _, cs1 := range codespaces1 {
				// fmt.Printf("--- cs0=%#v\n", cs0)
				// fmt.Printf("+++ cs1=%#v\n", cs1)
				if isCodespacePrefix(cs0, cs1) {
					common.Log.Debug("ERROR: Not prefix-free. cmap=%s", cmap)
					return false
				}
				// fmt.Println("ok ===========")
			}
		}
	}
	return true
}

// byNumBytes returns a map of `codespaces` keyed by number of bytes
// `order` is the keys of numSpace in ascending order
func byNumBytes(codespaces []Codespace) (order []int, numSpace map[int][]Codespace) {
	numSpace = map[int][]Codespace{}
	for _, cs := range codespaces {
		numSpace[cs.NumBytes] = append(numSpace[cs.NumBytes], cs)
	}
	for num := range numSpace {
		order = append(order, num)
		sort.Slice(numSpace[num],
			func(i, j int) bool {
				return numSpace[num][i].Low < numSpace[num][j].Low
			})
	}
	sort.Ints(order)
	return
}

// isCodespacePrefix returns true is any code in `cs0` is a prefix of a code in `cs1`.
// |----|----|----|----|
//           |cs0 |
//           |   cs1   |
func isCodespacePrefix(cs0, cs1 Codespace) bool {
	if cs1.NumBytes <= cs0.NumBytes {
		panic("gggg")
	}
	shift := uint(cs1.NumBytes-cs0.NumBytes) * 8
	lo1, hi1 := cs1.Low>>shift, cs1.High>>shift
	return (cs0.Low <= lo1 && lo1 <= cs0.High) || (cs0.Low <= hi1 && hi1 <= cs0.High)
}
