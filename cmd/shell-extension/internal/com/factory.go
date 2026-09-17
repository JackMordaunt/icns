//go:build windows

package com

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"
)

// Constructor creates a new object and stores the interface identified by riid
// in *ppv, returning E_NOINTERFACE when the object does not implement riid.
type Constructor func(riid *GUID, ppv *unsafe.Pointer) HRESULT

// ClassFactory implements IClassFactory for a single COM class.
//
// A ClassFactory is a process-lifetime singleton: AddRef and Release are
// no-ops and the object is pinned so COM may hold its pointer indefinitely.
// The vtable pointer must remain the first field.
type ClassFactory struct {
	vtbl   *IClassFactoryVtbl
	create Constructor
	pin    runtime.Pinner
}

// NewClassFactory returns a factory whose CreateInstance defers to create.
func NewClassFactory(create Constructor) *ClassFactory {
	f := &ClassFactory{vtbl: classFactoryVtbl, create: create}
	f.pin.Pin(f)
	return f
}

// QueryInterface implements IUnknown::QueryInterface for the factory.
func (f *ClassFactory) QueryInterface(riid *GUID, ppv *unsafe.Pointer) HRESULT {
	if ppv == nil {
		return E_POINTER
	}
	if !IsEqualGUID(riid, IID_IUnknown) && !IsEqualGUID(riid, IID_IClassFactory) {
		*ppv = nil
		return E_NOINTERFACE
	}
	*ppv = unsafe.Pointer(f)
	return S_OK
}

// CreateInstance implements IClassFactory::CreateInstance.
func (f *ClassFactory) CreateInstance(outer unsafe.Pointer, riid *GUID, ppv *unsafe.Pointer) HRESULT {
	if ppv == nil {
		return E_POINTER
	}
	*ppv = nil
	if outer != nil {
		return CLASS_E_NOAGGREGATION
	}
	return f.create(riid, ppv)
}

// Guard runs a COM method body and converts a panic into E_FAIL.
//
// A panic that unwinds out of a syscall.NewCallback trampoline crosses C
// frames and takes the host process down with it, which in the shell's case
// means Explorer. COM callers expect failures as HRESULTs, so report it as one.
func Guard(method string, body func() HRESULT) (hr HRESULT) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("panic in COM method", "method", method, "panic", p, "stack", string(debug.Stack()))
			hr = E_FAIL
		}
	}()
	return body()
}

// classFactoryVtbl is shared by all factories. The trampolines recover the
// concrete factory from the `this` pointer COM passes back to us.
var classFactoryVtbl = &IClassFactoryVtbl{
	IUnknownVtbl: IUnknownVtbl{
		QueryInterface: syscall.NewCallback(func(this *ClassFactory, riid *GUID, ppv *unsafe.Pointer) uintptr {
			return Guard("IClassFactory::QueryInterface", func() HRESULT { return this.QueryInterface(riid, ppv) })
		}),
		AddRef: syscall.NewCallback(func(this *ClassFactory) uintptr {
			return 1
		}),
		Release: syscall.NewCallback(func(this *ClassFactory) uintptr {
			return 1
		}),
	},
	CreateInstance: syscall.NewCallback(func(this *ClassFactory, outer unsafe.Pointer, riid *GUID, ppv *unsafe.Pointer) uintptr {
		return Guard("IClassFactory::CreateInstance", func() HRESULT { return this.CreateInstance(outer, riid, ppv) })
	}),
	LockServer: syscall.NewCallback(func(this *ClassFactory, lock uintptr) uintptr {
		// The Go runtime can never be unloaded from a host process (see
		// DllCanUnloadNow), so there is nothing to lock.
		return S_OK
	}),
}
