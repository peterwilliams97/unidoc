package model

import (
	"errors"
	"testing"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
)

func init() {
	common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
}

var simpleFontDicts = []string{
	`<< /Type /Font
		/BaseFont /Helvetica
		/Subtype /Type1
		/Encoding /WinAnsiEncoding
		>>`,
	`<< /Type /Font
		/BaseFont /Helvetica-Oblique
		/Subtype /Type1
		/Encoding /WinAnsiEncoding
		>>`,
	`<< /Type /Font
		/Subtype /Type1
		/FirstChar 71
		/LastChar 79
		/Widths [ 778 722 278 500 667 556 833 722 778 ]
		/Encoding /WinAnsiEncoding
		/BaseFont /AOMFKK+Helvetica
		>>`,
	`<< /Type /Font
		/Subtype /Type1
		/FirstChar 71
		/LastChar 79
		/Widths [ 778 722 278 500 667 556 833 722 778 ]
		/Encoding /WinAnsiEncoding
		/BaseFont /PETER+Helvetica
		/FontDescriptor <<
			/Type /FontDescriptor
			/Ascent 718
			/CapHeight 718
			/Descent -207
			/Flags 32
			/FontBBox [ -166 -225 1000 931 ]
			/FontName /PETER+Helvetica
			/ItalicAngle 0
			/StemV 88
			/XHeight 523
			/StemH 88
			/CharSet (/G/O)
			%/FontFile3 19 0 R
			>>
		>>`,
}

/*
    ~/testdata/3GABP.pdf

    15 0 obj
    <</Subtype/Type1/BaseFont/Times-Roman/Type/Font/Name/R15/FontDescriptor 14 0 R/FirstChar 0/LastChar 255/Widths[
    760 333 333 333 278 556 556 167 333 611 278 564 333 333 611 444
    250 250 250 250 250 250 250 250 250 250 250 250 250 250 250 250
    250 333 408 500 500 833 778 180 333 333 500 564 250 333 250 278
    500 500 500 500 500 500 500 500 500 500 278 278 564 564 564 444
    921 722 667 667 722 611 556 722 722 333 389 722 611 889 722 722
    556 722 667 556 611 722 722 944 722 722 611 333 278 333 469 500
    333 444 500 444 500 444 333 500 500 278 278 500 278 778 500 500
    500 500 333 389 278 500 500 722 500 500 444 480 200 480 541 250
    500 250 333 500 444 1000 500 500 333 1000 556 333 889 250 250 250
    250 333 333 444 444 350 500 1000 333 980 389 333 722 250 250 722
    250 333 500 500 500 500 200 500 333 760 276 500 564 250 760 333
    400 564 300 300 333 500 453 250 333 300 310 500 750 750 750 444
    722 722 722 722 722 722 889 667 611 611 611 611 333 333 333 333
    722 722 722 722 722 722 722 564 722 722 722 722 722 722 556 500
    444 444 444 444 444 444 667 444 444 444 444 444 278 278 278 278
    500 500 500 500 500 500 500 564 500 500 500 500 500 500 500 500]
    /Encoding 147 0 R>>
    endobj

    147 0 obj
    <</Type/Encoding/Differences[
    0/copyright
    39/quotesingle
    133/ellipsis
    146/quoteright/quotedblleft/quotedblright
    150/endash]>>
    endobj

    14 0 obj
    <</Type/FontDescriptor/FontName/Times-Roman/FontBBox[-168 -281 1000 924]/Flags 34
    /Ascent 924
    /CapHeight 676
    /Descent -281
    /ItalicAngle 0
    /StemV 111
    /MissingWidth 250
    /XHeight 461
    /FontFile3 13 0 R>>
    endobj

    ~/testdata/DepLing2017invited.pdf

    85 0 obj
    << /Type /Font /Subtype /Type1 /BaseFont /EZZEEU+NimbusSanL-Regu /FontDescriptor
    3353 0 R /Encoding /MacRomanEncoding /FirstChar 44 /LastChar 222 /Widths [
    278 333 0 0 0 0 556 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
    0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 556 556 500 556 556 278 556 556 222
    222 500 222 833 556 556 556 0 333 500 278 556 500 722 500 500 500 0 0 0 0
    0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
    0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0
    0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 500 ] >>
    endobj

    3353 0 obj
    << /Type /FontDescriptor /FontName /EZZEEU+NimbusSanL-Regu /Flags 32 /FontBBox
    [-205 -316 1032 984] /ItalicAngle 0 /Ascent 953 /Descent -285 /CapHeight 747
    /StemV 85 /XHeight 544 /StemH 78 /MaxWidth 1237 /FontFile 3354 0 R >>
    endobj

    ~/testdata/TRPB_1_1171727696.pdf

    31 0 obj <<
    /Type /Font
    /Subtype /Type1
    /Name /G1F34
    /BaseFont /HelveticaNeue-LightExtObl
    /Encoding /WinAnsiEncoding
    /FirstChar 0
    /LastChar 255
    /Widths [501 501 501 501 501 501 501 501 501 501 501 501 501 501 501 501
     501 501 501 501 501 501 501 501 501 501 501 501 501 501 501 501
     277 295 407 667 667 962 777 277 352 352 425 600 277 443 277 407
     667 443 667 667 667 667 667 592 667 667 277 277 600 600 600 647
     799 722 741 814 777 685 610 852 795 222 556 741 592 980 795 852
     704 852 741 667 629 777 685 1056 667 686 647 352 407 352 600 500
     184 610 667 610 667 647 314 667 629 184 184 574 184 1000 629 667
     667 667 370 556 352 629 556 889 574 556 556 352 222 352 600 501
     501 501 277 667 407 1000 667 667 184 1167 667 314 1167 501 647 501
     501 277 277 407 407 501 500 1000 184 990 556 314 1092 501 556 686
     277 295 667 667 667 667 222 667 184 799 396 500 600 443 799 184
     399 600 434 434 184 629 600 277 184 289 434 500 1000 1000 1000 647
     722 722 722 722 722 722 1092 814 685 685 685 685 222 222 222 222
     777 795 852 852 852 852 852 600 852 777 777 777 777 686 704 629
     610 610 610 610 610 610 1074 610 647 647 647 647 184 184 184 184
     667 629 667 667 667 667 667 600 667 629 629 629 629 556 667 556]
    /FontDescriptor 32 0 R
    >> endobj

    32 0 obj <<
    /Type /FontDescriptor
    /Ascent 779
    /CapHeight 714
    /Descent -220
    /FontBBox [-159 -220 1161 953]
    /FontName /HelveticaNeue-LightExtObl
    /Flags 32
    /ItalicAngle -120
    /StemV 0
    /FontFile 33 0 R
    >> endobj

    ~/testdata/2012060560369753.pdf
    45 0 obj
    <<
    /Type /Font
    /Subtype /Type1
    /FirstChar 32
    /LastChar 181
    /Widths [ 278 278 444 556 556 1000 648 278 278 278 370 600 278 389 278 352
    556 556 556 556 556 556 556 556 556 556 278 278 600 600 600 556
    800 667 704 722 722 630 593 759 722 278 537 685 574 889 722 760
    667 760 704 648 593 722 611 944 648 648 630 296 352 296 600 500
    241 556 611 556 611 556 315 593 574 241 241 537 241 870 574 593
    611 611 352 519 333 574 519 778 537 519 500 296 222 296 600 500
    500 500 0 0 0 0 0 0 0 0 0 0 0 500 500 500 500 0 278 0 0 500 500
    0 0 0 0 0 0 500 500 0 278 0 556 556 0 0 0 0 241 800 0 0 0 389 0
    0 0 600 0 0 0 574 ]
    /Encoding /WinAnsiEncoding
    /BaseFont /KCJDKB+HelveticaNeue-Medium
    /FontDescriptor 47 0 R
    >>
    endobj

    47 0 obj
    <<
    /Type /FontDescriptor
    /Ascent 714
    /CapHeight 714
    /Descent -191
    /Flags 262176
    /FontBBox [ -165 -221 1066 952 ]
    /FontName /KCJDKB+HelveticaNeue-Medium
    /ItalicAngle 0
    /StemV 114
    /XHeight 517
    /CharSet (/T/bullet/B/r/six/V/b/d/C/F/s/seven/W/c/E/D/comma/t/eight/X/e/P/G/hyphen\
    /u/nine/f/Y/I/x/period/v/colon/h/J/w/H/bracketleft/i/endash/L/slash/semi\
    colon/y/dieresis/zero/g/l/M/j/ampersand/A/one/z/k/O/bracketright/two/m/q\
    uoteright/Q/three/o/parenleft/R/a/question/n/four/p/S/parenright/K/N/at/\
    five/q/U)
    /FontFile3 78 0 R
    >>
    endobj

     ~/testdata/pdl.pd
    20 0 obj
    <</BaseFont/Helvetica-Oblique/Encoding/WinAnsiEncoding/Subtype/Type1/Type/Font>>
    endobj
    21 0 obj
    <</BaseFont/Helvetica-Bold/Encoding/WinAnsiEncoding/Subtype/Type1/Type/Font>>
    endobj

    ~/testdata/shamirturing.pdf
    34 0 obj
    <<
    /Type /Font
    /Subtype /TrueType
    /Name /F1
    /BaseFont /TimesNewRoman
    /Encoding /WinAnsiEncoding
    >>
    endobj

     ~/testdata/crypto/B02.pdf
     17 0 obj
    <<
    /StemH 0
    /CapHeight 1005
    /FontFile2 16 0 R
    /Type/FontDescriptor
    /Flags 4
    /Descent -219
    /Ascent 1005
    /FontName/VCZCOB++Symbol
    /FontBBox[0 -219 1113 1004]
    /StemV 0
    /ItalicAngle 0
    /XHeight 0
    >>
    endobj
    18 0 obj
    <<
    /Widths[631 549 0 0 549 0 0 247 384 384 384 384 384 384]
    /FontDescriptor 17 0 R
    /BaseFont/VCZCOB++Symbol
    /Type/Font
    /FirstChar 1
    /Subtype/TrueType
    /Name/F7
    /LastChar 14
    >>
    endobj
*/

var compositeFontDicts = []string{
	`<< /Type /Font
		/Subtype /Type0
		/Encoding /Identity-H
		/DescendantFonts [<<
			/Type /Font
			/Subtype /CIDFontType2
			/BaseFont /FLDOLC+PingFangSC-Regular
			/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >>
			/W [ ]
			/DW 1000
			/FontDescriptor <<
				/Type /FontDescriptor
				/FontName /FLDOLC+PingFangSC-Regular
				/Flags 4
				/FontBBox [-123 -263 1177 1003]
				/ItalicAngle 0
				/Ascent 972
				/Descent -232
				/CapHeight 864
				/StemV 70
				/XHeight 648
				/StemH 64
				/AvgWidth 1000
				/MaxWidth 1300
				% /FontFile3 182 0 R
				>>
			>>]
		/BaseFont /FLDOLC+PingFangSC-Regular
		>>`,
}

// TestSimpleFonts checks that we correctly recreate simple fonts that we parse.
func TestSimpleFonts(t *testing.T) {
	for _, d := range simpleFontDicts {
		objFontObj(t, d)
	}
}

// TestCompositeFonts checks that we correctly recreate composite fonts that we parse.
func TestCompositeFonts(t *testing.T) {
	for _, d := range compositeFontDicts {
		objFontObj(t, d)
	}
}

// objFontObj parses `fontDict` to a make a Font, creates a PDF object from the Font and checks that
// the new PDF object is the same as the input object
func objFontObj(t *testing.T, fontDict string) error {

	parser := NewParserFromString(fontDict)
	obj, err := parser.ParseDict()
	if err != nil {
		t.Errorf("objFontObj: Failed to parse dict obj. fontDict=%q err=%v", fontDict, err)
		return err
	}
	font, err := NewPdfFontFromPdfObject(obj)
	if err != nil {
		t.Errorf("Failed to parse font object. obj=%s err=%v", obj, err)
		return err
	}

	// Resolve all the indirect references in the font objects so we can compare their contents.
	obj1 := FlattenObject(obj)
	obj2 := FlattenObject(font.ToPdfObject())

	// Check that the reconstituted font is the same as the original.
	if !EqualObjects(obj1, obj2) {
		t.Errorf("Different objects.\nobj1=%s\nobj2=%s\nfont=%s", obj1, obj2, font)
		return errors.New("different objects")
	}

	return nil
}
