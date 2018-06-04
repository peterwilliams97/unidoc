/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"errors"
	"fmt"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model"
)

// ExtractShapes returns the marked paths on the pdf page in `e` as a ShapeList.
func (e *Extractor) ExtractShapes() (*ShapeList, error) {
	shapeList := &ShapeList{}
	shape := Shape{}
	cp := Point{}

	cstreamParser := contentstream.NewContentStreamParser(e.contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return shapeList, err
	}

	processor := contentstream.NewContentStreamProcessor(*operations)

	inText := false

	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState,
			resources *model.PdfPageResources) error {
			operand := op.Operand
			fmt.Printf("op=%+v\n", op)
			switch operand {
			case "BT":
				inText = true
			case "ET":
				inText = false

			case "m": // move to
				if inText {
					common.Log.Debug("m operand inside text")
					return nil
				}
				if len(op.Params) != 2 {
					return errors.New("m: Invalid number of inputs")
				}

				shapeList.AppendPath(shape)
				shape = NewShape()
				cp = Point{}

				cp, err = toPageCoords(gs, op.Params)
				if err != nil {
					return err
				}
				if !shape.Empty() {
					panic("path exists")
				}
				shape = NewShape()
				shape.AppendPoint(cp)
				common.Log.Debug("m operator. shape=%+v", shape)
				if shape.Empty() {
					panic("path not created   ")
				}

			case "l": // line to
				if inText {
					common.Log.Debug("l operand inside text")
					return nil
				}
				if len(op.Params) != 2 {
					return errors.New("l: Invalid number of inputs")
				}
				if shape.Empty() {
					common.Log.Debug("l operator with no cp. shape=%+v", shape)
				}
				cp, err = toPageCoords(gs, op.Params)
				if err != nil {
					return err
				}
				shape.AppendPoint(cp)

			case "c": // curve to  cp, p1, p2, p3
				if inText {
					common.Log.Debug("c operand inside text")
					return nil
				}
				if len(op.Params) != 6 {
					return errors.New("c: Invalid number of inputs")
				}
				if shape.Empty() {
					common.Log.Debug("c operator with no cp. shape=%+v", shape)
				}
				points, err := toPagePointList(gs, op.Params)
				if err != nil {
					return err
				}
				p1, p2, p3 := points[0], points[1], points[2]
				if shape.Empty() {
					common.Log.Debug("c operator with no cp. shape=%+v", shape)
					shape.AppendPoint(p3)
				} else {
					shape.AppendCurve(cp, p1, p2, p3)
				}
				cp = p3

			case "v": // curve to  cp, cp, p2, p3
				if inText {
					common.Log.Debug("v operand inside text")
					return nil
				}
				if len(op.Params) != 4 {
					return errors.New("v: Invalid number of inputs")
				}
				if shape.Empty() {
					common.Log.Debug("c operator with no cp. shape=%+v", shape)
				}
				points, err := toPagePointList(gs, op.Params)
				if err != nil {
					return err
				}
				p2, p3 := points[0], points[1]
				if shape.Empty() {
					common.Log.Debug("cv operator with no cp. shape=%+v", shape)
					shape.AppendPoint(p3)
				} else {
					shape.AppendCurve(cp, cp, p2, p3)
				}
				cp = p3

			case "y": // curve to  cp, p1, cp, p3
				if inText {
					common.Log.Debug("yv operand inside text")
					return nil
				}
				if len(op.Params) != 4 {
					return errors.New("v: Invalid number of inputs")
				}
				if shape.Empty() {
					common.Log.Debug("c operator with no cp. shape=%+v", shape)
				}
				points, err := toPagePointList(gs, op.Params)
				if err != nil {
					return err
				}
				p1, p3 := points[0], points[1]
				if shape.Empty() {
					common.Log.Debug("cv operator with no cp. shape=%+v", shape)
					shape.AppendPoint(cp)
				} else {
					shape.AppendCurve(cp, p1, cp, p3)
				}
				cp = p3

			case "re": // rectangle
				if inText {
					common.Log.Debug("re operand inside text")
					return nil
				}
				if len(op.Params) != 4 {
					return errors.New("re: Invalid number of inputs")
				}
				floats, err := toFloatList(op.Params)
				if err != nil {
					return err
				}
				x, y, w, h := floats[0], floats[1], floats[2], floats[3]
				p0 := toPagePoint(gs, x, y)
				p1 := toPagePoint(gs, x+w, y)
				p2 := toPagePoint(gs, x+w, y+h)
				p3 := toPagePoint(gs, x, y+h)

				shapeList.AppendPath(shape)
				shape = NewShape()
				shape.AppendPoint(p0)
				shape.AppendPoint(p1)
				shape.AppendPoint(p2)
				shape.AppendPoint(p3)
				shape.AppendPoint(p0)
				cp = p0

			case "h": // close path
				if inText {
					common.Log.Debug("h operand inside text")
					return nil
				}
				if !shape.Empty() {
					shape.AppendPoint(shape.Origin())
					shapeList.AppendPath(shape)
					shape = NewShape()
					cp = Point{}
				}

			case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n": // filling, stroking and closing paths
				if inText {
					common.Log.Debug("%s operand inside text", operand)
					return nil
				}
				switch operand {
				case "s", "f", "F", "b", "b*", "n":
					if !shape.Empty() {
						shape.AppendPoint(shape.Origin())
						shapeList.AppendPath(shape)
						shape = NewShape()
						cp = Point{}
					}
				}
				lastPath := shapeList.LastPath(shape)
				switch operand {
				case "s", "S":
					lastPath.ColorStroking = gs.ColorStroking
				case "f", "F": // close and fill path
					lastPath.ColorNonStroking = gs.ColorNonStroking
					lastPath.FillType = FillRuleWinding
				case "f*": // close and fill path
					lastPath.ColorNonStroking = gs.ColorNonStroking
					lastPath.FillType = FillRuleOddEven
				case "b", "B": // close and fill path
					lastPath.ColorStroking = gs.ColorStroking
					lastPath.ColorNonStroking = gs.ColorNonStroking
					lastPath.FillType = FillRuleWinding
				case "b*", "B*": // close and fill path
					lastPath.ColorStroking = gs.ColorStroking
					lastPath.ColorNonStroking = gs.ColorNonStroking
					lastPath.FillType = FillRuleOddEven
				}
			}
			return nil
		})

	err = processor.Process(e.resources)
	if err != nil {
		common.Log.Error("Error processing: %v", err)
		return nil, err
	}

	return shapeList, nil
}

// ShapeList is a list of pdf paths
type ShapeList struct {
	Shapes []Shape
}

// Shape describes a pdf path
type Shape struct {
	Lines            Path            // Line segmnents
	Curves           CubicBezierPath // Curve segments
	Segments         []PathSegment   // All segments
	ColorStroking    model.PdfColor  // Colour that shape is stroked with, if any
	ColorNonStroking model.PdfColor  // Colour that shape is filled with, if any
	FillType         FillRule        // Filling rule of filled shaped
}

type PathSegment struct {
	Index  int
	Curved bool
}

type FillRule int

const (
	FillRuleWinding FillRule = iota
	FillRuleOddEven
)

// NewShape returns an empty Shape
func NewShape() Shape {
	return Shape{}
}

// AppendPoint appends `point` to `shape`
// This can be used to move the current pointer or to add a line segment
// point is assumed to be in page coordinates
func (shape *Shape) AppendPoint(point Point) {
	n := shape.Lines.Length()
	shape.Lines.AppendPoint(point)
	shape.Segments = append(shape.Segments, PathSegment{n, false})
	common.Log.Debug("AppendPath: point=%+v shape=%+v", point, shape)
	if shape.Empty() {
		panic("empty!")
	}
}

// AppendCurve appends Bezier curve with control points p0,p1,p2,p3 to `shape`
// This can be used to move the current pointer or to add a line segmebnt
func (shape *Shape) AppendCurve(p0, p1, p2, p3 Point) {
	n := shape.Lines.Length()
	curve := CubicBezierCurve{
		P0: p0,
		P1: p1,
		P2: p2,
		P3: p3,
	}
	shape.Curves.AppendCurve(curve)
	shape.Segments = append(shape.Segments, PathSegment{n, true})
	common.Log.Debug("AppendPath: curve=%+v shape=%+v", curve, shape)
	if shape.Empty() {
		panic("empty!")
	}
}

// Origin returns the first point in `shape`
// Do NOT call Origin with an empty shape
func (shape *Shape) Origin() Point {
	if shape.Empty() {
		panic("Shape.Origin: No points")
	}
	i := shape.Segments[0].Index
	if shape.Segments[0].Curved {
		return shape.Curves.Curves[i].P0
	}
	return shape.Lines.Points[i]
}

// Length returns the number of segments in `shape`
func (shape *Shape) Length() int {
	numLines := shape.Lines.Length() - 1
	if numLines < 0 {
		numLines = 0
	}
	return numLines + shape.Curves.Length()
}

// Empty returns true if no points or curves have been added to `shape`
func (shape *Shape) Empty() bool {
	return len(shape.Segments) == 0
}

// Copy returns a copy of `shape`
func (shape *Shape) Copy() Shape {
	shape2 := NewShape()
	shape2.Lines = shape.Lines.Copy()
	shape2.Curves = shape.Curves.Copy()
	for _, s := range shape.Segments {
		shape2.Segments = append(shape2.Segments, s)
	}
	return shape2
}

// Transform transforms `shape` by the affine transformation a, b, c, d, tx, ty
func (shape *Shape) Transform(a, b, c, d, tx, ty float64) {
	m := contentstream.NewMatrix(a, b, c, d, tx, ty)
	shape.transformByMatrix(m)
}

// transformByMatrix transforms `shape` by the affine transformation `m`
func (shape *Shape) transformByMatrix(m contentstream.Matrix) {
	shape.Lines.transformByMatrix(m)
	shape.Curves.transformByMatrix(m)
}

// GetBoundingBox returns `shape`s  bounding box
func (shape *Shape) GetBoundingBox() BoundingBox {
	bboxL := shape.Lines.GetBoundingBox()
	bboxC := shape.Curves.GetBoundingBox()
	if shape.Lines.Length() == 0 && shape.Curves.Length() == 0 {
		return BoundingBox{}
	} else if shape.Lines.Length() == 0 {
		return bboxC
	} else if shape.Curves.Length() == 0 {
		return bboxL
	}
	return BoundingBox{
		Ll: Point{minFloat(bboxL.Ll.X, bboxC.Ll.X), minFloat(bboxL.Ll.Y, bboxC.Ll.Y)},
		Ur: Point{maxFloat(bboxL.Ur.X, bboxC.Ur.X), maxFloat(bboxL.Ur.Y, bboxC.Ur.Y)},
	}
}

func (sl *ShapeList) Length() int {
	return len(sl.Shapes)
}

func (sl *ShapeList) LastPath(currentPath Shape) *Shape {
	if currentPath.Empty() {
		currentPath = sl.Shapes[len(sl.Shapes)-1]
	}
	return &currentPath
}

// add appends a Shape to the path list
func (sl *ShapeList) AppendPath(s Shape) {
	if s.Length() > 0 {
		sl.Shapes = append(sl.Shapes, s)
	}
}

// Transform transforms all shapes of  `sl` by the affine transformation a, b, c, d, tx, ty
func (sl *ShapeList) Transform(a, b, c, d, tx, ty float64) {
	m := contentstream.NewMatrix(a, b, c, d, tx, ty)
	sl.transformByMatrix(m)
}

// transformByMatrix transforms `shape` by the affine transformation `m`
func (sl *ShapeList) transformByMatrix(m contentstream.Matrix) {
	for _, shape := range sl.Shapes {
		shape.transformByMatrix(m)
	}
}

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
	x, y = gs.Transform(x, y)
	return Point{x, y}
}

func toFloatXY(objs []core.PdfObject) (x, y float64, err error) {
	if len(objs) != 2 {
		err = fmt.Errorf("Invalid number of params: %d", len(objs))
		common.Log.Debug("toFloatXY: err=%v", err)
		return
	}
	floats, err := toFloatList(objs)
	if err != nil {
		return
	}
	x, y = floats[0], floats[1]
	return
}

func toFloatList(objs []core.PdfObject) ([]float64, error) {
	floats := []float64{}
	for _, o := range objs {
		x, err := toFloat(o)
		if err != nil {
			return nil, err
		}
		floats = append(floats, x)
	}
	return floats, nil
}

func toFloat(o core.PdfObject) (float64, error) {
	if x, ok := o.(*core.PdfObjectFloat); ok {
		return float64(*x), nil
	}
	if xint, ok := o.(*core.PdfObjectInteger); ok {
		return float64(*xint), nil
	}
	err := fmt.Errorf("Invalid float param: %T", o)
	common.Log.Debug("toFloat: err=%v", err)
	return 0.0, err
}
