package model

import (
	"errors"
	"fmt"
	"io/ioutil"
	"sort"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/cmap"
	"github.com/unidoc/unidoc/pdf/model/fonts"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

/*
   9.7.2 CID-Keyed Fonts Overview (page 267)
   The CID-keyed font architecture specifies the external representation of certain font programs,
   called *CMap* and *CIDFont* files, along with some conventions for combining and using those files.

   The term CID-keyed font reflects the fact that CID (character identifier) numbers are used to
   index and access the glyph descriptions in the font.

   A character collection is an ordered set of glyphs. The order of the glyphs in the character
   collection shall determine the CID number for each glyph. Each CID-keyed font shall explicitly
   reference the character collection on which its CID numbers are based; see 9.7.3, "CIDSystemInfo
   Dictionaries".

   A *CMap* (character map) file shall specify the correspondence between character codes and the CID
   numbers used to identify glyphs. It is equivalent to the concept of an encoding in simple fonts.
   Whereas a simple font allows a maximum of 256 glyphs to be encoded and accessible at one time, a
   CMap can describe a mapping from multiple-byte codes to thousands of glyphs in a large CID-keyed
   font.

   EXAMPLE A CMap can describe Shift-JIS, one of several widely used encodings for Japanese.

   A CMap file may reference an entire character collection or a subset of a character collection.
   The CMap file’s mapping yields a font number (which in PDF shall be 0) and a character selector
   (which in PDF shall be a CID). Furthermore, a CMap file may incorporate another CMap file by
   reference, without having to duplicate it. These features enable character collections to be
   combined or supplemented and make all the constituent characters accessible to text-showing
   operations through a single encoding.

   A *CIDFont* contains the glyph descriptions for a character collection. The glyph descriptions
   themselves are typically in a format similar to those used in simple fonts, such as Type 1.
   However, they are identified by CIDs rather than by names, and they are organized differently.
   In PDF, the data from a CMap file and CIDFont shall be represented by PDF objects as described
   in 9.7.4, "CIDFonts" and 9.7.5, "CMaps". The CMap file and CIDFont programs themselves may be
   either referenced by name or embedded as stream objects in the PDF file.

   A CID-keyed font, then, shall be the combination of a CMap with a CIDFont containing glyph
   descriptions. It shall be represented as a Type 0 font. It contains an Encoding entry whose
   value shall be a CMap dictionary, and its DescendantFonts entry shall reference the CIDFont
   dictionary with which the CMap has been combined.

   9.7.3 CIDSystemInfo Dictionaries (page 268)

   CIDFont and CMap dictionaries shall contain a CIDSystemInfo entry specifying the character
   collection assumed by the CIDFont associated with the CMap—that is, the interpretation of the CID
   numbers used by the CIDFont. A character collection shall be uniquely identified by the Registry,
   Ordering, and Supplemententries in the CIDSystemInfo dictionary, as described in Table 116. In
   order to be compatible, the Registry and Ordering values must be the same.

   The CIDSystemInfo entry in a CIDFont is a dictionary that shall specify the CIDFont’s character
   collection. The CIDFont need not contain glyph descriptions for all the CIDs in a collection;
   it may contain a subset. The CIDSystemInfo entry in a CMap file shall be either a single
   dictionary or an array of dictionaries, depending on whether it associates codes with a single
   character collection or with multiple character collections; see 9.7.5, "CMaps".

   9.7.4 CIDFonts (page 269)

   A CIDFont program contains glyph descriptions that are accessed using a CID as the character
   selector. There are two types of CIDFonts:
   • A Type 0 CIDFont contains glyph descriptions based on CFF
   • A Type 2 CIDFont contains glyph descriptions based on the TrueType font format

   A CIDFont dictionary is a PDF object that contains information about a CIDFont program. Although
   its Type value is Font, a CIDFont is not actually a font.
       It does not have an Encoding entry,
       it may not be listed in the Font subdictionary of a resource dictionary, and
       it may not be used as the operand of the Tf operator.
       It shall be used only as a descendant of a Type 0 font.
   The CMap in the Type 0 font shall be what defines the encoding that maps character codes to CIDs
   in  the CIDFont.

   9.7.4.2 Glyph Selection in CIDFonts (page 270)

   Type 0 and Type 2 CIDFonts handle the mapping from CIDs to glyph descriptions in somewhat
   different ways.

   For Type 0, the CIDFont program contains glyph descriptions that are identified by CIDs. The
   CIDFont program identifies the character collection by a CIDSystemInfo dictionary, which should
   be copied into the PDF CIDFont dictionary. CIDs shall be interpreted uniformly in all CIDFont
   programs supporting a given character collection, whether the program is embedded in the PDF file
   or obtained from an external source.
   When the CIDFont contains an embedded font program that is represented in the Compact Font Format
   (CFF), the FontFile3 entry in the font descriptor (see Table 126) may be CIDFontType0C or
   OpenType. There are two cases, depending on the contents of the font program:
   • The “CFF” font program has a Top DICT that uses CIDFont operators: The CIDs shall be used to
     determine the GID value for the glyph procedure using the charset table in the CFF program.
     The GID value shall then be used to look up the glyph procedure using the CharStrings INDEX
     table.
     NOTE Although in many fonts the CID value and GID value are the same, the CID and GID values
          may differ.
   • The “CFF” font program has a Top DICT that does not use CIDFont operators: The CIDs shall be
     used directly as GID values, and the glyph procedure shall be retrieved using the CharStrings
     INDEX.

   For Type 2, the CIDFont program is actually a TrueType font program, which has no native notion
   of CIDs. In a TrueType font program, glyph descriptions are identified by glyph index values.
   Glyph indices are internal to the font and are not defined consistently from one font to another.
   Instead, a TrueType font program contains a “cmap” table that provides mappings directly from
   character codes to glyph indices for one or more predefined encodings.

   9.7.5 CMaps (Page 272)

   A CMap shall specify the mapping from character codes to character selectors. In PDF, the
   character selectors shall be CIDs in a CIDFont. A CMap serves a function analogous to the
   Encoding dictionary for a simple font. The CMap shall not refer directly to a specific CIDFont;
   instead, it shall be combined with it as part of a CID-keyed font, represented in PDF as a Type 0
   font dictionary (see 9.7.6, "Type 0 Font Dictionaries"). Within the CMap, the character mappings
   shall refer to the associated CIDFont by font number, which in PDF shall be 0.
   PDF also uses a special type of CMap to map character codes to Unicode values (see 9.10.3,
   "ToUnicode CMaps").
   A CMap shall specify the writing mode—horizontal or vertical—for any CIDFont with which the CMap
   is combined. The writing mode determines which metrics shall be used when glyphs are painted from
   that font.
   NOTE Writing mode is specified as part of the CMap because, in some cases, different shapes are
   used when writing horizontally and vertically. In such cases, the horizontal and vertical
   variants of a CMap specify different CIDs for a given character code.
   A CMap shall be specified in one of two ways:
   • As a name object identifying a predefined CMap, whose value shall be one of the predefined CMap
     names defined in Table 118.
   • As a stream object whose contents shall be a CMap file

    The CMap programs that define the predefined CMaps are available through the ASN Web site.

    9.7.5.3 Embedded CMap Files (page 277)

     For character encodings that are not predefined, the PDF file shall contain a stream that
     defines the CMap. In addition to the standard entries for streams (listed in Table 5), the
     CMap stream dictionary contains the entries listed in Table 120. The data in the stream defines
     the mapping from character codes to a font number and a character selector. The data shall
     follow the syntax defined in Adobe Technical Note #5014, Adobe CMap and CIDFont Files
     Specification (see bibliography).


   !@#$% Extract table with Tabula


    9.7.6 Type 0 Font Dictionaries (page 279)

    Type      Font
    Subtype   Type0
    BaseFont  (Required) The name of the font. If the descendant is a Type 0 CIDFont, this name
              should be the concatenation of the CIDFont’s BaseFont name, a hyphen, and the CMap
              name given in the Encoding entry (or the CMapName entry in the CMap). If the
              descendant is a Type 2 CIDFont, this name should be the same as the CIDFont’s BaseFont
              name.
              NOTE In principle, this is an arbitrary name, since there is no font program
                   associated directly with a Type 0 font dictionary. The conventions described here
                   ensure maximum compatibility with existing readers.
    Encoding name or stream (Required)
             The name of a predefined CMap, or a stream containing a CMap that maps character codes
             to font numbers and CIDs. If the descendant is a Type 2 CIDFont whose associated
             TrueType font program is not embedded in the PDF file, the Encoding entry shall be a
             predefined CMap name (see 9.7.4.2, "Glyph Selection in CIDFonts").

    Type 0 font from 000046.pdf

    103 0 obj
    << /Type /Font /Subtype /Type0 /Encoding /Identity-H /DescendantFonts [179 0 R]
    /BaseFont /FLDOLC+PingFangSC-Regular >>
    endobj
    179 0 obj
    << /Type /Font /Subtype /CIDFontType0 /BaseFont /FLDOLC+PingFangSC-Regular
    /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>
    /W 180 0 R /DW 1000 /FontDescriptor 181 0 R >>
    endobj
    180 0 obj
    [ ]
    endobj
    181 0 obj
    << /Type /FontDescriptor /FontName /FLDOLC+PingFangSC-Regular /Flags 4 /FontBBox
    [-123 -263 1177 1003] /ItalicAngle 0 /Ascent 972 /Descent -232 /CapHeight
    864 /StemV 70 /XHeight 648 /StemH 64 /AvgWidth 1000 /MaxWidth 1300 /FontFile3
    182 0 R >>
    endobj
    182 0 obj
    << /Length 183 0 R /Subtype /CIDFontType0C /Filter /FlateDecode >>
    stream
    ....

*/
// pdfFontType0 represents a Type0 font in PDF. Used for composite fonts which can encode multiple
// bytes for complex symbols (e.g. used in Asian languages). Represents the root font whereas the
// associated CIDFont is called its descendant.
type pdfFontType0 struct {
	container *PdfIndirectObject
	skeleton  *PdfFont

	encoder        textencoding.TextEncoder
	CMap           *cmap.CMap
	Encoding       PdfObject
	DescendantFont *PdfFont // Can be either CIDFontType0 or CIDFontType2 font.
	ToUnicode      PdfObject
}

func (font pdfFontType0) String() string {
	return fmt.Sprintf("%s\n\t%s\n\t%s", font.skeleton.String(), font.CMap.String(),
		font.DescendantFont.String())
}

func (font pdfFontType0) CharcodeBytesToUnicode(src []byte) string {
	switch t := font.DescendantFont.context.(type) {
	case *pdfCIDFontType0:
		cmap := font.CMap
		codes := cmap.ReadCodes(src)
		cidToRune := t.cidToRune
		if len(cidToRune) == 0 {
			fmt.Printf("*** cmap=%s\n", cmap.String())
			// panic("GGGG")
		}
		runes := []rune{}
		if len(cidToRune) > 0 {
			for _, code := range codes {
				cid := cmap.ToCID(code)
				r := cidToRune[cid]
				runes = append(runes, r)
			}
		} else {
			for _, code := range codes {
				cid := cmap.ToCID(code)
				r := rune(cid)
				runes = append(runes, r)
			}
		}
		return string(runes)
	}
	panic("not implemented")
	return fmt.Sprintf("%s\n\t%s\n\t%s", font.skeleton.String(), font.CMap.String(),
		font.DescendantFont.String())
}

// GetGlyphCharMetrics returns the character metrics for the specified glyph.  A bool flag is
// returned to indicate whether or not the entry was found in the glyph to charcode mapping.
func (font pdfFontType0) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	if font.DescendantFont == nil {
		common.Log.Debug("ERROR: No descendant. font=%s", font)
		return fonts.CharMetrics{}, false
	}
	return font.DescendantFont.GetGlyphCharMetrics(glyph)
}

// Encoder returns the font's text encoder.
func (font pdfFontType0) Encoder() textencoding.TextEncoder {
	return font.encoder
}

// SetEncoder sets the encoder for the truetype font.
func (font pdfFontType0) SetEncoder(encoder textencoding.TextEncoder) {
	font.encoder = encoder
}

// ToPdfObject converts the pdfFontType0 to a PDF representation.
func (font *pdfFontType0) ToPdfObject() PdfObject {
	if font.container == nil {
		font.container = &PdfIndirectObject{}
	}
	d := font.skeleton.toDict("Type0")

	if font.Encoding != nil {
		d.Set("Encoding", font.Encoding)
	}
	if font.DescendantFont != nil {
		// Shall be 1 element array.
		d.Set("DescendantFonts", MakeArray(font.DescendantFont.ToPdfObject()))
	}
	if font.ToUnicode != nil {
		d.Set("ToUnicode", font.ToUnicode)
	}

	return font.container
}

// newPdfFontType0FromPdfObject makes a pdfFontType0 based on the input PdfObject which should be
// represented by a dictionary. If a problem is encountered, an error is returned.
func newPdfFontType0FromPdfObject(obj PdfObject, skeleton *PdfFont) (*pdfFontType0, error) {

	d := skeleton.dict

	// DescendantFonts.
	obj = TraceToDirectObject(d.Get("DescendantFonts"))
	arr, ok := obj.(*PdfObjectArray)
	if !ok {
		common.Log.Debug("ERROR: Invalid DescendantFonts - not an array (%T) %s", obj, skeleton)
		return nil, ErrRangeError
	}
	if len(*arr) != 1 {
		common.Log.Debug("ERROR: Array length != 1 (%d)", len(*arr))
		return nil, ErrRangeError
	}
	df, err := newPdfFontFromPdfObject((*arr)[0], false)
	if err != nil {
		common.Log.Debug("ERROR: Failed loading descendant font: err=%v %s", err, skeleton)
		return nil, err
	}

	var cm *cmap.CMap
	switch t := TraceToDirectObject(d.Get("Encoding")).(type) {
	case *PdfObjectName:
		name := string(*t)
		cm, err = cmap.GetPredefinedCmap(name)
		if err != nil {
			common.Log.Debug("ERROR: There is no prefedined CMap %#v. font=%s", name, skeleton)
			return nil, err
		}
	default:
		common.Log.Debug("Incompatibility ERROR: Type 0 font encoding not a name (%T) font=%s",
			obj, skeleton)
		panic(ErrTypeError)
		return nil, ErrTypeError
	}

	font := &pdfFontType0{
		skeleton:       skeleton,
		DescendantFont: df,
		CMap:           cm,
		ToUnicode:      TraceToDirectObject(d.Get("ToUnicode")),
	}
	fmt.Printf("font=%s\n", font)
	// panic("3333")

	return font, nil
}

// pdfCIDFontType0 represents a CIDFont Type0 font dictionary.
type pdfCIDFontType0 struct {
	container *PdfIndirectObject
	skeleton  *PdfFont // Elements common to all font types. !@#$% Possibly doesn't belong here.

	encoder textencoding.TextEncoder

	// Table 117 – Entries in a CIDFont dictionary (page 269)
	CIDSystemInfo  PdfObject // (Required) Dictionary that defines the character collection of the CIDFont. See Table 116.
	FontDescriptor PdfObject // (Required) Describes the CIDFont’s default metrics other than its glyph widths
	DW             PdfObject // (Optional) Default width for glyphs in the CIDFont Default value: 1000 (defined in user units)
	W              PdfObject // (Optional) Widths for the glyphs in the CIDFont. Default value: none (the DW value shall be used for all glyphs).
	// DW2, W2: (Optional; applies only to CIDFonts used for vertical writing)
	DW2 PdfObject // An array of two numbers specifying the default metrics for vertical writing. Default value: [880 −1000].
	W2  PdfObject // A description of the metrics for vertical writing for the glyphs in the CIDFont. Default value: none (the DW2 value shall be used for all glyphs).

	// Mapping from CIDs to unicode runes
	cidToRune map[int]rune

	// Mapping from unicode runes to widths.
	runeToWidthMap map[uint16]int

	// Also mapping from GIDs (glyph index) to widths.
	gidToWidthMap map[uint16]int
}

// Encoder returns the font's text encoder.
func (font pdfCIDFontType0) Encoder() textencoding.TextEncoder {
	return font.encoder
}

// SetEncoder sets the encoder for the truetype font.
func (font pdfCIDFontType0) SetEncoder(encoder textencoding.TextEncoder) {
	font.encoder = encoder
}

// GetGlyphCharMetrics returns the character metrics for the specified glyph.  A bool flag is
// returned to indicate whether or not the entry was found in the glyph to charcode mapping.
func (font pdfCIDFontType0) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	metrics := fonts.CharMetrics{}
	// Not implemented yet. !@#$
	return metrics, true
}

// ToPdfObject converts the pdfCIDFontType0 to a PDF representation.
func (font *pdfCIDFontType0) ToPdfObject() PdfObject {
	if font.container == nil {
		font.container = &PdfIndirectObject{}
	}
	d := font.skeleton.toDict("CIDFontType2")
	font.container.PdfObject = d

	if font.CIDSystemInfo != nil {
		d.Set("CIDSystemInfo", font.CIDSystemInfo)
	}
	if font.DW != nil {
		d.Set("DW", font.DW)
	}
	if font.DW2 != nil {
		d.Set("DW2", font.DW2)
	}
	if font.W != nil {
		d.Set("W", font.W)
	}
	if font.W2 != nil {
		d.Set("W2", font.W2)
	}

	return font.container
}

// newPdfCIDFontType0FromPdfObject creates a pdfCIDFontType0 object from a dictionary (either direct
// or via indirect object). If a problem occurs with loading an error is returned.
func newPdfCIDFontType0FromPdfObject(obj PdfObject, skeleton *PdfFont) (*pdfCIDFontType0, error) {
	if skeleton.subtype != "CIDFontType0" {
		common.Log.Debug("ERROR: Font SubType != CIDFontType0. font=%s", skeleton)
		return nil, ErrRangeError
	}

	font := &pdfCIDFontType0{skeleton: skeleton}
	d := skeleton.dict

	// CIDSystemInfo.
	obj = TraceToDirectObject(d.Get("CIDSystemInfo"))
	if obj == nil {
		common.Log.Debug("CIDSystemInfo (Required) missing. font=%s", skeleton)
		return nil, ErrRequiredAttributeMissing
	}
	font.CIDSystemInfo = obj
	cidSystemInfo, err := cmap.NewCIDSystemInfo(obj)
	if err != nil {
		return nil, err
	}
	cidToRune, ok := cmap.GetPredefinedCidToRune(cidSystemInfo)
	if !ok {
		common.Log.Debug("Unkown CIDSystemInfo %s. font=%s", cidSystemInfo, skeleton)
		return nil, ErrRequiredAttributeMissing
	}
	font.cidToRune = cidToRune

	// Optional attributes.
	font.DW = TraceToDirectObject(d.Get("DW"))
	font.W = TraceToDirectObject(d.Get("W"))
	font.DW2 = TraceToDirectObject(d.Get("DW2"))
	font.W2 = TraceToDirectObject(d.Get("W2"))
	// font.CIDToGIDMap = d.Get("CIDToGIDMap")

	// d=[BaseFont CIDSystemInfo DW FontDescriptor Subtype Type W]
	fmt.Println("############################&&$$$$$$$$$$$$$$$$$$$$$$")
	fmt.Printf("d=%s\n", d.Keys())
	fmt.Printf(" CIDSystemInfo=%s\n", font.CIDSystemInfo)
	// fmt.Printf(" CIDSystemInfo=%#v\n", newCIDSystemInfo(font.CIDSystemInfo))
	fmt.Printf("   W=%s\n", font.W)
	fmt.Printf("  DW=%s\n", font.DW)
	fmt.Printf("skeleton=%s\n", skeleton)
	// fmt.Printf("font=%#v\n", font)

	return font, nil
}

// pdfCIDFontType2 represents a CIDFont Type2 font dictionary.
type pdfCIDFontType2 struct {
	container *PdfIndirectObject
	skeleton  *PdfFont // Elements common to all font types

	encoder   textencoding.TextEncoder
	ttfParser *fonts.TtfType

	CIDSystemInfo PdfObject
	DW            PdfObject
	W             PdfObject
	DW2           PdfObject
	W2            PdfObject
	CIDToGIDMap   PdfObject

	// Mapping between unicode runes to widths.
	runeToWidthMap map[uint16]int

	// Also mapping between GIDs (glyph index) and width.
	gidToWidthMap map[uint16]int
}

// Encoder returns the font's text encoder.
func (font pdfCIDFontType2) Encoder() textencoding.TextEncoder {
	return font.encoder
}

// SetEncoder sets the encoder for the truetype font.
func (font pdfCIDFontType2) SetEncoder(encoder textencoding.TextEncoder) {
	font.encoder = encoder
}

// GetGlyphCharMetrics returns the character metrics for the specified glyph.  A bool flag is
// returned to indicate whether or not the entry was found in the glyph to charcode mapping.
func (font pdfCIDFontType2) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	metrics := fonts.CharMetrics{}

	enc := textencoding.NewTrueTypeFontEncoder(font.ttfParser.Chars)

	// Convert the glyph to character code.
	rune, found := enc.GlyphToRune(glyph)
	if !found {
		common.Log.Debug("Unable to convert glyph %s to charcode (identity)", glyph)
		return metrics, false
	}

	w, found := font.runeToWidthMap[uint16(rune)]
	if !found {
		return metrics, false
	}
	metrics.GlyphName = glyph
	metrics.Wx = float64(w)

	return metrics, true
}

// ToPdfObject converts the pdfCIDFontType2 to a PDF representation.
func (font *pdfCIDFontType2) ToPdfObject() PdfObject {
	if font.container == nil {
		font.container = &PdfIndirectObject{}
	}
	d := font.skeleton.toDict("CIDFontType2")
	font.container.PdfObject = d

	if font.CIDSystemInfo != nil {
		d.Set("CIDSystemInfo", font.CIDSystemInfo)
	}
	if font.DW != nil {
		d.Set("DW", font.DW)
	}
	if font.DW2 != nil {
		d.Set("DW2", font.DW2)
	}
	if font.W != nil {
		d.Set("W", font.W)
	}
	if font.W2 != nil {
		d.Set("W2", font.W2)
	}
	if font.CIDToGIDMap != nil {
		d.Set("CIDToGIDMap", font.CIDToGIDMap)
	}

	return font.container
}

// newPdfCIDFontType2FromPdfObject creates a pdfCIDFontType2 object from a dictionary (either direct
// or via indirect object). If a problem occurs with loading an error is returned.
func newPdfCIDFontType2FromPdfObject(obj PdfObject, skeleton *PdfFont) (*pdfCIDFontType2, error) {
	if skeleton.subtype != "CIDFontType2" {
		common.Log.Debug("ERROR: Font SubType != CIDFontType2. font=%s", skeleton)
		return nil, ErrRangeError
	}

	font := &pdfCIDFontType2{skeleton: skeleton}
	d := skeleton.dict

	// CIDSystemInfo.
	obj = d.Get("CIDSystemInfo")
	if obj == nil {
		common.Log.Debug("ERROR: CIDSystemInfo (Required) missing. font=%s", skeleton)
		return nil, ErrRequiredAttributeMissing
	}
	font.CIDSystemInfo = obj

	// Optional attributes.
	font.DW = d.Get("DW")
	font.W = d.Get("W")
	font.DW2 = d.Get("DW2")
	font.W2 = d.Get("W2")
	font.CIDToGIDMap = d.Get("CIDToGIDMap")

	return font, nil
}

// NewCompositePdfFontFromTTFFile loads a composite font from a TTF font file. Composite fonts can
// be used to represent unicode fonts which can have multi-byte character codes, representing a wide
// range of values.
// It is represented by a Type0 Font with an underlying CIDFontType2 and an Identity-H encoding map.
// TODO: May be extended in the future to support a larger variety of CMaps and vertical fonts.
func NewCompositePdfFontFromTTFFile(filePath string) (*PdfFont, error) {
	// Load the truetype font data.
	ttf, err := fonts.TtfParse(filePath)
	if err != nil {
		common.Log.Debug("ERROR: while loading ttf font: %v", err)
		return nil, err
	}

	// Prepare the inner descendant font (CIDFontType2).
	skeleton := &PdfFont{}
	cidfont := &pdfCIDFontType2{skeleton: skeleton}
	cidfont.ttfParser = &ttf

	// 2-byte character codes -> runes
	runes := []uint16{}
	for r := range ttf.Chars {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool {
		return runes[i] < runes[j]
	})

	skeleton.BaseFont = MakeName(ttf.PostScriptName)

	k := 1000.0 / float64(ttf.UnitsPerEm)

	if len(ttf.Widths) <= 0 {
		return nil, errors.New("ERROR: Missing required attribute (Widths)")
	}

	missingWidth := k * float64(ttf.Widths[0])

	// Construct a rune -> width map.
	runeToWidthMap := map[uint16]int{}
	gidToWidthMap := map[uint16]int{}
	for _, r := range runes {
		glyphIndex := ttf.Chars[r]

		w := k * float64(ttf.Widths[glyphIndex])
		runeToWidthMap[r] = int(w)
		gidToWidthMap[glyphIndex] = int(w)
	}
	cidfont.runeToWidthMap = runeToWidthMap
	cidfont.gidToWidthMap = gidToWidthMap

	// Default width.
	cidfont.DW = MakeInteger(int64(missingWidth))

	// Construct W array.  Stores character code to width mappings.
	wArr := &PdfObjectArray{}
	i := uint16(0)
	for int(i) < len(runes) {

		j := i + 1
		for int(j) < len(runes) {
			if runeToWidthMap[runes[i]] != runeToWidthMap[runes[j]] {
				break
			}
			j++
		}

		// The W maps from CID to width, here CID = GID.
		gid1 := ttf.Chars[runes[i]]
		gid2 := ttf.Chars[runes[j-1]]

		wArr.Append(MakeInteger(int64(gid1)))
		wArr.Append(MakeInteger(int64(gid2)))
		wArr.Append(MakeInteger(int64(runeToWidthMap[runes[i]])))

		i = j
	}
	cidfont.W = MakeIndirectObject(wArr)

	// Use identity character id (CID) to glyph id (GID) mapping.
	cidfont.CIDToGIDMap = MakeName("Identity")

	d := MakeDict()
	d.Set("Ordering", MakeString("Identity"))
	d.Set("Registry", MakeString("Adobe"))
	d.Set("Supplement", MakeInteger(0))
	cidfont.CIDSystemInfo = d

	// Make the font descriptor.
	descriptor := &PdfFontDescriptor{}
	descriptor.Ascent = MakeFloat(k * float64(ttf.TypoAscender))
	descriptor.Descent = MakeFloat(k * float64(ttf.TypoDescender))
	descriptor.CapHeight = MakeFloat(k * float64(ttf.CapHeight))
	descriptor.FontBBox = MakeArrayFromFloats([]float64{k * float64(ttf.Xmin), k * float64(ttf.Ymin), k * float64(ttf.Xmax), k * float64(ttf.Ymax)})
	descriptor.ItalicAngle = MakeFloat(float64(ttf.ItalicAngle))
	descriptor.MissingWidth = MakeFloat(k * float64(ttf.Widths[0]))

	// Embed the TrueType font program.
	ttfBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		common.Log.Debug("ERROR: :Unable to read file contents: %v", err)
		return nil, err
	}

	stream, err := MakeStream(ttfBytes, NewFlateEncoder())
	if err != nil {
		common.Log.Debug("ERROR: Unable to make stream: %v", err)
		return nil, err
	}
	stream.PdfObjectDictionary.Set("Length1", MakeInteger(int64(len(ttfBytes))))
	descriptor.FontFile2 = stream

	if ttf.Bold {
		descriptor.StemV = MakeInteger(120)
	} else {
		descriptor.StemV = MakeInteger(70)
	}

	// Flags.
	//flags := 1 << 5 // Non-Symbolic.
	flags := uint32(0)
	if ttf.IsFixedPitch {
		flags |= 1
	}
	if ttf.ItalicAngle != 0 {
		flags |= 1 << 6
	}
	flags |= 1 << 2 // Symbolic.
	descriptor.Flags = MakeInteger(int64(flags))

	skeleton.fontDescriptor = descriptor

	// Make root Type0 font.
	type0 := pdfFontType0{
		skeleton:       &PdfFont{BaseFont: skeleton.BaseFont, basefont: skeleton.basefont},
		DescendantFont: &PdfFont{context: cidfont, subtype: "Type0"},
		Encoding:       MakeName("Identity-H"),
		encoder:        textencoding.NewTrueTypeFontEncoder(ttf.Chars),
	}

	// Build Font.
	font := PdfFont{context: &type0}

	return &font, nil
}
