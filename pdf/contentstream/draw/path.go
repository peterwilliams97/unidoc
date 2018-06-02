/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package draw

import "github.com/unidoc/unidoc/common"

// Path describes a pdf path
// A path consists of straight line connections between each point defined in an array of points.
type Path struct {
	Points []Point
}

// NewPath returns an empty Path
func NewPath() Path {
	path := Path{}
	path.Points = []Point{}
	return path
}

// AppendPoint returns a copy of `path` with `point` appended to it.
// XXX: Is this intended? Why not update `path` in place?
func (path Path) AppendPoint(point Point) Path {
	path.Points = append(path.Points, point)
	common.Log.Debug("path.AppendPoint: point=%+v Points=%+v path=%+v", point, path.Points, path)
	return path
}

// RemovePoint returns a copy of `path` with the `number`th point in removed. `number` is 1-offset
// XXX: Is this intended? Why not update `path` in place?
func (path Path) RemovePoint(number int) Path {
	if number < 1 || number > len(path.Points) {
		return path
	}

	idx := number - 1
	path.Points = append(path.Points[:idx], path.Points[idx+1:]...)
	return path
}

// Length returns the number of points in `path`
func (path Path) Length() int {
	return len(path.Points)
}

// GetPointNumber returns the `number`th point in `path`. `number` is 1-offset
// If this point is not in the path then a zero Point is returned
func (path Path) GetPointNumber(number int) Point {
	if number < 1 || number > len(path.Points) {
		return Point{}
	}
	return path.Points[number-1]
}

// Copy returns a copy of `path`
func (path Path) Copy() Path {
	pathcopy := NewPath()
	for _, p := range path.Points {
		pathcopy.Points = append(pathcopy.Points, p)
	}
	return pathcopy
}

// Offset returns a copy of `path` translate by `dX`,`dY`
// XXX: Should this be called Translate?
func (path Path) Offset(dX, dY float64) Path {
	for i, p := range path.Points {
		path.Points[i] = p.Add(dX, dY)
	}
	return path
}

// GetBoundingBox returns `path`'s bounding box
func (path Path) GetBoundingBox() BoundingBox {
	if len(path.Points) == 0 {
		return BoundingBox{}
	}

	p := path.Points[0]
	minX := p.X
	maxX := p.X
	minY := p.Y
	maxY := p.Y
	for _, p := range path.Points[1:] {
		if p.X < minX {
			minX = p.X
		} else if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		} else if p.Y > maxY {
			maxY = p.Y
		}
	}

	return BoundingBox{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}
}

// BoundingBox describes a bounding box. X,Y is the bottom,left of the bounding box (i.e.
// lowest coordinate values)
// XXX Why not Llx, LLy, URx, URy like PdfRectangle?
type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}
