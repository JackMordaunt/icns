package icns

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"slices"
	"sync"

	"github.com/jackmordaunt/icns/v4/internal/resample"
)

// Encoder encodes ICNS files from a source image.
type Encoder struct {
	Wr        io.Writer
	Algorithm InterpolationFunction
}

// NewEncoder initialises an encoder.
func NewEncoder(wr io.Writer) *Encoder {
	return &Encoder{
		Wr:        wr,
		Algorithm: MitchellNetravali,
	}
}

// WithAlgorithm applies the interpolation function used to resize the image.
func (enc *Encoder) WithAlgorithm(a InterpolationFunction) *Encoder {
	enc.Algorithm = a
	return enc
}

// Encode icns with the given configuration.
func (enc *Encoder) Encode(img image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	iconset, err := NewIconSet(img, enc.Algorithm)
	if err != nil {
		return err
	}
	_, err = iconset.WriteTo(enc.Wr)
	return err
}

// EncodeSlots icns from artwork supplied per slot, so hand tuned art is used
// where it is given rather than resized from a single source.
func (enc *Encoder) EncodeSlots(images map[Slot]image.Image) error {
	if enc.Wr == nil {
		return errors.New("cannot write to nil writer")
	}
	iconset, err := NewIconSetEncoderFrom(images, enc.Algorithm)
	if err != nil {
		return err
	}
	_, err = iconset.WriteTo(enc.Wr)
	return err
}

// Encode writes img to wr in ICNS format.
// img is assumed to be a rectangle; non-square dimensions will be squared
// without preserving the aspect ratio.
// Uses nearest neighbor as interpolation algorithm.
func Encode(wr io.Writer, img image.Image) error {
	return NewEncoder(wr).Encode(img)
}

// NewIconSet uses the source image to create an IconSet.
// If width != height, the image will be resized using the largest side without
// preserving the aspect ratio.
func NewIconSet(img image.Image, interp InterpolationFunction) (*IconSetEncoder, error) {
	if img == nil {
		return nil, errors.New("cannot encode nil image")
	}
	return newIconSet(nil, img, interp)
}

// NewIconSetEncoderFrom uses artwork supplied per slot to create an IconSet, which is
// what an iconset directory holds. A slot given artwork of the wrong size has
// it resized; a slot given none is filled from the largest image supplied,
// which also sets the largest icon written.
func NewIconSetEncoderFrom(images map[Slot]image.Image, interp InterpolationFunction) (*IconSetEncoder, error) {
	if len(images) == 0 {
		return nil, errors.New("cannot encode without an image")
	}
	slots := make([]Slot, 0, len(images))
	for slot, img := range images {
		if img == nil {
			return nil, fmt.Errorf("cannot encode nil image for %s", slot)
		}
		slots = append(slots, slot)
	}
	// The largest artwork stands in for the slots left empty. Ties are broken
	// by slot so the choice does not depend on map ordering.
	slices.SortFunc(slots, func(a, b Slot) int {
		if order := cmp.Compare(resample.BiggestSide(images[b]), resample.BiggestSide(images[a])); order != 0 {
			return order
		}
		if order := cmp.Compare(b.Points, a.Points); order != 0 {
			return order
		}
		return cmp.Compare(b.Scale, a.Scale)
	})
	return newIconSet(images, images[slots[0]], interp)
}

func newIconSet(images map[Slot]image.Image, source image.Image, interp InterpolationFunction) (*IconSetEncoder, error) {
	biggest := findNearestSize(source)
	if biggest == 0 {
		return nil, ErrImageTooSmall{image: source, need: 16}
	}
	var plan []OsType
	for _, size := range sizesFrom(biggest) {
		types, ok := getTypesFromSize(size)
		if !ok {
			continue
		}
		plan = append(plan, types...)
	}
	icons := make([]*IconEncoder, len(plan))
	work := sync.WaitGroup{}
	for i, osType := range plan {
		work.Go(func() {
			art := source
			if supplied, ok := images[osType.slot]; ok {
				art = supplied
			}
			icons[i] = &IconEncoder{
				Type:  osType,
				Image: resample.Square(art, osType.Size, interp),
			}
		})
	}
	work.Wait()
	iconSet := &IconSetEncoder{
		Icons: icons,
	}
	return iconSet, nil
}

// IconEncoder encodes an icns icon.
type IconEncoder struct {
	Type  OsType
	Image image.Image

	elements []element
}

// WriteTo encodes the icon into wr.
func (i *IconEncoder) WriteTo(wr io.Writer) (int64, error) {
	if err := i.encode(); err != nil {
		return 0, err
	}
	var written int64
	for _, el := range i.elements {
		n, err := writeElement(wr, el)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// encode builds the elements this icon occupies in the file.
func (i *IconEncoder) encode() error {
	if len(i.elements) > 0 {
		return nil
	}
	if i.Type.enc == encodingRGB {
		planes, mask := splitPlanes(i.Image, int(i.Type.Size))
		i.elements = []element{
			{id: i.Type.ID, payload: padRLE(packRLE(planes), len(planes))},
			{id: i.Type.mask, payload: mask},
		}
		return nil
	}
	data, err := encodeImage(i.Image)
	if err != nil {
		return err
	}
	i.elements = []element{{id: i.Type.ID, payload: data}}
	return nil
}

func encodeImage(img image.Image) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeElement writes one element: its 4-byte type, its length counting the
// header, then its payload.
func writeElement(wr io.Writer, el element) (int64, error) {
	var header [elementHeaderSize]byte
	copy(header[:4], el.id)
	binary.BigEndian.PutUint32(header[4:], uint32(elementHeaderSize+len(el.payload)))
	n, err := wr.Write(header[:])
	written := int64(n)
	if err != nil {
		return written, err
	}
	n, err = wr.Write(el.payload)
	return written + int64(n), err
}

// IconSetEncoder encodes a set of icons into an ICNS file.
type IconSetEncoder struct {
	Icons []*IconEncoder
}

// WriteTo writes the ICNS file to wr.
func (s *IconSetEncoder) WriteTo(wr io.Writer) (int64, error) {
	var elements []element
	for _, icon := range s.Icons {
		if icon == nil {
			continue
		}
		if err := icon.encode(); err != nil {
			return 0, err
		}
		elements = append(elements, icon.elements...)
	}
	body := bytes.NewBuffer(nil)
	// The table of contents comes first and covers everything after it, so a
	// reader can index the file without walking every element.
	if _, err := writeElement(body, tableOfContents(elements)); err != nil {
		return 0, err
	}
	for _, el := range elements {
		if _, err := writeElement(body, el); err != nil {
			return 0, err
		}
	}
	// The file is itself an element enclosing all the others.
	return writeElement(wr, element{id: Magic, payload: body.Bytes()})
}

// tableOfContents lists each element's type and total size, in order.
func tableOfContents(elements []element) element {
	toc := element{
		id:      "TOC ",
		payload: make([]byte, 0, len(elements)*elementHeaderSize),
	}
	for _, el := range elements {
		toc.payload = append(toc.payload, el.id...)
		toc.payload = binary.BigEndian.AppendUint32(toc.payload, uint32(elementHeaderSize+len(el.payload)))
	}
	return toc
}

// findNearestSize finds the biggest icon size we can use for this image.
func findNearestSize(img image.Image) uint {
	size := resample.BiggestSide(img)
	for _, s := range sizes {
		if size >= s {
			return s
		}
	}
	return 0
}

// sizesFrom returns a slice containing the sizes less than and including max.
func sizesFrom(max uint) []uint {
	for ii, s := range sizes {
		if s <= max {
			return sizes[ii:]
		}
	}
	return []uint{}
}
