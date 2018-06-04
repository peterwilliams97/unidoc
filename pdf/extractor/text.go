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
	"github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/cmap"
	"github.com/unidoc/unidoc/pdf/model"
)

// XYText represents text and its position on a page
type XYText struct {
	X, Y             float64
	ColorStroking    model.PdfColor // Colour that text is stroked with, if any
	ColorNonStroking model.PdfColor // Colour that text is filled with, if any
	Orient           contentstream.Orientation
	Text             string
}

// String returns a string describing `t`
func (t *XYText) String() string {
	return fmt.Sprintf("(%.1f,%.1f,%t,%q)", t.X, t.Y, t.Orient, t.Text)
}

// TextList is a list of text and its position on a pdf page
type TextList []XYText

// add appends the location and position of text to a text list
func (tl *TextList) add(x, y float64, cs, cf model.PdfColor, o contentstream.Orientation, text string) {
	*tl = append(*tl, XYText{x, y, cs, cf, o, text})
}

// ToText returns the contents of tl as a single string
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

func (tl *TextList) PageOrientation() contentstream.Orientation {
	landscapeCount := 0
	for _, t := range *tl {
		if t.Orient == contentstream.OrientationLandscape {
			landscapeCount++
		}
	}
	portraitCount := len(*tl) - landscapeCount
	// fmt.Printf("PageOrientation: landscape=%.1f%% = %d / %d \n",
	// 	float64(landscapeCount)/float64(len(*tl))*100.0, landscapeCount, len(*tl))
	if landscapeCount > portraitCount {
		return contentstream.OrientationLandscape
	}
	return contentstream.OrientationPortrait
}

// Transform transforms all points in `tl` by the affine transformation a, b, c, d, tx, ty
func (tl *TextList) Transform(a, b, c, d, tx, ty float64) {
	m := contentstream.NewMatrix(a, b, c, d, tx, ty)
	// fmt.Println("^^^^^^^^^^^$$$$$$$$$^^^^^^^^^^^^^^^^")
	for _, t := range *tl {
		t.X, t.Y = m.Transform(t.X, t.Y)
		// fmt.Printf("%4d: %s\n", i, t)
	}
	// fmt.Println("^^^^^^^^^^^#########^^^^^^^^^^^^^^^^^")
}

// ExtractText processes and extracts all text data in content streams and returns as a string. Takes into
// account character encoding via CMaps in the PDF file.
// The text is processed linearly e.g. in the order in which it appears. A best effort is done to add
// spaces and newlines.
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

	cstreamParser := contentstream.NewContentStreamParser(e.contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return textList, err
	}

	processor := contentstream.NewContentStreamProcessor(*operations)

	var codemap *cmap.CMap
	inText := false
	xPos, yPos := float64(-1), float64(-1)
	orientation := contentstream.OrientationLandscape

	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState,
			resources *model.PdfPageResources) error {

			addText := func(text string) {
				textList.add(xPos, yPos, gs.ColorStroking, gs.ColorNonStroking, orientation, text)
			}

			operand := op.Operand
			switch operand {
			case "BT":
				inText = true
			case "ET":
				inText = false
			case "Tf":
				if !inText {
					common.Log.Debug("Tf operand outside text")
					return nil
				}

				if len(op.Params) != 2 {
					common.Log.Debug("Error Tf should only get 2 input params, got %d", len(op.Params))
					return errors.New("Incorrect parameter count")
				}

				codemap = nil

				fontName, ok := op.Params[0].(*core.PdfObjectName)
				if !ok {
					common.Log.Debug("Error Tf font input not a name")
					return errors.New("Tf range error")
				}

				if resources == nil {
					return nil
				}

				fontObj, found := resources.GetFontByName(*fontName)
				if !found {
					common.Log.Debug("Font not found...")
					return errors.New("Font not in resources")
				}

				fontObj = core.TraceToDirectObject(fontObj)
				if fontDict, isDict := fontObj.(*core.PdfObjectDictionary); isDict {
					toUnicode := fontDict.Get("ToUnicode")
					if toUnicode != nil {
						toUnicode = core.TraceToDirectObject(toUnicode)
						toUnicodeStream, ok := toUnicode.(*core.PdfObjectStream)
						if !ok {
							return errors.New("Invalid ToUnicode entry - not a stream")
						}
						decoded, err := core.DecodeStream(toUnicodeStream)
						if err != nil {
							return err
						}

						codemap, err = cmap.LoadCmapFromData(decoded)
						if err != nil {
							return err
						}
					}
				}
			case "T*":
				if !inText {
					common.Log.Debug("T* operand outside text")
					return nil
				}
				addText("\n")
			case "Td", "TD":
				if !inText {
					common.Log.Debug("Td/TD operand outside text")
					return nil
				}

				// Params: [tx ty], corresponds to Tm=Tlm=[1 0 0;0 1 0;tx ty 1]*Tm
				if len(op.Params) != 2 {
					common.Log.Debug("Td/TD invalid arguments")
					return nil
				}
				tx, err := getNumberAsFloat(op.Params[0])
				if err != nil {
					common.Log.Debug("Td Float parse error")
					return nil
				}
				ty, err := getNumberAsFloat(op.Params[1])
				if err != nil {
					common.Log.Debug("Td Float parse error")
					return nil
				}

				if tx > 0 {
					addText(" ")
				}
				if ty < 0 {
					// TODO: More flexible space characters?
					addText("\n")
				}
			case "Tm":
				if !inText {
					common.Log.Debug("Tm operand outside text")
					return nil
				}

				// Params: a,b,c,d,e,f as in Tm = [a b 0; c d 0; e f 1].
				// The last two (e,f) represent translation.
				if len(op.Params) != 6 {
					return errors.New("Tm: Invalid number of inputs")
				}
				xfloat, ok := op.Params[4].(*core.PdfObjectFloat)
				if !ok {
					xint, ok := op.Params[4].(*core.PdfObjectInteger)
					if !ok {
						return nil
					}
					xfloat = core.MakeFloat(float64(*xint))
				}
				yfloat, ok := op.Params[5].(*core.PdfObjectFloat)
				if !ok {
					yint, ok := op.Params[5].(*core.PdfObjectInteger)
					if !ok {
						return nil
					}
					yfloat = core.MakeFloat(float64(*yint))
				}
				xx, yy := gs.Transform(float64(*xfloat), float64(*yfloat))
				xfloat, yfloat = core.MakeFloat(xx), core.MakeFloat(yy)
				orientation = gs.PageOrientation()
				if yPos == -1 {
					yPos = float64(*yfloat)
				} else if yPos > float64(*yfloat) {
					addText("\n")
					xPos = float64(*xfloat)
					yPos = float64(*yfloat)
					return nil
				}
				if xPos == -1 {
					xPos = float64(*xfloat)
				} else if xPos < float64(*xfloat) {
					addText("\t")
					xPos = float64(*xfloat)
				}
			case "TJ":
				if !inText {
					common.Log.Debug("TJ operand outside text")
					return nil
				}
				if len(op.Params) < 1 {
					return nil
				}
				paramList, ok := op.Params[0].(*core.PdfObjectArray)
				if !ok {
					return fmt.Errorf("Invalid parameter type, no array (%T)", op.Params[0])
				}
				for _, obj := range *paramList {
					switch v := obj.(type) {
					case *core.PdfObjectString:
						if codemap != nil {
							addText(codemap.CharcodeBytesToUnicode([]byte(*v)))
						} else {
							addText(string(*v))
						}
					case *core.PdfObjectFloat:
						if *v < -100 {
							addText(" ")
						}
					case *core.PdfObjectInteger:
						if *v < -100 {
							addText(" ")
						}
					}
				}
			case "Tj":
				if !inText {
					common.Log.Debug("Tj operand outside text")
					return nil
				}
				if len(op.Params) < 1 {
					return nil
				}
				param, ok := op.Params[0].(*core.PdfObjectString)
				if !ok {
					return fmt.Errorf("Invalid parameter type, not string (%T)", op.Params[0])
				}
				if codemap != nil {
					addText(codemap.CharcodeBytesToUnicode([]byte(*param)))
				} else {
					addText(string(*param))
				}
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
