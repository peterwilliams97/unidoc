/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package transform

import (
	"fmt"
	"math"
	"testing"

	"github.com/unidoc/unidoc/common"
)

func init() {
	common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
}

// TestAngle tests the Matrix.Angle() function.
func TestAngle(t *testing.T) {
	extraTests := []angleCase{}
	for θ := 0.01; θ <= 360.0; θ *= 1.1 {
		extraTests = append(extraTests, makeAngleCase(2.0, θ))
	}

	const angleTol = 1.0e-10

	for _, test := range append(angleTests, extraTests...) {
		p := test.params
		m := NewMatrix(p.a, p.b, p.c, p.d, p.tx, p.ty)
		θ := m.Angle()
		if !equalsDegrees(θ, test.θ) {
			t.Fatalf("Bad angle: m=%s expected=%g° actual=%g°", m, test.θ, θ)
		}
		rot := IdentityMatrix().Rotate(test.θ)
		rotθ := rot.Angle()
		if !equalsDegrees(rotθ, test.θ) {
			t.Fatalf("Bad angle: m=%s expected=%g° actual=%g°", m, test.θ, rotθ)
		}

	}
}

type params struct{ a, b, c, d, tx, ty float64 }
type angleCase struct {
	params         // Affine transform.
	θ      float64 // Rotation of affine transform in degrees.
}

var angleTests = []angleCase{
	{params: params{1, 0, 0, 1, 0, 0}, θ: 0},
	{params: params{0, -1, 1, 0, 0, 0}, θ: 90},
	{params: params{-1, 0, 0, -1, 0, 0}, θ: 180},
	{params: params{0, 1, -1, 0, 0, 0}, θ: 270},
	{params: params{1, -1, 1, 1, 0, 0}, θ: 45},
	{params: params{-1, -1, 1, -1, 0, 0}, θ: 135},
	{params: params{-1, 1, -1, -1, 0, 0}, θ: 225},
	{params: params{1, 1, -1, 1, 0, 0}, θ: 315},
}

// makeAngleCase makes an angleCase for a Matrix with scale `r` and angle `θ` degrees.
func makeAngleCase(r, θ float64) angleCase {
	radians := θ / 180.0 * math.Pi
	a := r * math.Cos(radians)
	b := -r * math.Sin(radians)
	c := -b
	d := a
	return angleCase{params{a, b, c, d, 0, 0}, θ}
}

var (
	// baseMatrices are meant to be similar to matrices seen in production.
	baseMatrices = []Matrix{
		NewMatrix(1, 0, 0, 1, 0, 0),
		NewMatrix(1, 0, 0, -1, 0, 0),
		NewMatrix(1, 0, 0, -1, 0, 0),
		NewMatrix(-1, 0, 0, -1, 0, 0),
		NewMatrix(0, 1, 1, 0, 0, 0),
		NewMatrix(0, -1, -1, 0, 0, 0),
		NewMatrix(1, 1, 1, -1, 0, 0),
		NewMatrix(1, 1, -1, 1, 0, 0),
		NewMatrix(1, -1, -1, -1, 0, 0),
		NewMatrix(1, -1, -1, 0, 0, 0),
		NewMatrix(1, 0, 0, 1, 2, 5),
		NewMatrix(1, 0, 0, 2, 2, 5),
		NewMatrix(2, 0, 4, 5, 0, 0),
		NewMatrix(2, 3, 4, 5, 0, 0),
		NewMatrix(2, 0, 0, 5, 0.1, 0),
		NewMatrix(2, 6, 6, 5, 0.1, 0),
		NewMatrix(2, 6, 6, 5, 0.1, 0.3),
		NewMatrix(1, 1, -1, 1, 0.1, 0),
		NewMatrix(2, 3, 4, 5, 0.1, 0),
		NewMatrix(2, 3, 4, 5, 0.1, 0.2),
	}
	// extremeMatrices are designed to test floating point accuracy and rounding.
	extremeMatrices = []Matrix{
		NewMatrix(1e8, 0, 0, 1, 0.1, 0.2),
		NewMatrix(1e8, 0, 0, 1e-8, 0.1, 0.2),
		NewMatrix(0, 1e8, 1e-8, 0, 0.1, 0.2),
		NewMatrix(0, 1e8, 1e-8, 0, 1e-8, 1e8),
		NewMatrix(1e8, -1e8, 1e-8, -2e-8, 0, 0),
		NewMatrix(1e8, -1e8, 1e-8, -2e-8, 5, 5),
		NewMatrix(1e8, -1e8, 1e-8, -2e-8, 5, 1e-8),
		NewMatrix(1, 1-1e5, 1, 1+1e5, 0, 0),
	}
)

// TestInverse tests the Matrix.Inverse() function.
func TestInverse(t *testing.T) {
	m := NewMatrix(1, 1, 1, 1, 0, 0)
	_, hasInverse := m.Inverse()
	if hasInverse {
		t.Fatalf("%s has inverse", m)
	}
	matrices := append(baseMatrices, extremeMatrices...)
	for _, m := range matrices {
		testInverse(t, m)
	}
}

// testInverse tests if `m`.Inverse() is the inverse of `m`.
func testInverse(t *testing.T, m Matrix) {
	inv, hasInverse := m.Inverse()
	if !hasInverse {
		t.Fatalf("No inverse for %s", m)
	}
	pre := m.Mult(inv)
	if !isIdentity(pre) {
		t.Fatalf("Not pre-inverse:\n"+
			"\t   m=%s\n"+
			"\t inv=%s\n"+
			"\t pre=%s\n\t", m, inv, pre)
	}
	post := inv.Mult(m)
	if !isIdentity(post) {
		t.Fatalf("Not post-inverse:\n"+
			"\t   m=%s\n"+
			"\t inv=%s\n"+
			"\tpost=%s\n\t", m, inv, post)
	}
}

// TestInverseTransforms tests that transforms on inverses behave correctly.
// NOTE: This can be a little subltel as affine transforms don't have unique decompositions into
// scaling, rotation, shearing and translation.
func TestInverseTransforms(t *testing.T) {
	for _, m := range baseMatrices {
		testInverseRotations(t, m)
	}
}

// testInverseRotations checks that a) rotating `m` by θ and b) rotating the inverse of `m` by -θ
// gives matrices whose rotations (angle of rotated matrix - angle of original matrix) are the
// negative of each other.
func testInverseRotations(t *testing.T, m Matrix) {
	// NOTE: Decompositions of affine tranforms to scaling, rotation and shearing is generally not unique.
	//       If the 2x2 submatrix is  | cosθ -sinθ | then the rotation is unique but instead of
	//                                | sinθ  cosθ |
	//       enforcing this, we only require that the signs are consistent with a rotation.
	if (equals(m[1], 0) && equals(m[3], 0) && (m[0] < 0.0) != (m[4] < 0.0)) || (m[1] < 0.0) == (m[3] < 0.0) {
		return
	}

	θm := m.Angle()
	inv, hasInverse := m.Inverse()
	if !hasInverse {
		t.Fatalf("No inverse: m=%s", m)
	}
	θinv := inv.Angle()

	for _, θ := range []float64{0, 90, 180, 270, 45, 77, 1e-8} {
		rot := m.Rotate(θ)
		θrot := rot.Angle()
		rotinv := inv.Rotate(-θ)
		θrotinv := rotinv.Angle()

		description := fmt.Sprintf("\t     m=%s %3g° %s\n"+
			"\t   rot=%s %3g° %s\n"+
			"\t   inv=%s %3g° %s\n"+
			"\trotinv=%s %3g° %s",
			m, θm, showXform(m),
			rot, θrot, showXform(rot),
			inv, θinv, showXform(inv),
			rotinv, θrotinv, showXform(rotinv))

		if !equalsDegrees(θrot-θm, θ) {
			t.Fatalf("θ!=θrot-θm: θ=%g° θrot-θm=%g°\n%s\n", θ, θrot-θm, description)
		}
		if !equalsDegrees(θrotinv-θinv, -(θrot - θm)) {
			t.Fatalf("θrotinv-θinv != -(θrot - θm): θ=%g° θrotinv-θinv=%g° θrot-θm=%g°\n%s",
				θ, θrotinv-θinv, θrot-θm, description)
		}
		// post := rot.Mult(rotinv)
		// if !isIdentity(post) {
		// 	t.Fatalf("rot x rotinv != identity\n%s\n\t  post=%s", description, post)
		// }
	}
}

// showXform returns a string showing the coordinates `m` transforms (1, 0) to.
func showXform(m Matrix) string {
	dx, dy := m.Translation()
	x, y := m.Transform(1, 0)
	return fmt.Sprintf("(%5.2f,%5.2f)", x-dx, y-dy)
}

// isIdentity returns true if `m` approximates the identity matrix.
func isIdentity(m Matrix) bool {
	return equalsMatrix(m, IdentityMatrix())
}

// equalsMatrix returns true if `m1` is approximately the same as `m2`.
func equalsMatrix(m1, m2 Matrix) bool {
	for i, x1 := range m1 {
		if !equals(x1, m2[i]) {
			return false
		}
	}
	return true
}

// equals returns true if `x` is approximately the same as `y`.
func equals(x, y float64) bool {
	return math.Abs(x-y) <= tolerance
}

// equalsDegrees returns true if `x` is approximately the same as `y` where `x` and `y` are angles
// in degrees.
func equalsDegrees(x, y float64) bool {
	return math.Abs(math.Remainder(x-y, 360)) <= tolerance
}

// tolerance is the maximum that two numbers can differ by and still be considered equal.
const tolerance = 1.0e-10
