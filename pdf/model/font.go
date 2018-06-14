/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.

  createDefaultFont() throws IOException
    {
        COSDictionary dict = new COSDictionary();
        dict.setItem(COSName.TYPE, COSName.FONT);
        dict.setItem(COSName.SUBTYPE, COSName.TRUE_TYPE);
        dict.setString(COSName.BASE_FONT, "Arial");
        return createFont(dict);
    }

      // decode a character
            int before = in.available();
            int code = font.readCode(in);
            int codeLength = before - in.available();
            String unicode = font.toUnicode(code);
*/

package model

import (
	"errors"
	"fmt"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/cmap"
	"github.com/unidoc/unidoc/pdf/model/fonts"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

// PdfFont represents an underlying font structure which can be of type:
// - Type0
// - Type1
// - TrueType
// etc.
type PdfFont struct {
	context  fonts.Font // The underlying font: Type0, Type1, Truetype, etc..
	subtype  string
	basefont string

	BaseFont       PdfObject
	Subtype        PdfObject
	fontDescriptor *PdfFontDescriptor
}

func (font PdfFont) String() string {
	descriptor := ""
	if font.fontDescriptor != nil {
		descriptor = "(has descriptor)"
	}
	return fmt.Sprintf("%T %#q %#q %s", font.context, font.subtype, font.basefont, descriptor)
}

func (font PdfFont) CharcodeBytesToUnicode(codes []byte) string {
	if codemap := font.GetCMap(); codemap != nil {
		return codemap.CharcodeBytesToUnicode(codes)
	}
	if encoder := font.Encoder(); encoder != nil {
		runes := []rune{}
		for _, c := range codes {
			r, ok := encoder.CharcodeToRune(uint16(c))
			if !ok {
				common.Log.Debug("CharcodeBytesToUnicode: No rune. c=0x%04x font=%s", c, font)
				r = '?'
			}
			runes = append(runes, r)
		}
		return string(runes)
	}
	common.Log.Debug("CharcodeBytesToUnicode. Couldn't convert. Returning input bytes. font=%s", font)
	return string(codes)
}

func (font PdfFont) GetCMap() *cmap.CMap {
	switch t := font.context.(type) {
	case *pdfFontSimple:
		return t.CMap
		// default:
		// 	// common.Log.Debug("GetCMap. Not implemented for font type=%#T", font.context)
		// 	// XXX: Should we return a default encoding?
	}
	return nil
}

// Encoder returns the font's text encoder.
func (font PdfFont) Encoder() textencoding.TextEncoder {
	switch t := font.context.(type) {
	case fonts.FontCourier:
		return t.Encoder()
	case fonts.FontCourierBold:
		return t.Encoder()
	case fonts.FontCourierBoldOblique:
		return t.Encoder()
	case fonts.FontCourierOblique:
		return t.Encoder()
	case fonts.FontHelvetica:
		return t.Encoder()
	case fonts.FontHelveticaBold:
		return t.Encoder()
	case fonts.FontHelveticaBoldOblique:
		return t.Encoder()
	case fonts.FontHelveticaOblique:
		return t.Encoder()
	case fonts.FontTimesRoman:
		return t.Encoder()
	case fonts.FontTimesBold:
		return t.Encoder()
	case fonts.FontTimesBoldItalic:
		return t.Encoder()
	case fonts.FontTimesItalic:
		return t.Encoder()
	case fonts.FontSymbol:
		return t.Encoder()
	case fonts.FontZapfDingbats:
		return t.Encoder()
	case *pdfFontSimple:
		return t.Encoder()
	case *pdfFontType0:
		return t.Encoder()
	case *pdfCIDFontType2:
		return t.Encoder()
	}
	common.Log.Debug("Encoder. Not implemented for font type=%#T", font.context)
	// XXX: Should we return a default encoding?
	return nil
}

// SetEncoder sets the encoding for the underlying font.
// !@#$ Is this only possible for simple fonts?
func (font PdfFont) SetEncoder(encoder textencoding.TextEncoder) {
	switch t := font.context.(type) {
	case fonts.FontCourier:
		t.SetEncoder(encoder)
	case fonts.FontCourierBold:
		t.SetEncoder(encoder)
	case fonts.FontCourierBoldOblique:
		t.SetEncoder(encoder)
	case fonts.FontCourierOblique:
		t.SetEncoder(encoder)
	case fonts.FontHelvetica:
		t.SetEncoder(encoder)
	case fonts.FontHelveticaBold:
		t.SetEncoder(encoder)
	case fonts.FontHelveticaBoldOblique:
		t.SetEncoder(encoder)
	case fonts.FontHelveticaOblique:
		t.SetEncoder(encoder)
	case fonts.FontTimesRoman:
		t.SetEncoder(encoder)
	case fonts.FontTimesBold:
		t.SetEncoder(encoder)
	case fonts.FontTimesBoldItalic:
		t.SetEncoder(encoder)
	case fonts.FontTimesItalic:
		t.SetEncoder(encoder)
	case fonts.FontSymbol:
		t.SetEncoder(encoder)
	case fonts.FontZapfDingbats:
		t.SetEncoder(encoder)
	case *pdfFontSimple:
		t.SetEncoder(encoder)
	default:
		common.Log.Debug("SetEncoder. Not implemented for font type=%#T", font.context)
	}
}

// GetGlyphCharMetrics returns the specified char metrics for a specified glyph name.
func (font PdfFont) GetGlyphCharMetrics(glyph string) (fonts.CharMetrics, bool) {
	switch t := font.context.(type) {
	case *pdfFontSimple:
		return t.GetGlyphCharMetrics(glyph)
	case *pdfFontType0:
		return t.GetGlyphCharMetrics(glyph)
	case *pdfCIDFontType2:
		return t.GetGlyphCharMetrics(glyph)
	}
	common.Log.Debug("GetGlyphCharMetrics unsupported font type %T", font.context)

	return fonts.CharMetrics{}, false
}

// NewPdfFontFromPdfObject loads a PdfFont from a dictionary.  If there is a problem an error is
// returned.
func NewPdfFontFromPdfObject(fontObj PdfObject) (*PdfFont, error) {
	return newPdfFontFromPdfObject(fontObj, true)
}

// newPdfFontFromPdfObject loads a PdfFont from a dictionary.  If there is a problem an error is
// returned.
// The allowType0 indicates whether loading Type0 font should be supported.  Flag used to avoid
// cyclical loading.
func newPdfFontFromPdfObject(fontObj PdfObject, allowType0 bool) (*PdfFont, error) {
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

	basefont, err := GetName(d.Get("BaseFont"))
	if err == nil {
		font.basefont = basefont
		font.BaseFont = d.Get("BaseFont")
	}

	if obj := d.Get("Type"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || string(*oname) != "Font" {
			common.Log.Debug("Font Incompatibility ERROR: Type=%q Should be %q", string(*oname), "Font")
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
	subtype, err := GetName(TraceToDirectObject(obj))
	if err != nil {
		common.Log.Debug("Incompatibility ERROR: subtype not a name (%T) font=%s", obj, font)
		return nil, ErrTypeError
	}
	font.subtype = subtype

	switch subtype {
	case "Type0":
		if !allowType0 {
			common.Log.Debug("ERROR: Loading type0 not allowed. font=%s", font)
			return nil, errors.New("Cyclical type0 loading")
		}
		type0font, err := newPdfFontType0FromPdfObject(fontObj)
		if err != nil {
			common.Log.Debug("ERROR loading Type0 font: font=%s err=%v", font, err)
			return nil, err
		}
		font.context = type0font
	case "Type1", "Type3", "MMType1", "TrueType":
		if std, ok := fonts.Standard14Fonts[basefont]; ok && subtype == "Type1" {
			font.context = std
		} else {
			simplefont, err := newSimpleFontFromPdfObject(fontObj, font, d)
			if err != nil {
				common.Log.Debug("ERROR: loading simple font: font=%s err=%v", font, err)
				return nil, err
			}
			font.context = simplefont
		}
	case "CIDFontType0":
		common.Log.Debug("Unsupported font type: Subtype=%q *** font=%s", subtype, font)
		return nil, ErrUnsupportedFont
		// cidfont, err := newPdfFontType0FromPdfObject(fontObj)
		// if err != nil {
		// 	common.Log.Debug("Error loading cid font type0 font: %v", err)
		// 	return nil, err
		// }
		// font.context = cidfont
	case "CIDFontType2":
		cidfont, err := newPdfCIDFontType2FromPdfObject(fontObj)
		if err != nil {
			common.Log.Debug("ERROR: loading cid font type2 font. font=%s err=%v", font, err)
			return nil, err
		}
		font.context = cidfont
	default:
		common.Log.Debug("ERROR: Unsupported font type: Subtype=%q font=%s", subtype, font)
		return nil, ErrUnsupportedFont
	}

	obj = d.Get("FontDescriptor")
	if obj != nil {
		fontDescriptor, err := newPdfFontDescriptorFromPdfObject(obj)
		if err != nil {
			panic(err)
		}
		if err == nil {
			font.fontDescriptor = fontDescriptor
		}
	}
	return font, nil
}

// ToPdfObject converts the PdfFont object to its PDF representation.
func (font PdfFont) ToPdfObject() PdfObject {
	switch f := font.context.(type) {
	case *pdfFontSimple:
		return f.ToPdfObject()
	case *pdfFontType0:
		return f.ToPdfObject()
	case *pdfCIDFontType2:
		return f.ToPdfObject()
	}

	// If not supported, return null..
	common.Log.Debug("Unsupported font (%T) - returning null object", font.context)
	return MakeNull()
}

// toUnicodeToCmap returns a CMap of `toUnicode` if it exists
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

// PdfFontDescriptor specifies metrics and other attributes of a font and can refer to a FontFile
// for embedded fonts.
// 9.8 Font Descriptors (page 281)
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

// newPdfFontDescriptorFromPdfObject loads the font descriptor from a PdfObject.  Can either be a
// *PdfIndirectObject or a *PdfObjectDictionary.
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

	if obj := d.Get("FontName"); obj != nil {
		descriptor.FontName = obj
	} else {
		common.Log.Debug("Incompatibility: FontName (Required) missing")
	}
	fontname, _ := GetName(descriptor.FontName)

	if obj := d.Get("Type"); obj != nil {
		oname, is := obj.(*PdfObjectName)
		if !is || string(*oname) != "FontDescriptor" {
			common.Log.Debug("Incompatibility: Font descriptor Type invalid (%T) font=%q %T",
				obj, fontname, descriptor.FontName)
		}
	} else {
		common.Log.Trace("Incompatibility: Type (Required) missing. font=%q %T",
			fontname, descriptor.FontName)
		// return nil, errors.New("$$$$$")
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

// ToPdfObject returns the PdfFontDescriptor as a PDF dictionary inside an indirect object.
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
