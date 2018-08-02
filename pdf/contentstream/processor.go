/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package contentstream

import (
	"errors"
	"fmt"

	"github.com/unidoc/unidoc/common"
	. "github.com/unidoc/unidoc/pdf/core"
	. "github.com/unidoc/unidoc/pdf/model"
)

// Basic graphics state implementation.
// Initially only implementing and tracking a portion of the information specified.  Easy to add more.
type GraphicsState struct {
	ColorspaceStroking    PdfColorspace
	ColorspaceNonStroking PdfColorspace
	ColorStroking         PdfColor
	ColorNonStroking      PdfColor
	CTM                   Matrix
}

type Orientation int

const (
	OrientationPortrait Orientation = iota
	OrientationLandscape
)

type GraphicStateStack []GraphicsState

// Push pushes graphics state `gs` onto the graphics state stack.
func (gsStack *GraphicStateStack) Push(gs GraphicsState) {
	*gsStack = append(*gsStack, gs)
}

// Pop pops and returns the element on the top of the graphics state stack.
func (gsStack *GraphicStateStack) Pop() GraphicsState {
	gs := (*gsStack)[len(*gsStack)-1]
	*gsStack = (*gsStack)[:len(*gsStack)-1]
	return gs
}

// Transform returns coordinates x, y transformed by the CTM
func (gs *GraphicsState) Transform(x, y float64) (float64, float64) {
	return gs.CTM.Transform(x, y)
}

// PageOrientation returns the likely page orientation given the CTM.
// !@#$ get this from the PDF page
func (gs *GraphicsState) PageOrientation() Orientation {
	return gs.CTM.pageOrientation()
}

// ContentStreamProcessor defines a data structure and methods for processing a content stream,
// keeping track of the current graphics state, and allowing external handlers to define their own
// functions as a part of the processing, for example rendering or extracting certain information.
type ContentStreamProcessor struct {
	graphicsStack GraphicStateStack
	operations    []*ContentStreamOperation
	graphicsState GraphicsState

	handlers     []HandlerEntry
	currentIndex int
}

type HandlerFunc func(op *ContentStreamOperation, gs GraphicsState, resources *PdfPageResources) error

type HandlerEntry struct {
	Condition HandlerConditionEnum
	Operand   string
	Handler   HandlerFunc
}

type HandlerConditionEnum int

func (csp HandlerConditionEnum) All() bool {
	return csp == HandlerConditionEnumAllOperands
}

func (csp HandlerConditionEnum) Operand() bool {
	return csp == HandlerConditionEnumOperand
}

const (
	HandlerConditionEnumOperand     HandlerConditionEnum = iota
	HandlerConditionEnumAllOperands HandlerConditionEnum = iota
)

func NewContentStreamProcessor(ops []*ContentStreamOperation) *ContentStreamProcessor {
	csp := ContentStreamProcessor{}
	csp.graphicsStack = GraphicStateStack{}

	// Set defaults..
	gs := GraphicsState{}

	csp.graphicsState = gs

	csp.handlers = []HandlerEntry{}
	csp.currentIndex = 0
	csp.operations = ops

	return &csp
}

func (csp *ContentStreamProcessor) AddHandler(condition HandlerConditionEnum, operand string, handler HandlerFunc) {
	entry := HandlerEntry{}
	entry.Condition = condition
	entry.Operand = operand
	entry.Handler = handler
	csp.handlers = append(csp.handlers, entry)
}

func (csp *ContentStreamProcessor) getColorspace(name string, resources *PdfPageResources) (PdfColorspace, error) {
	switch name {
	case "DeviceGray":
		return NewPdfColorspaceDeviceGray(), nil
	case "DeviceRGB":
		return NewPdfColorspaceDeviceRGB(), nil
	case "DeviceCMYK":
		return NewPdfColorspaceDeviceCMYK(), nil
	case "Pattern":
		return NewPdfColorspaceSpecialPattern(), nil
	}

	// Next check the colorspace dictionary.
	cs, has := resources.ColorSpace.Colorspaces[name]
	if has {
		return cs, nil
	}

	// Lastly check other potential colormaps.
	switch name {
	case "CalGray":
		return NewPdfColorspaceCalGray(), nil
	case "CalRGB":
		return NewPdfColorspaceCalRGB(), nil
	case "Lab":
		return NewPdfColorspaceLab(), nil
	}

	// Otherwise unsupported.
	common.Log.Debug("Unknown colorspace requested: %s", name)
	return nil, errors.New("Unsupported colorspace")
}

// Get initial color for a given colorspace.
func (csp *ContentStreamProcessor) getInitialColor(cs PdfColorspace) (PdfColor, error) {
	switch cs := cs.(type) {
	case *PdfColorspaceDeviceGray:
		return NewPdfColorDeviceGray(0.0), nil
	case *PdfColorspaceDeviceRGB:
		return NewPdfColorDeviceRGB(0.0, 0.0, 0.0), nil
	case *PdfColorspaceDeviceCMYK:
		return NewPdfColorDeviceCMYK(0.0, 0.0, 0.0, 1.0), nil
	case *PdfColorspaceCalGray:
		return NewPdfColorCalGray(0.0), nil
	case *PdfColorspaceCalRGB:
		return NewPdfColorCalRGB(0.0, 0.0, 0.0), nil
	case *PdfColorspaceLab:
		l := 0.0
		a := 0.0
		b := 0.0
		if cs.Range[0] > 0 {
			l = cs.Range[0]
		}
		if cs.Range[2] > 0 {
			a = cs.Range[2]
		}
		return NewPdfColorLab(l, a, b), nil
	case *PdfColorspaceICCBased:
		if cs.Alternate == nil {
			// Alternate not defined.
			// Try to fall back to DeviceGray, DeviceRGB or DeviceCMYK.
			common.Log.Trace("ICC Based not defined - attempting fall back (N = %d)", cs.N)
			if cs.N == 1 {
				common.Log.Trace("Falling back to DeviceGray")
				return csp.getInitialColor(NewPdfColorspaceDeviceGray())
			} else if cs.N == 3 {
				common.Log.Trace("Falling back to DeviceRGB")
				return csp.getInitialColor(NewPdfColorspaceDeviceRGB())
			} else if cs.N == 4 {
				common.Log.Trace("Falling back to DeviceCMYK")
				return csp.getInitialColor(NewPdfColorspaceDeviceCMYK())
			} else {
				return nil, errors.New("Alternate space not defined for ICC")
			}
		}
		return csp.getInitialColor(cs.Alternate)
	case *PdfColorspaceSpecialIndexed:
		if cs.Base == nil {
			return nil, errors.New("Indexed base not specified")
		}
		return csp.getInitialColor(cs.Base)
	case *PdfColorspaceSpecialSeparation:
		if cs.AlternateSpace == nil {
			return nil, errors.New("Alternate space not specified")
		}
		return csp.getInitialColor(cs.AlternateSpace)
	case *PdfColorspaceDeviceN:
		if cs.AlternateSpace == nil {
			return nil, errors.New("Alternate space not specified")
		}
		return csp.getInitialColor(cs.AlternateSpace)
	case *PdfColorspaceSpecialPattern:
		// FIXME/check: A pattern does not have an initial color...
		return nil, nil
	}

	common.Log.Debug("Unable to determine initial color for unknown colorspace: %T", cs)
	return nil, errors.New("Unsupported colorspace")
}

// Process the entire operations.
func (csp *ContentStreamProcessor) Process(resources *PdfPageResources) error {
	// Initialize graphics state
	csp.graphicsState.ColorspaceStroking = NewPdfColorspaceDeviceGray()
	csp.graphicsState.ColorspaceNonStroking = NewPdfColorspaceDeviceGray()
	csp.graphicsState.ColorStroking = NewPdfColorDeviceGray(0)
	csp.graphicsState.ColorNonStroking = NewPdfColorDeviceGray(0)
	csp.graphicsState.CTM = IdentityMatrix()

	for _, op := range csp.operations {
		var err error

		// Internal handling.
		switch op.Operand {
		case "q":
			csp.graphicsStack.Push(csp.graphicsState)
		case "Q":
			csp.graphicsState = csp.graphicsStack.Pop()

		// Color operations (Table 74 p. 179)
		case "CS":
			err = csp.handleCommand_CS(op, resources)
		case "cs":
			err = csp.handleCommand_cs(op, resources)
		case "SC":
			err = csp.handleCommand_SC(op, resources)
		case "SCN":
			err = csp.handleCommand_SCN(op, resources)
		case "sc":
			err = csp.handleCommand_sc(op, resources)
		case "scn":
			err = csp.handleCommand_scn(op, resources)
		case "G":
			err = csp.handleCommand_G(op, resources)
		case "g":
			err = csp.handleCommand_g(op, resources)
		case "RG":
			err = csp.handleCommand_RG(op, resources)
		case "rg":
			err = csp.handleCommand_rg(op, resources)
		case "K":
			err = csp.handleCommand_K(op, resources)
		case "k":
			err = csp.handleCommand_k(op, resources)
		case "cm":
			err = csp.handleCommand_cm(op, resources)
		}
		if err != nil {
			common.Log.Debug("Processor handling error (%s): %v", op.Operand, err)
			common.Log.Debug("Operand: %#v", op.Operand)
			return err
		}

		// Check if have external handler also, and process if so.
		for _, entry := range csp.handlers {
			var err error
			if entry.Condition.All() {
				err = entry.Handler(op, csp.graphicsState, resources)
			} else if entry.Condition.Operand() && op.Operand == entry.Operand {
				err = entry.Handler(op, csp.graphicsState, resources)
			}
			if err != nil {
				common.Log.Debug("Processor handler error: %v", err)
				return err
			}
		}
	}

	return nil
}

// CS: Set the current color space for stroking operations.
func (csp *ContentStreamProcessor) handleCommand_CS(op *ContentStreamOperation, resources *PdfPageResources) error {
	if len(op.Params) < 1 {
		common.Log.Debug("Invalid cs command, skipping over")
		return errors.New("Too few parameters")
	}
	if len(op.Params) > 1 {
		common.Log.Debug("cs command with too many parameters - continuing")
		return errors.New("Too many parameters")
	}
	name, ok := op.Params[0].(*PdfObjectName)
	if !ok {
		common.Log.Debug("ERROR: cs command with invalid parameter, skipping over")
		return errors.New("Type check error")
	}
	// Set the current color space to use for stroking operations.
	// Either device based or referring to resource dict.
	cs, err := csp.getColorspace(string(*name), resources)
	if err != nil {
		return err
	}
	csp.graphicsState.ColorspaceStroking = cs

	// Set initial color.
	color, err := csp.getInitialColor(cs)
	if err != nil {
		return err
	}
	csp.graphicsState.ColorStroking = color

	return nil
}

// cs: Set the current color space for non-stroking operations.
func (csp *ContentStreamProcessor) handleCommand_cs(op *ContentStreamOperation, resources *PdfPageResources) error {
	if len(op.Params) < 1 {
		common.Log.Debug("Invalid CS command, skipping over")
		return errors.New("Too few parameters")
	}
	if len(op.Params) > 1 {
		common.Log.Debug("CS command with too many parameters - continuing")
		return errors.New("Too many parameters")
	}
	name, ok := op.Params[0].(*PdfObjectName)
	if !ok {
		common.Log.Debug("ERROR: CS command with invalid parameter, skipping over")
		return errors.New("Type check error")
	}
	// Set the current color space to use for non-stroking operations.
	// Either device based or referring to resource dict.
	cs, err := csp.getColorspace(string(*name), resources)
	if err != nil {
		return err
	}
	csp.graphicsState.ColorspaceNonStroking = cs

	// Set initial color.
	color, err := csp.getInitialColor(cs)
	if err != nil {
		return err
	}
	csp.graphicsState.ColorNonStroking = color

	return nil
}

// SC: Set the color to use for stroking operations in a device, CIE-based or Indexed colorspace. (not ICC based)
func (csp *ContentStreamProcessor) handleCommand_SC(op *ContentStreamOperation, resources *PdfPageResources) error {
	// For DeviceGray, CalGray, Indexed: one operand is required
	// For DeviceRGB, CalRGB, Lab: 3 operands required

	cs := csp.graphicsState.ColorspaceStroking
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for SC")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorStroking = color
	return nil
}

func isPatternCS(cs PdfColorspace) bool {
	_, isPattern := cs.(*PdfColorspaceSpecialPattern)
	return isPattern
}

// SCN: Same as SC but also supports Pattern, Separation, DeviceN and ICCBased color spaces.
func (csp *ContentStreamProcessor) handleCommand_SCN(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := csp.graphicsState.ColorspaceStroking

	if !isPatternCS(cs) {
		if len(op.Params) != cs.GetNumComponents() {
			common.Log.Debug("Invalid number of parameters for SC")
			common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
			return errors.New("Invalid number of parameters")
		}
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorStroking = color

	return nil
}

// sc: Same as SC except used for non-stroking operations.
func (csp *ContentStreamProcessor) handleCommand_sc(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := csp.graphicsState.ColorspaceNonStroking

	if !isPatternCS(cs) {
		if len(op.Params) != cs.GetNumComponents() {
			common.Log.Debug("Invalid number of parameters for SC")
			common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
			return errors.New("Invalid number of parameters")
		}
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorNonStroking = color

	return nil
}

// scn: Same as SCN except used for non-stroking operations.
func (csp *ContentStreamProcessor) handleCommand_scn(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := csp.graphicsState.ColorspaceNonStroking

	if !isPatternCS(cs) {
		if len(op.Params) != cs.GetNumComponents() {
			common.Log.Debug("Invalid number of parameters for SC")
			common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
			return errors.New("Invalid number of parameters")
		}
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		common.Log.Debug("ERROR: Fail to get color from params: %+v (CS is %+v)", op.Params, cs)
		return err
	}

	csp.graphicsState.ColorNonStroking = color

	return nil
}

// G: Set the stroking colorspace to DeviceGray, and the color to the specified graylevel (range [0-1]).
// gray G
func (csp *ContentStreamProcessor) handleCommand_G(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceGray()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for SC")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorspaceStroking = cs
	csp.graphicsState.ColorStroking = color

	return nil
}

// g: Same as G, but for non-stroking colorspace and color (range [0-1]).
// gray g
func (csp *ContentStreamProcessor) handleCommand_g(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceGray()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for g")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		common.Log.Debug("ERROR: handleCommand_g Invalid params. cs=%T op=%s err=%v", cs, op, err)
		return err
	}

	csp.graphicsState.ColorspaceNonStroking = cs
	csp.graphicsState.ColorNonStroking = color

	return nil
}

// RG: Sets the stroking colorspace to DeviceRGB and the stroking color to r,g,b. [0-1] ranges.
// r g b RG
func (csp *ContentStreamProcessor) handleCommand_RG(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceRGB()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for RG")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorspaceStroking = cs
	csp.graphicsState.ColorStroking = color

	return nil
}

// rg: Same as RG but for non-stroking colorspace, color.
func (csp *ContentStreamProcessor) handleCommand_rg(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceRGB()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for SC")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorspaceNonStroking = cs
	csp.graphicsState.ColorNonStroking = color

	return nil
}

// K: Sets the stroking colorspace to DeviceCMYK and the stroking color to c,m,y,k. [0-1] ranges.
// c m y k K
func (csp *ContentStreamProcessor) handleCommand_K(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceCMYK()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for SC")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorspaceStroking = cs
	csp.graphicsState.ColorStroking = color

	return nil
}

// k: Same as K but for non-stroking colorspace, color.
func (csp *ContentStreamProcessor) handleCommand_k(op *ContentStreamOperation, resources *PdfPageResources) error {
	cs := NewPdfColorspaceDeviceCMYK()
	if len(op.Params) != cs.GetNumComponents() {
		common.Log.Debug("Invalid number of parameters for SC")
		common.Log.Debug("Number %d not matching colorspace %T", len(op.Params), cs)
		return errors.New("Invalid number of parameters")
	}

	color, err := cs.ColorFromPdfObjects(op.Params)
	if err != nil {
		return err
	}

	csp.graphicsState.ColorspaceNonStroking = cs
	csp.graphicsState.ColorNonStroking = color

	return nil
}

// cm: concatenates an affine transform to the CTM
func (csp *ContentStreamProcessor) handleCommand_cm(op *ContentStreamOperation,
	resources *PdfPageResources) error {
	if len(op.Params) != 6 {
		common.Log.Debug("Invalid number of parameters for cm: %d", len(op.Params))
		return errors.New("Invalid number of parameters")
	}

	f, err := GetNumbersAsFloat(op.Params)
	if err != nil {
		return err
	}
	m := NewMatrix(f[0], f[1], f[2], f[3], f[4], f[5])
	csp.graphicsState.CTM.Concat(m)

	return nil
}

// Matrix is a linear transform matrix in homogenous coordinates
// PDF coordinate transforms are always affine so we only need 6 of these. See newMatrix
type Matrix [9]float64

// IdentityMatrix returns the identity transform
func IdentityMatrix() Matrix {
	return NewMatrix(1, 0, 0, 1, 0, 0)
}

// NewMatrix returns an affine transform matrix laid out in homogenous coordinates as
//      a  b  0
//      c  d  0
//      tx ty 1
func NewMatrix(a, b, c, d, tx, ty float64) Matrix {
	m := Matrix{
		a, b, 0,
		c, d, 0,
		tx, ty, 1,
	}
	m.fixup()
	return m
}

// String returns a string describing `m`.
func (m Matrix) String() string {
	a, b, c, d, tx, ty := m[0], m[1], m[3], m[4], m[6], m[7]
	return fmt.Sprintf("MATRIX{%5.1f,%5.1f,%5.1f,%5.1f: %5.1f,%5.1f}", a, b, c, d, tx, ty)
}

// Set sets `m` to affine transform {a,b,c,d,tx,ty}.
func (m *Matrix) Set(a, b, c, d, tx, ty float64) {
	m[0], m[1] = a, b
	m[3], m[4] = c, d
	m[6], m[7] = tx, ty
	m.fixup()
}

// Concat sets `m` to `m` × `b`.
// `b` needs to be created by newMatrix. i.e. It must be an affine transform.
func (m *Matrix) Concat(b Matrix) {
	*m = Matrix{
		m[0]*b[0] + m[1]*b[3], m[0]*b[1] + m[1]*b[4], 0,
		m[3]*b[0] + m[4]*b[3], m[3]*b[1] + m[4]*b[4], 0,
		m[6]*b[0] + m[7]*b[3] + b[6], m[6]*b[1] + m[7]*b[4] + b[7], 1,
	}
	m.fixup()
}

// Translate appends a translation of `dx`,`dy` to `m`.
// m.Translate(dx, dy) is equivalent to m.Concat(NewMatrix(1, 0, 0, 1, dx, dy)).
// !@#$ Use functional style.
func (m *Matrix) Translate(dx, dy float64) {
	m[6] += dx
	m[7] += dy
	m.fixup()
}

// Transform returns coordinates `x`,`y` transformed by `m`.
func (m *Matrix) Transform(x, y float64) (float64, float64) {
	xp := x*m[0] + y*m[1] + m[6]
	yp := x*m[3] + y*m[4] + m[7]
	return xp, yp
}

// pageOrientation returns a guess at the pdf page orientation when text is printed with CTM `m`
// XXX: Use pageRotate flag instead !@#$
func (m *Matrix) pageOrientation() Orientation {
	switch {
	case m[1]*m[1]+m[3]*m[3] > m[0]*m[0]+m[4]*m[4]:
		return OrientationLandscape
	default:
		return OrientationPortrait
	}
}

// fixup forces `m` to have reasonable values. It is a guard against crazy values in corrupt PDF
// files.
// Currently it clamps elements to [-maxAbsNumber, -maxAbsNumber] to avoid floating point exceptions
func (m *Matrix) fixup() {
	for i, x := range m {
		if x > maxAbsNumber {
			m[i] = maxAbsNumber
		} else if x < -maxAbsNumber {
			m[i] = -maxAbsNumber
		}
	}
}

// maxAbsNumber is the largest number needed in PDF transforms. Is this correct?
const maxAbsNumber = 1e9
