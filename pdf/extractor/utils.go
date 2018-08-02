/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/common/license"
	"github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
)

// The text rendering mode, Tmode, determines whether showing text shall cause glyph outlines to be
// stroked, filled, used as a clipping boundary, or some combination of the three. Stroking,
// filling, and clipping shall have the same effects for a text object as they do for a path object
// (see 8.5.3, "Path-Painting Operators" and 8.5.4, "Clipping Path Operators"),
type RenderMode int

const (
	RenderModeStroke RenderMode = 1 << iota
	RenderModeFill
	RenderModeClip
)

func toPageCoords(gs contentstream.GraphicsState, objs []core.PdfObject) (Point, error) {
	x, y, err := toFloatXY(objs)
	if err != nil {
		return Point{}, err
	}
	return toPagePoint(gs, x, y), nil
}

func toPagePointList(gs contentstream.GraphicsState, objs []core.PdfObject) (points []Point, err error) {
	if len(objs)%2 != 0 {
		err = fmt.Errorf("Invalid number of params: %d", len(objs))
		common.Log.Debug("toPagePointList: err=%v", err)
		return
	}
	floats, err := toFloatList(objs)
	if err != nil {
		return
	}
	for i := 0; i <= len(floats)-1; i += 2 {
		x, y := floats[i], floats[i+1]
		points = append(points, toPagePoint(gs, x, y))
	}
	return
}

func toPagePoint(gs contentstream.GraphicsState, x, y float64) Point {
	xp, yp := gs.Transform(x, y)
	p := Point{xp, yp}
	// fmt.Printf("      toPagePoint(%5.1f,%5.1f) -> %s (%s)\n", x, y, p.String(), gs.CTM.String())
	return p
}

// toFloatList returns `objs` as 2 floats, if that's what it is, or an error if it isn't
func toFloatXY(objs []core.PdfObject) (x, y float64, err error) {
	if len(objs) != 2 {
		return 0, 0, fmt.Errorf("Invalid number of params: %d", len(objs))
	}
	floats, err := core.GetNumbersAsFloat(objs)
	if err != nil {
		return 0, 0, err
	}
	return floats[0], floats[1], nil
}

func toFloatList(objs []core.PdfObject) ([]float64, error) {
	return core.GetNumbersAsFloat(objs)
	floats := []float64{}
	for _, o := range objs {
		x, err := getNumberAsFloat(o)
		if err != nil {
			return nil, err
		}
		floats = append(floats, x)
	}
	return floats, nil
}

// getNumberAsFloat can retrieve numeric values from PdfObject (both integer/float).
func getNumberAsFloat(obj core.PdfObject) (float64, error) {
	// return model.GetNumberAsFloat(obj)
	if fObj, ok := obj.(*core.PdfObjectFloat); ok {
		return float64(*fObj), nil
	}

	if iObj, ok := obj.(*core.PdfObjectInteger); ok {
		return float64(*iObj), nil
	}

	return 0, errors.New("Not a number")
}

// minFloat returns the lesser of `a` and `b`
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// maxFloat returns the greater of `a` and `b`
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// chomp returns the first `n` characters in string `s`
func chomp(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func procBuf(buf *bytes.Buffer) {
	if isTesting {
		return
	}

	lk := license.GetLicenseKey()
	if lk != nil && lk.IsLicensed() {
		return
	}
	fmt.Printf("Unlicensed copy of unidoc\n")
	fmt.Printf("To get rid of the watermark and keep entire text - Please get a license on https://unidoc.io\n")

	s := "- [Unlicensed UniDoc - Get a license on https://unidoc.io]"
	if buf.Len() > 100 {
		s = "... [Truncated - Unlicensed UniDoc - Get a license on https://unidoc.io]"
		buf.Truncate(buf.Len() - 100)
	}
	buf.WriteString(s)
}

// truncate returns the first `n` characters in string `s`.
func truncate(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
