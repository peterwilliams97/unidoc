PDF Inference
=============
Extract a list of graphics objects from each PDF page.
There are three types of graphics objects:
* text
* shape  (a PDF path that has been stroked or filled)
* image

Each of these objects has a
* bounding box in page coordinates
* color
* rendering mode (fill, stroke, clip or some combination of these)
* content (e.g. text)

We only record graphics objects that mark the page.

TODO
----
Recurse through XObject forms with a cache
Simplify paths.  PathPoint {x, y, curve}
Efficient Bézier bounding box. https://stackoverflow.com/questions/2587751/an-algorithm-to-find-bounding-box-of-closed-bezier-curves

Questions
---------
Could we do callbacks instead of lists? This would allow downstream code that uses the graphics
objects to exit early. I don't want optimize prematurely so let's not consider this now.


References
----------
(PDF32000_2008.pdf)[https://www.adobe.com/content/dam/acom/en/devnet/pdf/pdfs/PDF32000_2008.pdf] Read section 9

9.3.2 Character Spacing
The character-spacing parameter, Tc, shall be a number specified in unscaled text space units
(although it shall be subject to scaling by the Th parameter if the writing mode is horizontal).
When the glyph for each character in the string is rendered, Tc shall be added to the horizontal
or vertical component of the glyph’s displacement, depending on the writing mode. See 9.2.4,
"Glyph Positioning and Metrics", for a discussion of glyph displacements. In the default
coordinate system, horizontal coordinates increase from left to right and vertical coordinates
from bottom to top. Therefore, for horizontal writing, a positive value of Tc has the effect of
expanding the distance between glyphs (see Figure 41), whereas for vertical writing, a negative
value of Tc has this effect.

9.3.7 Text Rise
Text rise, Trise, shall specify the distance, in unscaled text space units, to move the baseline
up or down from its default location. Positive values of text rise shall move the baseline up.
Figure 45 illustrates the effect of the text rise. Text rise shall apply to the vertical
coordinate in text space, regardless of the writing mode.

9.4.2 Text-Positioning Operators (page 249)
Text space is the coordinate system in which text is shown. It shall be defined by the text
matrix, Tm, and the text state parameters Tfs, Th, and Trise, which together shall determine
the transformation from text space to user space. Specifically, the origin of the first glyph
shown by a text-showing operator shall be placed at the origin of text space. If text space has
been translated, scaled, or rotated, then the position, size, or orientation of the glyph in
user space shall be correspondingly altered.
