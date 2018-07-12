package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model/textencoding"
)

type fontFile struct {
	name    string
	subtype string
	encoder textencoding.TextEncoder
	// binary  []byte
}

func (fontfile *fontFile) String() string {
	encoding := "[None]"
	if fontfile.encoder != nil {
		encoding = fontfile.encoder.String()
	}
	return fmt.Sprintf("FONTFILE{%#q encoder=%s}", fontfile.name, encoding)
}

// newFontFileFromPdfObject loads a FontFile from a PdfObject.  Can either be a
// *PdfIndirectObject or a *PdfObjectDictionary.
func newFontFileFromPdfObject(obj PdfObject) (*fontFile, error) {
	common.Log.Debug("newFontFileFromPdfObject: obj=%s", obj)
	fontfile := &fontFile{}

	obj = TraceToDirectObject(obj)

	streamObj, ok := obj.(*PdfObjectStream)
	if !ok {
		common.Log.Debug("ERROR: FontFile must be a stream (%T)", obj)
		return nil, ErrTypeError
	}
	d := streamObj.PdfObjectDictionary
	data, err := DecodeStream(streamObj)
	if err != nil {
		return nil, err
	}

	subtype, err := GetName(TraceToDirectObject(d.Get("Subtype")))
	if err != nil {
		fontfile.subtype = subtype
		if subtype == "Type1C" {
			// XXX: TODO Add Type1C support
			common.Log.Debug("Type1C fonts are currently not supported")
			return nil, ErrFontNotSupported
		}
	}

	length1 := int(*(TraceToDirectObject(d.Get("Length1")).(*PdfObjectInteger)))
	length2 := int(*(TraceToDirectObject(d.Get("Length2")).(*PdfObjectInteger)))
	if length1 > len(data) {
		length1 = len(data)
	}
	if length1+length2 > len(data) {
		length2 = len(data) - length1
	}

	segment1 := data[:length1]
	segment2 := []byte{}
	if length2 > 0 {
		segment2 = data[length1 : length1+length2]
	}

	// empty streams are  ignored
	if length1 > 0 && length2 > 0 {
		err := fontfile.loadFromSegments(segment1, segment2)
		if err != nil {
			return nil, err
		}
	}

	common.Log.Debug("fontfile=%s", fontfile)
	return fontfile, nil
}

// loadFromSegments loads a Type1Font object from two header-less .pfb segments.
// Based on pdfbox
func (fontfile *fontFile) loadFromSegments(segment1, segment2 []byte) error {
	common.Log.Debug("loadFromSegments: %d %d", len(segment1), len(segment2))
	err := fontfile.parseAsciiPart(segment1)
	if err != nil {
		common.Log.Debug("err=%v", err)
		return err
	}
	common.Log.Debug("fontfile=%s", fontfile)
	if len(segment2) == 0 {
		return nil
	}
	// err = fontfile.parseEexecPart(segment2)
	// if err != nil {
	// 	common.Log.Debug("err=%v", err)
	// 	return err
	// }

	common.Log.Debug("fontfile=%s", fontfile)
	return nil
}

// parseAsciiPart parses the ASCII part of the FontFile.
func (fontfile *fontFile) parseAsciiPart(data []byte) error {
	common.Log.Debug("parseAsciiPart: %d ", len(data))
	// fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~^^^~~~~~~~~~~~~~~~~~~~~~~~")
	// fmt.Printf("data=%s\n", string(data))
	// fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~!!!~~~~~~~~~~~~~~~~~~~~~~~")

	// The start of a FontFile looks like
	//     %!PS-AdobeFont-1.0: MyArial 003.002
	//     %%Title: MyArial
	// or
	//     %!FontType1-1.0
	if len(data) < 2 || string(data[:2]) != "%!" {
		return errors.New("Invalid start of ASCII segment")
	}

	keySection, encodingSection, err := getAsciiSections(data)
	if err != nil {
		common.Log.Debug("err=%v", err)
		return err
	}
	keyValues := getKeyValues(keySection)

	fontfile.name = keyValues["FontName"]
	if fontfile.name == "" {
		common.Log.Debug("ERROR: FontFile has no /FontName")
		return ErrRequiredAttributeMissing
	}

	// encodingName, ok := keyValues["Encoding"]
	// !@#$ I am not sure why we don't do this
	// if ok  {
	// 	encoder, err := textencoding.NewSimpleTextEncoder(encodingName, nil)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	fontfile.encoder = encoder
	// }
	if encodingSection != "" {
		encodings, err := getEncodings(encodingSection)
		if err != nil {
			return err
		}
		encoder, err := textencoding.NewEmbeddedSimpleTextEncoder(encodings, nil)
		if err != nil {
			// XXX: !@#$ We need to fix all these errors
			common.Log.Error("UNKNOWN GLYPH: err=%v", err)
			return nil
		}
		fontfile.encoder = encoder
	}
	return nil
}

// // parseEexecPart parses the binary encrypted part of the FontFile.
// func (fontfile *fontFile) parseEexecPart(data []byte) error {
// 	// Sometimes, fonts use  hex format
// 	if !isBinary(data) {
// 		decoded, err := hex.DecodeString(string(data))
// 		if err != nil {
// 			return err
// 		}
// 		data = decoded
// 	}
// 	decoded := decodeEexec(data)
// 	fmt.Println(":::::::::::::::::::::<<>>:::::::::::::::::::::")
// 	fmt.Printf("%s\n", string(decoded))
// 	fmt.Println(":::::::::::::::::::::<><>:::::::::::::::::::::")
// 	return nil
// }

var (
	reDictBegin   = regexp.MustCompile(`\d+ dict\s+(dup\s+)?begin`)
	reKeyVal      = regexp.MustCompile(`^\s*/(\S+?)\s+(.+?)\s+def\s*$`)
	reEncoding    = regexp.MustCompile(`dup\s+(\d+)\s*/(\w+)\s+put`)
	encodingBegin = "/Encoding 256 array"
	encodingEnd   = "readonly def"
	binaryStart   = "currentfile eexec"
)

// getAsciiSections returns two sections of `data`, the ASCII part of the FontFile
//   - the general key values in `keySection`
//   - the encoding in `encodingSection`
func getAsciiSections(data []byte) (keySection, encodingSection string, err error) {
	common.Log.Debug("getAsciiSections: %d ", len(data))
	loc := reDictBegin.FindIndex(data)
	if loc == nil {
		err = ErrTypeError
		common.Log.Debug("getAsciiSections: No dict.")
		return
	}
	i0 := loc[1]
	i := strings.Index(string(data[i0:]), encodingBegin)
	if i < 0 {
		keySection = string(data[i0:])
		return
	}
	i1 := i0 + i
	keySection = string(data[i0:i1])

	i2 := i1
	i = strings.Index(string(data[i2:]), encodingEnd)
	if i < 0 {
		err = ErrTypeError
		common.Log.Debug("err=%v", err)
		return
	}
	i3 := i2 + i
	encodingSection = string(data[i2:i3])
	return
}

// /Users/pcadmin/testdata/invoice61781040.pdf has \r line endings
var reEndline = regexp.MustCompile(`[\n\r]+`)

// getKeyValues returns the map encoded in `data`.
func getKeyValues(data string) map[string]string {
	// lines := strings.Split(data, "\n")
	lines := reEndline.Split(data, -1)
	keyValues := map[string]string{}
	for _, line := range lines {
		matches := reKeyVal.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		k, v := matches[1], matches[2]
		keyValues[k] = v
	}
	return keyValues
}

// getEncodings returns the encodings encoded in `data`.
func getEncodings(data string) (map[uint16]string, error) {
	lines := strings.Split(data, "\n")
	keyValues := map[uint16]string{}
	for _, line := range lines {
		matches := reEncoding.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		k, glyph := matches[1], matches[2]
		code, err := strconv.Atoi(k)
		if err != nil {
			common.Log.Debug("ERROR: Bad encoding line. %q", line)
			return nil, ErrTypeCheck
		}
		// if !textencoding.KnownGlyph(glyph) {
		// 	common.Log.Debug("ERROR: Unknown glyph %q. line=%q", glyph, line)
		// 	return nil, ErrTypeCheck
		// }
		keyValues[uint16(code)] = glyph
	}
	common.Log.Debug("getEncodings: keyValues=%#v", keyValues)
	return keyValues, nil
}

// decodeEexec returns the decoding of the eexec bytes `data`
func decodeEexec(data []byte) []byte {
	const c1 = 52845
	const c2 = 22719

	seed := 55665 // eexec key
	// Run the seed through the encoder 4 times
	for _, b := range data[:4] {
		seed = (int(b)+seed)*c1 + c2
	}
	decoded := make([]byte, len(data)-4)
	for i, b := range data[4:] {
		decoded[i] = byte(int(b) ^ seed>>8)
		seed = (int(b)+seed)*c1 + c2
	}
	return decoded
}

// isBinary returns true if `data` is binary. See Adobe Type 1 Font Format specification
// 7.2 eexec encryption
func isBinary(data []byte) bool {
	if len(data) < 4 {
		return true
	}
	for b := range data[:4] {
		r := rune(b)
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// truncate returns the first `n` characters in string `s`
func truncate(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
