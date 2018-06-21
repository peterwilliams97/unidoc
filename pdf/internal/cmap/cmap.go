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
	"os"
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

	// Text encoder to look up runes from input glyph names.
	encoder textencoding.TextEncoder

	systemInfo CIDSystemInfo

	// map of character code to string (sequence of runes) for 1-4 byte codes separately.
	codeMap [4]map[CharCode]string

	name       string
	ctype      int
	usecmap    string // Base this cmap on usecmap if usecmap is not empty
	codespaces []Codespace
	cidRanges  []CIDRange
}

func GetPredefinedCmap(name string) (*CMap, error) {
	cmap, ok := getPredefinedCmap(name)
	if !ok {
		return nil, errors.New("Resource does not exist")
	}
	return applyAncestors(cmap)
}

func getPredefinedCmap(name string) (*CMap, bool) {
	cmap, ok := cmapTable[name]
	if !ok {
		common.Log.Debug("GetPredefinedCmap %#q doesn't exist", name)
	}
	return &cmap, ok
}

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

func updateParent(parent, child *CMap) (*CMap, error) {
	base := *parent
	base.name = child.name
	base.ctype = child.ctype
	base.usecmap = child.usecmap
	base.systemInfo = child.systemInfo
	if child.HasCodemap() {
		base.codeMap = child.codeMap
	}
	if len(child.codespaces) > 0 {
		base.codespaces = child.codespaces
	}
	if len(child.cidRanges) > 0 {
		base.cidRanges = child.cidRanges
	}
	return &base, nil
}

func GetPredefinedCidToRune(cidInfo CIDSystemInfo) (map[CID]rune, bool) {
	name := cidInfo.IndexString()
	if name == "Adobe-Identity" {
		return map[CID]rune{}, true
	}
	c2r, ok := cid2Rune[name]
	return c2r, ok
}

func (cmap *CMap) HasCodemap() bool {
	for _, cm := range cmap.codeMap {
		if len(cm) > 0 {
			return true
		}
	}
	return false
}

func (cmap *CMap) String() string {
	si := cmap.systemInfo
	mapN := [4]int{}
	for i, m := range cmap.codeMap {
		mapN[i] = len(m)
	}
	return fmt.Sprintf("CMAP{%#q:%d %s %d codespaces, %d CID ranges, code maps +%v",
		cmap.name, cmap.ctype,
		si.String(), len(cmap.codespaces), len(cmap.cidRanges), mapN)
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

func (cidInfo *CIDSystemInfo) String() string {
	return fmt.Sprintf("%s-%s:%d", cidInfo.Registry, cidInfo.Ordering, cidInfo.Supplement)
}

func (cidInfo *CIDSystemInfo) IndexString() string {
	return fmt.Sprintf("%s-%s", cidInfo.Registry, cidInfo.Ordering)
}

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

func (cmap *CMap) CidRanges() []CIDRange {
	return cmap.cidRanges
}

func (cmap *CMap) Codespaces() []Codespace {
	return cmap.codespaces
}

func (cmap *CMap) Usecmap() string {
	return cmap.usecmap
}

// Maximum number of possible bytes per code.
const maxCodeLen = 4

// CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
func (cmap *CMap) CharcodeBytesToUnicode(src []byte) string {
	var buf bytes.Buffer
	j := 0
	for i := 0; i < len(src); i += j + 1 {
		// code is used to test the 4 candidate charcodes starting at src[i]
		code := CharCode(0)
		matched := false
		for j = 0; j < maxCodeLen && i+j < len(src); j++ {
			code <<= 8 // multibyte charcodes are bigendian in codeMap
			code |= CharCode(src[i+j])
			if c, has := cmap.codeMap[j][code]; has {
				buf.WriteString(c)
				matched = true
				break
			}
		}
		if !matched {
			fmt.Fprintf(os.Stderr, "i=%d j=%d src=%d %+v %#q \n", i, j, len(src), src, string(src))
			fmt.Fprintf(os.Stderr, "%s\n", cmap.String())
			fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
			fmt.Printf("%d codespaces\n", len(cmap.codespaces))
			for k, c := range cmap.codespaces {
				fmt.Printf("codespace %d: %#v\n", k, c)
			}
			panic("33333")
		}
	}
	// if cmap.name != `Adobe-Identity-UCS`
	if !strings.HasSuffix(cmap.name, "-UCS") {
		codes := cmap.ReadCodes(src)
		var buf2 bytes.Buffer
		for _, code := range codes {
			j := codeSize(code) - 1
			c, has := cmap.codeMap[j][code]
			if !has {
				fmt.Fprintf(os.Stderr, "0x%04x %d %s\nsrc=%d %#q\ncodes=%d 0x%04x\n",
					code, j, cmap.String(), len(src), string(src), len(codes), codes)
				fmt.Printf("%d cidRanges\n", len(cmap.cidRanges))
				fmt.Printf("%d codespaces\n", len(cmap.codespaces))
				for k, c := range cmap.codespaces {
					fmt.Printf("codespace %d: %#v\n", k, c)
				}
				panic("GGGG")
			}
			buf2.WriteString(c)
		}
		if buf2.String() != buf.String() {
			panic("HHHHH")
		}
	}
	return buf.String()
}

// CharcodeToUnicode converts a single character code to unicode string.
// Note that CharcodeBytesToUnicode is typically more efficient.
func (cmap *CMap) CharcodeToUnicode(code CharCode) string {
	// Search through different code lengths.
	for j := 0; j < maxCodeLen; j++ {
		if c, has := cmap.codeMap[j][code]; has {
			return c
		}
	}

	// Not found.
	return "?"
}

func codeSize(code CharCode) int {
	for i := 3; i >= 0; i-- {
		if code>>CharCode(i*8) != 0 {
			return i + 1
		}
	}
	return 1
}

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

// newCMap returns an initialized CMap.
func newCMap() *CMap {
	cmap := &CMap{
		codespaces: []Codespace{},
		codeMap: [4]map[CharCode]string{
			// Maps for 1-4 bytes are initialized. Minimal overhead if not used (most commonly used
			// are 1-2 bytes).
			map[CharCode]string{}, // 1 byte code map
			map[CharCode]string{}, // 2 byte code map
			map[CharCode]string{}, // 3 byte code map
			map[CharCode]string{}, // 4 byte code map
		},
	}
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
	fmt.Println("===============*******===========")
	fmt.Printf("%s\n", string(data))
	fmt.Println("===============&&&&&&&===========")

	err := cmap.parse()
	return cmap, err
}

// parse parses the CMap file and loads into the CMap structure.
func (cmap *CMap) parse() error {
	var prev cmapObject
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
		default:
			common.Log.Trace("Unhandled object: %T %#v", o, o)
			// fmt.Printf("Unhandled object: %T %#v\n", o, o)
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

// /CMapName /83pv-RKSJ-H def
func (cmap *CMap) parseName() error {

	name := ""
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
				return errors.New("Bad state")
			}
		case cmapName:
			name = t.Name
		}
	}
	cmap.name = name
	// panic("fff")
	return nil
}

// /CMapType 1 def
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
				return errors.New("Bad state")
			}
		case cmapInt:
			ctype = int(t.val)
		}
	}
	cmap.ctype = ctype
	// panic("fff")
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

		low := hexToCharCode(hexLow)
		high := hexToCharCode(hexHigh)
		numBytes := hexLow.numBytes

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
		var srcCode CharCode
		var numBytes int

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			srcCode = hexToCharCode(v)
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
		var srcCodeFrom CharCode
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
				srcCodeFrom = hexToCharCode(v)
				numBytes = v.numBytes
			default:
				return errors.New("Unexpected type")
			}
		}

		// Src code to.
		var srcCodeTo CharCode
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
				srcCodeTo = hexToCharCode(v)
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
			target := hexToCharCode(v)
			i := CharCode(0)
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
