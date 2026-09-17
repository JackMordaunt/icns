//go:build windows

package com

import (
	"runtime"
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

// classFactoryVtbl is shared by all factories. The trampolines recover the
// concrete factory from the `this` pointer COM passes back to us.
var classFactoryVtbl = &IClassFactoryVtbl{
	IUnknownVtbl: IUnknownVtbl{
		QueryInterface: syscall.NewCallback(func(this *ClassFactory, riid *GUID, ppv *unsafe.Pointer) uintptr {
			return this.QueryInterface(riid, ppv)
		}),
		AddRef: syscall.NewCallback(func(this *ClassFactory) uintptr {
			return 1
		}),
		Release: syscall.NewCallback(func(this *ClassFactory) uintptr {
			return 1
		}),
	},
	CreateInstance: syscall.NewCallback(func(this *ClassFactory, outer unsafe.Pointer, riid *GUID, ppv *unsafe.Pointer) uintptr {
		return this.CreateInstance(outer, riid, ppv)
	}),
	LockServer: syscall.NewCallback(func(this *ClassFactory, lock uintptr) uintptr {
		// The Go runtime can never be unloaded from a host process (see
		// DllCanUnloadNow), so there is nothing to lock.
		return S_OK
	}),
}
