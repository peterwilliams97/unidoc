/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/disintegration/imaging"
	"github.com/unidoc/unidoc/common"
	"github.com/unidoc/unidoc/pdf/contentstream"
	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/internal/transform"
	"github.com/unidoc/unidoc/pdf/model"
)

// ImageExtractOptions contains options for controlling image extraction from
// PDF pages.
type ImageExtractOptions struct {
	IncludeInlineStencilMasks bool
}

// ExtractPageImages returns the image contents of the page extractor, including data
// and position, size information for each image.
// A set of options to control page image extraction can be passed in. The options
// parameter can be nil for the default options. By default, inline stencil masks
// are not extracted.
func (e *Extractor) ExtractPageImages(options *ImageExtractOptions) (*PageImages, error) {
	ctx := &imageExtractContext{
		options: options,
	}

	err := ctx.extractContentStreamImages(e.contents, e.resources, 0)
	if err != nil {
		return nil, err
	}

	return &PageImages{
		Images: ctx.extractedImages,
	}, nil
}

// PageImages represents extracted images on a PDF page with spatial information:
// display position and size.
type PageImages struct {
	Images []ImageMark
}

// ImageMark represents an image drawn on a page and its position in device coordinates.
// All coordinates are in device coordinates.
type ImageMark struct {
	Image  *model.Image
	CTM    transform.Matrix
	Inline bool
	Lossy  bool
}

// String returns a string describing `mark`.
func (mark ImageMark) String() string {
	img := mark.Image
	imgStr := fmt.Sprintf("%dx%d cpts=%d bpp=%d",
		img.Width, img.Height, img.ColorComponents, img.BitsPerComponent)
	ctm := mark.CTM
	tx, ty := ctm.Translation()
	ctmStr := fmt.Sprintf("scale=(%.1fx%.1f) ϴ=%.1f° translation=(%.1f,%.1f)",
		ctm.ScalingFactorX(), ctm.ScalingFactorY(), ctm.Angle(), tx, ty)
	return fmt.Sprintf("%s %s %s lossy=%t inline=%t", imgStr, ctm, ctmStr, mark.Lossy, mark.Inline)
}

// Clip returns `mark`.Image clipped to `box`.
// TODO(peterwilliams): Return image in orginal colorspace. The github.com/disintegration/imaging
// library we are using converts all images to image.NRGBA.
// This function can be used to clip extracted images the same way they are clipped in the PDF they
// are extracted from to give the same image the user sees in the enclosing PDF.
func (mark ImageMark) Clip(box model.PdfRectangle, doClip bool) (*image.NRGBA, error) {
	inv, hasInverse := mark.CTM.Inverse()
	if !hasInverse {
		return nil, errors.New("CTM has no inverse")
	}
	clp := model.PdfRectangle{}
	clp.Llx, clp.Lly = inv.Transform(box.Llx, box.Lly)
	clp.Urx, clp.Ury = inv.Transform(box.Urx, box.Ury)
	clp.Llx, clp.Lly = maxFloat(0, clp.Llx), maxFloat(0, clp.Lly)
	clp.Urx, clp.Ury = minFloat(1, clp.Urx), minFloat(1, clp.Ury)

	if !doClip {
		clp = model.PdfRectangle{Llx: 0, Lly: 0, Urx: 1, Ury: 1}
	}

	img, err := mark.Image.ToGoImage()
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w := float64(b.Max.X - b.Min.X)
	h := float64(b.Max.Y - b.Min.Y)

	rect := image.Rectangle{
		Min: image.Point{
			X: round(w * clp.Llx),
			Y: round(h * clp.Lly),
		},
		Max: image.Point{
			X: round(w * clp.Urx),
			Y: round(h * clp.Ury),
		},
	}

	imgRgb := imaging.Crop(img, rect)
	return imgRgb, nil
}

// PageView returns `mark`.Image transformed to appear as it appears the PDF page it was extracted
// from.
//    `bbox` is a clipping rectangle. It should be the clipping path in effect when the image was
//          rendered. TODO(peterwilliams97) support non-rectangular clipping paths.
//    If `doScale` is true the image is scaled as it is on the PDF page. `doScale` will typically
//          only be set false for debugging to check if the scaling is correct.
func (mark ImageMark) PageView(bbox model.PdfRectangle, doScale, doRotate, doClip bool) (*image.NRGBA, error) {
	img, err := mark.Clip(bbox, doClip)
	if err != nil {
		return nil, err
	}

	ctm := mark.CTM
	bgColor := color.White
	img = imaging.Rotate(img, -ctm.Angle(), bgColor)

	if doScale {
		wi, hi := int(mark.Image.Width), int(mark.Image.Height)
		wf, hf := float64(wi), float64(hi)
		ws, hs := ctm.ScalingFactorX(), ctm.ScalingFactorY()
		if ws*hf != wf*hs {
			if ws*hf > wf*hs {
				wi = round(hf * (ws / hs))
			} else {
				hi = round(wf * (hs / ws))
			}
			img = imaging.Resize(img, wi, hi, imaging.CatmullRom)
		}
	}

	if doRotate {
		theta := mark.CTM.Angle()
		if theta != 0 {
			common.Log.Trace("PageView: theta=%3g° Bounds=%+v", theta, img.Bounds())
			img = imaging.Rotate(img, 360-theta, color.Black)
			common.Log.Trace("PageView: After rotation. Bounds=%+v", img.Bounds())
		}
	}

	return img, nil
}

// round returns `x` rounded the nearest int.
func round(x float64) int {
	return int(math.Round(x))
}

// round64 returns `x` rounded the nearest int64.
func round64(x float64) int64 {
	return int64(math.Round(x))
}

// Provide context for image extraction content stream processing.
type imageExtractContext struct {
	extractedImages []ImageMark
	inlineImages    int
	xObjectImages   int
	xObjectForms    int

	// Cache to avoid processing same image many times.
	cacheXObjectImages map[*core.PdfObjectStream]*cachedImage

	// Extract options.
	options *ImageExtractOptions
}

type cachedImage struct {
	image *model.Image
	cs    model.PdfColorspace
	enc   core.StreamEncoder
}

func (ctx *imageExtractContext) extractContentStreamImages(contents string,
	resources *model.PdfPageResources, level int) error {

	cstreamParser := contentstream.NewContentStreamParser(contents)
	operations, err := cstreamParser.Parse()
	if err != nil {
		return err
	}

	if ctx.cacheXObjectImages == nil {
		ctx.cacheXObjectImages = map[*core.PdfObjectStream]*cachedImage{}
	}
	if ctx.options == nil {
		ctx.options = &ImageExtractOptions{}
	}

	processor := contentstream.NewContentStreamProcessor(*operations)
	processor.AddHandler(contentstream.HandlerConditionEnumAllOperands, "",
		func(op *contentstream.ContentStreamOperation, gs contentstream.GraphicsState,
			resources *model.PdfPageResources) error {
			return ctx.processOperand(op, gs, resources)
		})

	return processor.Process(resources)
}

// Process individual content stream operands for image extraction.
func (ctx *imageExtractContext) processOperand(op *contentstream.ContentStreamOperation,
	gs contentstream.GraphicsState, resources *model.PdfPageResources) error {
	if op.Operand == "BI" && len(op.Params) == 1 {
		// BI: Inline image.
		iimg, ok := op.Params[0].(*contentstream.ContentStreamInlineImage)
		if !ok {
			return nil
		}

		if isImageMask, ok := core.GetBoolVal(iimg.ImageMask); ok {
			if isImageMask && !ctx.options.IncludeInlineStencilMasks {
				return nil
			}
		}

		return ctx.extractInlineImage(iimg, gs, resources)

	} else if op.Operand == "Do" && len(op.Params) == 1 {
		// Do: XObject.
		name, ok := core.GetName(op.Params[0])
		if !ok {
			common.Log.Debug("ERROR: Type")
			return errTypeCheck
		}

		_, xtype := resources.GetXObjectByName(*name)
		switch xtype {
		case model.XObjectTypeImage:
			return ctx.extractXObjectImage(name, gs, resources)
		case model.XObjectTypeForm:
			return ctx.extractFormImages(name, gs, resources)
		}
	}
	return nil
}

func (ctx *imageExtractContext) extractInlineImage(iimg *contentstream.ContentStreamInlineImage,
	gs contentstream.GraphicsState, resources *model.PdfPageResources) error {
	img, err := iimg.ToImage(resources)
	if err != nil {
		return err
	}

	cs, err := iimg.GetColorSpace(resources)
	if err != nil {
		return err
	}
	if cs == nil {
		// Default if not specified?
		cs = model.NewPdfColorspaceDeviceGray()
	}

	lossy := contentstream.IsIILossy(iimg)

	imgMark := ImageMark{Image: img, CTM: gs.CTM, Lossy: lossy, Inline: true}

	ctx.extractedImages = append(ctx.extractedImages, imgMark)
	ctx.inlineImages++
	return nil
}

func (ctx *imageExtractContext) extractXObjectImage(name *core.PdfObjectName,
	gs contentstream.GraphicsState, resources *model.PdfPageResources) error {

	stream, _ := resources.GetXObjectByName(*name)
	if stream == nil {
		return nil
	}

	// Cache on stream pointer so can ensure that it is the same object (better than using name).
	cimg, cached := ctx.cacheXObjectImages[stream]
	if !cached {
		ximg, err := resources.GetXObjectImageByName(*name)
		if err != nil {
			return err
		}
		if ximg == nil {
			return nil
		}

		img, err := ximg.ToImage()
		if err != nil {
			return err
		}

		cimg = &cachedImage{
			image: img,
			cs:    ximg.ColorSpace,
			enc:   ximg.Filter,
		}
		ctx.cacheXObjectImages[stream] = cimg
	}
	img := cimg.image

	lossy := core.IsLossy(cimg.enc)

	common.Log.Debug("@Do CTM: %s", gs.CTM.String())
	imgMark := ImageMark{Image: img, CTM: gs.CTM, Lossy: lossy, Inline: false}
	ctx.extractedImages = append(ctx.extractedImages, imgMark)
	ctx.xObjectImages++
	return nil
}

// Go through the XObject Form content stream (recursive processing).
func (ctx *imageExtractContext) extractFormImages(name *core.PdfObjectName,
	gs contentstream.GraphicsState, resources *model.PdfPageResources) error {

	xform, err := resources.GetXObjectFormByName(*name)
	if err != nil {
		return err
	}
	if xform == nil {
		return nil
	}

	formContent, err := xform.GetContentStream()
	if err != nil {
		return err
	}

	// Process the content stream in the Form object too:
	formResources := xform.Resources
	if formResources == nil {
		formResources = resources
	}

	// Process the content stream in the Form object too:
	err = ctx.extractContentStreamImages(string(formContent), formResources, 1)
	if err != nil {
		return err
	}
	ctx.xObjectForms++
	return nil
}
