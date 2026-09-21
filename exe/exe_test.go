package exe

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackmordaunt/icns/v4/ico"
)

// The fixtures are built by mingw's windres and linker from an ico this
// module wrote, so the test runs against a resource section a Windows
// toolchain laid out rather than one written here.
//
//	x86_64-w64-mingw32-windres icon.rc icon.o
//	x86_64-w64-mingw32-gcc -shared -nostdlib -o icon.dll stub.c icon.o
const (
	withIcons    = "testdata/icon.dll"
	withoutIcons = "testdata/plain.dll"
	aProgram     = "testdata/tiny.exe"
	original     = "testdata/icon.ico"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestIconsFromABinary(t *testing.T) {
	groups, err := Icons(open(t, withIcons))
	if err != nil {
		t.Fatalf("reading icons: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("found %d groups, want 1", len(groups))
	}
	group := groups[0]
	if group.ID != 1 {
		t.Errorf("group id is %d, want 1", group.ID)
	}
	if want := []int{32, 24, 16}; !reflect.DeepEqual(group.Sizes, want) {
		t.Errorf("sizes are %v, want %v", group.Sizes, want)
	}
}

// TestICOMatchesWhatWentIn is the point of the package: the resource section
// holds an ico taken apart, and putting it back together returns the file
// the linker was given.
func TestICOMatchesWhatWentIn(t *testing.T) {
	groups, err := Icons(open(t, withIcons))
	if err != nil {
		t.Fatalf("reading icons: %v", err)
	}
	want, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("reading the original: %v", err)
	}
	if got := groups[0].ICO(); !bytes.Equal(got, want) {
		t.Errorf("reassembled %d bytes, want the %d that went in", len(got), len(want))
	}
}

func TestDecodeFromABinary(t *testing.T) {
	img, err := Decode(open(t, withIcons))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got := img.Bounds().Size(); got.X != 32 || got.Y != 32 {
		t.Fatalf("decoded a %v icon, want the largest at 32x32", got)
	}
	// Each size was drawn in its own colour, so the wrong one would show.
	c := color.NRGBAModel.Convert(img.At(16, 16)).(color.NRGBA)
	if want := (color.NRGBA{R: 0x10, G: 0x20, B: 0xE0, A: 0xFF}); c != want {
		t.Errorf("centre is %v, want the 32 pixel artwork %v", c, want)
	}
}

func TestIconsWithoutResources(t *testing.T) {
	_, err := Icons(open(t, withoutIcons))
	if !errors.Is(err, ErrNoIcons) {
		t.Errorf("reading a binary with no icons returned %v, want ErrNoIcons", err)
	}
}

// TestIdentifyReadsTheHeaderNotTheName covers telling a library from a
// program without the file name, which is what the header is for. Both begin
// with the same two bytes.
func TestIdentifyReadsTheHeaderNotTheName(t *testing.T) {
	for _, tt := range []struct {
		path string
		want Kind
	}{
		{withIcons, Library},
		{withoutIcons, Library},
		{aProgram, Program},
	} {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := Identify(open(t, tt.path))
			if !ok {
				t.Fatal("not recognised as a binary")
			}
			if got != tt.want {
				t.Errorf("identified as %s, want %s", got, tt.want)
			}
		})
	}
}

// TestIdentifyRefusesWhatMerelyLooksLikeOne covers the formats that share a
// binary's first two bytes without sharing its header.
func TestIdentifyRefusesWhatMerelyLooksLikeOne(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"nothing", nil},
		{"not a binary at all", []byte("just text")},
		{"the two bytes alone", []byte("MZ")},
		{"a stub with no header behind it", []byte("MZ\x90\x00\x03\x00\x00\x00")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := Identify(bytes.NewReader(tt.data)); ok {
				t.Error("recognised as a binary")
			}
		})
	}
}

func TestIconsRejectsWhatIsNotABinary(t *testing.T) {
	if _, err := Icons(bytes.NewReader([]byte("not a binary"))); err == nil {
		t.Error("reading rubbish returned no error")
	}
}

// group builds a group icon directory naming the images given, so the error
// paths can be reached without a binary that holds them.
func group(id uint16, images ...uint16) leaf {
	out := make([]byte, 0, groupHeaderSize+len(images)*groupEntrySize)
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, uint16(len(images)))
	for _, image := range images {
		out = append(out, 32, 32, 0, 0)
		out = binary.LittleEndian.AppendUint16(out, 1)
		out = binary.LittleEndian.AppendUint16(out, 32)
		out = binary.LittleEndian.AppendUint32(out, 16)
		out = binary.LittleEndian.AppendUint16(out, image)
	}
	return leaf{id: id, data: out}
}

func TestAssembleRejectsAGroupItCannotComplete(t *testing.T) {
	for _, tt := range []struct {
		name  string
		group leaf
		want  string
	}{
		{
			name:  "names an image that is absent",
			group: group(1, 7),
			want:  "names image 7",
		},
		{
			name:  "shorter than a header",
			group: leaf{id: 1, data: []byte{0, 0}},
			want:  "holds 2 bytes",
		},
		{
			name:  "lists more icons than it holds",
			group: leaf{id: 1, data: []byte{0, 0, 1, 0, 9, 0}},
			want:  "lists 9 icons",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := assemble(tt.group, nil)
			if err == nil {
				t.Fatal("assembling returned no error")
			}
			if !errors.Is(err, ErrMalformed) {
				t.Errorf("error is %v, want ErrMalformed", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error is %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestAssembleTakesTheLengthFromTheResource keeps a directory that misstates
// a size from producing an ico nothing can read.
func TestAssembleTakesTheLengthFromTheResource(t *testing.T) {
	// The directory above says every image is 16 bytes; this one is not, so
	// a file built from what it claims would hand back the wrong pixels.
	pixels := bytes.Repeat([]byte{0xAB}, 64)
	got, err := assemble(group(1, 3), []leaf{{id: 3, data: pixels}})
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	d, err := ico.NewDecoder(bytes.NewReader(got.ICO()))
	if err != nil {
		t.Fatalf("reading what was assembled: %v", err)
	}
	icons := d.Icons()
	if len(icons) != 1 {
		t.Fatalf("found %d icons, want 1", len(icons))
	}
	if payload := icons[0].Payload(); !bytes.Equal(payload, pixels) {
		t.Errorf("icon holds %d bytes, want the %d the resource holds", len(payload), len(pixels))
	}
}
