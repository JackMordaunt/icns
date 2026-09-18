package icns

import (
	"fmt"
	"regexp"
	"strconv"
)

// Slot identifies an icon by the size it is drawn at and the display scale it
// is drawn for, the way an iconset names its files. Artwork for 16x16@2x and
// for 32x32 is 32 pixels either way, but the two fill different slots and
// need not be the same drawing.
type Slot struct {
	// Points is the size the icon is drawn at.
	Points uint
	// Scale is the display scale, 1 for a plain display and 2 for retina.
	Scale uint
}

// Pixels returns the dimensions artwork for this slot has.
func (s Slot) Pixels() uint {
	return s.Points * s.Scale
}

// String renders the slot the way an iconset names it, such as "16x16@2x".
func (s Slot) String() string {
	if s.Scale > 1 {
		return fmt.Sprintf("%dx%d@%dx", s.Points, s.Points, s.Scale)
	}
	return fmt.Sprintf("%dx%d", s.Points, s.Points)
}

// Slots returns the slots an encoded icns holds, largest artwork first.
func Slots() []Slot {
	var slots []Slot
	for _, size := range sizes {
		types, ok := getTypesFromSize(size)
		if !ok {
			continue
		}
		for _, t := range types {
			slots = append(slots, t.slot)
		}
	}
	return slots
}

// iconsetName matches the file names iconutil accepts in an iconset.
var iconsetName = regexp.MustCompile(`^icon_(\d+)x(\d+)(?:@(\d+)x)?\.png$`)

// ParseSlot reads an iconset file name, such as "icon_16x16@2x.png". The
// boolean reports whether the name is one.
func ParseSlot(name string) (Slot, bool) {
	match := iconsetName.FindStringSubmatch(name)
	if match == nil {
		return Slot{}, false
	}
	width, err := strconv.ParseUint(match[1], 10, 32)
	if err != nil {
		return Slot{}, false
	}
	height, err := strconv.ParseUint(match[2], 10, 32)
	if err != nil || width != height || width == 0 {
		return Slot{}, false
	}
	scale := uint64(1)
	if match[3] != "" {
		if scale, err = strconv.ParseUint(match[3], 10, 32); err != nil || scale == 0 {
			return Slot{}, false
		}
	}
	return Slot{Points: uint(width), Scale: uint(scale)}, true
}
