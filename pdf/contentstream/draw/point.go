/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package draw

import "fmt"

// Point defines a point
type Point struct {
	X float64
	Y float64
}

// NewPoint returns a Point at x,y
func NewPoint(x, y float64) Point {
	point := Point{}
	point.X = x
	point.Y = y
	return point
}

// Add returns a copy of `p` translated  by `dx`,`dy`
// XXX: Should this be called Translate()?
func (p Point) Add(dx, dy float64) Point {
	p.X += dx
	p.Y += dy
	return p
}

// AddVector returns a copy of `p` translated by `v`
// XXX: Should this be called TranslateVector()?
func (p Point) AddVector(v Vector) Point {
	p.X += v.Dx
	p.Y += v.Dy
	return p
}

// String returns a string describing `p`
func (p Point) String() string {
	return fmt.Sprintf("(%.1f,%.1f)", p.X, p.Y)
}
