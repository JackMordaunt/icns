//go:build windows

package provider

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32                = windows.NewLazySystemDLL("gdi32.dll")
	procCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
)

const (
	biRGB         = 0
	dibRGBColors  = 0
	bitsPerPixel  = 32
	bytesPerPixel = bitsPerPixel / 8
)

// bitmapInfoHeader mirrors BITMAPINFOHEADER from wingdi.h.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// bitmapInfo mirrors BITMAPINFO; the colour table is unused at 32bpp.
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

// CreateDIB copies img into a new top-down 32bpp BGRA device-independent
// bitmap. Go's [image.RGBA] is already alpha-premultiplied, which is exactly
// what the shell expects for WTSAT_ARGB thumbnails; only the channel order
// changes. The caller owns the returned handle.
func CreateDIB(img *image.RGBA) (windows.Handle, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return 0, fmt.Errorf("empty image %v", img.Rect)
	}
	bmi := bitmapInfo{Header: bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h), // Negative height selects a top-down bitmap.
		Planes:      1,
		BitCount:    bitsPerPixel,
		Compression: biRGB,
	}}
	var bits unsafe.Pointer
	r, _, err := procCreateDIBSection.Call(
		0, // No device context is needed for DIB_RGB_COLORS.
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, // No file mapping.
		0,
	)
	if r == 0 || bits == nil {
		return 0, fmt.Errorf("CreateDIBSection: %w", err)
	}
	// A 32bpp stride is always DWORD aligned, so rows are packed.
	stride := w * bytesPerPixel
	dst := unsafe.Slice((*byte)(bits), stride*h)
	for y := 0; y < h; y++ {
		src := img.Pix[y*img.Stride : y*img.Stride+stride]
		row := dst[y*stride : (y+1)*stride]
		for x := 0; x < stride; x += bytesPerPixel {
			row[x+0] = src[x+2] // B
			row[x+1] = src[x+1] // G
			row[x+2] = src[x+0] // R
			row[x+3] = src[x+3] // A
		}
	}
	return windows.Handle(r), nil
}

// DeleteObject frees a GDI object such as the bitmap returned by [CreateDIB].
func DeleteObject(h windows.Handle) error {
	r, _, err := procDeleteObject.Call(uintptr(h))
	if r == 0 {
		return fmt.Errorf("DeleteObject: %w", err)
	}
	return nil
}
