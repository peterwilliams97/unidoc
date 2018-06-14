package model

import (
	"errors"
	"io/ioutil"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/cmap"
	"github.com/unidoc/unidoc/pdf/model/fonts"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

// pdfFontSimple describes a Simple Font
//
// 9.6 Simple Fonts (page 254)
// 9.6.1 General
// There are several types of simple fonts, all of which have these properties:
// • Glyphs in the font shall be selected by single-byte character codes obtained from a string that
//   is shown by the text-showing operators. Logically, these codes index into a table of 256 glyphs;
//   the mapping from codes to glyphs is called the font’s encoding. Under some circumstances, the
//   encoding may be altered by means described in 9.6.6, "Character Encoding".
// • Each glyph shall have a single set of metrics, including a horizontal displacement or width,
//   as described in 9.2.4, "Glyph Positioning and Metrics"; that is, simple fonts support only
//   horizontal writing mode.
// • Except for Type 0 fonts, Type 3 fonts in non-Tagged PDF documents, and certain standard Type 1
//   fonts, every font dictionary shall contain a subsidiary dictionary, the font descriptor,
//   containing font-wide metrics and other attributes of the font; see 9.8, "Font Descriptors".
//   Among those attributes is an optional font filestream containing the font program.
type pdfFontSimple struct {
	parent *PdfFont

	encoder    textencoding.TextEncoder
	firstChar  int
	lastChar   int
	charWidths []float64

	// Encoding is subject to limitations that are described in 9.6.6, "Character Encoding".
	// BaseFont is derived differently.
	FirstChar      PdfObject
	LastChar       PdfObject
	Widths         PdfObject
	FontDescriptor *PdfFontDescriptor
	Encoding       PdfObject
	ToUnicode      PdfObject

	CMap *cmap.CMap

	container *PdfIndirectObject
}

// Encoder returns the font's text encoder.
func (font *pdfFontSimple) Encoder() textencoding.TextEncoder {
	return font.encoder
}

// SetEncoder sets the encoding for the underlying font.
func (font *pdfFontSimple) SetEncoder(encoder textencoding.TextEncoder) {
	font.encoder = encoder
}

// GetGlyphCharMetrics returns the character metrics for the specified glyph.  A bool flag is
// returned to indicate whether or not the entry was found in the glyph to charcode mapping.
func (font pdfFontSimple) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	metrics := fonts.CharMetrics{}

	code, found := font.encoder.GlyphToCharcode(glyph)
	if !found {
		return metrics, false
	}
	metrics.GlyphName = glyph

	if int(code) < font.firstChar {
		common.Log.Debug("Code lower than firstchar (%d < %d)", code, font.firstChar)
		return metrics, false
	}

	if int(code) > font.lastChar {
		common.Log.Debug("Code higher than lastchar (%d < %d)", code, font.lastChar)
		return metrics, false
	}

	index := int(code) - font.firstChar
	if index >= len(font.charWidths) {
		common.Log.Debug("Code outside of widths range")
		return metrics, false
	}

	width := font.charWidths[index]
	metrics.Wx = width

	return metrics, true
}

// newSimpleFontFromPdfObject creates a pdfFontSimple from a dictionary. An error is returned
// if there is a problem with loading.
// !@#$ Just return a base 14 font, if obj is a base 14 font
//
// The value of Encoding is subject to limitations that are described in 9.6.6, "Character Encoding".
// • The value of BaseFont is derived differently.
//
// !@#$ 9.6.6.4 Encodings for TrueType Fonts (page 265)
//      Need to get TrueType font's cmap
func newSimpleFontFromPdfObject(obj PdfObject, parent *PdfFont, d *PdfObjectDictionary) (*pdfFontSimple, error) {
	font := &pdfFontSimple{parent: parent}

	// !@#$ Failing on ~/testdata/The-Byzantine-Generals-Problem.pdf
	obj = d.Get("FirstChar")
	if obj == nil {
		if parent.subtype == "TrueType" {
			common.Log.Debug("ERROR: FirstChar attribute missing")
			return nil, ErrRequiredAttributeMissing
		}
		obj = PdfObject(MakeInteger(0))
	}
	font.FirstChar = obj

	intVal, ok := TraceToDirectObject(obj).(*PdfObjectInteger)
	if !ok {
		common.Log.Debug("Invalid FirstChar type (%T)", obj)
		return nil, ErrTypeError
	}
	font.firstChar = int(*intVal)

	obj = d.Get("LastChar")
	if obj == nil {
		if parent.subtype == "TrueType" {
			common.Log.Debug("ERROR: LastChar attribute missing")
			return nil, ErrRequiredAttributeMissing
		}
		obj = PdfObject(MakeInteger(0))
	}
	font.LastChar = obj
	intVal, ok = TraceToDirectObject(obj).(*PdfObjectInteger)
	if !ok {
		common.Log.Debug("Invalid LastChar type (%T)", obj)
		return nil, ErrTypeError
	}
	font.lastChar = int(*intVal)

	font.charWidths = []float64{}
	obj = d.Get("Widths")
	if obj == nil {
		common.Log.Debug("Widths missing from font")
		return nil, ErrRequiredAttributeMissing
	}
	font.Widths = obj

	arr, ok := TraceToDirectObject(obj).(*PdfObjectArray)
	if !ok {
		common.Log.Debug("Widths attribute != array (%T)", arr)
		return nil, ErrTypeError
	}

	widths, err := arr.ToFloat64Array()
	if err != nil {
		common.Log.Debug("Error converting widths to array")
		return nil, err
	}

	if len(widths) != (font.lastChar - font.firstChar + 1) {
		common.Log.Debug("Invalid widths length != %d (%d)", font.lastChar-font.firstChar+1, len(widths))
		return nil, ErrRangeError
	}

	font.charWidths = widths

	if obj := d.Get("FontDescriptor"); obj != nil {
		descriptor, err := newPdfFontDescriptorFromPdfObject(obj)
		if err != nil {
			common.Log.Debug("Error loading font descriptor: %v", err)
			return nil, err
		}
		font.FontDescriptor = descriptor
	}
	// }

	font.Encoding = TraceToDirectObject(d.Get("Encoding"))
	font.ToUnicode = TraceToDirectObject(d.Get("ToUnicode"))

	// if f, ok := fonts.Standard14Fonts[basefont]; ok && subtype == "Type1" {
	// 	font.SetEncoder(f.Encoder())
	// } else {

	baseEncoder, differences, err := getFontEncoding(TraceToDirectObject(font.Encoding))
	if err != nil {
		common.Log.Debug("Error: BaseFont=%q Subtype=%q Encoding=%s (%T) err=%v", parent.basefont,
			parent.subtype, font.Encoding, font.Encoding, err)
		return nil, err
	}
	encoder, err := textencoding.NewSimpleTextEncoder(baseEncoder, differences)
	if err != nil {
		return nil, err
	}
	font.SetEncoder(encoder)

	if font.ToUnicode != nil {
		codemap, err := toUnicodeToCmap(font.ToUnicode)
		if err != nil {
			return nil, err
		}
		font.CMap = codemap
	}
	return font, nil
}

// getFontEncoding returns font encoding of `obj` the "Encoding" entry in a font dict
// Table 114 – Entries in an encoding dictionary (page 263)
// 9.6.6.1 General (page 262)
// A font’s encoding is the association between character codes (obtained from text strings that
// are shown) and glyph descriptions. This sub-clause describes the character encoding scheme used
// with simple PDF fonts. Composite fonts (Type 0) use a different character mapping algorithm, as
// discussed in 9.7, "Composite Fonts".
// Except for Type 3 fonts, every font program shall have a built-in encoding. Under certain
// circumstances, a PDF font dictionary may change the encoding used with the font program to match
// the requirements of the conforming writer generating the text being shown.
func getFontEncoding(obj PdfObject) (string, map[byte]string, error) {
	baseName := "StandardEncoding"

	if obj == nil {
		// common.Log.Debug("Incompatibility ERROR: Font Encoding (Required) missing")
		return baseName, nil, nil
		return "", nil, ErrRequiredAttributeMissing
	}

	switch encoding := obj.(type) {
	case *PdfObjectName:
		return string(*encoding), nil, nil
	case *PdfObjectDictionary:
		typ, err := GetName(TraceToDirectObject(encoding.Get("Type")))
		if err == nil && typ == "Encoding" {
			// common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. Type=%q!=%q err=%v",
			// 	typ, "Encoding", err)
			// return "", nil, ErrTypeError

			base, err := GetName(TraceToDirectObject(encoding.Get("BaseEncoding")))
			if err == nil {
				baseName = base
			}
			//  common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. BaseEncoding=%q (%T) err=%v",
			// 	baseName, encoding.Get("BaseEncoding"), err)
			// baseName = "StandardEncoding"
			// return "", nil, ErrTypeError
			// }
		}
		diffList, err := GetArray(TraceToDirectObject(encoding.Get("Differences")))
		if err != nil {
			common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. %+v err=%v", encoding, err)
			return "", nil, ErrTypeError
		}

		differences, err := textencoding.FromFontDifferences(diffList)
		return baseName, differences, err
	default:
		common.Log.Debug("Incompatibility ERROR: encoding not a name or dict (%T) %s",
			obj, obj.String())
		return "", nil, ErrTypeError
	}
}

// ToPdfObject converts the pdfFontTrueType to its PDF representation for outputting.
func (this *pdfFontSimple) ToPdfObject() PdfObject {
	if this.container == nil {
		this.container = &PdfIndirectObject{}
	}
	d := MakeDict()
	this.container.PdfObject = d

	d.Set("Type", MakeName("Font"))
	d.Set("Subtype", this.parent.Subtype)

	if this.parent.BaseFont != nil {
		d.Set("BaseFont", this.parent.BaseFont)
	}
	if this.FirstChar != nil {
		d.Set("FirstChar", this.FirstChar)
	}
	if this.LastChar != nil {
		d.Set("LastChar", this.LastChar)
	}
	if this.Widths != nil {
		d.Set("Widths", this.Widths)
	}
	if this.FontDescriptor != nil {
		d.Set("FontDescriptor", this.FontDescriptor.ToPdfObject())
	}
	if this.Encoding != nil {
		d.Set("Encoding", this.Encoding)
	}
	if this.ToUnicode != nil {
		d.Set("ToUnicode", this.ToUnicode)
	}

	return this.container
}

// NewPdfFontFromTTFFile loads a TTF font and returns a PdfFont type that can be used in text
// styling functions.
// Uses a WinAnsiTextEncoder and loads only character codes 32-255.
func NewPdfFontFromTTFFile(filePath string) (*PdfFont, error) {
	ttf, err := fonts.TtfParse(filePath)
	if err != nil {
		common.Log.Debug("Error loading ttf font: %v", err)
		return nil, err
	}

	truefont := &pdfFontSimple{}

	// TODO: Make more generic to allow customization... Need to know which glyphs are to be used,
	// then can derive
	// TODO: needed encoding via a BaseEncoding and a Differences entry if needed.
	// TODO: Subsetting fonts.
	truefont.encoder = textencoding.NewWinAnsiTextEncoder()
	truefont.firstChar = 32
	truefont.lastChar = 255

	truefont.parent.BaseFont = MakeName(ttf.PostScriptName)
	truefont.FirstChar = MakeInteger(32)
	truefont.LastChar = MakeInteger(255)

	k := 1000.0 / float64(ttf.UnitsPerEm)
	if len(ttf.Widths) <= 0 {
		return nil, errors.New("Missing required attribute (Widths)")
	}

	missingWidth := k * float64(ttf.Widths[0])
	vals := []float64{}

	for charcode := 32; charcode <= 255; charcode++ {
		runeVal, found := truefont.Encoder().CharcodeToRune(uint16(charcode))
		if !found {
			common.Log.Debug("Rune not found (charcode: %d)", charcode)
			vals = append(vals, missingWidth)
			continue
		}

		pos, ok := ttf.Chars[uint16(runeVal)]
		if !ok {
			common.Log.Debug("Rune not in TTF Chars")
			vals = append(vals, missingWidth)
			continue
		}

		w := k * float64(ttf.Widths[pos])

		vals = append(vals, w)
	}

	truefont.Widths = &PdfIndirectObject{PdfObject: MakeArrayFromFloats(vals)}

	if len(vals) < (255 - 32 + 1) {
		common.Log.Debug("Invalid length of widths, %d < %d", len(vals), 255-32+1)
		return nil, errors.New("Range check error")
	}

	truefont.charWidths = vals[:255-32+1]

	// Use WinAnsiEncoding by default.
	truefont.Encoding = MakeName("WinAnsiEncoding")

	descriptor := &PdfFontDescriptor{}
	descriptor.Ascent = MakeFloat(k * float64(ttf.TypoAscender))
	descriptor.Descent = MakeFloat(k * float64(ttf.TypoDescender))
	descriptor.CapHeight = MakeFloat(k * float64(ttf.CapHeight))
	descriptor.FontBBox = MakeArrayFromFloats([]float64{k * float64(ttf.Xmin), k * float64(ttf.Ymin), k * float64(ttf.Xmax), k * float64(ttf.Ymax)})
	descriptor.ItalicAngle = MakeFloat(float64(ttf.ItalicAngle))
	descriptor.MissingWidth = MakeFloat(k * float64(ttf.Widths[0]))

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
	flags := 1 << 5
	if ttf.IsFixedPitch {
		flags |= 1
	}
	if ttf.ItalicAngle != 0 {
		flags |= 1 << 6
	}
	descriptor.Flags = MakeInteger(int64(flags))

	// Build Font.
	truefont.FontDescriptor = descriptor

	font := &PdfFont{}
	font.context = truefont

	return font, nil
}
