// Package fax supports CCITT Group 4 image decompression
// as described by ITU-T Recommendation T.6.
// See http://www.itu.int/rec/T-REC-T.6-198811-I
package got6

import (
	"errors"
	"image"
	"io"
)

const (
	white = 0xFF
	black = 0x00
)

var negativeWidth = errors.New("fax: negative width specified")

// DecodeG4 parses a Group 4 fax image from reader.
// The width will be applied as specified and the
// (estimated) height helps memory allocation.
func DecodeG4(reader io.ByteReader, width, height int) (image.Image, error) {
	if width < 0 {
		return nil, negativeWidth
	}
	if width == 0 {
		return new(image.Gray), nil
	}
	if height <= 0 {
		height = width
	}
	// include imaginary first line
	height++
	pixels := make([]byte, width, width*height)
	for i := width - 1; i >= 0; i-- {
		pixels[i] = white
	}

	d := &decoder{
		reader:    reader,
		pixels:    pixels,
		width:     width,
		atNewLine: true,
		color:     white,
	}

	// initiate d.head
	if err := d.pop(0); err != nil {
		return nil, err
	}

	return d.parse()
}

type decoder struct {
	// reader is the data source
	reader io.ByteReader

	// head contains the current data in the stream.
	// The first upcoming bit is packed in the 32nd bit, the
	// second upcoming bit in the 31st, etc.
	head uint

	// bitCount is the number of bits loaded in head.
	bitCount uint

	// pixels are black and white values.
	pixels []byte

	// width is the line length in pixels.
	width int

	// atNewLine is whether a0 is before the beginning of a line.
	atNewLine bool

	// color represents the state of a0.
	color byte
}

// pop advances n bits in the stream.
// The 24-bit end-of-facsimile block exceeds all
// Huffman codes in length.
func (d *decoder) pop(n uint) error {
	head := d.head
	count := d.bitCount
	head <<= n
	count -= n
	for count < 24 {
		next, err := d.reader.ReadByte()
		if err != nil {
			return err
		}
		head |= uint(next) << (24 - count)
		count += 8
	}
	d.head = head
	d.bitCount = count
	return nil
}

// paint adds n d.pixels in the specified color.
func (d *decoder) paint(n int, color byte) {
	a := d.pixels
	for ; n != 0; n-- {
		a = append(a, color)
	}
	d.pixels = a
}

func (d *decoder) parse() (result image.Image, err error) {
	// parse until end-of-facsimile block: 0x001001
	for d.head&0xFE000000 != 0 && err == nil {
		i := (d.head >> 28) & 0xF
		err = modeTable[i](d)
	}

	width := d.width
	pixels := d.pixels[width:] // strip imaginary line
	bounds := image.Rect(0, 0, width, len(pixels)/width)
	result = &image.Gray{pixels, width, bounds}
	return
}

var modeTable = [16]func(d *decoder) error{
	func(d *decoder) error {
		i := (d.head >> 25) & 7
		return modeTable2[i](d)
	},
	pass,
	horizontal,
	horizontal,
	verticalLeft1,
	verticalLeft1,
	verticalRight1,
	verticalRight1,
	vertical0, vertical0, vertical0, vertical0,
	vertical0, vertical0, vertical0, vertical0,
}

var modeTable2 = [8]func(d *decoder) error{
	nil,
	extension,
	verticalLeft3,
	verticalRight3,
	verticalLeft2,
	verticalLeft2,
	verticalRight2,
	verticalRight2,
}

func pass(d *decoder) error {
	if e := d.pop(4); e != nil {
		return e
	}

	color := d.color
	width := d.width
	pixels := d.pixels
	a := len(pixels)
	lineStart := (a / width) * width
	b := a - width // reference element
	if !d.atNewLine {
		for b != lineStart && pixels[b] != color {
			b++
		}
	}
	for b != lineStart && pixels[b] == color {
		b++
	}
	// found b1
	for b != lineStart && pixels[b] != color {
		b++
	}
	// found b2

	if b == lineStart {
		d.atNewLine = true
		d.color = white
	} else {
		d.atNewLine = false
	}
	d.paint(b-a+width, color)
	return nil
}

func vertical0(d *decoder) error {
	d.vertical(0)
	return d.pop(1)
}

func verticalLeft1(d *decoder) error {
	d.vertical(-1)
	return d.pop(3)
}

func verticalLeft2(d *decoder) error {
	d.vertical(-2)
	return d.pop(6)
}

func verticalLeft3(d *decoder) error {
	d.vertical(-3)
	return d.pop(7)
}

func verticalRight1(d *decoder) error {
	d.vertical(1)
	return d.pop(3)
}

func verticalRight2(d *decoder) error {
	d.vertical(2)
	return d.pop(6)
}

func verticalRight3(d *decoder) error {
	d.vertical(3)
	return d.pop(7)
}

func (d *decoder) vertical(offset int) {
	color := d.color
	width := d.width
	pixels := d.pixels
	a := len(pixels)
	lineStart := (a / width) * width
	b := a - width // reference element
	if !d.atNewLine {
		for b != lineStart && pixels[b] != color {
			b++
		}
	}
	for b != lineStart && pixels[b] == color {
		b++
	}
	// found b1

	b += offset
	if b >= lineStart {
		b = lineStart
		d.atNewLine = true
		d.color = white
	} else {
		d.atNewLine = false
		d.color = color ^ 0xFF
	}
	if count := b - a + width; count >= 0 {
		d.paint(count, color)
	}
}

func horizontal(d *decoder) (err error) {
	if err = d.pop(3); err != nil {
		return
	}

	color := d.color
	flip := color ^ 0xFF
	var rl1, rl2 int
	if rl1, err = d.runLength(color); err == nil {
		rl2, err = d.runLength(flip)
	}

	// pixels left in the line:
	remaining := d.width - (len(d.pixels) % d.width)
	if rl1 > remaining {
		rl1 = remaining
	}
	d.paint(rl1, color)
	remaining -= rl1
	if rl2 >= remaining {
		rl2 = remaining
		d.atNewLine = true
		d.color = white
	} else {
		d.atNewLine = false
	}
	d.paint(rl2, flip)
	return
}

// runLength reads the amount of pixels for a color.
func (d *decoder) runLength(color byte) (count int, err error) {
	match := uint16(0xFFFF) // lookup entry
	for match&0xFC0 != 0 && err == nil {
		if color == black {
			if d.head&0xF0000000 != 0 {
				match = blackShortLookup[(d.head>>26)&0x3F]
			} else if d.head&0xFE000000 != 0 {
				match = blackLookup[(d.head>>19)&0x1FF]
			} else {
				match = sharedLookup[(d.head>>20)&0x1F]
			}
		} else {
			if d.head&0xFE000000 != 0 {
				match = whiteLookup[(d.head>>23)&0x1FF]
			} else {
				match = sharedLookup[(d.head>>20)&0x1F]
			}
		}

		err = d.pop(uint(match) >> 12)
		count += int(match) & 0x0FFF
	}
	return
}

// Lookup tables are used by runLength to find Huffman codes. Their index
// size is large enough to fit the longest code in the group. Shorter codes
// have duplicate entries with all possible tailing bits.
// Entries consist of two parts. The 4 most significant bits contain the
// Huffman code length in bits and the 12 least significant bits contain
// the pixel count.

var blackShortLookup = [64]uint16{
	0x0, 0x0, 0x0, 0x0, 0x6009, 0x6008, 0x5007, 0x5007,
	0x4006, 0x4006, 0x4006, 0x4006, 0x4005, 0x4005, 0x4005, 0x4005,
	0x3001, 0x3001, 0x3001, 0x3001, 0x3001, 0x3001, 0x3001, 0x3001,
	0x3004, 0x3004, 0x3004, 0x3004, 0x3004, 0x3004, 0x3004, 0x3004,
	0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003,
	0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003, 0x2003,
	0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002,
	0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002, 0x2002,
}

var blackLookup = [512]uint16{
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0xa012, 0xa012, 0xa012, 0xa012, 0xa012, 0xa012, 0xa012, 0xa012,
	0xc034, 0xc034, 0xd280, 0xd2c0, 0xd300, 0xd340, 0xc037, 0xc037,
	0xc038, 0xc038, 0xd500, 0xd540, 0xd580, 0xd5c0, 0xc03b, 0xc03b,
	0xc03c, 0xc03c, 0xd600, 0xd640, 0xb018, 0xb018, 0xb018, 0xb018,
	0xb019, 0xb019, 0xb019, 0xb019, 0xd680, 0xd6c0, 0xc140, 0xc140,
	0xc180, 0xc180, 0xc1c0, 0xc1c0, 0xd200, 0xd240, 0xc035, 0xc035,
	0xc036, 0xc036, 0xd380, 0xd3c0, 0xd400, 0xd440, 0xd480, 0xd4c0,
	0xa040, 0xa040, 0xa040, 0xa040, 0xa040, 0xa040, 0xa040, 0xa040,
	0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d,
	0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d,
	0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d,
	0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d, 0x800d,
	0xb017, 0xb017, 0xb017, 0xb017, 0xc032, 0xc032, 0xc033, 0xc033,
	0xc02c, 0xc02c, 0xc02d, 0xc02d, 0xc02e, 0xc02e, 0xc02f, 0xc02f,
	0xc039, 0xc039, 0xc03a, 0xc03a, 0xc03d, 0xc03d, 0xc100, 0xc100,
	0xa010, 0xa010, 0xa010, 0xa010, 0xa010, 0xa010, 0xa010, 0xa010,
	0xa011, 0xa011, 0xa011, 0xa011, 0xa011, 0xa011, 0xa011, 0xa011,
	0xc030, 0xc030, 0xc031, 0xc031, 0xc03e, 0xc03e, 0xc03f, 0xc03f,
	0xc01e, 0xc01e, 0xc01f, 0xc01f, 0xc020, 0xc020, 0xc021, 0xc021,
	0xc028, 0xc028, 0xc029, 0xc029, 0xb016, 0xb016, 0xb016, 0xb016,
	0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e,
	0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e,
	0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e,
	0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e, 0x800e,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a, 0x700a,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b, 0x700b,
	0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f,
	0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f, 0x900f,
	0xc080, 0xc080, 0xc0c0, 0xc0c0, 0xc01a, 0xc01a, 0xc01b, 0xc01b,
	0xc01c, 0xc01c, 0xc01d, 0xc01d, 0xb013, 0xb013, 0xb013, 0xb013,
	0xb014, 0xb014, 0xb014, 0xb014, 0xc022, 0xc022, 0xc023, 0xc023,
	0xc024, 0xc024, 0xc025, 0xc025, 0xc026, 0xc026, 0xc027, 0xc027,
	0xb015, 0xb015, 0xb015, 0xb015, 0xc02a, 0xc02a, 0xc02b, 0xc02b,
	0xa000, 0xa000, 0xa000, 0xa000, 0xa000, 0xa000, 0xa000, 0xa000,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
	0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c, 0x700c,
}

var whiteLookup = [512]uint16{
	0x0, 0x0, 0x0, 0x0, 0x801d, 0x801d, 0x801e, 0x801e,
	0x802d, 0x802d, 0x802e, 0x802e, 0x7016, 0x7016, 0x7016, 0x7016,
	0x7017, 0x7017, 0x7017, 0x7017, 0x802f, 0x802f, 0x8030, 0x8030,
	0x600d, 0x600d, 0x600d, 0x600d, 0x600d, 0x600d, 0x600d, 0x600d,
	0x7014, 0x7014, 0x7014, 0x7014, 0x8021, 0x8021, 0x8022, 0x8022,
	0x8023, 0x8023, 0x8024, 0x8024, 0x8025, 0x8025, 0x8026, 0x8026,
	0x7013, 0x7013, 0x7013, 0x7013, 0x801f, 0x801f, 0x8020, 0x8020,
	0x6001, 0x6001, 0x6001, 0x6001, 0x6001, 0x6001, 0x6001, 0x6001,
	0x600c, 0x600c, 0x600c, 0x600c, 0x600c, 0x600c, 0x600c, 0x600c,
	0x8035, 0x8035, 0x8036, 0x8036, 0x701a, 0x701a, 0x701a, 0x701a,
	0x8027, 0x8027, 0x8028, 0x8028, 0x8029, 0x8029, 0x802a, 0x802a,
	0x802b, 0x802b, 0x802c, 0x802c, 0x7015, 0x7015, 0x7015, 0x7015,
	0x701c, 0x701c, 0x701c, 0x701c, 0x803d, 0x803d, 0x803e, 0x803e,
	0x803f, 0x803f, 0x8000, 0x8000, 0x8140, 0x8140, 0x8180, 0x8180,
	0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a,
	0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a, 0x500a,
	0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b,
	0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b, 0x500b,
	0x701b, 0x701b, 0x701b, 0x701b, 0x803b, 0x803b, 0x803c, 0x803c,
	0x95c0, 0x9600, 0x9640, 0x96c0, 0x7012, 0x7012, 0x7012, 0x7012,
	0x7018, 0x7018, 0x7018, 0x7018, 0x8031, 0x8031, 0x8032, 0x8032,
	0x8033, 0x8033, 0x8034, 0x8034, 0x7019, 0x7019, 0x7019, 0x7019,
	0x8037, 0x8037, 0x8038, 0x8038, 0x8039, 0x8039, 0x803a, 0x803a,
	0x60c0, 0x60c0, 0x60c0, 0x60c0, 0x60c0, 0x60c0, 0x60c0, 0x60c0,
	0x6680, 0x6680, 0x6680, 0x6680, 0x6680, 0x6680, 0x6680, 0x6680,
	0x81c0, 0x81c0, 0x8200, 0x8200, 0x92c0, 0x9300, 0x8280, 0x8280,
	0x8240, 0x8240, 0x9340, 0x9380, 0x93c0, 0x9400, 0x9440, 0x9480,
	0x94c0, 0x9500, 0x9540, 0x9580, 0x7100, 0x7100, 0x7100, 0x7100,
	0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002,
	0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002,
	0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002,
	0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002, 0x4002,
	0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003,
	0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003,
	0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003,
	0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003, 0x4003,
	0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080,
	0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080, 0x5080,
	0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008,
	0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008, 0x5008,
	0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009,
	0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009, 0x5009,
	0x6010, 0x6010, 0x6010, 0x6010, 0x6010, 0x6010, 0x6010, 0x6010,
	0x6011, 0x6011, 0x6011, 0x6011, 0x6011, 0x6011, 0x6011, 0x6011,
	0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004,
	0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004,
	0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004,
	0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004, 0x4004,
	0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005,
	0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005,
	0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005,
	0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005, 0x4005,
	0x600e, 0x600e, 0x600e, 0x600e, 0x600e, 0x600e, 0x600e, 0x600e,
	0x600f, 0x600f, 0x600f, 0x600f, 0x600f, 0x600f, 0x600f, 0x600f,
	0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040,
	0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040, 0x5040,
	0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006,
	0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006,
	0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006,
	0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006, 0x4006,
	0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007,
	0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007,
	0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007,
	0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007, 0x4007,
}

var sharedLookup = [32]uint16{
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0xb700, 0xb700, 0xc7c0, 0xc800, 0xc840, 0xc880, 0xc8c0, 0xc900,
	0xb740, 0xb740, 0xb780, 0xb780, 0xc940, 0xc980, 0xc9c0, 0xca00,
}
