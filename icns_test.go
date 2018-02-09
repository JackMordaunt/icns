package icns

import (
	"image"
	"testing"

	"github.com/jackmordaunt/deep"
)

func TestSizesFromMax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		from uint
		want []uint ``
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
