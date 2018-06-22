/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

// CharCode is a character code or Unicode
//  Why 64 bit? rune is int32 https://golang.org/doc/go1#rune
type CharCode uint64

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
	ctype      int
	usecmap    string // Base this cmap on usecmap if usecmap is not empty
	systemInfo CIDSystemInfo

	// Text encoder to look up runes from input glyph names.
	encoder textencoding.TextEncoder

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
		fmt.Sprintf("type:%d", cmap.ctype),
		fmt.Sprintf("systemInfo:%s", si.String()),
		fmt.Sprintf("codeToUnicode:%d", len(cmap.codeToUnicode)),
		fmt.Sprintf("codespaces:%d", len(cmap.codespaces)),
		fmt.Sprintf("cidRanges:%d", len(cmap.cidRanges)),
		fmt.Sprintf("codeToCID:%d", len(cmap.codeToCID)),
	}
	return fmt.Sprintf("CMAP{%#q %s}", cmap.name, strings.Join(parts, " "))
}

// newCMap returns an initialized CMap.
func newCMap() *CMap {
	cmap := &CMap{
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
		fmt.Printf("0x%04x %#q\n", c, cmap.codeToUnicode[c])
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
	// !@#$ Binary search ?
	for _, r := range cmap.cidRanges {
		if r.From <= code && code <= r.To {
			return r.Cid - CID(r.From) + CID(code)
		}
	}
	panic("GGG")
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

// Usecmap returns the usecmap of `cmap`
func (cmap *CMap) Usecmap() string {
	return cmap.usecmap
}

// CharcodeBytesToUnicode converts `src` to unicode
// If `isToUnicode`, it used the ToUnicode method
func (cmap *CMap) CharcodeBytesToUnicode(src []byte, isToUnicode bool) string {
	if isToUnicode {
		u := cmap.charcodeBytesToUnicodeUcs(src)
		// if cmap.ctype != 2 {
		// 	fmt.Fprintf(os.Stderr, "cmap%s\n", cmap)
		// 	panic("Bad ctype")
		// }
		// if cmap.name != `Adobe-Identity-UCS` {
		// 	fmt.Fprintf(os.Stderr, "cmap%s\n", cmap)
		// 	panic("Bad name")
		// }
		return u
	}
	// if cmap.ctype != 1 || cmap.name == `Adobe-Identity-UCS` {
	// 	fmt.Fprintf(os.Stderr, "cmap%s\n", cmap)
	// 	panic("Bad ctype")
	// }
	return ""
	// return charcodeBytesToUnicodeCID(src)
}

// Maximum number of possible bytes per code.
const maxCodeLen = 4
const mismatch = "[!@#$ mismatch]"

// CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
// 9.10.3 ToUnicode CMaps (page 293)
// The CMap defined in the ToUnicode entry of the font dictionary shall follow the syntax for CMaps
// • The only pertinent entry in the CMap stream dictionary is UseCMap,
// • The CMap file shall contain begincodespacerange and endcodespacerange operators that are
//   consistent with the encoding that the font uses. In particular, for a simple font, the
//   codespace shall be one byte long.
// • It shall use the beginbfchar, endbfchar, beginbfrange, and endbfrange operators to define the
//    mapping from character codes to Unicode character sequences expressed in UTF-16BE encoding
func (cmap *CMap) charcodeBytesToUnicodeUcs(src []byte) string {
	// fmt.Println("--------------------------")
	parts := []string{}
	j := 0
	for i := 0; i < len(src); i += j {
		// code is used to test the 4 candidate charcodes starting at src[i]
		code := CharCode(0)
		matched := false
		for j = 0; j < maxCodeLen && i+j < len(src); {
			code <<= 8 // multibyte charcodes are bigendian in codeMap
			b := src[i+j]
			code |= CharCode(b)
			s, ok := cmap.codeToUnicode[code]
			// common.Log.Debug("|-- %d+%d=%d '%c'=0x%02x -> 0x%04x %t [% 02x] %q",
			// 	i, j, i+j, b, b, code, ok, src, src0)
			j++
			if ok {
				matched = true
				parts = append(parts, s)
				break
			}
		}
		if !matched {
			common.Log.Debug("i=%d j=%d src=%d %+v %#q", i, j, len(src), src, string(src))
			common.Log.Debug("%s", cmap.String())
			common.Log.Debug("%d cidRanges", len(cmap.cidRanges))
			common.Log.Debug("%d codespaces", len(cmap.codespaces))
			for k, c := range cmap.codespaces {
				fmt.Printf("codespace %d: %#v\n", k, c)
			}
			cmap.printCodeToUnicode()
			parts = append(parts, mismatch)
			// panic(mismatch)
		}
	}
	return strings.Join(parts, "")
}

// // CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
// func (cmap *CMap) charcodeBytesToUnicodeCID(src []byte) string {
// 	var buf bytes.Buffer
// 	j := 0
// 	for i := 0; i < len(src); i += j + 1 {
// 		// code is used to test the 4 candidate charcodes starting at src[i]
// 		code := CharCode(0)
// 		matched := false
// 		for j = 0; j < maxCodeLen && i+j < len(src); j++ {
// 			code <<= 8 // multibyte charcodes are bigendian in codeMap
// 			code |= CharCode(src[i+j])
// 			if c, has := cmap.codeMap[j][code]; has {
// 				buf.WriteString(c)
// 				matched = true
// 				break
// 			}
// 		}
// 		if !matched {
// 			fmt.Fprintf(os.Stderr, "i=%d j=%d src=%d %+v %#q \n", i, j, len(src), src, string(src))
// 			fmt.Fprintf(os.Stderr, "%s\n", cmap.String())
// 			fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
// 			fmt.Printf("%d codespaces\n", len(cmap.codespaces))
// 			for k, c := range cmap.codespaces {
// 				fmt.Printf("codespace %d: %#v\n", k, c)
// 			}
// 			panic("33333")
// 		}
// 	}
// 	// // if cmap.name != `Adobe-Identity-UCS`
// 	// if !strings.HasSuffix(cmap.name, "-UCS") {
// 	codes := cmap.ReadCodes(src)
// 	var buf2 bytes.Buffer
// 	for _, code := range codes {
// 		j := codeSize(code) - 1
// 		c, has := cmap.codeMap[j][code]
// 		if !has {
// 			fmt.Fprintf(os.Stderr, "0x%04x %d %s\nsrc=%d %#q\ncodes=%d 0x%04x\n",
// 				code, j, cmap.String(), len(src), string(src), len(codes), codes)
// 			fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
// 			fmt.Printf("%d codespaces\n", len(cmap.codespaces))
// 			for k, c := range cmap.codespaces {
// 				fmt.Printf("codespace %d: %#v\n", k, c)
// 			}
// 			panic("GGGG")
// 		}
// 		buf2.WriteString(c)
// 	}
// 	if buf2.String() != buf.String() {
// 		panic("HHHHH")
// 	}
// 	// }
// 	return buf.String()
// }

// // CharcodeToUnicode converts a single character code to unicode string.
// // Note that CharcodeBytesToUnicode is typically more efficient.
// func (cmap *CMap) CharcodeToUnicode(code CharCode) string {
// 	// Search through different code lengths.
// 	code := CharCode(src[i]<<8 + src[i+1])
//         s := cmap.codeToUnicode[code]

// 	// Not found.
// 	return "?"
// }

// ReadCodes converts the bytes in `src` to CID codes
func (cmap *CMap) ReadCodes(src []byte) (codes []CharCode) {
	j := 0
	for i := 0; i < len(src); i += j + 1 {
		// code is used to test the 4 candidate charcodes starting at src[i]
		code := CharCode(0)
		matched := false
		for j = 0; j < maxCodeLen && i+j < len(src); j++ {
			code <<= 8 // multibyte charcodes are bigendian in codeMap
			code |= CharCode(src[i+j])
			matched = cmap.matchCodespace(code, j+1)
			fmt.Printf("-- %3d+%3d=%3d %c=0x%02x -> 0x%04x %t \n",
				i, j, i+j, src[i+j], src[i+j], code, matched)
			if matched {
				codes = append(codes, code)
				break
			}
		}
		if !matched {
			fmt.Printf("i=%d j=%d src=%d %+v %#q \n", i, j, len(src), src, string(src))
			fmt.Printf("%#q\n", cmap.name)
			fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
			fmt.Printf("%d codespaces\n", len(cmap.codespaces))
			for k, c := range cmap.codespaces {
				fmt.Printf("codespace %d: %#v\n", k, c)
			}
			panic("9999999")
		}
	}
	return
}

func (cmap *CMap) matchCodespace(code CharCode, numBytes int) bool {
	for _, cs := range cmap.codespaces {
		if cs.NumBytes == numBytes {
			fmt.Printf("+ %+v\n", cs)
			if cs.Low <= code && code <= cs.High {
				return true
			}
		}
	}
	return false
}

// /**
//      * Returns an int for the given byte array
//      */
// func toInt(data []byte, dataLen int) (code uint64){
//     for i := 0; i < dataLen; i++ {
//         code <<= 8;
//         code |= data[i]
//         code |= uint64(src[i+j])
//     }
//     return code
// }

// LoadCmapFromFile parses in-memory cmap file `filename` and returns the resulting CMap
// This is currenty only used by make_table.go for building the predefined cmap tables.
// XXX:TODO: Another way of doing this is to ship the Adobe cmap files and load them at runtime
func LoadCmapFromFile(filename string) (*CMap, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadCmapFromData(data)
}

// LoadCmapFromData parses in-memory cmap `data` and returns the resulting CMap
func LoadCmapFromData(data []byte) (*CMap, error) {
	cmap := newCMap()
	cmap.cMapParser = newCMapParser(data)
	fmt.Println("===============*******===========")
	fmt.Printf("%s\n", string(data))
	fmt.Println("===============&&&&&&&===========")

	err := cmap.parse()
	return cmap, err
}

// parse parses the CMap file and loads into the CMap structure.
func (cmap *CMap) parse() error {
	inCmap := false
	var prev cmapObject
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			common.Log.Debug("ERROR: parsing CMap: %v", err)
			return err
		}
		// fmt.Printf("-- %#v\n", o)

		switch t := o.(type) {
		case cmapOperand:
			op := t

			switch op.Operand {
			case begincmap:
				inCmap = true
			case endcmap:
				inCmap = false
			case begincodespacerange:
				err := cmap.parseCodespaceRange()
				if err != nil {
					return err
				}
			case beginbfchar:
				err := cmap.parseBfchar()
				if err != nil {
					return err
				}
			case beginbfrange:
				err := cmap.parseBfrange()
				if err != nil {
					return err
				}
			case begincidrange:
				err := cmap.parseCidrange()
				if err != nil {
					return err
				}
			case usecmap:
				if prev == nil {
					common.Log.Debug("ERROR: usecmap with no arg")
					return errors.New("Bad cmap")
				}
				name, ok := prev.(cmapName)
				if !ok {
					common.Log.Debug("ERROR: usecmap arg not a name %#v", prev)
					return errors.New("Bad cmap")
				}
				cmap.usecmap = name.Name
			case cisSystemInfo:
				// Some PDF generators leave the "/"" off CIDSystemInfo
				// e.g. ~/testdata/459474_809.pdf
				err := cmap.parseSystemInfo()
				if err != nil {
					return err
				}
			}
		case cmapName:
			n := t
			switch n.Name {
			case cisSystemInfo:
				err := cmap.parseSystemInfo()
				if err != nil {
					return err
				}
			case cmapname:
				err := cmap.parseName()
				if err != nil {
					return err
				}
			case cmaptype:
				err := cmap.parseType()
				if err != nil {
					return err
				}
			}
		case cmapInt:

		default:
			if inCmap {
				common.Log.Debug("Unhandled object: %#v", o)
				panic("whoaa")
			}
		}
		prev = o
	}

	// fmt.Printf("^^^^%#q %d codespaces, %d cidRanges\n",
	// 	cmap.name, len(cmap.codespaces), len(cmap.cidRanges))
	// if len(cmap.codespaces) == 0 {
	// 	panic("^^***^^")
	// }
	return nil
}

// parseName parses a cmap name and adds it to `cmap`.
// cmap names are defined like this: /CMapName /83pv-RKSJ-H def
func (cmap *CMap) parseName() error {
	name := ""
	done := false
	for i := 0; i < 10 && !done; i++ {
		o, err := cmap.parseObject()
		if err != nil {
			return err
		}
		// fmt.Printf("^^ %d %#v\n", i, o)
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "def":
				done = true
			default:
				// This is not an error because some PDF files don't have valid PostScript here
				// e.g. ~/testdata/Papercut vs Equitrac.pdf
				// /CMapName /Adobe-SI-*Courier New-6164-0 def
				common.Log.Debug("parseName: state error. o=%#v", o)
				if name != "" {
					name = fmt.Sprintf("%s %s", name, t.Operand)
				}
				// return errors.New("Bad state")
			}
		case cmapName:
			name = t.Name
		}
	}
	if !done {
		common.Log.Error("parseName: No def. ")
		return errors.New("Bad state")
	}
	cmap.name = name
	return nil
}

// parseType parses a cmap type and adds it to `cmap`.
// cmap names are defined like this: /CMapType 1 def
func (cmap *CMap) parseType() error {

	ctype := 0
	done := false
	for i := 0; i < 3 && !done; i++ {
		o, err := cmap.parseObject()
		if err != nil {
			return err
		}
		// fmt.Printf("^^ %d %#v\n", i, o)
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "def":
				done = true
			default:
				common.Log.Error("parseType: state error. o=%#v", o)
				return errors.New("Bad state")
			}
		case cmapInt:
			ctype = int(t.val)
		}
	}
	cmap.ctype = ctype
	return nil
}

// parseSystemInfo parses a cmap CIDSystemInfo and adds it to `cmap`.
// cmap CIDSystemInfo is define like this:
// /CIDSystemInfo 3 dict dup begin
//   /Registry (Adobe) def
//   /Ordering (Japan1) def
//   /Supplement 1 def
// end def
func (cmap *CMap) parseSystemInfo() error {
	inDict := false
	inDef := false
	name := ""
	done := false
	systemInfo := CIDSystemInfo{}

	for i := 0; i < 50 && !done; i++ {
		o, err := cmap.parseObject()
		// fmt.Printf("%2d: %#v\n", i, o)
		if err != nil {
			panic(err)
		}
		switch t := o.(type) {
		case cmapDict:
			d := t.Dict
			r, ok := d["Registry"]
			if !ok {
				panic("1")
			}
			rr, ok := r.(cmapString)
			if !ok {
				panic("1a")
			}
			systemInfo.Registry = rr.String

			r, ok = d["Ordering"]
			if !ok {
				panic("2")
			}
			rr, ok = r.(cmapString)
			if !ok {
				panic("2a")
			}

			systemInfo.Ordering = rr.String

			s, ok := d["Supplement"]
			if !ok {
				panic("3")
			}
			ss, ok := s.(cmapInt)
			if !ok {
				panic("3a")
			}
			systemInfo.Supplement = int(ss.val)

			done = true
		case cmapOperand:
			switch t.Operand {
			case "begin":
				inDict = true
			case "end":
				done = true
			case "def":
				inDef = false
			}
		case cmapName:
			if inDict {
				name = t.Name
				inDef = true
			}
		case cmapString:
			if inDef {
				switch name {
				case "Registry":
					systemInfo.Registry = t.String
				case "Ordering":
					systemInfo.Ordering = t.String
				}
			}
		case cmapInt:
			if inDef {
				switch name {
				case "Supplement":
					systemInfo.Supplement = int(t.val)
				}
			}
		}
	}
	if !done {
		panic("Parsed dict incorrectly")
	}
	// fmt.Printf("%#v\n", systemInfo)
	cmap.systemInfo = systemInfo
	return nil
}

// parseCodespaceRange parses the codespace range section of a CMap.
func (cmap *CMap) parseCodespaceRange() error {
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		hexLow, ok := o.(cmapHexString)
		if !ok {
			if op, isOperand := o.(cmapOperand); isOperand {
				if op.Operand == endcodespacerange {
					return nil
				}
				return errors.New("Unexpected operand")
			}
		}

		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		hexHigh, ok := o.(cmapHexString)
		if !ok {
			return errors.New("Non-hex high")
		}

		if len(hexLow.b) != len(hexHigh.b) {
			return errors.New("Unequal number of bytes in range")
		}

		low := hexToCharCode(hexLow)
		high := hexToCharCode(hexHigh)
		if high < low {
			panic("high < low")
		}
		numBytes := codeSize(high)
		cspace := Codespace{NumBytes: numBytes, Low: low, High: high}
		cmap.codespaces = append(cmap.codespaces, cspace)

		common.Log.Trace("Codespace low: 0x%X, high: 0x%X", low, high)
	}

	if len(cmap.codespaces) == 0 {
		panic("^^^^^^")
	}

	return nil
}

// parseBfchar parses a bfchar section of a CMap file.
func (cmap *CMap) parseBfchar() error {
	for {
		// Src code.
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// fmt.Printf("--- %#v\n", o)
		var code CharCode

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			code = hexToCharCode(v)
		default:
			return errors.New("Unexpected type")
		}

		// Target code.
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		target := ""

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			target = hexToString(v)
		case cmapName:
			target = "?"
			if cmap.encoder != nil {
				if r, found := cmap.encoder.GlyphToRune(v.Name); found {
					target = string(r)
				}
			}
		default:
			return errors.New("Unexpected type")
		}

		cmap.codeToUnicode[code] = target
		// fmt.Printf("    code=0x%x r='%c'=0x%x\n", code, r, r)
		// if code == 3 {
		// 	panic("$$$")
		// }
	}

	return nil
}

// parseBfrange parses a c section of a CMap file.
func (cmap *CMap) parseBfrange() error {
	// fmt.Println("parseBfrange--------------------------")
	for {
		// The specifications are in triplets.
		// <srcCodeFrom> <srcCodeTo> <target>
		// where target can be either <destFrom> as a hex code, or a list.

		// Src code from.
		var srcCodeFrom CharCode
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// fmt.Printf("-== %#v\n", o)
		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfrange {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			srcCodeFrom = hexToCharCode(v)
		default:
			return errors.New("Unexpected type")
		}

		// Src code to.
		var srcCodeTo CharCode
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch v := o.(type) {
		case cmapOperand:
			common.Log.Debug("ERROR: Imcomplete triplet")
			return errors.New("Unexpected operand")
		case cmapHexString:
			srcCodeTo = hexToCharCode(v)
		default:
			return errors.New("Unexpected type")
		}

		// target(s).
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch v := o.(type) {
		case cmapArray:
			if len(v.Array) != int(srcCodeTo-srcCodeFrom)+1 {
				return errors.New("Invalid number of items in array")
			}
			for code := srcCodeFrom; code <= srcCodeTo; code++ {
				o := v.Array[code-srcCodeFrom]
				hexs, ok := o.(cmapHexString)
				if !ok {
					return errors.New("Non-hex string in array")
				}
				s := hexToString(hexs)
				cmap.codeToUnicode[code] = s
			}

		case cmapHexString:
			// <codeFrom> <codeTo> <dst>, maps [from,to] to [dst,dst+to-from].
			target := hexToString(v)
			runes := []rune(target)
			for code := srcCodeFrom; code <= srcCodeTo; code++ {
				cmap.codeToUnicode[code] = string(runes)
				runes[len(runes)-1]++
			}
		default:
			return errors.New("Unexpected type")
		}
	}

	return nil
}

// // Typical ToUnicode cmap
// //   /CMapName /Adobe-Identity-UCS def
// //   /CMapType 2 def
// //   1 begincodespacerange
// //   <00><FF>
// //   endcodespacerange
// //   50 beginbfrange
// //   <21><21><0032>
// //   <22><22><0037>
// //  <23><23><002f>
// func (cmp CMap) addMapping(code CharCode, uStr string) error {

// 	if n <= 4 {
// 		if len(uStr) > 2*n {
// 			common.Log.Debug("Bad CMap")
// 			return errors.New("@@@")
// 		}
// 		u := strconv.AtoiHex(uStr)
// 		codeMap[code] = u + offset
// 	} else {
// 		entry := Entry{
// 			code: code,
// 			size: n / 4,
// 			uStr: uStr,
// 		}
// 	}
// }

// type Entry struct {
// 	code CharCode
// 	size int
// 	uStr string
// }

// parseCidrange parses a bfrange section of a CMap file.
func (cmap *CMap) parseCidrange() error {
	to := CharCode(0)
	from := CharCode(0)
	cid := CID(0)
	state := 0
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endcidrange {

				// fmt.Printf("cidRanges=%#v\n", cmap.cidRanges)
				// panic("GGG")
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			switch state {
			case 0:
				from = hexToCharCode(v)
				state = 1
			case 1:
				to = hexToCharCode(v)
				state = 3
			default:
				fmt.Printf("state=%d\n", state)
				panic("bad state 1")
			}
		case cmapInt:
			if state != 3 {
				panic("bad state2")
			}
			cid = CID(v.val)
			state = 0
			cmap.cidRanges = append(cmap.cidRanges, CIDRange{From: from, To: to, Cid: cid})
		default:
			return errors.New("Unexpected type")
		}
	}
	return nil
}
