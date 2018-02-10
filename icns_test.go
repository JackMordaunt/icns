package icns

import (
	"image"
	"io"
	"io/ioutil"
	"testing"

	"github.com/jackmordaunt/deep"
)

// TestEncode tests for input validation, sanity checks and errors.
// The validity of the encoding is not tested here.
// Super large images are not tested because the resizing takes too
// long for unit testing.
func TestEncode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		wr   io.Writer
		img  image.Image

		wantErr bool
	}{
		{
			"nil image",
			ioutil.Discard,
			nil,
			true,
		},
		{
			"nil writer",
			nil,
			rect(0, 0, 50, 50),
			true,
		},
		{
			"valid sqaure",
			ioutil.Discard,
			rect(0, 0, 50, 50),
			false,
		},
		{
			"valid non-square",
			ioutil.Discard,
			rect(0, 0, 10, 50),
			false,
		},
		{
			"valid non-square, weird dimensions",
			ioutil.Discard,
			rect(0, 0, 17, 77),
			false,
		},
		{
			"invalid zero img",
			ioutil.Discard,
			rect(0, 0, 0, 0),
			true,
		},
		{
			"invalid small img",
			ioutil.Discard,
			rect(0, 0, 1, 1),
			true,
		},
		{
			"valid square not at origin point",
			ioutil.Discard,
			rect(10, 10, 50, 50),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			err := Encode(tt.wr, tt.img)
			if !tt.wantErr && err != nil {
				st.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSizesFromMax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		from uint
		want []uint
	}{
		{
			"small",
			100,
			[]uint{64, 32},
		},
		{
			"large",
			99999,
			[]uint{1024, 512, 256, 64, 32},
		},
		{
			"smallest",
			0,
			[]uint{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := sizesFrom(tt.from)
			if !deep.EqualContents(got, tt.want) {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func TestBiggestSide(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		img  image.Image
		want uint
	}{
		{
			"equal",
			rect(0, 0, 100, 100),
			100,
		},
		{
			"right larger",
			rect(0, 0, 50, 100),
			100,
		},
		{
			"left larger",
			rect(0, 0, 100, 50),
			100,
		},
		{
			"off by one",
			rect(0, 0, 100, 99),
			100,
		},
		{
			"empty",
			rect(0, 0, 0, 0),
			0,
		},
		{
			"left empty",
			rect(0, 0, 0, 10),
			10,
		},
		{
			"right empty",
			rect(0, 0, 10, 0),
			10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := biggestSide(tt.img)
			if got != tt.want {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func TestFindNearestSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		img  image.Image
		want uint
	}{
		{
			"small",
			rect(0, 0, 100, 100),
			64,
		},
		{
			"very large",
			rect(0, 0, 123456789, 123456789),
			1024,
		},
		{
			"too small",
			rect(0, 0, 16, 16),
			0,
		},
		{
			"off by one",
			rect(0, 0, 33, 33),
			32,
		},
		{
			"exact",
			rect(0, 0, 256, 256),
			256,
		},
		{
			"exact",
			rect(0, 0, 1024, 1024),
			1024,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(st *testing.T) {
			got := findNearestSize(tt.img)
			if tt.want != got {
				st.Errorf("want=%d, got=%d", tt.want, got)
			}
		})
	}
}

func rect(x0, y0, x1, y1 int) image.Image {
	return image.Rect(x0, y0, x1, y1)
}
