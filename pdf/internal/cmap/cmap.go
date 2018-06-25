/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"errors"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
)

// CharCode is a character code or Unicode
// rune is int32 https://golang.org/doc/go1#rune
type CharCode uint32

// Maximum number of possible bytes per code.
const maxCodeLen = 4

// CID is a character ID
type CID int64

type CIDRange struct {
	From CharCode
	To   CharCode
	Cid  CID
}

// Codespace represents a single codespace range used in the CMap.
type Codespace struct {
	NumBytes int
	Low      CharCode
	High     CharCode
}

// CIDSystemInfo=Dict("Registry": Adobe, "Ordering": Korea1, "Supplement": 0, )
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int
}

// CMap represents a character code to unicode mapping used in PDF files.
//
// 9.7.5 CMaps (Page 272)
//
// A CMap shall specify the mapping from character codes to CIDs in a CIDFont. A CMap serves a
// function analogous to the Encoding dictionary for a simple font. The CMap shall not refer
// directly to a specific CIDFont; instead, it shall be combined with it as part of a CID-keyed
// font, represented in PDF as a Type 0 font dictionary (see 9.7.6, "Type 0 Font Dictionaries").
// PDF also uses a special type of CMap to map character codes to Unicode values (see 9.10.3,
// "ToUnicode CMaps").
//
// Page 278
// c) The beginbfchar and endbfchar shall not appear in a CMap that is used as the Encoding entry of
// a Type 0 font; however, they may appear in the definition of a ToUnicode CMap
//
// https://www.adobe.com/content/dam/acom/en/devnet/acrobat/pdfs/5411.ToUnicode.pdf
// https://github.com/adobe-type-tools/cmap-resources/releases ***
type CMap struct {
	*cMapParser

	name       string
	nbits      int // 8 bits for simple fonts, 16 bits for CID fonts
	ctype      int
	version    string
	usecmap    string // Base this cmap on usecmap if usecmap is not empty
	systemInfo CIDSystemInfo

	// Text encoder to look up runes from input glyph names. !@#$ Not used
	// encoder textencoding.TextEncoder

	// For ToUnicode (ctype 2) cmaps
	codeToUnicode map[CharCode]string

	// For regular cmaps
	codespaces []Codespace
	cidRanges  []CIDRange
	codeToCID  map[CharCode]CID
}

// String retuns a human readable description of `cmap`
func (cmap *CMap) String() string {
	si := cmap.systemInfo
	parts := []string{
		fmt.Sprintf("nbits:%d", cmap.nbits),
		fmt.Sprintf("type:%d", cmap.ctype),
	}
	if cmap.version != "" {
		parts = append(parts, fmt.Sprintf("version:%s", cmap.version))
	}
	parts = append(parts, fmt.Sprintf("systemInfo:%s", si.String()))
	if len(cmap.codeToUnicode) > 0 {
		parts = append(parts, fmt.Sprintf("codeToUnicode:%d", len(cmap.codeToUnicode)))
	}
	if len(cmap.codespaces) > 0 {
		parts = append(parts, fmt.Sprintf("codespaces:%d", len(cmap.codespaces)))
	}
	if len(cmap.cidRanges) > 0 {
		parts = append(parts, fmt.Sprintf("cidRanges:%d", len(cmap.cidRanges)))
	}
	if len(cmap.codeToCID) > 0 {
		parts = append(parts, fmt.Sprintf("codeToCID:%d", len(cmap.codeToCID)))
	}
	return fmt.Sprintf("CMAP{%#q %s}", cmap.name, strings.Join(parts, " "))
}

// newCMap returns an initialized CMap.
func newCMap(nbits int) *CMap {
	cmap := &CMap{
		nbits:         nbits,
		codeToUnicode: map[CharCode]string{},
		codeToCID:     map[CharCode]CID{},
	}
	return cmap
}

// printCodeToUnicode is a debugging funcion
func (cmap *CMap) printCodeToUnicode() {
	codes := []CharCode{}
	for c := range cmap.codeToUnicode {
		codes = append(codes, c)
	}
	fmt.Printf("--- printCodeToUnicode %d codes\n", len(codes))
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	for _, c := range codes {
		fmt.Printf("0x%04x %+q\n", c, cmap.codeToUnicode[c])
	}
}

// GetPredefinedCmap returns predefined cmap with name `name` if it exists
// It looks up and applies usecmap entries in the cmap
func GetPredefinedCmap(name string) (*CMap, error) {
	cmap, ok := getPredefinedCmap(name)
	if !ok {
		return nil, errors.New("Resource does not exist")
	}
	return applyAncestors(cmap)
}

// getPredefinedCmap returns predefined cmap with name `name` if it exists
// It doesn't apply usecmap entries in the cmap
func getPredefinedCmap(name string) (*CMap, bool) {
	cmap, ok := cmapTable[name]
	if !ok {
		common.Log.Debug("GetPredefinedCmap %#q doesn't exist", name)
	}
	return &cmap, ok
}

// applyAncestors looks up and applies the usecmap entries in `cmap`
// It does this recursively. XXX: TODO: Remove this function and call updateParent directly as
// there is only ever one ancestor
func applyAncestors(cmap *CMap) (*CMap, error) {
	ancestors := []*CMap{cmap}
	names := map[string]bool{}
	for {
		if cmap.usecmap == "" {
			break
		}
		parent, ok := getPredefinedCmap(cmap.usecmap)
		if !ok {
			break
		}
		cmap = parent
		if _, ok := names[cmap.name]; ok {
			common.Log.Debug("Circular reference. name=%#q, names=%+v", cmap.name, names)
			return nil, errors.New("Circular reference")
		}
		ancestors = append(ancestors, cmap)
	}
	parent := ancestors[len(ancestors)-1]
	for i := len(ancestors) - 2; i >= 0; i-- {
		var err error
		parent, err = updateParent(parent, ancestors[i])
		if err != nil {
			return nil, err
		}
	}
	return parent, nil
}

// updateParent applies the non-empty entries in `child` to the base map `parent` and returns the
// resulting CMap
func updateParent(parent, child *CMap) (*CMap, error) {
	base := *parent
	base.name = child.name
	base.ctype = child.ctype
	base.usecmap = child.usecmap
	base.systemInfo = child.systemInfo

	if len(child.codespaces) > 0 {
		base.codespaces = child.codespaces
	}
	if len(child.cidRanges) > 0 {
		base.cidRanges = child.cidRanges
	}
	return &base, nil
}

// GetPredefinedCidToRune returns a predefined CID to rune map for `info`
// In the special case of Adobe-Identity, an empty map is returned <--- !@#$ Stupid hack
func GetPredefinedCidToRune(info CIDSystemInfo) (map[CID]rune, bool) {
	name := info.IndexString()
	if name == "Adobe-Identity" {
		return map[CID]rune{}, true
	}
	c2r, ok := cid2Rune[name]
	return c2r, ok
}

// ToCID returns the CID for character code `code`
// returns 0 if no match
func (cmap *CMap) ToCID(code CharCode) CID {
	for _, r := range cmap.cidRanges {
		if r.From <= code && code <= r.To {
			return r.Cid - CID(r.From) + CID(code)
		}
	}
	return 0
}

// String returns a human readable description of `info`
func (info *CIDSystemInfo) String() string {
	return fmt.Sprintf("%s-%s-%d", info.Registry, info.Ordering, info.Supplement)
}

// IndexString returns a string that can be used to lookup `info` in cid2Rune
func (info *CIDSystemInfo) IndexString() string {
	return fmt.Sprintf("%s-%s", info.Registry, info.Ordering)
}

// NewCIDSystemInfo returns the CIDSystemInfo encoded in PDFObject `obj`
func NewCIDSystemInfo(obj PdfObject) (info CIDSystemInfo, err error) {
	obj = TraceToDirectObject(obj)
	d := *obj.(*PdfObjectDictionary)
	registry, err := GetString(d.Get("Registry"))
	if err != nil {
		return
	}
	ordering, err := GetString(d.Get("Ordering"))
	if err != nil {
		return
	}
	supplement, err := GetInteger(d.Get("Supplement"))
	if err != nil {
		return
	}
	info = CIDSystemInfo{
		Registry:   registry,
		Ordering:   ordering,
		Supplement: supplement,
	}
	return
}

// Name returns the name of the CMap.
func (cmap *CMap) Name() string {
	return cmap.name
}

// Type returns the type of the CMap.
func (cmap *CMap) Type() int {
	return cmap.ctype
}

// Usecmap returns the usecmap of `cmap`
func (cmap *CMap) Usecmap() string {
	return cmap.usecmap
}

// SystemInfo returns the CIDSystemInfo of the CMap.
func (cmap *CMap) SystemInfo() CIDSystemInfo {
	return cmap.systemInfo
}

// SystemInfo returns the cid ranges of `cmap`.
func (cmap *CMap) CidRanges() []CIDRange {
	return cmap.cidRanges
}

// Codespaces returns the codespaces of `cmap`.
func (cmap *CMap) Codespaces() []Codespace {
	return cmap.codespaces
}

// Codespaces returns the codespaces of `cmap`.
func (cmap *CMap) CodeToUnicode() map[CharCode]string {
	return cmap.codeToUnicode
}

// const mismatch = "[!@#$ mismatch]"

// CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
// NOTE: This only works for ToUnicode cmaps
// 9.10.3 ToUnicode CMaps (page 293)
// The CMap defined in the ToUnicode entry of the font dictionary shall follow the syntax for CMaps
// • The only pertinent entry in the CMap stream dictionary is UseCMap,
// • The CMap file shall contain begincodespacerange and endcodespacerange operators that are
//   consistent with the encoding that the font uses. In particular, for a simple font, the
//   codespace shall be one byte long.
// • It shall use the beginbfchar, endbfchar, beginbfrange, and endbfrange operators to define the
//    mapping from character codes to Unicode character sequences expressed in UTF-16BE encoding
func (cmap *CMap) CharcodeBytesToUnicode(data []byte) string {
	charcodes, matched := cmap.bytesToCharcodes(data)
	if !matched {
		panic("No match")
	}
	// common.Log.Debug("~~~~~~~~")
	// common.Log.Debug("charcodes=[% 02x]", charcodes)
	parts := []string{}
	for _, code := range charcodes {
		s, ok := cmap.codeToUnicode[code]
		if !ok {
			for _, cs := range cmap.codespaces {
				common.Log.Error("   %x", cs)
			}
			common.Log.Error("data=[% 02x]", data)
			common.Log.Error("charcodes=[% 02x]", charcodes)
			common.Log.Error("charcodeBytesToUnicodeUcs: no match for code=0x%04x", code)

			s = "?"
		}
		// common.Log.Debug("|--%2d: 0x%04x -> %+q=%#q", i, code, s, s)
		parts = append(parts, s)
	}
	return strings.Join(parts, "")
}

// matchCode attempts to match the entirr byte array `data` a sequence of character code in `cmap`'s
// codespaces
// Returns:
//      character code sequence (if there is a match complete match)
//      matched?
func (cmap *CMap) bytesToCharcodes(data []byte) ([]CharCode, bool) {
	charcodes := []CharCode{}
	if cmap.nbits == 8 {
		for _, b := range data {
			charcodes = append(charcodes, CharCode(b))
		}
		return charcodes, true
	}
	// common.Log.Debug("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	// common.Log.Debug("data=% 02x", data)

	for i := 0; i < len(data); {
		// common.Log.Debug("===%2d: 0x%02x", i, data[i])
		code, n, matched := cmap.matchCode(data[i:])
		if !matched {
			common.Log.Debug("ERROR: No code match at i=%d bytes=%02x=%#q", i, data, string(data))
			return charcodes, false
		}
		charcodes = append(charcodes, code)
		i += n
	}
	return charcodes, true
}

// matchCode attempts to match the byte array `data` with a character code in `cmap`'s codespaces
// Returns:
//      character code (if there is a match)
//      number of bytes read (if there is a match)
//      matched?
func (cmap *CMap) matchCode(data []byte) (code CharCode, n int, matched bool) {
	for j := 0; j < maxCodeLen; j++ {
		if j < len(data) {
			code = code<<8 | CharCode(data[j])
			n++
		}
		matched = cmap.inCodespace(code, j+1)
		// common.Log.Debug("|==%2d: 0x%04x %t", j, code, matched)
		if matched {
			return
		}
	}
	// No codespace matched data. Serious problem
	common.Log.Debug("ERROR: No codespace matches bytes=% 02x=%#q", data, string(data))
	common.Log.Error("ERROR: cmap=%s", cmap.String())
	for _, cs := range cmap.codespaces {
		common.Log.Debug("   %x", cs)
	}
	panic("1x11")
	n = 0
	return
}

// inCodespace returns true if `code` in `numBytes` byte codespace
func (cmap *CMap) inCodespace(code CharCode, numBytes int) bool {
	for _, cs := range cmap.codespaces {
		if cs.Low <= code && code <= cs.High && numBytes == cs.NumBytes {
			return true
		}
	}
	return false
}

var (
	maxSimpleCodeLen = 0
	maxCIDCodeLen    = 0
)

// func (cmap *CMap) charcodeBytesToUnicodeUcs8(charcodes []byte) string {
// 	fmt.Println("--------------------------")
// 	parts := []string{}
// 	j := 0
// 	for i := 0; i < len(charcodes); i += j {
// 		// code is used to test the 4 candidate charcodes starting at charcodes[i]
// 		code := CharCode(0)
// 		matched := false
// 		for j = 0; j < maxCodeLen && i+j < len(charcodes); {
// 			code <<= 8 // multibyte charcodes are bigendian in codeMap
// 			b := charcodes[i+j]
// 			code |= CharCode(b)
// 			s, ok := cmap.codeToUnicode[code]
// 			common.Log.Debug("|-- %d+%d=%d '%c'=0x%02x -> 0x%04x %t [% 02x]",
// 				i, j, i+j, b, b, code, ok, charcodes)
// 			j++
// 			if ok {
// 				matched = true
// 				parts = append(parts, s)
// 				if j > maxSimpleCodeLen {
// 					maxSimpleCodeLen = j
// 					common.Log.Debug("maxSimpleCodeLen=%d", maxSimpleCodeLen)
// 					fmt.Fprintf(os.Stderr, "maxSimpleCodeLen=%d\n", maxSimpleCodeLen)
// 				}
// 				break
// 			}
// 		}
// 		if !matched {
// 			common.Log.Debug("NO MATCH simple i=%d j=%d charcodes=%d [% 02x] %#q", i, j, len(charcodes), charcodes,
// 				string(charcodes))
// 			common.Log.Debug("%s", cmap.String())
// 			// common.Log.Debug("%d cidRanges", len(cmap.cidRanges))
// 			// common.Log.Debug("%d codespaces", len(cmap.codespaces))
// 			// for k, c := range cmap.codespaces {
// 			//  fmt.Printf("codespace %d: %#v\n", k, c)
// 			// }
// 			// cmap.printCodeToUnicode()
// 			// panic("1111")
// 			parts = append(parts, "?")
// 		}
// 	}
// 	return strings.Join(parts, "")
// }

// func (cmap *CMap) charcodeBytesToUnicodeUcs16(charcodes []uint16) string {
// 	fmt.Println("--------------------------(")
// 	parts := []string{}
// 	j := 0
// 	for i := 0; i < len(charcodes); i += j {
// 		// code is used to test the 4 candidate charcodes starting at charcodes[i]
// 		code := CharCode(0)
// 		matched := false
// 		for j = 0; j < maxCodeLen && i+j < len(charcodes); {
// 			code <<= 16 // multibyte charcodes are bigendian in codeMap
// 			b := charcodes[i+j]
// 			code |= CharCode(b)
// 			s, ok := cmap.codeToUnicode[code]
// 			common.Log.Debug("|-- %d+%d=%d '%c'=0x%04x -> 0x%04x %t [% 04x]",
// 				i, j, i+j, b, b, code, ok, charcodes)
// 			j++
// 			if ok {
// 				matched = true
// 				parts = append(parts, s)
// 				if j > maxCIDCodeLen {
// 					maxCIDCodeLen = j
// 					common.Log.Debug("maxCIDCodeLen=%d", maxCIDCodeLen)
// 					fmt.Fprintf(os.Stderr, "maxCIDCodeLen=%d\n", maxCIDCodeLen)
// 				}
// 				break
// 			}
// 		}
// 		if !matched {
// 			common.Log.Debug("NO MATCH CID i=%d j=%d charcodes=%d [% 02x]", i, j,
// 				len(charcodes), charcodes)
// 			common.Log.Debug("%s", cmap.String())
// 			// panic("1221")
// 			// common.Log.Debug("i=%d j=%d charcodes=%d %+v %#q", i, j, len(charcodes), charcodes, string(charcodes))
// 			// common.Log.Debug("%s", cmap.String())
// 			// common.Log.Debug("%d cidRanges", len(cmap.cidRanges))
// 			// common.Log.Debug("%d codespaces", len(cmap.codespaces))
// 			// for k, c := range cmap.codespaces {
// 			//  fmt.Printf("codespace %d: %#v\n", k, c)
// 			// }
// 			// cmap.printCodeToUnicode()
// 			parts = append(parts, "?")
// 		}
// 	}
// 	return strings.Join(parts, "")
// }

// CharcodeToUnicode converts a single character code to unicode string.
// Note that CharcodeBytesToUnicode is typically more efficient.
func (cmap *CMap) CharcodeToUnicode(code CharCode) string {
	if s, ok := cmap.codeToUnicode[code]; ok {
		return s
	}
	fmt.Printf("$$$ %d\n", len(cmap.codeToUnicode))
	fmt.Printf("$$$ %s\n", cmap)
	// Not found.
	return "?"
}

// ReadCodes converts the bytes in `charcodes` to CID codes
func (cmap *CMap) ReadCodes(charcodes []byte) (codes []CharCode) {
	cids, matched := cmap.bytesToCharcodes(charcodes)
	if !matched {
		panic("No match")
	}
	return cids
	// j := 0
	// for i := 0; i < len(charcodes); i += j + 1 {
	// 	// code is used to test the 4 candidate charcodes starting at charcodes[i]
	// 	code := CharCode(0)
	// 	matched := false
	// 	for j = 0; j < maxCodeLen && i+j < len(charcodes); j++ {
	// 		code <<= 8 // multibyte charcodes are bigendian in codeMap
	// 		code |= CharCode(charcodes[i+j])
	// 		matched = cmap.matchCodespace(code, j+1)
	// 		fmt.Printf("-- %3d+%3d=%3d %c=0x%02x -> 0x%04x %t \n",
	// 			i, j, i+j, charcodes[i+j], charcodes[i+j], code, matched)
	// 		if matched {
	// 			codes = append(codes, code)
	// 			break
	// 		}
	// 	}
	// 	if !matched {
	// 		fmt.Printf("i=%d j=%d charcodes=%d %+v %#q \n", i, j, len(charcodes), charcodes, string(charcodes))
	// 		fmt.Printf("%#q\n", cmap.name)
	// 		fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
	// 		fmt.Printf("%d codespaces\n", len(cmap.codespaces))
	// 		for k, c := range cmap.codespaces {
	// 			fmt.Printf("codespace %d: %#v\n", k, c)
	// 		}
	// 		panic("q9889999 ReadCodes")
	// 	}
	// }
	// return
}

// func (cmap *CMap) matchCodespace(code CharCode, numBytes int) bool {
// 	for _, cs := range cmap.codespaces {
// 		if cs.NumBytes == numBytes {
// 			fmt.Printf("+ %+v\n", cs)
// 			if cs.Low <= code && code <= cs.High {
// 				return true
// 			}
// 		}
// 	}
// 	return false
// }

// LoadCmapFromFile parses in-memory cmap file `filename` and returns the resulting CMap
// This is currenty only used by make_table.go for building the predefined cmap tables.
// XXX:TODO: Another way of doing this is to ship the Adobe cmap files and load them at runtime
func LoadCmapFromFile(filename string, nbits int) (*CMap, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return loadCmapFromData(data, nbits)
}

func LoadCmapFromDataSimple(data []byte) (*CMap, error) {
	return loadCmapFromData(data, 8)
}

func LoadCmapFromDataCID(data []byte) (*CMap, error) {
	return loadCmapFromData(data, 16)
}

// LoadCmapFromData parses in-memory cmap `data` and returns the resulting CMap
func loadCmapFromData(data []byte, nbits int) (*CMap, error) {
	cmap := newCMap(nbits)
	cmap.cMapParser = newCMapParser(data)
	common.Log.Trace("LoadCmapFromData: nbits=%d", nbits)
	// fmt.Println("===============*******===========")
	// fmt.Printf("%s\n", string(data))
	// fmt.Println("===============&&&&&&&===========")

	err := cmap.parse()
	if err != nil {
		return cmap, err
	}
	sort.Slice(cmap.codespaces, func(i, j int) bool {
		return cmap.codespaces[i].Low < cmap.codespaces[j].Low
	})

	if !cmap.codespacePrefixFree() {
		return nil, errors.New("Not prefix-free.")
	}
	// logCMap(cmap, data, nbits)
	return cmap, nil
}
