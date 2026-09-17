//go:build windows

// Package com contains the minimal set of COM definitions needed to implement
// an in-process COM server in pure Go: GUIDs, HRESULT codes, the IUnknown and
// IClassFactory vtables, and an [io.Reader] adapter over IStream.
//
// A COM interface pointer points at a struct whose first word is a pointer to
// a vtable: a struct of function pointers. To implement an interface in Go we
// allocate a vtable populated with [syscall.NewCallback] trampolines and hand
// COM a pointer to a Go struct whose first field is that vtable pointer. Each
// trampoline receives the struct pointer back as its `this` argument.
//
// Callbacks created by [syscall.NewCallback] are never freed, so every vtable
// is a package-level singleton shared by all instances of an interface.
package com

import (
	"fmt"
	"io"
	"math"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GUID identifies COM classes (CLSID) and interfaces (IID).
type GUID = windows.GUID

// MustGUID parses a GUID of the form "{XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}",
// panicking on malformed input. Intended for package-level constants.
func MustGUID(s string) *GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(fmt.Sprintf("com: invalid GUID %q: %v", s, err))
	}
	return &g
}

// IsEqualGUID reports whether two GUIDs are equal. Nil never equals anything.
func IsEqualGUID(a, b *GUID) bool {
	return a != nil && b != nil && *a == *b
}

// HRESULT is a COM status code.
//
// It is sized as uintptr rather than the 32-bit LONG that COM defines because
// [syscall.NewCallback] requires callbacks to return exactly one uintptr-sized
// value. Callers only read the low 32 bits.
type HRESULT = uintptr

// Well-known HRESULT values.
const (
	S_OK    HRESULT = 0x00000000
	S_FALSE HRESULT = 0x00000001

	E_NOTIMPL     HRESULT = 0x80004001
	E_NOINTERFACE HRESULT = 0x80004002
	E_POINTER     HRESULT = 0x80004003
	E_FAIL        HRESULT = 0x80004005
	E_UNEXPECTED  HRESULT = 0x8000FFFF
	E_OUTOFMEMORY HRESULT = 0x8007000E
	E_INVALIDARG  HRESULT = 0x80070057

	CLASS_E_NOAGGREGATION     HRESULT = 0x80040110
	CLASS_E_CLASSNOTAVAILABLE HRESULT = 0x80040111

	// HRESULT_FROM_WIN32(ERROR_ALREADY_INITIALIZED): the documented result of
	// IInitializeWithStream::Initialize when called a second time.
	HRESULT_ALREADY_INITIALIZED HRESULT = 0x800704DF

	// WTS_E_FAILEDEXTRACTION signals that a thumbnail could not be produced.
	WTS_E_FAILEDEXTRACTION HRESULT = 0x8004B200
)

// Failed reports whether hr denotes failure (the sign bit of the 32-bit code).
func Failed(hr HRESULT) bool {
	return int32(uint32(hr)) < 0
}

// Error wraps a failing HRESULT as a Go error.
type Error HRESULT

func (e Error) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X", uint32(e))
}

// Well-known interface identifiers.
var (
	IID_IUnknown      = MustGUID("{00000000-0000-0000-C000-000000000046}")
	IID_IClassFactory = MustGUID("{00000001-0000-0000-C000-000000000046}")
	IID_IStream       = MustGUID("{0000000C-0000-0000-C000-000000000046}")
)

// IUnknownVtbl is the vtable every COM interface begins with.
type IUnknownVtbl struct {
	QueryInterface uintptr // HRESULT (*)(This, REFIID riid, void **ppvObject)
	AddRef         uintptr // ULONG   (*)(This)
	Release        uintptr // ULONG   (*)(This)
}

// IClassFactoryVtbl is the vtable of IClassFactory.
type IClassFactoryVtbl struct {
	IUnknownVtbl
	CreateInstance uintptr // HRESULT (*)(This, IUnknown *pUnkOuter, REFIID riid, void **ppvObject)
	LockServer     uintptr // HRESULT (*)(This, BOOL fLock)
}

// IStreamVtbl is the vtable of IStream (which extends ISequentialStream).
type IStreamVtbl struct {
	IUnknownVtbl
	Read         uintptr // HRESULT (*)(This, void *pv, ULONG cb, ULONG *pcbRead)
	Write        uintptr
	Seek         uintptr
	SetSize      uintptr
	CopyTo       uintptr
	Commit       uintptr
	Revert       uintptr
	LockRegion   uintptr
	UnlockRegion uintptr
	Stat         uintptr
	Clone        uintptr
}

// IStream is a COM stream owned by the caller. It implements [io.Reader] so
// COM-provided data can be consumed by ordinary Go decoders.
type IStream struct {
	vtbl *IStreamVtbl
}

var _ io.Reader = (*IStream)(nil)

// Read implements [io.Reader] over IStream::Read.
func (s *IStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(p) > math.MaxUint32 {
		p = p[:math.MaxUint32]
	}
	var n uint32
	r, _, _ := syscall.SyscallN(
		s.vtbl.Read,
		uintptr(unsafe.Pointer(s)),
		uintptr(unsafe.Pointer(unsafe.SliceData(p))),
		uintptr(len(p)),
		uintptr(unsafe.Pointer(&n)),
	)
	// Only the low 32 bits of the return register hold the HRESULT.
	hr := HRESULT(uint32(r))
	if Failed(hr) {
		return int(n), Error(hr)
	}
	// IStream::Read reports end of stream either with S_FALSE or with S_OK
	// and zero bytes read, depending on the implementation.
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

// AddRef increments the stream's reference count.
func (s *IStream) AddRef() uint32 {
	r, _, _ := syscall.SyscallN(s.vtbl.AddRef, uintptr(unsafe.Pointer(s)))
	return uint32(r)
}

// Release decrements the stream's reference count.
func (s *IStream) Release() uint32 {
	r, _, _ := syscall.SyscallN(s.vtbl.Release, uintptr(unsafe.Pointer(s)))
	return uint32(r)
}
