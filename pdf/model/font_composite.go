package model

import (
	"errors"
	"io/ioutil"
	"sort"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model/fonts"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

// pdfFontType0 represents a Type0 font in PDF. Used for composite fonts which can encode multiple
// bytes for complex symbols (e.g. used in Asian languages). Represents the root font whereas the
// associated CIDFont is called its descendant.
type pdfFontType0 struct {
	container *PdfIndirectObject
	skeleton  *PdfFont

	encoder        textencoding.TextEncoder
	Encoding       PdfObject
	DescendantFont *PdfFont // Can be either CIDFontType0 or CIDFontType2 font.
	ToUnicode      PdfObject
}

// GetGlyphCharMetrics returns the character metrics for the specified glyph.  A bool flag is
// returned to indicate whether or not the entry was found in the glyph to charcode mapping.
func (font pdfFontType0) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	metrics := fonts.CharMetrics{}

	if font.DescendantFont == nil {
		return metrics, false
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
		common.Log.Debug("Invalid DescendantFonts - not an array (%T)", obj)
		return nil, ErrRangeError
	}
	if len(*arr) != 1 {
		common.Log.Debug("Array length != 1 (%d)", len(*arr))
		return nil, ErrRangeError
	}
	df, err := newPdfFontFromPdfObject((*arr)[0], false)
	if err != nil {
		common.Log.Debug("Failed loading descendant font: %v", err)
		return nil, err
	}

	font := &pdfFontType0{}
	font.DescendantFont = df

	// ToUnicode.
	obj = TraceToDirectObject(d.Get("ToUnicode"))
	font.ToUnicode = obj

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
		common.Log.Debug("Font SubType != CIDFontType2 (%s) font=%s", skeleton)
		return nil, ErrRangeError
	}

	font := &pdfCIDFontType2{}
	d := skeleton.dict

	// CIDSystemInfo.
	obj = d.Get("CIDSystemInfo")
	if obj == nil {
		common.Log.Debug("CIDSystemInfo (Required) missing")
		return nil, ErrRequiredAttributeMissing
	}
	font.CIDSystemInfo = obj

	// FontDescriptor.
	obj = d.Get("FontDescriptor")
	if obj == nil {
		common.Log.Debug("FontDescriptor (Required) missing")
		return nil, ErrRequiredAttributeMissing
	}

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
		common.Log.Debug("Error loading ttf font: %v", err)
		return nil, err
	}

	// Prepare the inner descendant font (CIDFontType2).
	skeleton := &PdfFont{}
	cidfont := &pdfCIDFontType2{skeleton: skeleton}
	cidfont.ttfParser = &ttf

	// 2-byte character codes. -> runes
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
		return nil, errors.New("Missing required attribute (Widths)")
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
		common.Log.Debug("Unable to read file contents: %v", err)
		return nil, err
	}

	stream, err := MakeStream(ttfBytes, NewFlateEncoder())
	if err != nil {
		common.Log.Debug("Unable to make stream: %v", err)
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
