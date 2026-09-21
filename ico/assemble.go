package ico

import (
	"encoding/binary"
	"fmt"
)

// Stored is an icon whose pixels are already encoded, as a file or a Windows
// resource holds them.
type Stored struct {
	// Width and Height are the icon's dimensions in pixels, from 1 to 256.
	// The format writes 256 as zero, which this hides.
	Width, Height int
	// Colours is the size of the palette, and zero for the icons that carry
	// no table.
	Colours uint8
	// Planes and Bits are what the directory records about the pixels. They
	// are carried through rather than derived, so a file taken apart keeps
	// what it said about itself.
	Planes, Bits uint16
	// Data is the icon as stored: a whole PNG, or a bitmap followed by its
	// mask.
	Data []byte
}

// Assemble writes icons that are already encoded as an ico file.
//
// The pixels are copied rather than decoded and written again, so a file
// taken apart and put back together is byte for byte the file that went in.
// That is what makes it usable on icons pulled out of somewhere else, such as
// the resources of a Windows binary, where re-encoding would lose whatever
// the original encoder chose.
func Assemble(icons []Stored) ([]byte, error) {
	if len(icons) == 0 {
		return nil, ErrNoIcons
	}
	if len(icons) > maxIcons {
		return nil, fmt.Errorf("%w: %d icons, and a directory counts them in two bytes", ErrMalformed, len(icons))
	}
	body := 0
	for i, icon := range icons {
		if icon.Width < 1 || icon.Width > largest || icon.Height < 1 || icon.Height > largest {
			return nil, fmt.Errorf("%w: icon %d is %dx%d", ErrMalformed, i, icon.Width, icon.Height)
		}
		if len(icon.Data) == 0 {
			return nil, fmt.Errorf("%w: icon %d holds no pixels", ErrMalformed, i)
		}
		body += len(icon.Data)
	}

	var (
		table  = directorySize + entrySize*len(icons)
		out    = make([]byte, 0, table+body)
		offset = table
	)
	out = binary.LittleEndian.AppendUint16(out, 0) // Reserved.
	out = binary.LittleEndian.AppendUint16(out, 1) // An icon, not a cursor.
	out = binary.LittleEndian.AppendUint16(out, uint16(len(icons)))
	for _, icon := range icons {
		out = append(out, side(icon.Width), side(icon.Height), icon.Colours, 0)
		out = binary.LittleEndian.AppendUint16(out, icon.Planes)
		out = binary.LittleEndian.AppendUint16(out, icon.Bits)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(icon.Data)))
		out = binary.LittleEndian.AppendUint32(out, uint32(offset))
		offset += len(icon.Data)
	}
	for _, icon := range icons {
		out = append(out, icon.Data...)
	}
	return out, nil
}

const (
	// largest is the biggest icon a directory can describe, since it records
	// a side in one byte and spends zero on this.
	largest = 256
	// maxIcons is as many as a directory can count.
	maxIcons = 1<<16 - 1
)

// side renders a dimension the way the directory holds it, where the largest
// icon does not fit in a byte and is written as zero.
func side(pixels int) byte {
	if pixels == largest {
		return 0
	}
	return byte(pixels)
}
