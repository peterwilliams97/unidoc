/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"errors"
	"fmt"
	"sort"

	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model"
)

// Coord represents a position on a page
type Coord struct {
	X, Y float64
}

// Rect represents a rectange
type Rect struct {
	LL, UR Coord // Lower left, Upper right
}

// Path is a list of coordinates on a pdf path
type Path []Coord

// Path is a list of pdf paths
type PathList []Path

// add appends the location of text to a path
func (p *Path) add(c *Coord) {
	// fmt.Printf("    add c=%+v ", *c)
	// fmt.Printf("p=%+v\n", *p)
	*p = append(*p, *c)
}

func (p *Path) BBox() Rect {
	if len(*p) == 0 {
		panic("Empty path")
	}
	c := (*p)[0]
	b := Rect{
		LL: Coord{c.X, c.Y},
		UR: Coord{c.X, c.Y},
	}
	for _, c := range *p {
		if c.X < b.LL.X {
			b.LL.X = c.X
		} else if c.X > b.UR.X {
			b.UR.X = c.X
		}
		if c.Y < b.LL.Y {
			b.LL.Y = c.Y
		} else if c.Y > b.UR.Y {
			b.UR.Y = c.Y
		}
	}
	return b
}

// add appends a path to the path list
func (pl *PathList) add(p *Path) {
	*pl = append(*pl, *p)
}

// SortPosition sorts a text list by its elements position on a page. Top to bottom, left to right.
func (tl *Path) SortPosition() {
	sort.SliceStable(*tl, func(i, j int) bool {
		ti, tj := (*tl)[i], (*tl)[j]
		if ti.Y != tj.Y {
			return ti.Y > tj.Y
		}
		return ti.X < tj.X
	})
}

func toPageCoords(gs contentstream.GraphicsState, objs []core.PdfObject) (p *Coord, err error) {
	x, y, err := toFloatXY(objs)
	if err != nil {
		return
	}
	x, y = gs.Transform(x, y)
	p = &Coord{x, y}
	return
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

// ExtractPaths returns the marked paths of `e` as a PathList.
func (e *Extractor) ExtractPaths() (*PathList, error) {
	pathList := &PathList{}
	var path *Path = nil
	var cp *Coord = nil

	cstreamParser := contentstream.NewContentStreamParser(e.contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return pathList, err
	}

	processor := contentstream.NewContentStreamProcessor(*operations)

	inText := false

	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState,
			resources *model.PdfPageResources) error {
			operand := op.Operand
			// fmt.Printf("op=%+v\n", op)
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

				if path != nil {
					pathList.add(path)
					path = nil
					cp = nil
				}

				cp, err = toPageCoords(gs, op.Params)
				if err != nil {
					return err
				}
				if path != nil {
					panic("path exists")
				}
				path = &Path{}
				path.add(cp)

			case "l": // line to
				if inText {
					common.Log.Debug("l operand inside text")
					return nil
				}
				if len(op.Params) != 2 {
					return errors.New("l: Invalid number of inputs")
				}
				if cp == nil {
					common.Log.Debug("l operator with no cp")
				}
				cp, err = toPageCoords(gs, op.Params)
				if err != nil {
					return err
				}
				path.add(cp)

			case "h": // close path
				if inText {
					common.Log.Debug("h operand inside text")
					return nil
				}
				if path != nil {
					pathList.add(path)
					path = nil
					cp = nil
				}

			case "n": // end path
				if inText {
					common.Log.Debug("n operand inside text")
					return nil
				}
				if path != nil {
					pathList.add(path)
					path = nil
					cp = nil
				}

			}

			return nil
		})

	err = processor.Process(e.resources)
	if err != nil {
		common.Log.Error("Error processing: %v", err)
		return nil, err
	}

	return pathList, nil
}
