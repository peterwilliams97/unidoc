/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

type cmapObject interface {
}

type cmapName struct {
	Name string
}

type cmapOperand struct {
	Operand string
}

type cmapFloat struct {
	val float64
}

type cmapInt struct {
	val int64
}

type cmapString struct {
	String string
}

// cmapHexString represents a PostScript hex string such as <FFFF>
type cmapHexString struct {
	b []byte
}

type cmapArray struct {
	Array []cmapObject
}

type cmapDict struct {
	Dict map[string]cmapObject
}

func makeDict() cmapDict {
	return cmapDict{Dict: map[string]cmapObject{}}
}
