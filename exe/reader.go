package exe

import (
	"bytes"
	"cmp"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"slices"

	"github.com/jackmordaunt/icns/v4/ico"
)

// resourceSection is the PE section a binary keeps its resources in.
const resourceSection = ".rsrc"

// largestSide is the biggest icon a directory can describe. It does not fit
// in the byte a row stores a side in, so a stored zero stands for it.
const largestSide = 256

// side reads a dimension from a directory row, where the largest icon is
// written as zero.
func side(stored byte) int {
	if stored == 0 {
		return largestSide
	}
	return int(stored)
}

// Identify reports what a Windows binary is, and whether the data is one at
// all. The two bytes such a file begins with are shared with the formats it
// succeeded, so the header decides rather than the magic, and whether it is a
// library is what the header says rather than what the name suggests.
func Identify(r io.ReaderAt) (Kind, bool) {
	file, err := pe.NewFile(r)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	if file.FileHeader.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return Library, true
	}
	return Program, true
}

// Icons returns the icons a Windows binary carries, lowest ordinal first,
// which is the order Explorer draws them in.
func Icons(r io.ReaderAt) ([]Group, error) {
	file, err := pe.NewFile(r)
	if err != nil {
		return nil, fmt.Errorf("reading binary: %w", err)
	}
	defer file.Close()
	section := file.Section(resourceSection)
	if section == nil {
		return nil, ErrNoIcons
	}
	data, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("reading resource section: %w", err)
	}
	// Section.Data returns the bytes on disk, which may be padded out past
	// the size the section declares.
	if int(section.VirtualSize) < len(data) {
		data = data[:section.VirtualSize]
	}
	res := resources{data: data, base: section.VirtualAddress}
	images, err := res.leaves(typeIcon)
	if err != nil {
		return nil, err
	}
	groups, err := res.leaves(typeIconGroup)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, ErrNoIcons
	}
	out := make([]Group, 0, len(groups))
	for _, entry := range groups {
		group, err := assemble(entry, images)
		if err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	slices.SortStableFunc(out, func(a, b Group) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return out, nil
}

// Decode returns the largest icon in the binary that can be decoded.
func Decode(r io.ReaderAt) (image.Image, error) {
	groups, err := Icons(r)
	if err != nil {
		return nil, err
	}
	return groups[0].Decode()
}

// Decode returns the largest icon in the group that can be decoded.
func (g Group) Decode() (image.Image, error) {
	return ico.Decode(bytes.NewReader(g.ico))
}

// leaf is one resource: the ordinal it is filed under and its bytes.
type leaf struct {
	id   uint16
	data []byte
}

// resources walks the tree in a resource section. The tree is three levels
// deep, by type, then by name or ordinal, then by language, and every offset
// inside it is measured from the start of the section.
type resources struct {
	data []byte
	base uint32
}

// leaves returns every resource of a type, taking the first language of each.
func (res resources) leaves(kind uint32) ([]leaf, error) {
	types, err := res.entries(0)
	if err != nil {
		return nil, err
	}
	var out []leaf
	for _, t := range types {
		if t.name != kind || !t.directory {
			continue
		}
		named, err := res.entries(t.offset)
		if err != nil {
			return nil, err
		}
		for _, n := range named {
			if !n.directory {
				continue
			}
			languages, err := res.entries(n.offset)
			if err != nil {
				return nil, err
			}
			for _, l := range languages {
				if l.directory {
					continue
				}
				data, err := res.at(l.offset)
				if err != nil {
					return nil, err
				}
				out = append(out, leaf{id: uint16(n.name), data: data})
				// One language is enough: the images are the same icon.
				break
			}
		}
	}
	return out, nil
}

// entry is one row of a resource directory.
type entry struct {
	// name is the ordinal the resource is filed under, or the offset of its
	// name when it has one rather than a number.
	name uint32
	// offset is where the row points, from the start of the section.
	offset uint32
	// directory reports whether the row points at another directory rather
	// than at the bytes of a resource.
	directory bool
}

// The two uses of a directory entry's high bit, which the rest of the field
// is measured from.
const (
	// nameFlag marks a name held as an offset into the string table rather
	// than an ordinal, which icons are not filed under.
	nameFlag = 0x80000000
	// directoryFlag marks a row that points at another directory rather
	// than at the bytes of a resource.
	directoryFlag = 0x80000000
)

// entries reads the rows of the resource directory at offset.
func (res resources) entries(offset uint32) ([]entry, error) {
	if int(offset)+directoryHeaderSize > len(res.data) {
		return nil, fmt.Errorf("%w: a directory lies at %d, outside the section", ErrMalformed, offset)
	}
	var (
		header = res.data[offset:]
		named  = int(binary.LittleEndian.Uint16(header[12:14]))
		ids    = int(binary.LittleEndian.Uint16(header[14:16]))
		count  = named + ids
		at     = int(offset) + directoryHeaderSize
	)
	if at+count*directoryEntrySize > len(res.data) {
		return nil, fmt.Errorf("%w: a directory of %d entries runs past the section", ErrMalformed, count)
	}
	out := make([]entry, 0, count)
	for i := range count {
		row := res.data[at+i*directoryEntrySize:]
		var (
			name   = binary.LittleEndian.Uint32(row[0:4])
			target = binary.LittleEndian.Uint32(row[4:8])
		)
		out = append(out, entry{
			name:      name &^ nameFlag,
			offset:    target &^ directoryFlag,
			directory: target&directoryFlag != 0,
		})
	}
	return out, nil
}

// at reads the resource the data entry at offset points to. The entry holds
// an address in the loaded image, which the section's own address turns back
// into a position in the file.
func (res resources) at(offset uint32) ([]byte, error) {
	if int(offset)+dataEntrySize > len(res.data) {
		return nil, fmt.Errorf("%w: a data entry lies at %d, outside the section", ErrMalformed, offset)
	}
	var (
		row     = res.data[offset:]
		address = binary.LittleEndian.Uint32(row[0:4])
		size    = binary.LittleEndian.Uint32(row[4:8])
	)
	if address < res.base {
		return nil, fmt.Errorf("%w: a resource lies at %d, before the section", ErrMalformed, address)
	}
	start := address - res.base
	if int(start)+int(size) > len(res.data) {
		return nil, fmt.Errorf("%w: a resource of %d bytes at %d runs past the section", ErrMalformed, size, start)
	}
	return res.data[start : start+size], nil
}

// assemble turns a group icon directory and the images it names back into an
// ico file. The two differ only in the last field of a row, where the group
// names a resource and an ico gives the position of the image.
//
// TODO(jfm): can we normalize the representation of the icon and dry up ico.Icon + ico.entry?
// Perhaps not, but surely the PE cannot specify data that an .ico cannot? So why does ico.entry
// have less fields than ico.Icon?
func assemble(group leaf, images []leaf) (Group, error) {
	if len(group.data) < groupHeaderSize {
		return Group{}, fmt.Errorf("%w: icon group %d holds %d bytes", ErrMalformed, group.id, len(group.data))
	}
	count := int(binary.LittleEndian.Uint16(group.data[4:6]))
	if groupHeaderSize+count*groupEntrySize > len(group.data) {
		return Group{}, fmt.Errorf("%w: icon group %d lists %d icons it does not hold", ErrMalformed, group.id, count)
	}
	var (
		stored = make([]ico.Icon, 0, count)
		sizes  []int
	)
	for i := range count {
		row := group.data[groupHeaderSize+i*groupEntrySize:]
		id := binary.LittleEndian.Uint16(row[12:14])
		index := slices.IndexFunc(images, func(l leaf) bool { return l.id == id })
		if index < 0 {
			return Group{}, fmt.Errorf("%w: icon group %d names image %d, which is not present", ErrMalformed, group.id, id)
		}
		// A group row says everything an ico row says except where the image
		// lies, which it answers with a resource instead. The length is taken
		// from the resource rather than from the field that names it.
		icon := ico.Icon{
			Width:   side(row[0]),
			Height:  side(row[1]),
			Colours: row[2],
			Planes:  binary.LittleEndian.Uint16(row[4:6]),
			Bits:    binary.LittleEndian.Uint16(row[6:8]),
			Data:    images[index].data,
		}
		stored = append(stored, icon)
		sizes = append(sizes, icon.Width)
	}
	file, err := ico.Assemble(stored)
	if err != nil {
		return Group{}, fmt.Errorf("icon group %d: %w", group.id, err)
	}
	slices.SortStableFunc(sizes, func(a, b int) int { return cmp.Compare(b, a) })
	return Group{
		ID:    group.id,
		Sizes: sizes,
		ico:   file,
	}, nil
}
