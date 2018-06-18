/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

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

	// Text encoder to look up runes from input glyph names.
	encoder textencoding.TextEncoder

	systemInfo CIDSystemInfo

	// map of character code to string (sequence of runes) for 1-4 byte codes separately.
	codeMap [4]map[uint64]string

	name       string
	ctype      int
	codespaces []codespace
	cidRanges  []CIDRange
}

// toCID returns the CID for character code `code`
// returns 0 if no match
func (cmap *CMap) toCID(code int) int {
	// !@#$ Binary search ?
	for _, r := range cmap.cidRanges {
		if r.from <= code && code <= r.to {
			return r.cid - r.from + code
		}
	}
	return 0
}

// func (cmap *CMap) Print() {
// 	fmt.Printf("\tname: %#q,\n", cmap.name)
// 	fmt.Printf("\tctype: %d,\n", cmap.ctype)
// 	fmt.Printf("\tsystemInfo: %#v,\n", cmap.systemInfo)
// 	fmt.Printf("\tcodespaces []codespace{ //%d codespaces\n", len(cmap.codespaces))
// 	for i, cs := range cmap.codespaces {
// 		fmt.Printf("\t%#v, // %d of %d\n", cs, i+1, len(cmap.codespaces))
// 	}
// 	fmt.Println("},")
// 	// codeMap [4]map[uint64]string
// 	fmt.Println("\tcodeMap [4]map[uint64]string{")
// 	for i := 0; i < 4; i++ {
// 		codeMap := cmap.codeMap[i]
// 		fmt.Printf("\tmap[uint64]string{ //%d entries\n", len(codeMap))
// 		for j, cs := range codeMap {
// 			fmt.Printf("\t%#v, // %d of %d\n", cs, j+1, len(codeMap))
// 		}
// 		fmt.Println("},")
// 	}
// 	fmt.Println("},")
// }

// codespace represents a single codespace range used in the CMap.
type codespace struct {
	numBytes int
	low      uint64
	high     uint64
}

// CIDSystemInfo=Dict("Registry": Adobe, "Ordering": Korea1, "Supplement": 0, )
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int
}

type CIDRange struct {
	from int
	to   int
	cid  int
}

// Name returns the name of the CMap.
func (cmap *CMap) Name() string {
	return cmap.name
}

// Type returns the type of the CMap.
func (cmap *CMap) Type() int {
	return cmap.ctype
}

// Maximum number of possible bytes per code.
const maxLen = 4

// CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
func (cmap *CMap) CharcodeBytesToUnicode(src []byte) string {
	var buf bytes.Buffer
	j := 0
	for i := 0; i < len(src); i += j + 1 {
		// code is used to test the 4 candidate charcodes starting at src[i]
		code := uint64(0)
		for j = 0; j < maxLen && i+j < len(src); j++ {
			code <<= 8 // multibyte charcodes are bigendian in codeMap
			code |= uint64(src[i+j])
			if c, has := cmap.codeMap[j][code]; has {
				buf.WriteString(c)
				break
			}
		}
	}
	return buf.String()
}

// CharcodeToUnicode converts a single character code to unicode string.
// Note that CharcodeBytesToUnicode is typically more efficient.
func (cmap *CMap) CharcodeToUnicode(code uint64) string {
	// Search through different code lengths.
	for j := 0; j < maxLen; j++ {
		if c, has := cmap.codeMap[j][code]; has {
			return c
		}
	}

	// Not found.
	return "?"
}

// newCMap returns an initialized CMap.
func newCMap() *CMap {
	cmap := &CMap{}
	cmap.codespaces = []codespace{}
	cmap.codeMap = [4]map[uint64]string{}
	// Maps for 1-4 bytes are initialized. Minimal overhead if not used (most commonly used are 1-2 bytes).
	cmap.codeMap[0] = map[uint64]string{} // 1 byte code map
	cmap.codeMap[1] = map[uint64]string{} // 2 byte code map
	cmap.codeMap[2] = map[uint64]string{} // 3 byte code map
	cmap.codeMap[3] = map[uint64]string{} // 4 byte code map
	return cmap
}

func LoadCmapFromFile(filename string) (*CMap, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadCmapFromData(data)
}

// LoadCmapFromData parses CMap data in memory through a byte vector and returns a CMap which
// can be used for character code to unicode conversion.
func LoadCmapFromData(data []byte) (*CMap, error) {
	cmap := newCMap()
	cmap.cMapParser = newCMapParser(data)

	err := cmap.parse()
	return cmap, err
}

// parse parses the CMap file and loads into the CMap structure.
func (cmap *CMap) parse() error {
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}

			common.Log.Debug("Error parsing CMap: %v", err)
			return err
		}

		switch t := o.(type) {
		case cmapOperand:
			op := t

			switch op.Operand {
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
				o, err := cmap.parseObject()
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				name, ok := o.(cmapName)
				if !ok {
					return errors.New("CMap name not a name")
				}
				cmap.name = name.Name
			case cmaptype:
				o, err := cmap.parseObject()
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				typeInt, ok := o.(cmapInt)
				if !ok {
					return errors.New("CMap type not an integer")
				}
				cmap.ctype = int(typeInt.val)
			}
		default:
			common.Log.Trace("Unhandled object: %T %#v", o, o)
			fmt.Printf("Unhandled object: %T %#v\n", o, o)

		}
	}

	return nil
}

// /CIDSystemInfo 3 dict dup begin
//   /Registry (Adobe) def
//   /Ordering (Japan1) def
//   /Supplement 1 def
// end def

func (cmap *CMap) parseSystemInfo() error {
	inDict := false
	inDef := false
	name := ""
	systemInfo := CIDSystemInfo{}
	for i := 0; i < 20; i++ {
		o, err := cmap.parseObject()
		fmt.Printf("%2d: %#v\n", i, o)
		if err != nil {
			panic(err)
		}
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "begin":
				inDict = true
			case "end":
				break
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
					// "Supplement"
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
	fmt.Printf("%#v\n", systemInfo)
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

		hexLow, isHex := o.(cmapHexString)
		if !isHex {
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

		if hexLow.numBytes != hexHigh.numBytes {
			return errors.New("Unequal number of bytes in range")
		}

		low := hexToUint64(hexLow)
		high := hexToUint64(hexHigh)
		numBytes := hexLow.numBytes

		cspace := codespace{numBytes: numBytes, low: low, high: high}
		cmap.codespaces = append(cmap.codespaces, cspace)

		common.Log.Trace("Codespace low: 0x%X, high: 0x%X", low, high)
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
		var srcCode uint64
		var numBytes int

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			srcCode = hexToUint64(v)
			numBytes = v.numBytes
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
		var toCode string

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			toCode = hexToString(v)
		case cmapName:
			toCode = "?"
			if cmap.encoder != nil {
				if r, found := cmap.encoder.GlyphToRune(v.Name); found {
					toCode = string(r)
				}
			}
		default:
			return errors.New("Unexpected type")
		}

		if numBytes <= 0 || numBytes > 4 {
			return errors.New("Invalid code length")
		}

		cmap.codeMap[numBytes-1][srcCode] = toCode
	}

	return nil
}

// parseBfrange parses a bfrange section of a CMap file.
func (cmap *CMap) parseBfrange() error {
	for {
		// The specifications are in pairs of 3.
		// <srcCodeFrom> <srcCodeTo> <target>
		// where target can be either <destFrom> as a hex code, or a list.

		// Src code from.
		var srcCodeFrom uint64
		var numBytes int
		{
			o, err := cmap.parseObject()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			switch v := o.(type) {
			case cmapOperand:
				if v.Operand == endbfrange {
					return nil
				}
				return errors.New("Unexpected operand")
			case cmapHexString:
				srcCodeFrom = hexToUint64(v)
				numBytes = v.numBytes
			default:
				return errors.New("Unexpected type")
			}
		}

		// Src code to.
		var srcCodeTo uint64
		{
			o, err := cmap.parseObject()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			switch v := o.(type) {
			case cmapOperand:
				if v.Operand == endbfrange {
					return nil
				}
				return errors.New("Unexpected operand")
			case cmapHexString:
				srcCodeTo = hexToUint64(v)
			default:
				return errors.New("Unexpected type")
			}
		}

		// target(s).
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if numBytes <= 0 || numBytes > 4 {
			return errors.New("Invalid code length")
		}

		switch v := o.(type) {
		case cmapArray:
			sc := srcCodeFrom
			for _, o := range v.Array {
				hexs, ok := o.(cmapHexString)
				if !ok {
					return errors.New("Non-hex string in array")
				}
				cmap.codeMap[numBytes-1][sc] = hexToString(hexs)
				sc++
			}
			if sc != srcCodeTo+1 {
				return errors.New("Invalid number of items in array")
			}
		case cmapHexString:
			// <srcCodeFrom> <srcCodeTo> <dstCode>, maps [from,to] to [dstCode,dstCode+to-from].
			// in hex format.
			target := hexToUint64(v)
			i := uint64(0)
			for sc := srcCodeFrom; sc <= srcCodeTo; sc++ {
				r := target + i
				cmap.codeMap[numBytes-1][sc] = string(r)
				i++
			}
		default:
			return errors.New("Unexpected type")
		}
	}

	return nil
}

// parseCidrange parses a bfrange section of a CMap file.
func (cmap *CMap) parseCidrange() error {
	to := 0
	from := 0
	cid := 0
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
				from = int(hexToUint64(v))
				state = 1
			case 1:
				to = int(hexToUint64(v))
				state = 3
			default:
				fmt.Printf("state=%d\n", state)
				panic("bad state 1")
			}
		case cmapInt:
			if state != 3 {
				panic("bad state2")
			}
			cid = int(v.val)
			state = 0
			cmap.cidRanges = append(cmap.cidRanges, CIDRange{from: from, to: to, cid: cid})
		default:
			return errors.New("Unexpected type")
		}
	}
	return nil
}
