/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package core

import "errors"

var (
	// ErrUnsupportedEncodingParameters error indicates that encoding/decoding was attempted with unsupported
	// encoding parameters.
	// For example when trying to encode with an unsupported Predictor (flate).
	ErrUnsupportedEncodingParameters = errors.New("Unsupported encoding parameters")
	ErrNoCCITTFaxDecode              = errors.New("CCITTFaxDecode encoding is not yet implemented")
	ErrNoJBIG2Decode                 = errors.New("JBIG2Decode encoding is not yet implemented")
	ErrNoJPXDecode                   = errors.New("JPXDecode encoding is not yet implemented")
	ErrNoPdfVersion                  = errors.New("Version not found")
	ErrTypeCheck                     = errors.New("Type check error")
	ErrNotSupported                  = errors.New("Feature not currently supported")
	ErrFontNotSupported              = errors.New("Unsupported font")
	ErrType1CFontNotSupported        = errors.New("Type1C fonts are not currently supported")
	ErrTTCmapNotSupported            = errors.New("Unsupported TrueType cmap format")
)
