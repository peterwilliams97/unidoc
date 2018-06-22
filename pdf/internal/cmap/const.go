/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package cmap

import "regexp"

const (
	cisSystemInfo       = "CIDSystemInfo"
	begincmap           = "begincmap"
	endcmap             = "endcmap"
	begincodespacerange = "begincodespacerange"
	endcodespacerange   = "endcodespacerange"
	beginbfchar         = "beginbfchar"
	endbfchar           = "endbfchar"
	beginbfrange        = "beginbfrange"
	endbfrange          = "endbfrange"
	begincidrange       = "begincidrange"
	endcidrange         = "endcidrange"
	usecmap             = "usecmap"

	cmapname = "CMapName"
	cmaptype = "CMapType"
)

var reNumeric = regexp.MustCompile(`^[\+-.]*([0-9.]+)`)
