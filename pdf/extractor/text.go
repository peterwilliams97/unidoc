/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/contentstream" // Import all? !@#$
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/cmap"
	"github.com/unidoc/unidoc/pdf/model"
	"github.com/unidoc/unidoc/pdf/model/fonts"
)

// ExtractText processes and extracts all text data in content streams and returns as a string.
// Takes into account character encoding via CMaps in the PDF file.
// The text is processed linearly e.g. in the order in which it appears. A best effort is done to
// add spaces and newlines.
func (e *Extractor) ExtractText() (string, error) {
	textList, err := e.ExtractXYText()
	if err != nil {
		return "", err
	}
	return textList.ToText(), nil
}

// ExtractXYText returns the text contents of `e` as a TextList.
func (e *Extractor) ExtractXYText() (*TextList, error) {
	textList := &TextList{}
	// var codemap *cmap.CMap
	state := newTextState()
	var to *TextObject

	cstreamParser := contentstream.NewContentStreamParser(e.contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return textList, err
	}
	processor := contentstream.NewContentStreamProcessor(*operations)

	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState,
			resources *model.PdfPageResources) error {

			operand := op.Operand
			// common.Log.Debug("++Operand: %s", op.String())

			switch operand {
			case "BT": // Begin text
				// Begin a text object, initializing the text matrix, Tm, and the text line matrix, Tlm,
				// to the identity matrix. Text objects shall not be nested; a second BT shall not appear
				// before an ET.
				if to != nil {
					common.Log.Debug("BT called while in a text object")
				}
				to = newTextObject(e, gs, &state)
			case "ET": // End Text
				*textList = append(*textList, to.Texts...)
				to = nil
			case "T*": // Move to start of next text line
				to.nextLine()
			case "Td": // Move text location
				if ok, err := checkOp(op, to, 2, true); !ok {
					return err
				}
				x, y, err := toFloatXY(op.Params)
				if err != nil {
					return err
				}
				to.moveText(x, y)
			case "TD": // Move text location and set leading
				if ok, err := checkOp(op, to, 2, true); !ok {
					return err
				}
				x, y, err := toFloatXY(op.Params)
				if err != nil {
					return err
				}
				to.moveTextSetLeading(x, y)
			case "Tj": // Show text
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				text, err := core.GetString(op.Params[0])
				if err != nil {
					return err
				}
				return to.showText(text)
			case "TJ": // Show text with adjustable spacing
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				args, err := core.GetArray(op.Params[0])
				if err != nil {
					return err
				}
				return to.showTextAdjusted(args)
			case "'": // Move to next line and show text
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				text, err := core.GetString(op.Params[0])
				if err != nil {
					return err
				}
				to.nextLine()
				return to.showText(text)
			case `"`: // Set word and character spacing, move to next line, and show text
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				x, y, err := toFloatXY(op.Params[:2])
				if err != nil {
					return err
				}
				text, err := core.GetString(op.Params[2])
				if err != nil {
					return err
				}
				to.setCharSpacing(x)
				to.setWordSpacing(y)
				to.nextLine()
				return to.showText(text)
			case "TL": // Set text leading
				ok, y, err := checkOpFloat(op, to)
				if !ok || err != nil {
					return err
				}
				to.setTextLeading(y)
			case "Tc": // Set character spacing
				ok, y, err := checkOpFloat(op, to)
				if !ok || err != nil {
					return err
				}
				to.setCharSpacing(y)
			case "Tf": // Set font
				if ok, err := checkOp(op, to, 2, true); !ok {
					return err
				}
				name, err := core.GetName(op.Params[0])
				if err != nil {
					return err
				}
				size, err := getNumberAsFloat(op.Params[1])
				if err != nil {
					return err
				}
				return to.setFont(name, size)
			case "Tm": // Set text matrix
				if ok, err := checkOp(op, to, 6, true); !ok {
					return err
				}
				floats, err := model.GetNumbersAsFloat(op.Params)
				if err != nil {
					return err
				}
				to.setTextMatrix(floats)
			case "Tr": // Set text rendering mode
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				mode, err := model.GetNumberAsInt(op.Params[0])
				if err != nil {
					return err
				}
				to.setTextRenderMode(mode)
			case "Ts": // Set text rise
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				y, err := getNumberAsFloat(op.Params[0])
				if err != nil {
					return err
				}
				to.setTextRise(y)
			case "Tw": // Set word spacing
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				y, err := getNumberAsFloat(op.Params[0])
				if err != nil {
					return err
				}
				to.setWordSpacing(y)
			case "Tz": // Set horizontal scaling
				if ok, err := checkOp(op, to, 1, true); !ok {
					return err
				}
				y, err := getNumberAsFloat(op.Params[0])
				if err != nil {
					return err
				}
				to.setHorizScaling(y)
			}

			return nil
		})

	err = processor.Process(e.resources)
	if err != nil {
		common.Log.Error("Error processing: %v", err)
		return textList, err
	}

	return textList, nil
}

//
// Text operators
//

// moveText "Td" Moves start of text by `tx`,`ty`
// Move to the start of the next line, offset from the start of the current line by (tx, ty).
// tx and ty are in unscaled text space units.
func (to *TextObject) moveText(tx, ty float64) {
	to.moveTo(tx, ty)
}

// moveTextSetLeading "TD" Move text location and set leading
// Move to the start of the next line, offset from the start of the current line by (tx, ty). As a
// side effect, this operator shall set the leading parameter in the text state. This operator shall
// have the same effect as this code:
//  −ty TL
//  tx ty Td
func (to *TextObject) moveTextSetLeading(tx, ty float64) {
	to.State.Tl = -ty
	to.moveTo(tx, ty)
}

// nextLine "T*"" Moves start of text `Line` to next text line
// Move to the start of the next line. This operator has the same effect as the code
//    0 -Tl Td
// where Tl denotes the current leading parameter in the text state. The negative of Tl is used
// here because Tl is the text leading expressed as a positive number. Going to the next line
// entails decreasing the y coordinate. (page 250)
func (to *TextObject) nextLine() {
	to.moveTo(0, -to.State.Tl)
}

// setTextMatrix "Tm"
// Set the text matrix, Tm, and the text line matrix, Tlm to the Matrix specified by the 6 numbers
// in `f`  (page 250)
func (to *TextObject) setTextMatrix(f []float64) {
	a, b, c, d, tx, ty := f[0], f[1], f[2], f[3], f[4], f[5]
	to.Tm = contentstream.NewMatrix(a, b, c, d, tx, ty)
	to.Tlm = contentstream.NewMatrix(a, b, c, d, tx, ty)
}

// showText "Tj" Show a text string
func (to *TextObject) showText(text string) error {
	to.renderText(text)
	return nil
}

// showTextAdjusted "TJ" Show text with adjustable spacing
func (to *TextObject) showTextAdjusted(args []core.PdfObject) error {
	vertical := false
	for _, o := range args {
		switch o.(type) {
		case *core.PdfObjectFloat, *core.PdfObjectInteger:
			x, err := getNumberAsFloat(o)
			if err != nil {
				return err
			}
			dx, dy := -x*0.001*to.State.Tfs, 0.0
			if vertical {
				dy, dx = dx, dy
			}
			to.Tm.Translate(dx, dy)
		case *core.PdfObjectString:
			text, err := core.GetString(o)
			if err != nil {
				common.Log.Debug("showTextAdjusted args=%+v err=%v", args, err)
				return err
			}
			to.renderText(text)
		default:
			common.Log.Debug("showTextAdjusted. Unexpected type args=%+v", args)
			return errors.New("typecheck")
		}
	}
	return nil
}

// setTextLeading "TL" Set text leading
func (to *TextObject) setTextLeading(y float64) {
	to.State.Tl = y
}

// setCharSpacing "Tc" Set character spacing
func (to *TextObject) setCharSpacing(x float64) {
	to.State.Tc = x
}

// setFont "Tf" Set font
func (to *TextObject) setFont(name string, size float64) error {
	font, err := to.getFont(name)
	if err != nil {
		return err
	}
	to.State.Tf = font
	to.State.Tfs = size
	// to.testFont(name)
	return nil
}

// setTextRenderMode "Tr" Set text rendering mode
func (to *TextObject) setTextRenderMode(mode int) {
	to.State.Tmode = RenderMode(mode)
}

// setTextRise "Ts" Set text rise
func (to *TextObject) setTextRise(y float64) {
	to.State.Trise = y
}

// setWordSpacing "Tw" Set word spacing
func (to *TextObject) setWordSpacing(y float64) {
	to.State.Tw = y
}

// setHorizScaling "Tz" Set horizontal scaling
func (to *TextObject) setHorizScaling(y float64) {
	to.State.Th = y
}

// Operator validation
func checkOpFloat(op *contentstream.ContentStreamOperation, to *TextObject) (ok bool, x float64, err error) {
	if ok, err = checkOp(op, to, 1, true); !ok {
		return
	}
	x, err = getNumberAsFloat(op.Params[0])
	return
}

// checkOp returns true if we are in a text stream and `op` has `numParams` params
// If `hard` is true and the number of params don't match then an error is returned
func checkOp(op *contentstream.ContentStreamOperation, to *TextObject, numParams int, hard bool) (ok bool, err error) {
	if to == nil {
		common.Log.Debug("%#q operand outside text", op.Operand)
		return
	}
	if numParams >= 0 {
		if len(op.Params) != numParams {
			if hard {
				err = errors.New("Incorrect parameter count")
			}
			common.Log.Debug("Error: %#q should have %d input params, got %d %+v",
				op.Operand, numParams, len(op.Params), op.Params)
			return
		}
	}
	ok = true
	return
}

// 9.3 Text State Parameters and Operators (page 243)
// Some of these parameters are expressed in unscaled text space units. This means that they shall
// be specified in a coordinate system that shall be defined by the text matrix, Tm but shall not be
// scaled by the font size parameter, Tfs.
type TextState struct {
	Tc    float64        // Character spacing. Unscaled text space units.
	Tw    float64        // Word spacing. Unscaled text space units.
	Th    float64        // Horizontal scaling
	Tl    float64        // Leading. Unscaled text space units. Used by TD,T*,'," see Table 108
	Tfs   float64        // Text font size
	Tmode RenderMode     // Text rendering mode
	Trise float64        // Text rise. Unscaled text space units. Set by Ts
	Tf    *model.PdfFont // Text font
	// Tk    bool                 // Text knockout. Not used for now
}

// 9.4.1 General (page 248)
// A PDF text object consists of operators that may show text strings, move the text position, and
// set text state and certain other parameters. In addition, two parameters may be specified only
// within a text object and shall not persist from one text object to the next:
//   •Tm, the text matrix
//   •Tlm, the text line matrix
//
// Text space is converted to device space by this transform (page 252)
//        | Tfs x Th   0      0 |
// Trm  = | 0         Tfs     0 | × Tm × CTM
//        | 0         Trise   1 |
//
type TextObject struct {
	e     *Extractor
	gs    contentstream.GraphicsState
	State *TextState
	Tm    contentstream.Matrix // Text matrix. For the character pointer.
	Tlm   contentstream.Matrix // Text line matrix. For the start of line pointer.
	Texts []XYText             // Text gets written here.
}

// newTextState returns a default TextState
func newTextState() TextState {
	return TextState{
		Th:    100,
		Tmode: RenderModeFill,
	}
}

// newTextObject returns a default TextObject
func newTextObject(e *Extractor, gs contentstream.GraphicsState, state *TextState) *TextObject {
	return &TextObject{
		e:     e,
		gs:    gs,
		State: state,
		Tm:    contentstream.IdentityMatrix(),
		Tlm:   contentstream.IdentityMatrix(),
	}
}

// renderText emits `text` to the calling program
// see 9.10.3, "ToUnicode CMaps"), whose value shall be a stream object containing a special
func (to *TextObject) renderText(text string) {
	text0 := text
	text = to.State.Tf.CharcodeBytesToUnicode([]byte(text))
	cp := to.getCp()
	fmt.Printf("renderText: %q->%q (%.1f,%.1f)\n", text0, text, cp.X, cp.Y)
	to.Texts = append(to.Texts, XYText{Point: cp, Text: text})

	// s := to.State
	// fontSize := s.Tfs
	// horizontalScaling := s.Th / 100.0
	// charSpacing := s.Tc
	// trm := contentstream.NewMatrix(s.Tfs*s.Th, 0, 0, s.Tfs, 0, s.Trise)
	// trm.Concat(to.Tm)
	// trm.Concat(to.gs.CTM)

	// encoder := to.State.Tf.GetEncoder()
	// for _, r := range text {
	// 	wordSpacing := 0.0
	// 	if r == ' ' {
	// 		wordSpacing = s.Tw
	// 	}
	// 	// TODO: Handle vertical text

	// 	glyph, found := encoder.RuneToGlyph(r)
	// 	if !found {
	// 		common.Log.Debug("Error! Glyph not found for rune=%s", r)
	// 		glyph = "space"
	// 	}

	// 	metrics, ok := to.State.Tf.GetGlyphCharMetrics(glyph)
	// 	if !ok {
	// 		common.Log.Debug("Error! Metrics not found for rune=%+v glyph=%#q", r, glyph)
	// 	}
	// 	showGlyph(trm, font, r, metrics)

	// }
}

// getCp returns the current text position in device coordinates
//        | Tfs x Th   0      0 |
// Trm  = | 0         Tfs     0 | × Tm × CTM
//        | 0         Trise   1 |
func (to *TextObject) getCp() Point {
	s := to.State
	m := contentstream.NewMatrix(s.Tfs*s.Th, 0, 0, s.Tfs, 0, s.Trise)
	m.Concat(to.Tm)
	m.Concat(to.gs.CTM)
	cp := Point{}
	x, y := m.Transform(cp.X, cp.Y)
	return Point{x, y}
}

// moveTo moves the start of line pointer by `tx`,`ty` and sets the text pointer to the
// start of line pointer
// Move to the start of the next line, offset from the start of the current line by (tx, ty).
// `tx` and `ty` are in unscaled text space units.
func (to *TextObject) moveTo(tx, ty float64) {
	to.Tlm.Concat(contentstream.NewMatrix(1, 0, 0, 1, tx, ty))
	to.Tm = to.Tlm
}

// Text list

// XYText represents text and its position in device coordinates
type XYText struct { // !@#$ Text
	Point
	ColorStroking    model.PdfColor // Colour that text is stroked with, if any
	ColorNonStroking model.PdfColor // Colour that text is filled with, if any
	Orient           contentstream.Orientation
	Text             string
}

// String returns a string describing `t`
func (t *XYText) String() string {
	return fmt.Sprintf("(%.1f,%.1f) stroke:%+v fill:%+v orient:%+v %q",
		t.X, t.Y, t.ColorStroking, t.ColorNonStroking, t.Orient, chomp(t.Text, 100))
}

// TextList is a list of texts and their position on a pdf page
type TextList []XYText

func (tl *TextList) Length() int {
	return len(*tl)
}

// AppendText appends the location and contents of `text` to a text list
func (tl *TextList) AppendText(gs contentstream.GraphicsState, p Point, text string) {
	t := XYText{p, gs.ColorStroking, gs.ColorNonStroking, gs.PageOrientation(), text}
	common.Log.Debug("AppendText: %s", t.String())
	*tl = append(*tl, t)
}

// ToText returns the contents of `tl` as a single string
func (tl *TextList) ToText() string {
	var buf bytes.Buffer
	for _, t := range *tl {
		// fmt.Printf("---- %4d: %4.1f %4.1f %q\n", i, t.X, t.Y, t.Text)
		buf.WriteString(t.Text)
	}
	procBuf(&buf)
	return buf.String()
}

// SortPosition sorts a text list by its elements position on a page. Top to bottom, left to right.
func (tl *TextList) SortPosition() {
	sort.SliceStable(*tl, func(i, j int) bool {
		ti, tj := (*tl)[i], (*tl)[j]
		if ti.Y != tj.Y {
			return ti.Y > tj.Y
		}
		return ti.X < tj.X
	})
}

// PageOrientation is a heuristic for the orientation of a page
// XXX: Use Page's Rotate flag instead 12#$
func (tl *TextList) PageOrientation() contentstream.Orientation {
	landscapeCount := 0
	for _, t := range *tl {
		if t.Orient == contentstream.OrientationLandscape {
			landscapeCount++
		}
	}
	portraitCount := len(*tl) - landscapeCount
	if landscapeCount > portraitCount {
		return contentstream.OrientationLandscape
	}
	return contentstream.OrientationPortrait
}

// Transform transforms all points in `tl` by the affine transformation a, b, c, d, tx, ty
func (tl *TextList) Transform(a, b, c, d, tx, ty float64) {
	m := contentstream.NewMatrix(a, b, c, d, tx, ty)
	for _, t := range *tl {
		t.X, t.Y = m.Transform(t.X, t.Y)
	}
}

func (to *TextObject) getCodemap(name string) (codemap *cmap.CMap, err error) {

	fontObj, err := to.getFontDict(name)
	if err != nil {
		return
	}
	fontDict := fontObj.(*core.PdfObjectDictionary)
	toUnicode := fontDict.Get("ToUnicode")
	toUnicode = core.TraceToDirectObject(toUnicode)
	if toUnicode == nil {
		return
	}
	toUnicodeStream, ok := toUnicode.(*core.PdfObjectStream)
	if !ok {
		err = errors.New("Invalid ToUnicode entry - not a stream")
		return
	}
	var decoded []byte
	decoded, err = core.DecodeStream(toUnicodeStream)
	if err != nil {
		return
	}
	codemap, err = cmap.LoadCmapFromData(decoded)
	return
}

// getFont returns the font named `name` if it exists in the page's resources
func (to *TextObject) getFont(name string) (*model.PdfFont, error) {
	fontObj, err := to.getFontDict(name)
	if err != nil {
		return nil, err
	}
	font, err := model.NewPdfFontFromPdfObject(fontObj)
	if err != nil {
		common.Log.Debug("getFont: NewPdfFontFromPdfObject failed. name=%#q err=%v", name, err)
	}
	return font, err
}

// getFontDict returns the font object called `name` if it exists in the page's Font resources or
// an error if it doesn't
func (to *TextObject) getFontDict(name string) (fontObj core.PdfObject, err error) {
	resources := to.e.resources
	if resources == nil {
		common.Log.Debug("getFontDict: No resouces")
		return
	}

	fontObj, found := resources.GetFontByName(core.PdfObjectName(name))
	if !found {
		err = errors.New("Font not in resources")
		common.Log.Debug("getFontDict: Font not found: name=%q err=%v", name, err)
		return
	}
	fontObj = core.TraceToDirectObject(fontObj)
	return
}

// getCharMetrics returns the character metrics for the code points in `text1` for font `font`
func getCharMetrics(font *model.PdfFont, text string) (metrics []fonts.CharMetrics, err error) {
	encoder := font.GetEncoder()
	if encoder == nil {
		err = errors.New("No font encoder")
	}
	for _, r := range text {
		glyph, found := encoder.RuneToGlyph(r)
		if !found {
			common.Log.Debug("Error! Glyph not found for rune=%s", r)
			glyph = "space"
		}
		m, ok := font.GetGlyphCharMetrics(glyph)
		if !ok {
			common.Log.Debug("Error! Metrics not found for rune=%+v glyph=%#q", r, glyph)
		}
		metrics = append(metrics, m)
	}
	return
}

// testing function:
// XXX move this to a go test
func (to *TextObject) testFont(name string) {
	font, err := to.getFont(name)
	if err != nil {
		common.Log.Debug("testFont: Font does not exist. name=%#q err=%v", name, err)
		return
	}
	text := "^`_-|WjāXYZabc123ABCDEFG"
	metrics, err := getCharMetrics(font, text)
	if err != nil {
		if err != nil {
			common.Log.Debug("testFont: getCharMetrics failed. name=%#q text=%q err=%v",
				name, text, err)
			return
		}
	}
	fmt.Println("=====================================")
	fmt.Printf("font=%#q text=%q\n", name, text)
	fmt.Printf("metrics=%+v\n", metrics)
}
