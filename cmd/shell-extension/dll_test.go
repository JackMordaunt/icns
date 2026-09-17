//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/com"
	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/provider"
	"github.com/jackmordaunt/icns/v3"
	"golang.org/x/sys/windows"
)

// The test image has three regions: the top-left quarter is opaque red, the
// top-right quarter is half-transparent blue, and the bottom half is opaque
// green. Region interiors survive the icns encoder's resampling unchanged, so
// we can assert exact BGRA values that prove the channel order, that alpha is
// premultiplied, and (via the asymmetry) that the bitmap is top-down.
var (
	topLeftColor  = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	topRightColor = color.NRGBA{R: 0, G: 0, B: 255, A: 128}
	bottomColor   = color.NRGBA{R: 0, G: 255, B: 0, A: 255}

	topLeftBGRA  = [4]byte{0, 0, 255, 255}
	topRightBGRA = [4]byte{128, 0, 0, 128} // Premultiplied: 255 * 128/255 = 128.
	bottomBGRA   = [4]byte{0, 255, 0, 255}
)

// checkPixels asserts the three regions of the test image at the expected
// positions in a top-down bitmap of the given side length.
func checkPixels(t *testing.T, ds dibSection, prefix string) {
	t.Helper()
	side := int(ds.Width)
	for _, tc := range []struct {
		name string
		x, y int
		want [4]byte
	}{
		{"top-left", side / 4, side / 4, topLeftBGRA},
		{"top-right", side * 3 / 4, side / 4, topRightBGRA},
		{"bottom", side / 2, side * 3 / 4, bottomBGRA},
	} {
		if got := pixel(ds, tc.x, tc.y); got != tc.want {
			t.Errorf("%s%s pixel BGRA = %v, want %v", prefix, tc.name, got, tc.want)
		}
	}
}

// testICNS encodes a 128px icon set (128, 64, 32 and 16 px variants).
func testICNS(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			switch {
			case y >= 64:
				img.SetNRGBA(x, y, bottomColor)
			case x < 64:
				img.SetNRGBA(x, y, topLeftColor)
			default:
				img.SetNRGBA(x, y, topRightColor)
			}
		}
	}
	var buf bytes.Buffer
	if err := icns.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test icns: %v", err)
	}
	return buf.Bytes()
}

// buildDLL compiles this package as a shared library. The DLL is built outside
// t.TempDir because a loaded Go DLL cannot be unloaded, and Windows refuses to
// delete a mapped image.
func buildDLL(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "icns-shellext-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) }) // Best effort; see above.
	out := filepath.Join(dir, "icns-shellext.dll")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, ".")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("building dll: %v", err)
	}
	return out
}

// call invokes the method at index of a COM object's vtable.
func call(obj unsafe.Pointer, index int, args ...uintptr) com.HRESULT {
	vtbl := *(**[32]uintptr)(obj)
	r, _, _ := syscall.SyscallN(vtbl[index], append([]uintptr{uintptr(obj)}, args...)...)
	return com.HRESULT(uint32(r))
}

const (
	methodQueryInterface = iota
	methodAddRef
	methodRelease
	methodFirst // First method of the derived interface.
)

func queryInterface(t *testing.T, obj unsafe.Pointer, iid *com.GUID) (unsafe.Pointer, com.HRESULT) {
	t.Helper()
	var out unsafe.Pointer
	hr := call(obj, methodQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, hr
}

func release(obj unsafe.Pointer) uint32 {
	return uint32(call(obj, methodRelease))
}

var (
	shlwapi                = windows.NewLazySystemDLL("shlwapi.dll")
	procSHCreateMemStream  = shlwapi.NewProc("SHCreateMemStream")
	testGdi32              = windows.NewLazySystemDLL("gdi32.dll")
	procGetObject          = testGdi32.NewProc("GetObjectW")
	iidIShellItemImageFact = com.MustGUID("{BCC18B79-BA16-442F-80C4-8A59C30C463B}")
)

func memStream(t *testing.T, data []byte) unsafe.Pointer {
	t.Helper()
	r, _, _ := procSHCreateMemStream.Call(uintptr(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)))
	if r == 0 {
		t.Fatal("SHCreateMemStream returned nil")
	}
	// The stream lives in COM's heap, not Go's; smuggle the address past vet's
	// uintptr-to-pointer check without a conversion it would flag.
	var stream unsafe.Pointer
	*(*uintptr)(unsafe.Pointer(&stream)) = r
	return stream
}

// dibSection mirrors DIBSECTION from wingdi.h.
type dibSection struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       unsafe.Pointer
	Header     struct {
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
	Bitfields [3]uint32
	Section   windows.Handle
	Offset    uint32
}

func inspect(t *testing.T, hbmp windows.Handle) dibSection {
	t.Helper()
	var ds dibSection
	r, _, err := procGetObject.Call(uintptr(hbmp), unsafe.Sizeof(ds), uintptr(unsafe.Pointer(&ds)))
	if r == 0 {
		t.Fatalf("GetObject: %v", err)
	}
	return ds
}

func pixel(ds dibSection, x, y int) [4]byte {
	stride := int(ds.WidthBytes)
	bits := unsafe.Slice((*byte)(ds.Bits), stride*int(ds.Height))
	off := y*stride + x*4
	return [4]byte(bits[off : off+4])
}

func TestDLL(t *testing.T) {
	dll, err := windows.LoadDLL(buildDLL(t))
	if err != nil {
		t.Fatal(err)
	}
	getClassObject := dll.MustFindProc("DllGetClassObject")
	canUnloadNow := dll.MustFindProc("DllCanUnloadNow")

	getFactory := func(clsid, iid *com.GUID) (unsafe.Pointer, com.HRESULT) {
		var out unsafe.Pointer
		r, _, _ := getClassObject.Call(uintptr(unsafe.Pointer(clsid)), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
		return out, com.HRESULT(uint32(r))
	}

	if _, hr := getFactory(com.IID_IStream, com.IID_IClassFactory); hr != com.CLASS_E_CLASSNOTAVAILABLE {
		t.Fatalf("DllGetClassObject(unknown CLSID) = %#x, want CLASS_E_CLASSNOTAVAILABLE", hr)
	}
	if _, hr := getFactory(provider.CLSID, com.IID_IStream); hr != com.E_NOINTERFACE {
		t.Fatalf("DllGetClassObject(CLSID, IStream) = %#x, want E_NOINTERFACE", hr)
	}
	factory, hr := getFactory(provider.CLSID, com.IID_IClassFactory)
	if hr != com.S_OK || factory == nil {
		t.Fatalf("DllGetClassObject(CLSID, IClassFactory) = %#x, %p", hr, factory)
	}
	if unk, hr := queryInterface(t, factory, com.IID_IUnknown); hr != com.S_OK || unk != factory {
		t.Fatalf("factory QueryInterface(IUnknown) = %#x, %p; want S_OK, %p", hr, unk, factory)
	}

	createInstance := func(outer unsafe.Pointer, iid *com.GUID) (unsafe.Pointer, com.HRESULT) {
		var out unsafe.Pointer
		hr := call(factory, methodFirst, uintptr(outer), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
		return out, hr
	}
	if _, hr := createInstance(factory, provider.IID_IInitializeWithStream); hr != com.CLASS_E_NOAGGREGATION {
		t.Fatalf("CreateInstance(aggregated) = %#x, want CLASS_E_NOAGGREGATION", hr)
	}
	if _, hr := createInstance(nil, com.IID_IStream); hr != com.E_NOINTERFACE {
		t.Fatalf("CreateInstance(IStream) = %#x, want E_NOINTERFACE", hr)
	}

	data := testICNS(t)

	newProvider := func(t *testing.T) (init, thumb unsafe.Pointer) {
		t.Helper()
		init, hr := createInstance(nil, provider.IID_IInitializeWithStream)
		if hr != com.S_OK || init == nil {
			t.Fatalf("CreateInstance(IInitializeWithStream) = %#x, %p", hr, init)
		}
		stream := memStream(t, data)
		defer release(stream)
		if hr := call(init, methodFirst, uintptr(stream), 0); hr != com.S_OK {
			t.Fatalf("Initialize = %#x", hr)
		}
		if hr := call(init, methodFirst, uintptr(stream), 0); hr != com.HRESULT_ALREADY_INITIALIZED {
			t.Fatalf("second Initialize = %#x, want HRESULT_ALREADY_INITIALIZED", hr)
		}
		thumb, hr = queryInterface(t, init, provider.IID_IThumbnailProvider)
		if hr != com.S_OK || thumb == nil {
			t.Fatalf("QueryInterface(IThumbnailProvider) = %#x, %p", hr, thumb)
		}
		if thumb == init {
			t.Fatal("IThumbnailProvider and IInitializeWithStream share a vtable pointer")
		}
		// COM identity: IUnknown must be the same pointer from every interface.
		unk, hr := queryInterface(t, thumb, com.IID_IUnknown)
		if hr != com.S_OK || unk != init {
			t.Fatalf("QueryInterface(IUnknown) via IThumbnailProvider = %#x, %p; want %p", hr, unk, init)
		}
		release(unk)
		return init, thumb
	}

	getThumbnail := func(t *testing.T, thumb unsafe.Pointer, cx uint32) (windows.Handle, uint32) {
		t.Helper()
		var (
			hbmp  windows.Handle
			alpha uint32
		)
		hr := call(thumb, methodFirst, uintptr(cx), uintptr(unsafe.Pointer(&hbmp)), uintptr(unsafe.Pointer(&alpha)))
		if hr != com.S_OK {
			t.Fatalf("GetThumbnail(%d) = %#x", cx, hr)
		}
		if hbmp == 0 {
			t.Fatalf("GetThumbnail(%d) returned a null bitmap", cx)
		}
		return hbmp, alpha
	}

	for _, tc := range []struct {
		cx   uint32
		want int32 // Expected bitmap side.
	}{
		{cx: 64, want: 64},    // Exact icon size available.
		{cx: 100, want: 100},  // 128px icon downscaled to fit.
		{cx: 1000, want: 128}, // Nothing large enough: largest icon, unscaled.
	} {
		init, thumb := newProvider(t)
		hbmp, alpha := getThumbnail(t, thumb, tc.cx)
		if alpha != provider.WTSAT_ARGB {
			t.Errorf("cx=%d: alpha = %d, want WTSAT_ARGB", tc.cx, alpha)
		}
		ds := inspect(t, hbmp)
		if ds.Width != tc.want || ds.Height != tc.want {
			t.Errorf("cx=%d: bitmap is %dx%d, want %dx%d", tc.cx, ds.Width, ds.Height, tc.want, tc.want)
		}
		if ds.BitsPixel != 32 || ds.Bits == nil {
			t.Errorf("cx=%d: bitmap is %dbpp with bits %p, want a 32bpp DIB section", tc.cx, ds.BitsPixel, ds.Bits)
		}
		checkPixels(t, ds, fmt.Sprintf("cx=%d: ", tc.cx))
		if err := provider.DeleteObject(hbmp); err != nil {
			t.Error(err)
		}
		if n := release(thumb); n != 1 {
			t.Errorf("cx=%d: Release(thumb) = %d, want 1", tc.cx, n)
		}
		if n := release(init); n != 0 {
			t.Errorf("cx=%d: Release(init) = %d, want 0", tc.cx, n)
		}
	}

	// A provider that was never initialised must fail cleanly.
	init, hr := createInstance(nil, provider.IID_IThumbnailProvider)
	if hr != com.S_OK {
		t.Fatalf("CreateInstance(IThumbnailProvider) = %#x", hr)
	}
	var (
		hbmp  windows.Handle
		alpha uint32
	)
	if hr := call(init, methodFirst, 64, uintptr(unsafe.Pointer(&hbmp)), uintptr(unsafe.Pointer(&alpha))); hr != com.E_UNEXPECTED {
		t.Errorf("GetThumbnail before Initialize = %#x, want E_UNEXPECTED", hr)
	}
	release(init)

	// Garbage in must not crash the surrogate process.
	init, _ = createInstance(nil, provider.IID_IInitializeWithStream)
	garbage := memStream(t, []byte("not an icns file, definitely not"))
	if hr := call(init, methodFirst, uintptr(garbage), 0); hr != com.S_OK {
		t.Fatalf("Initialize(garbage) = %#x, want S_OK (validation is deferred)", hr)
	}
	release(garbage)
	thumb, _ := queryInterface(t, init, provider.IID_IThumbnailProvider)
	if hr := call(thumb, methodFirst, 64, uintptr(unsafe.Pointer(&hbmp)), uintptr(unsafe.Pointer(&alpha))); hr != com.WTS_E_FAILEDEXTRACTION {
		t.Errorf("GetThumbnail(garbage) = %#x, want WTS_E_FAILEDEXTRACTION", hr)
	}
	release(thumb)
	release(init)

	if r, _, _ := canUnloadNow.Call(); com.HRESULT(uint32(r)) != com.S_FALSE {
		t.Errorf("DllCanUnloadNow = %#x, want S_FALSE", r)
	}
}

// TestExplorer asks the shell itself for a thumbnail, which exercises the
// registry entries and the out-of-process surrogate. It needs the DLL to be
// registered with regsvr32 first, so it only runs when ICNS_SHELLEXT_E2E is set.
func TestExplorer(t *testing.T) {
	if os.Getenv("ICNS_SHELLEXT_E2E") == "" {
		t.Skip("set ICNS_SHELLEXT_E2E=1 after registering the DLL to run this test")
	}
	// A fresh filename per run sidesteps the shell's thumbnail cache.
	path := filepath.Join(t.TempDir(), "sample.icns")
	if err := os.WriteFile(path, testICNS(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()

	var (
		shell32                         = windows.NewLazySystemDLL("shell32.dll")
		procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	)
	var factory unsafe.Pointer
	r, _, _ := procSHCreateItemFromParsingName.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(path))),
		0,
		uintptr(unsafe.Pointer(iidIShellItemImageFact)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if hr := com.HRESULT(uint32(r)); hr != com.S_OK {
		t.Fatalf("SHCreateItemFromParsingName = %#x", hr)
	}
	defer release(factory)

	const (
		siigbfBiggerSizeOK  = 0x01
		siigbfThumbnailOnly = 0x08
		size                = 128
	)
	// SIZE is passed by value; on x64 an 8-byte struct travels in one register.
	packedSize := uintptr(size) | uintptr(size)<<32
	var hbmp windows.Handle
	hr := call(factory, methodFirst, packedSize, siigbfThumbnailOnly|siigbfBiggerSizeOK, uintptr(unsafe.Pointer(&hbmp)))
	if hr != com.S_OK || hbmp == 0 {
		t.Fatalf("IShellItemImageFactory::GetImage = %#x, hbmp %#x (is the DLL registered?)", hr, hbmp)
	}
	defer provider.DeleteObject(hbmp)

	ds := inspect(t, hbmp)
	if ds.Width != size || ds.Height != size {
		t.Fatalf("shell thumbnail is %dx%d, want %dx%d", ds.Width, ds.Height, size, size)
	}
	if ds.Bits == nil {
		t.Fatal("shell returned a device-dependent bitmap; expected a DIB section")
	}
	// The shell may hand back a bottom-up DIB; GetObject reports a positive
	// header height either way, so probe the top half only.
	if got := pixel(ds, size/4, size/2); got != topLeftBGRA && got != bottomBGRA {
		t.Errorf("left pixel BGRA = %v, want %v or %v", got, topLeftBGRA, bottomBGRA)
	}
}
