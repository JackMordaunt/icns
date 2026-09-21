// Package exe reads the icons a Windows executable or DLL carries.
//
// A portable executable keeps its icons in the resource section as two
// resource types that refer to each other: RT_GROUP_ICON holds a directory of
// the sizes one icon is drawn at, and RT_ICON holds the image for each of
// them. A directory and the images it names are an ico file in all but the
// offsets, so this package puts them back together and hands out ico files
// the ico package reads.
//
// A binary may carry several groups. Explorer draws the first by ordinal,
// which is the one Icons returns first.
package exe

import (
	"errors"
	"fmt"
)

// Errors returned by the reader. They are wrapped with detail, so compare
// with errors.Is.
var (
	// ErrNoIcons means the binary carries no icon resources.
	ErrNoIcons = errors.New("no icons found")
	// ErrMalformed means a resource offset or length disagrees with the
	// section holding it.
	ErrMalformed = errors.New("malformed resource section")
)

// Kind is what a Windows binary is.
type Kind int

const (
	// Program is a binary the system runs.
	Program Kind = iota
	// Library is a binary loaded into another, which a DLL is.
	Library
)

func (k Kind) String() string {
	switch k {
	case Program:
		return "program"
	case Library:
		return "library"
	}
	return fmt.Sprintf("unknown kind %d", int(k))
}

// Resource types, as the resource directory numbers them. Windows knows them
// as RT_ICON and RT_GROUP_ICON; a file carries only the numbers. The two are
// not adjacent because a group is numbered eleven above the type it groups,
// the offset winuser.h calls DIFFERENCE, which also turns RT_CURSOR into
// RT_GROUP_CURSOR.
const (
	typeIcon      = 3
	typeIconGroup = 14
)

const (
	// directoryHeaderSize is the fixed part of a resource directory, before
	// its entries.
	directoryHeaderSize = 16
	// directoryEntrySize is one entry in a resource directory.
	directoryEntrySize = 8
	// dataEntrySize is the leaf that points at the bytes of a resource.
	dataEntrySize = 16
	// groupHeaderSize is the fixed part of a group icon directory.
	groupHeaderSize = 6
	// groupEntrySize is one icon's row in a group icon directory. It differs
	// from the ico row only in naming a resource rather than an offset.
	groupEntrySize = 14
)

// Group is one icon a binary carries, at every size it holds.
type Group struct {
	// ID is the ordinal the resource directory gives the group. Explorer
	// draws the lowest.
	ID uint16
	// Sizes are the dimensions the directory lists, largest first.
	Sizes []int

	ico []byte
}

func (g Group) String() string {
	return fmt.Sprintf("icon %d (%d sizes)", g.ID, len(g.Sizes))
}

// ICO returns the group as an ico file, which ico.Decode reads.
//
// The bytes are not copied, and must not be modified.
func (g Group) ICO() []byte {
	return g.ico
}
