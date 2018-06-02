/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package draw

import "math"

// Vector defines the displacement of a Point
type Vector struct {
	Dx float64
	Dy float64
}

// NewVector returns a Vector with Cartesian displacement dx, dy
func NewVector(dx, dy float64) Vector {
	v := Vector{}
	v.Dx = dx
	v.Dy = dy
	return v
}

// NewVector returns the Vector that displaces a → b
func NewVectorBetween(a, b Point) Vector {
	v := Vector{}
	v.Dx = b.X - a.X
	v.Dy = b.Y - a.Y
	return v
}

// NewVectorPolar returns the Vector with polar radius `mag` and angle `theta`
func NewVectorPolar(mag, theta float64) Vector {
	v := Vector{}
	v.Dx = mag * math.Cos(theta)
	v.Dy = mag * math.Sin(theta)
	return v
}

// Add returns `v` + `other`
func (v Vector) Add(other Vector) Vector {
	v.Dx += other.Dx
	v.Dy += other.Dy
	return v
}

// Rotate returns `v` rotated by `phi`
func (v Vector) Rotate(phi float64) Vector {
	mag := v.Magnitude()
	angle := v.GetPolarAngle()
	return NewVectorPolar(mag, angle+phi)
}

// Flip returns `v` rotated by 180° a
// XXX doc said:  Change the sign of the vector: -vector.
func (v Vector) Flip() Vector {
	mag := v.Magnitude()
	theta := v.GetPolarAngle()

	v.Dx = mag * math.Cos(theta+math.Pi)
	v.Dy = mag * math.Sin(theta+math.Pi)
	return v
}

// FlipY returns `v` flipped on the Y axis
func (v Vector) FlipY() Vector {
	v.Dy = -v.Dy
	return v
}

// FlipX returns `v` flipped on the X axis
func (v Vector) FlipX() Vector {
	v.Dx = -v.Dx
	return v
}

// Scale returns `v` with its polar radius multiplied by `factor` while preserving its polar angle
func (v Vector) Scale(factor float64) Vector {
	mag := v.Magnitude()
	theta := v.GetPolarAngle()

	v.Dx = factor * mag * math.Cos(theta)
	v.Dy = factor * mag * math.Sin(theta)
	return v
}

// Magnitude returns `v`'s polar radius
func (v Vector) Magnitude() float64 {
	return math.Sqrt(math.Pow(v.Dx, 2.0) + math.Pow(v.Dy, 2.0))
}

// GetPolarAngle returns `v`'s polar angle
// XXX Why not Magnitude + PolarAngle, or GetMagnitude + GetPolarAngle?
func (v Vector) GetPolarAngle() float64 {
	return math.Atan2(v.Dy, v.Dx)
}
