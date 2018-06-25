/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import (
	"errors"
	"fmt"
	"io"

	"github.com/unidoc/unidoc/common"
)

// parse parses the CMap file and loads into the CMap structure.
func (cmap *CMap) parse() error {
	inCmap := false
	var prev cmapObject
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			common.Log.Debug("ERROR: parsing CMap: %v", err)
			return err
		}
		// fmt.Printf("-- %#v\n", o)

		switch t := o.(type) {
		case cmapOperand:
			op := t

			switch op.Operand {
			case begincmap:
				inCmap = true
			case endcmap:
				inCmap = false
			case begincodespacerange:
				err := cmap.parseCodespaceRange()
				if err != nil {
					return err
				}
			case beginbfchar:
				err := cmap.parseBfchar()
				if err != nil {
					return err
				}
			case beginbfrange:
				err := cmap.parseBfrange()
				if err != nil {
					return err
				}
			case begincidrange:
				err := cmap.parseCidrange()
				if err != nil {
					return err
				}
			case usecmap:
				if prev == nil {
					common.Log.Debug("ERROR: usecmap with no arg")
					return ErrBadCMap
				}
				name, ok := prev.(cmapName)
				if !ok {
					common.Log.Debug("ERROR: usecmap arg not a name %#v", prev)
					return ErrBadCMap
				}
				cmap.usecmap = name.Name
			case cisSystemInfo:
				// Some PDF generators leave the "/"" off CIDSystemInfo
				// e.g. ~/testdata/459474_809.pdf
				err := cmap.parseSystemInfo()
				if err != nil {
					return err
				}
			}
		case cmapName:
			n := t
			switch n.Name {
			case cisSystemInfo:
				err := cmap.parseSystemInfo()
				if err != nil {
					return err
				}
			case cmapname:
				err := cmap.parseName()
				if err != nil {
					return err
				}
			case cmaptype:
				err := cmap.parseType()
				if err != nil {
					return err
				}
			case cmapversion:
				err := cmap.parseVersion()
				if err != nil {
					return err
				}
			}
		case cmapInt:

		default:
			if inCmap {
				// Don't log this noise for now
				// common.Log.Trace("Unhandled object: %#v", o)
			}
		}
		prev = o
	}


	return nil
}

// parseName parses a cmap name and adds it to `cmap`.
// cmap names are defined like this: /CMapName /83pv-RKSJ-H def
func (cmap *CMap) parseName() error {
	name := ""
	done := false
	for i := 0; i < 10 && !done; i++ {
		o, err := cmap.parseObject()
		if err != nil {
			return err
		}
		// fmt.Printf("^^ %d %#v\n", i, o)
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "def":
				done = true
			default:
				// This is not an error because some PDF files don't have valid PostScript names.
				// e.g. ~/testdata/Papercut vs Equitrac.pdf
				// /CMapName /Adobe-SI-*Courier New-6164-0 def
				// We just append the non-existant operator "New-6164-0" to the name
				common.Log.Debug("parseName: State error. o=%#v name=%#q", o, name)
				if name != "" {
					name = fmt.Sprintf("%s %s", name, t.Operand)
				}
				common.Log.Debug("parseName: Recovered. name=%#q", name)
			}
		case cmapName:
			name = t.Name
		}
	}
	if !done {
		common.Log.Error("parseName: No def. ")
		return errors.New("Bad state")
	}
	cmap.name = name
	return nil
}

// parseType parses a cmap type and adds it to `cmap`.
// cmap names are defined like this: /CMapType 1 def
func (cmap *CMap) parseType() error {

	ctype := 0
	done := false
	for i := 0; i < 3 && !done; i++ {
		o, err := cmap.parseObject()
		if err != nil {
			return err
		}
		// fmt.Printf("^^ %d %#v\n", i, o)
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "def":
				done = true
			default:
				common.Log.Error("parseType: state error. o=%#v", o)
				return ErrBadCMap
			}
		case cmapInt:
			ctype = int(t.val)
		}
	}
	cmap.ctype = ctype
	return nil
}

// parseVersion parses a cmap version and adds it to `cmap`.
// cmap names are defined like this: /CMapType 1 def
// We don't need the version. We do this to eat up the version code in the cmap definition
// to reduce unhandled parse object warnings.
func (cmap *CMap) parseVersion() error {

	version := ""
	done := false
	for i := 0; i < 3 && !done; i++ {
		o, err := cmap.parseObject()
		if err != nil {
			return err
		}
		// fmt.Printf("^^ %d %#v\n", i, o)
		switch t := o.(type) {
		case cmapOperand:
			switch t.Operand {
			case "def":
				done = true
			default:
				common.Log.Debug("ERROR: parseVersion: state error. o=%#v", o)
				return ErrBadCMap
			}
		case cmapInt:
			version = fmt.Sprintf("%d", t.val)
		case cmapFloat:
			version = fmt.Sprintf("%f", t.val)
		case cmapString:
			version = t.String
		default:
			common.Log.Debug("ERROR: parseVersion: Bad type. o=%#v", o)
		}
	}
	cmap.version = version
	return nil
}

// parseSystemInfo parses a cmap CIDSystemInfo and adds it to `cmap`.
// cmap CIDSystemInfo is define like this:
// /CIDSystemInfo 3 dict dup begin
//   /Registry (Adobe) def
//   /Ordering (Japan1) def
//   /Supplement 1 def
// end def
func (cmap *CMap) parseSystemInfo() error {
	inDict := false
	inDef := false
	name := ""
	done := false
	systemInfo := CIDSystemInfo{}

	for i := 0; i < 50 && !done; i++ {
		o, err := cmap.parseObject()
		// fmt.Printf("%2d: %#v\n", i, o)
		if err != nil {
			return err
		}
		switch t := o.(type) {
		case cmapDict:
			d := t.Dict
			r, ok := d["Registry"]
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}
			rr, ok := r.(cmapString)
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}
			systemInfo.Registry = rr.String

			r, ok = d["Ordering"]
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}
			rr, ok = r.(cmapString)
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}

			systemInfo.Ordering = rr.String

			s, ok := d["Supplement"]
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}
			ss, ok := s.(cmapInt)
			if !ok {
				common.Log.Debug("ERROR: Bad System Info")
				return ErrBadCMap
			}
			systemInfo.Supplement = int(ss.val)

			done = true
		case cmapOperand:
			switch t.Operand {
			case "begin":
				inDict = true
			case "end":
				done = true
			case "def":
				inDef = false
			}
		case cmapName:
			if inDict {
				name = t.Name
				inDef = true
			}
		case cmapString:
			if inDef {
				switch name {
				case "Registry":
					systemInfo.Registry = t.String
				case "Ordering":
					systemInfo.Ordering = t.String
				}
			}
		case cmapInt:
			if inDef {
				switch name {
				case "Supplement":
					systemInfo.Supplement = int(t.val)
				}
			}
		}
	}
	if !done {
		common.Log.Debug("ERROR: Parsed System Info dict incorrectly")
		return ErrBadCMap
	}

	cmap.systemInfo = systemInfo
	return nil
}

// parseCodespaceRange parses the codespace range section of a CMap.
func (cmap *CMap) parseCodespaceRange() error {
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		hexLow, ok := o.(cmapHexString)
		if !ok {
			if op, isOperand := o.(cmapOperand); isOperand {
				if op.Operand == endcodespacerange {
					return nil
				}
				return errors.New("Unexpected operand")
			}
		}

		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		hexHigh, ok := o.(cmapHexString)
		if !ok {
			return errors.New("Non-hex high")
		}

		if len(hexLow.b) != len(hexHigh.b) {
			return errors.New("Unequal number of bytes in range")
		}

		low := hexToCharCode(hexLow)
		high := hexToCharCode(hexHigh)
		if high < low {
			common.Log.Debug("ERROR: Bad codespace. low=0x%02x high=0x%02x", low, high)
			return ErrBadCMap
		}
		numBytes := hexHigh.numBytes
		cspace := Codespace{NumBytes: numBytes, Low: low, High: high}
		cmap.codespaces = append(cmap.codespaces, cspace)

		common.Log.Trace("Codespace low: 0x%X, high: 0x%X", low, high)
	}

	if len(cmap.codespaces) == 0 {
		common.Log.Debug("ERROR: No codespaces in cmap.")
		return ErrBadCMap
	}

	return nil
}

// parseBfchar parses a bfchar section of a CMap file.
func (cmap *CMap) parseBfchar() error {
	for {
		// Src code.
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// fmt.Printf("--- %#v\n", o)
		var code CharCode

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			code = hexToCharCode(v)
		default:
			return errors.New("Unexpected type")
		}

		// Target code.
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		target := ""

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfchar {
				return nil
			}
			common.Log.Debug("ERROR: Unexpected operand. %#v", v)
			return ErrBadCMap
		case cmapHexString:
			target = hexToString(v)
		case cmapName:
			common.Log.Debug("ERROR: Unexpected name. %#v", v)
			common.Log.Debug("*** v=%#v", v)
			panic("^^^^^")
			target = "?"
		default:
			common.Log.Debug("ERROR: Unexpected type. %#v", o)
			return ErrBadCMap
		}

		cmap.codeToUnicode[code] = target
	}

	return nil
}

// parseBfrange parses a c section of a CMap file.
func (cmap *CMap) parseBfrange() error {
	// fmt.Println("parseBfrange--------------------------")
	for {
		// The specifications are in triplets.
		// <srcCodeFrom> <srcCodeTo> <target>
		// where target can be either <destFrom> as a hex code, or a list.

		// Src code from.
		var srcCodeFrom CharCode
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// fmt.Printf("-== %#v\n", o)
		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endbfrange {
				return nil
			}
			return errors.New("Unexpected operand")
		case cmapHexString:
			srcCodeFrom = hexToCharCode(v)
		default:
			return errors.New("Unexpected type")
		}

		// Src code to.
		var srcCodeTo CharCode
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch v := o.(type) {
		case cmapOperand:
			common.Log.Debug("ERROR: Imcomplete bfrange triplet")
			return ErrBadCMap
		case cmapHexString:
			srcCodeTo = hexToCharCode(v)
		default:
			common.Log.Debug("ERROR: Unexpected type %T", o)
			return ErrBadCMap
		}

		// target(s).
		o, err = cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch v := o.(type) {
		case cmapArray:
			if len(v.Array) != int(srcCodeTo-srcCodeFrom)+1 {
				common.Log.Debug("ERROR: Invalid number of items in array")
				return ErrBadCMap
			}
			for code := srcCodeFrom; code <= srcCodeTo; code++ {
				o := v.Array[code-srcCodeFrom]
				hexs, ok := o.(cmapHexString)
				if !ok {
					return errors.New("Non-hex string in array")
				}
				s := hexToString(hexs)
				cmap.codeToUnicode[code] = s
			}

		case cmapHexString:
			// <codeFrom> <codeTo> <dst>, maps [from,to] to [dst,dst+to-from].
			target := hexToString(v)
			runes := []rune(target)
			for code := srcCodeFrom; code <= srcCodeTo; code++ {
				cmap.codeToUnicode[code] = string(runes)
				runes[len(runes)-1]++
			}
		default:
			common.Log.Debug("ERROR: Unexpected type %T", o)
			return ErrBadCMap
		}
	}

	return nil
}

// parseCidrange parses a bfrange section of a CMap file.
func (cmap *CMap) parseCidrange() error {
	to := CharCode(0)
	from := CharCode(0)
	cid := CID(0)
	state := 0
	for {
		o, err := cmap.parseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch v := o.(type) {
		case cmapOperand:
			if v.Operand == endcidrange {
				return nil
			}
			common.Log.Debug("ERROR: Unexpected operand %#v", v)
			return ErrBadCMap
		case cmapHexString:
			switch state {
			case 0:
				from = hexToCharCode(v)
				state = 1
			case 1:
				to = hexToCharCode(v)
				state = 3
			default:
				common.Log.Debug("ERROR: Bad state %d", state)
				return ErrBadCMap
			}
		case cmapInt:
			if state != 3 {
				common.Log.Debug("ERROR: Bad state %d", state)
				return ErrBadCMap
			}
			cid = CID(v.val)
			state = 0
			cmap.cidRanges = append(cmap.cidRanges, CIDRange{From: from, To: to, Cid: cid})
		default:
			common.Log.Debug("ERROR: Unexpected type %T", o)
			return ErrBadCMap
		}
	}
	return nil
}
