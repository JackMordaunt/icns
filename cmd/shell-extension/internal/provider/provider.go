//go:build windows

// Package provider implements the icns thumbnail provider COM object: a single
// class exposing IInitializeWithStream (to receive the .icns bytes) and
// IThumbnailProvider (to hand Explorer an HBITMAP).
package provider

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/com"
	"github.com/jackmordaunt/icns/v3"
	"github.com/nfnt/resize"
	"golang.org/x/sys/windows"
)

// CLSID identifies this thumbnail provider class in the registry.
var CLSID = com.MustGUID("{E21C95C5-5086-4F9F-8876-7FF4CE4AC6EC}")

// Interface identifiers implemented by Provider.
var (
	IID_IInitializeWithStream = com.MustGUID("{B824B49D-22AC-4161-AC8A-9916E8FA3F7F}")
	IID_IThumbnailProvider    = com.MustGUID("{E357FCCD-A995-4576-B01F-234630154E96}")
)

// WTS_ALPHATYPE values reported through IThumbnailProvider::GetThumbnail.
const (
	WTSAT_UNKNOWN uint32 = 0
	WTSAT_RGB     uint32 = 1
	WTSAT_ARGB    uint32 = 2
)

type initializeWithStreamVtbl struct {
	com.IUnknownVtbl
	Initialize uintptr // HRESULT (*)(This, IStream *pstream, DWORD grfMode)
}

type thumbnailProviderVtbl struct {
	com.IUnknownVtbl
	GetThumbnail uintptr // HRESULT (*)(This, UINT cx, HBITMAP *phbmp, WTS_ALPHATYPE *pdwAlpha)
}

// Provider is one instance of the thumbnail provider; the shell creates one per
// file it wants a thumbnail for.
//
// The first two fields are the interface pointers handed to COM. The address of
// the struct doubles as the IUnknown/IInitializeWithStream pointer, and the
// address of thumbVtbl is the IThumbnailProvider pointer. The trampolines for
// the second interface subtract that field's offset to recover the Provider.
type Provider struct {
	initVtbl  *initializeWithStreamVtbl
	thumbVtbl *thumbnailProviderVtbl

	refs atomic.Int32
	pin  runtime.Pinner

	mu   sync.Mutex
	data []byte // raw .icns bytes supplied by Initialize
}

// live tracks every Provider COM still holds a reference to. This keeps the
// objects reachable from Go while the only pointers to them are in the shell.
var live = struct {
	sync.Mutex
	set map[*Provider]struct{}
}{set: map[*Provider]struct{}{}}

// Live reports the number of outstanding Provider instances.
func Live() int {
	live.Lock()
	defer live.Unlock()
	return len(live.set)
}

// New allocates a Provider with a reference count of one.
func New() *Provider {
	p := &Provider{initVtbl: initVtbl, thumbVtbl: thumbVtbl}
	p.refs.Store(1)
	p.pin.Pin(p)
	live.Lock()
	live.set[p] = struct{}{}
	live.Unlock()
	return p
}

// Create is a [com.Constructor] for use with [com.NewClassFactory].
func Create(riid *com.GUID, ppv *unsafe.Pointer) com.HRESULT {
	p := New()
	// QueryInterface takes the caller's reference; drop the constructor's.
	defer p.Release()
	return p.QueryInterface(riid, ppv)
}

// QueryInterface implements IUnknown::QueryInterface.
func (p *Provider) QueryInterface(riid *com.GUID, ppv *unsafe.Pointer) com.HRESULT {
	if ppv == nil {
		return com.E_POINTER
	}
	switch {
	case com.IsEqualGUID(riid, com.IID_IUnknown), com.IsEqualGUID(riid, IID_IInitializeWithStream):
		*ppv = unsafe.Pointer(p)
	case com.IsEqualGUID(riid, IID_IThumbnailProvider):
		*ppv = unsafe.Pointer(&p.thumbVtbl)
	default:
		*ppv = nil
		return com.E_NOINTERFACE
	}
	p.AddRef()
	return com.S_OK
}

// AddRef implements IUnknown::AddRef.
func (p *Provider) AddRef() uint32 {
	return uint32(p.refs.Add(1))
}

// Release implements IUnknown::Release, freeing the object on the last release.
func (p *Provider) Release() uint32 {
	n := p.refs.Add(-1)
	if n == 0 {
		live.Lock()
		delete(live.set, p)
		live.Unlock()
		p.mu.Lock()
		p.data = nil
		p.mu.Unlock()
		p.pin.Unpin()
	}
	return uint32(n)
}

// Initialize implements IInitializeWithStream::Initialize by buffering the
// whole stream. The stream is only guaranteed valid for the duration of this
// call, and icns decoding needs the entire file anyway.
func (p *Provider) Initialize(stream *com.IStream, grfMode uint32) com.HRESULT {
	if stream == nil {
		return com.E_POINTER
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.data != nil {
		return com.HRESULT_ALREADY_INITIALIZED
	}
	data, err := io.ReadAll(stream)
	if err != nil {
		slog.Error("reading icns stream", "err", err)
		return com.E_FAIL
	}
	p.data = data
	return com.S_OK
}

// GetThumbnail implements IThumbnailProvider::GetThumbnail. The returned bitmap
// is a top-down 32bpp DIB with premultiplied alpha; ownership passes to the
// caller.
func (p *Provider) GetThumbnail(cx uint32, phbmp *windows.Handle, pdwAlpha *uint32) com.HRESULT {
	if phbmp == nil || pdwAlpha == nil {
		return com.E_POINTER
	}
	*phbmp = 0
	*pdwAlpha = WTSAT_UNKNOWN

	p.mu.Lock()
	data := p.data
	p.mu.Unlock()
	if data == nil {
		return com.E_UNEXPECTED // Initialize was never called.
	}

	img, err := Thumbnail(bytes.NewReader(data), int(cx))
	if err != nil {
		slog.Error("extracting icns thumbnail", "cx", cx, "err", err)
		return com.WTS_E_FAILEDEXTRACTION
	}
	hbmp, err := CreateDIB(img)
	if err != nil {
		slog.Error("creating thumbnail bitmap", "err", err)
		return com.E_FAIL
	}
	*phbmp = hbmp
	*pdwAlpha = WTSAT_ARGB
	return com.S_OK
}

// Thumbnail decodes the icns and returns the icon best suited to a cx by cx
// square: the smallest icon at least that large, downscaled to fit, or the
// largest available icon when none is big enough.
func Thumbnail(r io.Reader, cx int) (*image.RGBA, error) {
	if cx <= 0 {
		return nil, fmt.Errorf("invalid thumbnail size %d", cx)
	}
	images, err := icns.DecodeAll(r) // Sorted largest first.
	if err != nil {
		return nil, err
	}
	chosen := images[0]
	for _, img := range images {
		if side(img) < cx {
			break
		}
		chosen = img
	}
	if side(chosen) > cx {
		chosen = resize.Thumbnail(uint(cx), uint(cx), chosen, resize.Lanczos3)
	}
	// Normalise to premultiplied RGBA, which is what a GDI ARGB bitmap wants.
	b := chosen.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), chosen, b.Min, draw.Src)
	return rgba, nil
}

func side(img image.Image) int {
	s := img.Bounds().Size()
	return max(s.X, s.Y)
}

// fromThumb recovers the Provider from an IThumbnailProvider `this` pointer.
func fromThumb(this unsafe.Pointer) *Provider {
	return (*Provider)(unsafe.Add(this, -int(unsafe.Offsetof(Provider{}.thumbVtbl))))
}

// Shared vtables. Every trampoline must return a single uintptr.
var (
	initVtbl = &initializeWithStreamVtbl{
		IUnknownVtbl: com.IUnknownVtbl{
			QueryInterface: syscall.NewCallback(func(this *Provider, riid *com.GUID, ppv *unsafe.Pointer) uintptr {
				return this.QueryInterface(riid, ppv)
			}),
			AddRef: syscall.NewCallback(func(this *Provider) uintptr {
				return uintptr(this.AddRef())
			}),
			Release: syscall.NewCallback(func(this *Provider) uintptr {
				return uintptr(this.Release())
			}),
		},
		Initialize: syscall.NewCallback(func(this *Provider, stream *com.IStream, grfMode uint32) uintptr {
			return this.Initialize(stream, grfMode)
		}),
	}

	thumbVtbl = &thumbnailProviderVtbl{
		IUnknownVtbl: com.IUnknownVtbl{
			QueryInterface: syscall.NewCallback(func(this unsafe.Pointer, riid *com.GUID, ppv *unsafe.Pointer) uintptr {
				return fromThumb(this).QueryInterface(riid, ppv)
			}),
			AddRef: syscall.NewCallback(func(this unsafe.Pointer) uintptr {
				return uintptr(fromThumb(this).AddRef())
			}),
			Release: syscall.NewCallback(func(this unsafe.Pointer) uintptr {
				return uintptr(fromThumb(this).Release())
			}),
		},
		GetThumbnail: syscall.NewCallback(func(this unsafe.Pointer, cx uint32, phbmp *windows.Handle, pdwAlpha *uint32) uintptr {
			return fromThumb(this).GetThumbnail(cx, phbmp, pdwAlpha)
		}),
	}
)
