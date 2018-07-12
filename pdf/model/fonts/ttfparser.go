/*
 * Copyright (c) 2013 Kurt Jung (Gmail: kurt.w.jung)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package fonts

// Utility to parse TTF font files
// Version:    1.0
// Date:       2011-06-18
// Author:     Olivier PLATHEY
// Port to Go: Kurt Jung, 2013-07-15

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

func (rec *TtfType) MakeEncoder() (textencoding.SimpleEncoder, error) {

	// // synthesize an encoding, so that getEncoding() is always usable
	//        PostScriptTable post = rec.getPostScript();
	//        Map<Integer, String> codeToName = new HashMap<Integer, String>();
	//        for (int code = 0; code <= 256; code++)
	//        {
	//            int gid = codeToGID(code);
	//            if (gid > 0)
	//            {
	//                String name = null;
	//                if (post != null)
	//                {
	//                    name = post.getName(gid);
	//                    System.out.println("@@@4a gid=" + gid + " name=" + name);
	//                }
	//                if (name == null)
	//                {
	//                    // GID pseudo-name
	//                    name = Integer.toString(gid);
	//                    System.out.println("@@@4b gid=" + gid + " name=" + name);
	//                }
	//                codeToName.put(code, name);
	//            }
	//        }
	encoding := map[uint16]string{}
	shownZero := false
	for code := uint16(0); code <= 256; code++ {
		gid, ok := rec.Chars[code]
		if !ok {
			continue
		}
		glyph := ""
		if 0 <= gid && int(gid) < len(rec.GlyphNames) {
			glyph = rec.GlyphNames[gid]
			// encoding[code] = glyph
		} else {
			// common.Log.Debug("No match for code=%d gid=%d", code, gid)
			glyph = string(rune(gid))
		}
		encoding[code] = glyph
		if gid != 0 || !shownZero {
			fmt.Printf(" *>> code=%d gid=0x%04x glyph=%q\n", code, gid, glyph)
		}
		if gid == 0 {
			shownZero = true
		}
	}
	if len(encoding) == 0 {
		common.Log.Error("rec=%s", rec)
		common.Log.Error("Chars=[% 02x]", rec.Chars)
		panic("no encoding")
	}
	return textencoding.NewEmbeddedSimpleTextEncoder(encoding, nil)
}

// TtfType contains metrics of a TrueType font.
type TtfType struct {
	Embeddable             bool
	UnitsPerEm             uint16
	PostScriptName         string
	Bold                   bool
	ItalicAngle            float64
	IsFixedPitch           bool
	TypoAscender           int16
	TypoDescender          int16
	UnderlinePosition      int16
	UnderlineThickness     int16
	Xmin, Ymin, Xmax, Ymax int16
	CapHeight              int16
	Widths                 []uint16

	// Map of rune values (unicode) to glyph index.
	Chars      map[uint16]uint16
	GlyphNames []string
}

func (ttf *TtfType) String() string {
	return fmt.Sprintf("FONT_FILE2{%#q Embeddable=%t UnitsPerEm=%d Bold=%t ItalicAngle=%f "+
		"CapHeight=%d Chars=%d GlyphNames=%d}",
		ttf.PostScriptName, ttf.Embeddable, ttf.UnitsPerEm, ttf.Bold, ttf.ItalicAngle,
		ttf.CapHeight, len(ttf.Chars), len(ttf.GlyphNames))
}

// ttfParser describes a TrueType font file.
// http://scripts.sil.org/cms/scripts/page.php?site_id=nrsi&id=iws-chapter08
// Hinting information is contained in three tables within the font—’cvt’, ‘fpgm’, and ‘prep’. These
// tables cannot be easily edited outside of specialized font hinting software.
type ttfParser struct {
	rec              TtfType
	f                io.ReadSeeker // *os.File
	tables           map[string]uint32
	numberOfHMetrics uint16
	numGlyphs        uint16
}

// NewFontFile2FromPdfObject returns metrics of the TrueType font file in `obj`.
func NewFontFile2FromPdfObject(obj PdfObject) (rec TtfType, err error) {

	obj = TraceToDirectObject(obj)

	streamObj, ok := obj.(*PdfObjectStream)
	if !ok {
		common.Log.Debug("ERROR: FontFile must be a stream (%T)", obj)
		err = errors.New("type error")
		return
	}
	data, err := DecodeStream(streamObj)
	if err != nil {
		return
	}

	// fmt.Println("===============&&&&===============")
	// fmt.Printf("%#q", string(data))
	// fmt.Println("===============####===============")

	f := bytes.NewReader(data)
	f.Seek(0, os.SEEK_SET)

	t := ttfParser{f: f}
	rec, err = t.Parse()

	return

	// err = ioutil.WriteFile("xxxx.ttf", data, 0777)
	// if err != nil {
	// 	return
	// }
	// return TtfParse("xxxx.ttf")
}

// TtfParse extracts various metrics from a TrueType font file.
func TtfParse(fileStr string) (rec TtfType, err error) {
	var t ttfParser
	f, err := os.Open(fileStr)
	if err != nil {
		return
	}
	defer f.Close()
	t.f = f
	rec, err = t.Parse()

	return
}

// TtfParse extracts various metrics from a TrueType font file.
func (t *ttfParser) Parse() (TtfRec TtfType, err error) {

	// data, err := t.ReadStr(32)
	// t.f.Seek(0, os.SEEK_SET)
	// for i := 0; i < len(data); i++ {
	// 	b := data[i]
	// 	fmt.Printf("%4d: 0x%02x %c\n", i, b, b)
	// }

	version, err := t.ReadStr(4)
	if err != nil {
		return
	}
	/*
	   if (stream.readTag().equals("OTTO"))
	      {
	          parser = new OTFParser(false, true);
	      }
	      else
	      {
	          parser = new TTFParser(false, true);
	      }
	*/
	if version == "OTTO" {
		err = fmt.Errorf("fonts based on PostScript outlines are not supported")
		panic("OTTO")
		return
	}
	// XXX: !@#$ Not sure what to do here. Have seen version="true"
	if version != "\x00\x01\x00\x00" {
		err = fmt.Errorf("unrecognized file format")
		common.Log.Debug("ERROR: !@#$ unrecognized file format %q", version)
		// return
	}
	numTables := int(t.ReadUShort())
	t.Skip(3 * 2) // searchRange, entrySelector, rangeShift
	t.tables = make(map[string]uint32)
	var tag string
	for j := 0; j < numTables; j++ {
		tag, err = t.ReadStr(4)
		if err != nil {
			return
		}
		t.Skip(4) // checkSum
		offset := t.ReadULong()
		t.Skip(4) // length
		t.tables[tag] = offset
	}

	common.Log.Debug(describeTables(t.tables))

	err = t.ParseComponents()
	if err != nil {
		return
	}

	TtfRec = t.rec
	return
}

func describeTables(tables map[string]uint32) string {
	tags := []string{}
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return tables[tags[i]] < tables[tags[j]] })
	parts := []string{fmt.Sprintf("TrueType tables: %d", len(tables))}
	for _, tag := range tags {
		parts = append(parts, fmt.Sprintf("\t%q %5d", tag, tables[tag]))
	}
	return strings.Join(parts, "\n")
}

// Standard TrueType tables
// "head"
// "hhea"
// "loca"
// "maxp"
// "cvt "
// "prep"
// "glyf"
// "hmtx"
// "fpgm"
// "gasp"
func (t *ttfParser) ParseComponents() (err error) {
	err = t.ParseHead()
	if err != nil {
		return
	}
	err = t.ParseHhea()
	if err != nil {
		return
	}
	err = t.ParseMaxp()
	if err != nil {
		return
	}
	err = t.ParseHmtx()
	if err != nil {
		return
	}

	if _, ok := t.tables["name"]; ok {
		err = t.ParseName()
		if err != nil {
			return
		}
	}
	if _, ok := t.tables["OS/2"]; ok {
		err = t.ParseOS2()
		if err != nil {
			return
		}
	}
	if _, ok := t.tables["post"]; ok {
		err = t.ParsePost()
		if err != nil {
			return
		}
	}
	if _, ok := t.tables["cmap"]; ok {
		err = t.ParseCmap()
		if err != nil {
			return
		}
	}

	return
}

func (t *ttfParser) ParseHead() (err error) {
	err = t.Seek("head")
	t.Skip(3 * 4) // version, fontRevision, checkSumAdjustment
	magicNumber := t.ReadULong()
	if magicNumber != 0x5F0F3CF5 {
		err = fmt.Errorf("incorrect magic number")
		return
	}
	t.Skip(2) // flags
	t.rec.UnitsPerEm = t.ReadUShort()
	t.Skip(2 * 8) // created, modified
	t.rec.Xmin = t.ReadShort()
	t.rec.Ymin = t.ReadShort()
	t.rec.Xmax = t.ReadShort()
	t.rec.Ymax = t.ReadShort()
	return
}

func (t *ttfParser) ParseHhea() (err error) {
	err = t.Seek("hhea")
	if err == nil {
		t.Skip(4 + 15*2)
		t.numberOfHMetrics = t.ReadUShort()
	}
	return
}

func (t *ttfParser) ParseMaxp() (err error) {
	err = t.Seek("maxp")
	if err == nil {
		t.Skip(4)
		t.numGlyphs = t.ReadUShort()
	}
	return
}

func (t *ttfParser) ParseHmtx() (err error) {
	err = t.Seek("hmtx")
	if err == nil {
		t.rec.Widths = make([]uint16, 0, 8)
		for j := uint16(0); j < t.numberOfHMetrics; j++ {
			t.rec.Widths = append(t.rec.Widths, t.ReadUShort())
			t.Skip(2) // lsb
		}
		if t.numberOfHMetrics < t.numGlyphs {
			lastWidth := t.rec.Widths[t.numberOfHMetrics-1]
			for j := t.numberOfHMetrics; j < t.numGlyphs; j++ {
				t.rec.Widths = append(t.rec.Widths, lastWidth)
			}
		}
	}
	return
}

// parseCmapSubtable31 parses information from an (3,1) subtable (Windows Unicode).
func (t *ttfParser) parseCmapSubtable31(offset31 int64) (err error) {
	common.Log.Debug("parseCmapSubtable31: offset31=%d", offset31)
	startCount := make([]uint16, 0, 8)
	endCount := make([]uint16, 0, 8)
	idDelta := make([]int16, 0, 8)
	idRangeOffset := make([]uint16, 0, 8)
	t.rec.Chars = make(map[uint16]uint16)
	t.f.Seek(int64(t.tables["cmap"])+offset31, os.SEEK_SET)
	format := t.ReadUShort()
	if format != 4 {
		err = fmt.Errorf("unexpected subtable format: %d", format)
		return
	}
	t.Skip(2 * 2) // length, language
	segCount := int(t.ReadUShort() / 2)
	t.Skip(3 * 2) // searchRange, entrySelector, rangeShift
	for j := 0; j < segCount; j++ {
		endCount = append(endCount, t.ReadUShort())
	}
	t.Skip(2) // reservedPad
	for j := 0; j < segCount; j++ {
		startCount = append(startCount, t.ReadUShort())
	}
	for j := 0; j < segCount; j++ {
		idDelta = append(idDelta, t.ReadShort())
	}
	offset, _ := t.f.Seek(int64(0), os.SEEK_CUR)
	for j := 0; j < segCount; j++ {
		idRangeOffset = append(idRangeOffset, t.ReadUShort())
	}
	for j := 0; j < segCount; j++ {
		c1 := startCount[j]
		c2 := endCount[j]
		d := idDelta[j]
		ro := idRangeOffset[j]
		if ro > 0 {
			t.f.Seek(offset+2*int64(j)+int64(ro), os.SEEK_SET)
		}
		for c := c1; c <= c2; c++ {
			if c == 0xFFFF {
				break
			}
			var gid int32
			if ro > 0 {
				gid = int32(t.ReadUShort())
				if gid > 0 {
					gid += int32(d)
				}
			} else {
				gid = int32(c) + int32(d)
			}
			if gid >= 65536 {
				gid -= 65536
			}
			if gid > 0 {
				t.rec.Chars[c] = uint16(gid)
			}
		}
	}
	// for code, glyphId := range data {
	// 	t.rec.Chars[uint16(code)] = uint16(glyphId)
	// 	if glyphId != 0 {
	// 		fmt.Printf("\t0x%02x -> 0x%02x=%c\n", code, glyphId, rune(glyphId))
	// 	}
	// }
	return
}

// parseCmapSubtable10 parses information from an (1,0) subtable (symbol).
func (t *ttfParser) parseCmapSubtable10(offset10 int64) error {
	common.Log.Debug("parseCmapSubtable10: offset10=%d", offset10)
	//startCount := make([]uint16, 0, 8)
	//endCount := make([]uint16, 0, 8)
	//idDelta := make([]int16, 0, 8)
	//idRangeOffset := make([]uint16, 0, 8)

	if t.rec.Chars == nil {
		t.rec.Chars = make(map[uint16]uint16)
	}

	//t.rec.Chars = make(map[uint16]uint16)

	t.f.Seek(int64(t.tables["cmap"])+offset10, os.SEEK_SET)
	var length, language uint32
	format := t.ReadUShort()
	if format < 8 {
		length = uint32(t.ReadUShort())
		language = uint32(t.ReadUShort())
	} else {
		t.ReadUShort()
		length = t.ReadULong()
		language = t.ReadULong()
	}
	common.Log.Debug("parseCmapSubtable10: format=%d length=%d language=%d",
		format, length, language)

	if format != 0 {
		panic("Unsupported format")
	}

	dataStr, err := t.ReadStr(256)
	if err != nil {
		return err
	}
	data := []byte(dataStr)

	for code, glyphId := range data {
		t.rec.Chars[uint16(code)] = uint16(glyphId)
		if glyphId != 0 {
			fmt.Printf("\t0x%02x -> 0x%02x=%c\n", code, glyphId, rune(glyphId))
		}
	}
	return nil

	// firstCode := t.ReadUShort()
	// entryCount := t.ReadUShort()

	// curCode := firstCode
	// for i := 0; i < int(entryCount); i++ {
	// 	glyphId := t.ReadUShort()
	// 	t.rec.Chars[curCode] = glyphId
	// 	fmt.Printf("%d -> %d\n", curCode, glyphId)

	// 	curCode++
	// }

	// fmt.Printf("Length: %d, language: %d\n", length, language)
	// fmt.Printf("First code: %d, entry count: %d\n", firstCode, entryCount)
	// return nil
}

// ParseCmap parses the cmap table in a TrueType font.
func (t *ttfParser) ParseCmap() (err error) {
	var offset int64
	if err = t.Seek("cmap"); err != nil {
		panic(err)
		return
	}
	common.Log.Debug("ParseCmap")
	version := t.ReadUShort()
	numTables := int(t.ReadUShort())
	offset10 := int64(0)
	offset31 := int64(0)
	tablesDone := map[string]bool{}
	for j := 0; j < numTables; j++ {
		platformID := t.ReadUShort()
		encodingID := t.ReadUShort()
		offset = int64(t.ReadULong())
		platEnc := fmt.Sprintf("(%d,%d)", platformID, encodingID)
		if _, ok := tablesDone[platEnc]; ok {
			panic(fmt.Errorf("duplicate %s cmap", platEnc))
		}
		tablesDone[platEnc] = true
		common.Log.Debug("ParseCmap: table %d: version=%d platformID=%d encodingID=%d offset=%d",
			j, version, platformID, encodingID, offset)
		if platformID == 3 && encodingID == 1 {
			// (3,1) subtable. Windows Unicode.
			// if offset31 != 0 {
			// 	panic("duplicate (3,1) cmap")
			// }
			offset31 = offset
		} else /*if platformID == 1 && encodingID == 0*/ {
			// (1,0) subtable.
			// if offset10 != 0 {
			// 	panic("duplicate (1,0) cmap")
			// }
			offset10 = offset
		} /* else {
			common.Log.Error("unsupported cmap version=%d platformID=%d encodingID=%d offset=%d",
				version, platformID, encodingID, offset)
			panic("unsupported cmap version")
		}*/
	}

	// Latin font support based on (3,1) table encoding.
	if offset31 != 0 {
		err = t.parseCmapSubtable31(offset31)
		if err != nil {
			return
		}
	}

	// Many non-Latin fonts (including asian fonts) use subtable (1,0).

	if offset10 != 0 {
		fmt.Printf("Offset 10: %d\n", offset10)
		err = t.parseCmapVersion(offset10)
		if err != nil {
			return
		}
	}

	return
}

func (t *ttfParser) parseCmapVersion(offset int64) error {
	common.Log.Debug("parseCmapVersion: offset=%d", offset)

	if t.rec.Chars == nil {
		t.rec.Chars = make(map[uint16]uint16)
	}

	t.f.Seek(int64(t.tables["cmap"])+offset, os.SEEK_SET)
	var length, language uint32
	format := t.ReadUShort()
	if format < 8 {
		length = uint32(t.ReadUShort())
		language = uint32(t.ReadUShort())
	} else {
		t.ReadUShort()
		length = t.ReadULong()
		language = t.ReadULong()
	}
	common.Log.Debug("parseCmapVersion: format=%d length=%d language=%d",
		format, length, language)

	switch format {
	case 0:
		return t.parseCmapFormat0()
	case 6:
		return t.parseCmapFormat6()
	default:
		common.Log.Debug("ERROR: Unsupported cmap format=%d", format)
		return ErrFontNotSupported
	}
}

func (t *ttfParser) parseCmapFormat0() error {
	dataStr, err := t.ReadStr(256)
	if err != nil {
		return err
	}
	data := []byte(dataStr)
	common.Log.Debug("parseCmapFormat0: %s\ndataStr=%+q\ndata=[% 02x]",
		t.rec.String(), dataStr, data)

	for code, glyphId := range data {
		t.rec.Chars[uint16(code)] = uint16(glyphId)
		if glyphId != 0 {
			fmt.Printf(" 0>> 0x%02x -> 0x%02x=%c\n", code, glyphId, rune(glyphId))
		}
	}
	return nil
}

func (t *ttfParser) parseCmapFormat6() error {

	firstCode := int(t.ReadUShort())
	entryCount := int(t.ReadUShort())

	common.Log.Debug("parseCmapFormat6: %s firstCode=%d entryCount=%d",
		t.rec.String(), firstCode, entryCount)

	for i := 0; i < entryCount; i++ {
		glyphId := t.ReadUShort()
		t.rec.Chars[uint16(i+firstCode)] = glyphId
		if glyphId != 0 {
			fmt.Printf(" 6>> 0x%02x -> 0x%02x=%+q\n", i+firstCode, glyphId, rune(glyphId))
		}
	}

	return nil
}

func (t *ttfParser) ParseName() (err error) {
	if err = t.Seek("name"); err != nil {
		return
	}
	tableOffset, _ := t.f.Seek(0, os.SEEK_CUR)
	t.rec.PostScriptName = ""
	t.Skip(2) // format
	count := t.ReadUShort()
	stringOffset := t.ReadUShort()
	for j := uint16(0); j < count && t.rec.PostScriptName == ""; j++ {
		t.Skip(3 * 2) // platformID, encodingID, languageID
		nameID := t.ReadUShort()
		length := t.ReadUShort()
		offset := t.ReadUShort()
		if nameID == 6 {
			// PostScript name
			t.f.Seek(int64(tableOffset)+int64(stringOffset)+int64(offset), os.SEEK_SET)
			var s string
			s, err = t.ReadStr(int(length))
			if err != nil {
				return
			}
			s = strings.Replace(s, "\x00", "", -1)
			var re *regexp.Regexp
			if re, err = regexp.Compile("[(){}<> /%[\\]]"); err != nil {
				return
			}
			t.rec.PostScriptName = re.ReplaceAllString(s, "")
		}
	}
	if t.rec.PostScriptName == "" {
		err = fmt.Errorf("the name PostScript was not found")
	}
	return
}

func (t *ttfParser) ParseOS2() (err error) {
	if err = t.Seek("OS/2"); err != nil {
		return
	}
	version := t.ReadUShort()
	t.Skip(3 * 2) // xAvgCharWidth, usWeightClass, usWidthClass
	fsType := t.ReadUShort()
	t.rec.Embeddable = (fsType != 2) && (fsType&0x200) == 0
	t.Skip(11*2 + 10 + 4*4 + 4)
	fsSelection := t.ReadUShort()
	t.rec.Bold = (fsSelection & 32) != 0
	t.Skip(2 * 2) // usFirstCharIndex, usLastCharIndex
	t.rec.TypoAscender = t.ReadShort()
	t.rec.TypoDescender = t.ReadShort()
	if version >= 2 {
		t.Skip(3*2 + 2*4 + 2)
		t.rec.CapHeight = t.ReadShort()
	} else {
		t.rec.CapHeight = 0
	}
	return
}

// Sets t.rec.GlyphNames
func (t *ttfParser) ParsePost() (err error) {
	if err = t.Seek("post"); err != nil {
		return
	}

	formatType := t.Read32Fixed()
	t.rec.ItalicAngle = t.Read32Fixed()
	t.rec.UnderlinePosition = t.ReadShort()
	t.rec.UnderlineThickness = t.ReadShort()
	t.rec.IsFixedPitch = t.ReadULong() != 0
	/*minMemType42 := */ t.ReadULong()
	/*maxMemType42 := */ t.ReadULong()
	/*mimMemType1 := */ t.ReadULong()
	/*maxMemType1 := */ t.ReadULong()

	common.Log.Debug("ParsePost: formatType=%f", formatType)

	switch formatType {
	case 1.0: // This font file contains exactly the 258 glyphs in the standard Macintosh TrueType.
		t.rec.GlyphNames = MAC_GLYPH_NAMES
	case 2.0:
		numGlyphs := int(t.ReadUShort())
		glyphNameIndex := make([]int, numGlyphs)
		t.rec.GlyphNames = make([]string, numGlyphs)
		maxIndex := -1
		for i := 0; i < numGlyphs; i++ {
			index := int(t.ReadUShort())
			glyphNameIndex[i] = index
			// PDFBOX-808: Index numbers between 32768 and 65535 are
			// reserved for future use, so we should just ignore them
			if index <= 32767 && index > maxIndex {
				maxIndex = index
			}
		}
		var nameArray []string
		if maxIndex >= len(MAC_GLYPH_NAMES) {
			nameArray = make([]string, maxIndex-len(MAC_GLYPH_NAMES)+1)
			for i := 0; i < maxIndex-len(MAC_GLYPH_NAMES)+1; i++ {
				numberOfChars := int(t.ReadByte())
				names, err := t.ReadStr(numberOfChars)
				if err != nil {
					return err
				}
				nameArray[i] = names
			}
		}
		for i := 0; i < numGlyphs; i++ {
			index := glyphNameIndex[i]
			if index < len(MAC_GLYPH_NAMES) {
				t.rec.GlyphNames[i] = MAC_GLYPH_NAMES[index]
			} else if index >= len(MAC_GLYPH_NAMES) && index <= 32767 {
				t.rec.GlyphNames[i] = nameArray[index-len(MAC_GLYPH_NAMES)]
			} else {
				// PDFBOX-808: Index numbers between 32768 and 65535 are
				// reserved for future use, so we should just ignore them
				t.rec.GlyphNames[i] = ".undefined"
			}
		}
	case 2.5:
		glyphNameIndex := make([]int, t.numGlyphs) // !@#$ Check that this is parsed first
		for i := 0; i < len(glyphNameIndex); i++ {
			offset := int(t.ReadSByte())
			glyphNameIndex[i] = i + 1 + offset
		}
		t.rec.GlyphNames = make([]string, len(glyphNameIndex))
		for i := 0; i < len(t.rec.GlyphNames); i++ {
			name := MAC_GLYPH_NAMES[glyphNameIndex[i]]
			t.rec.GlyphNames[i] = name
		}
	case 3.0:
		// no postscript information is provided.
		common.Log.Debug("No PostScript name information is provided for the font.")
	default:
		common.Log.Debug("ERROR: Unknown formatType=%f", formatType)
	}

	/*
		t.Skip(4 * 4) // Skip over memory specs.

		if versionUpper == 2 && versionFraction == 0 {
			numGlyps := t.ReadUShort()

			glyphNameIndexList := []uint16{}
			maxIndex := uint16(0)
			for i := 0; i < int(numGlyps); i++ {
				index := t.ReadUShort()
				glyphNameIndexList = append(glyphNameIndexList, index)
				if index > maxIndex {
					maxIndex = index
				}
			}

			numberNewGlyphs := maxIndex + 1
			if maxIndex > 257 {
				numberNewGlyphs -= 258
			}
			for i := 0; i < int(numberNewGlyphs); i++ {
				len := t.ReadByte()
				glyphName, err := t.ReadStr(int(len))
				if err != nil {
					return err
				}
			}
		}
	*/
	return
}

// The 258 standard mac glyph names a used in 'post' format 1 and 2.
var MAC_GLYPH_NAMES = []string{
	".notdef", ".null", "nonmarkingreturn", "space", "exclam", "quotedbl",
	"numbersign", "dollar", "percent", "ampersand", "quotesingle",
	"parenleft", "parenright", "asterisk", "plus", "comma", "hyphen",
	"period", "slash", "zero", "one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "colon", "semicolon", "less",
	"equal", "greater", "question", "at", "A", "B", "C", "D", "E", "F",
	"G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S",
	"T", "U", "V", "W", "X", "Y", "Z", "bracketleft", "backslash",
	"bracketright", "asciicircum", "underscore", "grave", "a", "b",
	"c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o",
	"p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "braceleft",
	"bar", "braceright", "asciitilde", "Adieresis", "Aring",
	"Ccedilla", "Eacute", "Ntilde", "Odieresis", "Udieresis", "aacute",
	"agrave", "acircumflex", "adieresis", "atilde", "aring",
	"ccedilla", "eacute", "egrave", "ecircumflex", "edieresis",
	"iacute", "igrave", "icircumflex", "idieresis", "ntilde", "oacute",
	"ograve", "ocircumflex", "odieresis", "otilde", "uacute", "ugrave",
	"ucircumflex", "udieresis", "dagger", "degree", "cent", "sterling",
	"section", "bullet", "paragraph", "germandbls", "registered",
	"copyright", "trademark", "acute", "dieresis", "notequal", "AE",
	"Oslash", "infinity", "plusminus", "lessequal", "greaterequal",
	"yen", "mu", "partialdiff", "summation", "product", "pi",
	"integral", "ordfeminine", "ordmasculine", "Omega", "ae", "oslash",
	"questiondown", "exclamdown", "logicalnot", "radical", "florin",
	"approxequal", "Delta", "guillemotleft", "guillemotright",
	"ellipsis", "nonbreakingspace", "Agrave", "Atilde", "Otilde", "OE",
	"oe", "endash", "emdash", "quotedblleft", "quotedblright",
	"quoteleft", "quoteright", "divide", "lozenge", "ydieresis",
	"Ydieresis", "fraction", "currency", "guilsinglleft",
	"guilsinglright", "fi", "fl", "daggerdbl", "periodcentered",
	"quotesinglbase", "quotedblbase", "perthousand", "Acircumflex",
	"Ecircumflex", "Aacute", "Edieresis", "Egrave", "Iacute",
	"Icircumflex", "Idieresis", "Igrave", "Oacute", "Ocircumflex",
	"apple", "Ograve", "Uacute", "Ucircumflex", "Ugrave", "dotlessi",
	"circumflex", "tilde", "macron", "breve", "dotaccent", "ring",
	"cedilla", "hungarumlaut", "ogonek", "caron", "Lslash", "lslash",
	"Scaron", "scaron", "Zcaron", "zcaron", "brokenbar", "Eth", "eth",
	"Yacute", "yacute", "Thorn", "thorn", "minus", "multiply",
	"onesuperior", "twosuperior", "threesuperior", "onehalf",
	"onequarter", "threequarters", "franc", "Gbreve", "gbreve",
	"Idotaccent", "Scedilla", "scedilla", "Cacute", "cacute", "Ccaron",
	"ccaron", "dcroat",
}

// Seek moves the file pointer to the table named `tag`.
func (t *ttfParser) Seek(tag string) error {
	ofs, ok := t.tables[tag]
	if !ok {
		return fmt.Errorf("table not found: %s", tag)
	}
	t.f.Seek(int64(ofs), os.SEEK_SET)
	return nil
}

func (t *ttfParser) Skip(n int) {
	t.f.Seek(int64(n), os.SEEK_CUR)
}

func (t *ttfParser) ReadStr(length int) (str string, err error) {
	var n int
	buf := make([]byte, length)
	n, err = t.f.Read(buf)
	// common.Log.Debug("ReadStr: [% 02x]", buf)
	if err != nil {
		return
	}
	if n == length {
		str = string(buf)
	} else {
		err = fmt.Errorf("unable to read %d bytes", length)
	}
	return
}

func (t *ttfParser) ReadByte() (val uint8) {
	binary.Read(t.f, binary.BigEndian, &val)
	return
}

func (t *ttfParser) ReadSByte() (val int8) {
	binary.Read(t.f, binary.BigEndian, &val)
	return
}

func (t *ttfParser) ReadUShort() (val uint16) {
	binary.Read(t.f, binary.BigEndian, &val)
	return
}

func (t *ttfParser) ReadShort() (val int16) {
	binary.Read(t.f, binary.BigEndian, &val)
	return
}

func (t *ttfParser) ReadULong() (val uint32) {
	binary.Read(t.f, binary.BigEndian, &val)
	return
}

func (t *ttfParser) Read32Fixed() float64 {
	whole := float64(t.ReadUShort())
	frac := float64(t.ReadUShort()) / 65536.0
	return whole + frac
}
