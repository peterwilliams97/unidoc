/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 *
 * Based on pdf/contentstream/draw/point.go
 */

// XXX(peterwilliams97) Change to functional style. i.e. Return new value, don't mutate.

package model

import (
	"fmt"
	"math"
)

// Point defines a point in Cartesian coordinates
type Point struct {
	X float64
	Y float64
}

// NewPoint returns a Point at 'x', 'y'.
func NewPoint(x, y float64) Point {
	return Point{X: x, Y: y}
}

// Set sets `p` to `x`, `y`.
func (p *Point) Set(x, y float64) {
	p.X, p.Y = x, y
}

// Transform transforms `p` by the affine transformation a, b, c, d, tx, ty.
func (p *Point) Transform(a, b, c, d, tx, ty float64) {
	m := NewMatrix(a, b, c, d, tx, ty)
	p.transformByMatrix(m)
}

// Displace returns `p` displaced by `delta`.
func (p Point) Displace(delta Point) Point {
	return Point{p.X + delta.X, p.Y + delta.Y}
}

// Rotate returns `p` rotated by `theta` degrees.
func (p Point) Rotate(theta float64) Point {
	radians := theta / 180.0 * math.Pi
	r := math.Hypot(p.X, p.Y)
	t := math.Atan2(p.Y, p.X)
	return Point{r * math.Cos(t+radians), r * math.Sin(t+radians)}
}

// transformByMatrix transforms `p` by the affine transformation `m`.
func (p *Point) transformByMatrix(m Matrix) {
	p.X, p.Y = m.Transform(p.X, p.Y)
}

// String returns a string describing `p`.
func (p Point) String() string {
	return fmt.Sprintf("(%.2f,%.2f)", p.X, p.Y)
}