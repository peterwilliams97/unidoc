/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

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

// The PdfFont structure represents an underlying font structure which can be of type:
// - Type0
// - Type1
// - TrueType
// etc.
type PdfFont struct {
	context interface{} // The underlying font: Type0, Type1, Truetype, etc..
}

// Set the encoding for the underlying font.
func (font PdfFont) SetEncoder(encoder textencoding.TextEncoder) {
	switch t := font.context.(type) {
	case *pdfFontTrueType:
		t.SetEncoder(encoder)
	default:
		common.Log.Debug("SetEncoder. Not implemented for font type=%#T", font.context)
	}
}

func (font PdfFont) GetEncoder() textencoding.TextEncoder {
	switch t := font.context.(type) {
	case *pdfFontTrueType:
		return t.Encoder
	default:
		common.Log.Debug("GetEncoder. Not implemented for font type=%#T", font.context)
		// XXX: Should we return a default encoding?
	}
	return nil
}

func (font PdfFont) CharcodeBytesToUnicode(codes []byte) string {
	if codemap := font.GetCMap(); codemap != nil {
		return codemap.CharcodeBytesToUnicode(codes)
	} else if encoder := font.GetEncoder(); encoder != nil {
		runes := []rune{}
		for _, c := range codes {
			r, ok := encoder.CharcodeToRune(c)
			if !ok {
				r = '?'
			}
			runes = append(runes, r)
		}
		return string(runes)
	}
	common.Log.Debug("CharcodeBytesToUnicode. Couldn't covert. Returning input bytes")

	return string(codes)
}

func (font PdfFont) GetCMap() *cmap.CMap {
	switch t := font.context.(type) {
	case *pdfFontTrueType:
		return t.CMap
	default:
		common.Log.Debug("GetCMap. Not implemented for font type=%#T", font.context)
		// XXX: Should we return a default encoding?
	}
	return nil
}

func (font PdfFont) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	switch t := font.context.(type) {
	case *pdfFontTrueType:
		return t.GetGlyphCharMetrics(glyph)
	default:
		common.Log.Debug("GetGlyphCharMetrics. Not implemented for font type=%#T", font.context)
	}

	return fonts.CharMetrics{}, false
}

func NewPdfFontFromPdfObject(fontObj PdfObject) (*PdfFont, error) {
	font := &PdfFont{}

	dictObj := fontObj
	if ind, is := fontObj.(*PdfIndirectObject); is {
		dictObj = ind.PdfObject
	}

	d, ok := dictObj.(*PdfObjectDictionary)
	if !ok {
		common.Log.Debug("Font not given by a dictionary (%T)", fontObj)
		return nil, ErrTypeError
	}

	if obj := d.Get("Type"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || string(*oname) != "Font" {
			common.Log.Debug("Incompatibility ERROR: Type (Required) defined but not Font name")
			return nil, ErrRangeError
		}
	} else {
		common.Log.Debug("Incompatibility ERROR: Type (Required) missing")
		return nil, ErrRequiredAttributeMissing
	}

	obj := d.Get("Subtype")
	if obj == nil {
		common.Log.Debug("Incompatibility ERROR: Subtype (Required) missing")
		return nil, ErrRequiredAttributeMissing
	}

	subtype, ok := TraceToDirectObject(obj).(*PdfObjectName)
	if !ok {
		common.Log.Debug("Incompatibility ERROR: subtype not a name (%T) ", obj)
		return nil, ErrTypeError
	}

	switch subtype.String() {
	case "TrueType":
		truefont, err := newPdfFontTrueTypeFromPdfObject(fontObj)
		if err != nil {
			common.Log.Debug("Error loading truetype font: %v", truefont)
			return nil, err
		}

		font.context = truefont
	default:
		common.Log.Debug("Unsupported font type: %s", subtype.String())
		return nil, errors.New("Unsupported font type")
	}

	obj = d.Get("Encoding")
	baseEncoder, differences, err := getFontEncoding(obj)
	if err != nil {
		panic(err)
		return nil, err
	}
	encoder, err := textencoding.NewSimpleTextEncoder(baseEncoder, differences)
	if err != nil {
		return nil, err
	}
	font.SetEncoder(encoder)
	return font, nil
}

// getFontEncoding returns font encoding of `obj` the "Encoding" entry in a font dict
// Table 114 – Entries in an encoding dictionary (page 263)
func getFontEncoding(obj PdfObject) (string, map[byte]string, error) {
	if obj == nil {
		common.Log.Debug("Incompatibility ERROR: Font Encoding (Required) missing")
		return "", nil, ErrRequiredAttributeMissing
	}
	obj = TraceToDirectObject(obj)

	switch encoding := obj.(type) {
	case *PdfObjectName:
		return string(*encoding), nil, nil
	case *PdfObjectDictionary:
		typ, err := GetName(encoding.Get("Type"))
		if err != nil || typ != "Encoding" {
			common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. %+v Type=%q err=%v",
				encoding, typ, err)
			return "", nil, ErrTypeError
		}
		baseName, err := GetName(encoding.Get("BaseEncoding"))
		if err != nil {
			common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. %+v BaseEncoding=%q err=%v",
				encoding, baseName, err)
			return "", nil, ErrTypeError
		}
		diffList, err := GetArray(encoding.Get("Differences"))
		if err != nil {
			common.Log.Debug("Incompatibility ERROR: Bad font encoding dict. %+v err=%v", encoding, err)
			return "", nil, ErrTypeError
		}

		differences, err := textencoding.FromFontDifferences(diffList)
		return baseName, differences, err
	default:
		common.Log.Debug("Incompatibility ERROR: encoding not a name or dict (%T) ", obj)
		return "", nil, ErrTypeError
	}
}

func (font PdfFont) ToPdfObject() PdfObject {
	switch f := font.context.(type) {
	case *pdfFontTrueType:
		return f.ToPdfObject()
	}

	// If not supported, return null..
	common.Log.Debug("Unsupported font (%T) - returning null object", font.context)
	return MakeNull()
}

type pdfFontTrueType struct {
	Encoder textencoding.TextEncoder

	firstChar  int
	lastChar   int
	charWidths []float64

	// Subtype shall be TrueType.
	// Encoding is subject to limitations that are described in 9.6.6, "Character Encoding".
	// BaseFont is derived differently.
	BaseFont       PdfObject
	FirstChar      PdfObject
	LastChar       PdfObject
	Widths         PdfObject
	FontDescriptor *PdfFontDescriptor
	Encoding       PdfObject
	ToUnicode      PdfObject
	CMap           *cmap.CMap

	container *PdfIndirectObject
}

func (font *pdfFontTrueType) SetEncoder(encoder textencoding.TextEncoder) {
	font.Encoder = encoder
}

func (font pdfFontTrueType) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	metrics := fonts.CharMetrics{}

	code, found := font.Encoder.GlyphToCharcode(glyph)
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

func newPdfFontTrueTypeFromPdfObject(obj PdfObject) (*pdfFontTrueType, error) {
	font := &pdfFontTrueType{}

	if ind, is := obj.(*PdfIndirectObject); is {
		font.container = ind
		obj = ind.PdfObject
	}

	d, ok := obj.(*PdfObjectDictionary)
	if !ok {
		common.Log.Debug("Font object invalid, not a dictionary (%T)", obj)
		return nil, ErrTypeError
	}

	if obj := d.Get("Type"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || oname.String() != "Font" {
			common.Log.Debug("Incompatibility: Type defined but not Font")
		}
	}

	if obj := d.Get("Subtype"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || oname.String() != "TrueType" {
			common.Log.Debug("Incompatibility: Loading TrueType font but Subtype != TrueType")
		}
	}

	font.BaseFont = d.Get("BaseFont")

	if obj := d.Get("FirstChar"); obj != nil {
		font.FirstChar = obj

		intVal, ok := TraceToDirectObject(obj).(*PdfObjectInteger)
		if !ok {
			common.Log.Debug("Invalid FirstChar type (%T)", obj)
			return nil, ErrTypeError
		}
		font.firstChar = int(*intVal)
	} else {
		common.Log.Debug("ERROR: FirstChar attribute missing")
		return nil, ErrRequiredAttributeMissing
	}

	if obj := d.Get("LastChar"); obj != nil {
		font.LastChar = obj

		intVal, ok := TraceToDirectObject(obj).(*PdfObjectInteger)
		if !ok {
			common.Log.Debug("Invalid LastChar type (%T)", obj)
			return nil, ErrTypeError
		}
		font.lastChar = int(*intVal)
	} else {
		common.Log.Debug("ERROR: FirstChar attribute missing")
		return nil, ErrRequiredAttributeMissing
	}

	font.charWidths = []float64{}
	if obj := d.Get("Widths"); obj != nil {
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
	} else {
		common.Log.Debug("Widths missing from font")
		return nil, ErrRequiredAttributeMissing
	}

	if obj := d.Get("FontDescriptor"); obj != nil {
		descriptor, err := newPdfFontDescriptorFromPdfObject(obj)
		if err != nil {
			common.Log.Debug("Error loading font descriptor: %v", err)
			return nil, err
		}
		font.FontDescriptor = descriptor
	}

	font.Encoding = TraceToDirectObject(d.Get("Encoding"))
	font.ToUnicode = TraceToDirectObject(d.Get("ToUnicode"))

	if font.ToUnicode != nil {
		codemap, err := toUnicodeToCmap(font.ToUnicode)
		if err != nil {
			return nil, err
		}
		font.CMap = codemap
	}
	return font, nil
}

func toUnicodeToCmap(toUnicode PdfObject) (*cmap.CMap, error) {
	toUnicodeStream, ok := toUnicode.(*PdfObjectStream)
	if !ok {
		common.Log.Debug("toUnicodeToCmap: Not a stream (%T)", toUnicode)
		return nil, errors.New("Invalid ToUnicode entry - not a stream")
	}
	decoded, err := DecodeStream(toUnicodeStream)
	if err != nil {
		return nil, err
	}
	return cmap.LoadCmapFromData(decoded)
}

func (this *pdfFontTrueType) ToPdfObject() PdfObject {
	if this.container == nil {
		this.container = &PdfIndirectObject{}
	}
	d := MakeDict()
	this.container.PdfObject = d

	d.Set("Type", MakeName("Font"))
	d.Set("Subtype", MakeName("TrueType"))

	if this.BaseFont != nil {
		d.Set("BaseFont", this.BaseFont)
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

func NewPdfFontFromTTFFile(filePath string) (*PdfFont, error) {
	ttf, err := fonts.TtfParse(filePath)
	if err != nil {
		common.Log.Debug("Error loading ttf font: %v", err)
		return nil, err
	}

	truefont := &pdfFontTrueType{}

	truefont.Encoder = textencoding.NewWinAnsiTextEncoder()
	truefont.firstChar = 32
	truefont.lastChar = 255

	truefont.BaseFont = MakeName(ttf.PostScriptName)
	truefont.FirstChar = MakeInteger(32)
	truefont.LastChar = MakeInteger(255)

	k := 1000.0 / float64(ttf.UnitsPerEm)

	if len(ttf.Widths) <= 0 {
		return nil, errors.New("Missing required attribute (Widths)")
	}

	missingWidth := k * float64(ttf.Widths[0])
	vals := []float64{}

	for charcode := 32; charcode <= 255; charcode++ {
		runeVal, found := truefont.Encoder.CharcodeToRune(byte(charcode))
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
		return nil, ErrRangeError
	}

	truefont.charWidths = vals[:255-32+1]

	// Default.
	// XXX/FIXME TODO: Only use the encoder object.

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

	// XXX/TODO: Encode the file...
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

// Font descriptors specifies metrics and other attributes of a font.
type PdfFontDescriptor struct {
	FontName     PdfObject
	FontFamily   PdfObject
	FontStretch  PdfObject
	FontWeight   PdfObject
	Flags        PdfObject
	FontBBox     PdfObject
	ItalicAngle  PdfObject
	Ascent       PdfObject
	Descent      PdfObject
	Leading      PdfObject
	CapHeight    PdfObject
	XHeight      PdfObject
	StemV        PdfObject
	StemH        PdfObject
	AvgWidth     PdfObject
	MaxWidth     PdfObject
	MissingWidth PdfObject
	FontFile     PdfObject
	FontFile2    PdfObject
	FontFile3    PdfObject
	CharSet      PdfObject

	// Additional entries for CIDFonts
	Style  PdfObject
	Lang   PdfObject
	FD     PdfObject
	CIDSet PdfObject

	// Container.
	container *PdfIndirectObject
}

// Load the font descriptor from a PdfObject.  Can either be a *PdfIndirectObject or
// a *PdfObjectDictionary.
func newPdfFontDescriptorFromPdfObject(obj PdfObject) (*PdfFontDescriptor, error) {
	descriptor := &PdfFontDescriptor{}

	if ind, is := obj.(*PdfIndirectObject); is {
		descriptor.container = ind
		obj = ind.PdfObject
	}

	d, ok := obj.(*PdfObjectDictionary)
	if !ok {
		common.Log.Debug("FontDescriptor not given by a dictionary (%T)", obj)
		return nil, ErrTypeError
	}

	if obj := d.Get("Type"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || string(*oname) != "FontDescriptor" {
			common.Log.Debug("Incompatibility: Font descriptor Type invalid (%T)", obj)
		}
	} else {
		common.Log.Debug("Incompatibility: Type (Required) missing")
	}

	if obj := d.Get("FontName"); obj != nil {
		descriptor.FontName = obj
	} else {
		common.Log.Debug("Incompatibility: FontName (Required) missing")
	}

	descriptor.FontFamily = d.Get("FontFamily")
	descriptor.FontStretch = d.Get("FontStretch")
	descriptor.FontWeight = d.Get("FontWeight")
	descriptor.Flags = d.Get("Flags")
	descriptor.FontBBox = d.Get("FontBBox")
	descriptor.ItalicAngle = d.Get("ItalicAngle")
	descriptor.Ascent = d.Get("Ascent")
	descriptor.Descent = d.Get("Descent")
	descriptor.Leading = d.Get("Leading")
	descriptor.CapHeight = d.Get("CapHeight")
	descriptor.XHeight = d.Get("XHeight")
	descriptor.StemV = d.Get("StemV")
	descriptor.StemH = d.Get("StemH")
	descriptor.AvgWidth = d.Get("AvgWidth")
	descriptor.MaxWidth = d.Get("MaxWidth")
	descriptor.MissingWidth = d.Get("MissingWidth")
	descriptor.FontFile = d.Get("FontFile")
	descriptor.FontFile2 = d.Get("FontFile2")
	descriptor.FontFile3 = d.Get("FontFile3")
	descriptor.CharSet = d.Get("CharSet")
	descriptor.Style = d.Get("Style")
	descriptor.Lang = d.Get("Lang")
	descriptor.FD = d.Get("FD")
	descriptor.CIDSet = d.Get("CIDSet")

	return descriptor, nil
}

// Convert to a PDF dictionary inside an indirect object.
func (this *PdfFontDescriptor) ToPdfObject() PdfObject {
	d := MakeDict()
	if this.container == nil {
		this.container = &PdfIndirectObject{}
	}
	this.container.PdfObject = d

	d.Set("Type", MakeName("FontDescriptor"))

	if this.FontName != nil {
		d.Set("FontName", this.FontName)
	}

	if this.FontFamily != nil {
		d.Set("FontFamily", this.FontFamily)
	}

	if this.FontStretch != nil {
		d.Set("FontStretch", this.FontStretch)
	}

	if this.FontWeight != nil {
		d.Set("FontWeight", this.FontWeight)
	}

	if this.Flags != nil {
		d.Set("Flags", this.Flags)
	}

	if this.FontBBox != nil {
		d.Set("FontBBox", this.FontBBox)
	}

	if this.ItalicAngle != nil {
		d.Set("ItalicAngle", this.ItalicAngle)
	}

	if this.Ascent != nil {
		d.Set("Ascent", this.Ascent)
	}

	if this.Descent != nil {
		d.Set("Descent", this.Descent)
	}

	if this.Leading != nil {
		d.Set("Leading", this.Leading)
	}

	if this.CapHeight != nil {
		d.Set("CapHeight", this.CapHeight)
	}

	if this.XHeight != nil {
		d.Set("XHeight", this.XHeight)
	}

	if this.StemV != nil {
		d.Set("StemV", this.StemV)
	}

	if this.StemH != nil {
		d.Set("StemH", this.StemH)
	}

	if this.AvgWidth != nil {
		d.Set("AvgWidth", this.AvgWidth)
	}

	if this.MaxWidth != nil {
		d.Set("MaxWidth", this.MaxWidth)
	}

	if this.MissingWidth != nil {
		d.Set("MissingWidth", this.MissingWidth)
	}

	if this.FontFile != nil {
		d.Set("FontFile", this.FontFile)
	}

	if this.FontFile2 != nil {
		d.Set("FontFile2", this.FontFile2)
	}

	if this.FontFile3 != nil {
		d.Set("FontFile3", this.FontFile3)
	}

	if this.CharSet != nil {
		d.Set("CharSet", this.CharSet)
	}

	if this.Style != nil {
		d.Set("FontName", this.FontName)
	}

	if this.Lang != nil {
		d.Set("Lang", this.Lang)
	}

	if this.FD != nil {
		d.Set("FD", this.FD)
	}

	if this.CIDSet != nil {
		d.Set("CIDSet", this.CIDSet)
	}

	return this.container
}
