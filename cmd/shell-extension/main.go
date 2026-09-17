//go:build windows

// Command shell-extension builds a Windows shell extension DLL that lets
// Explorer render thumbnails for .icns files.
//
// The DLL is an in-process COM server implemented entirely in Go. cgo is used
// only to export the four entry points COM requires from any such server; the
// COM objects themselves live in the internal/provider and internal/com
// packages and contain no C.
//
// Build and register (per-user, no elevation needed):
//
//	go build -buildmode=c-shared -o icns-shellext.dll .
//	regsvr32 icns-shellext.dll
//
// Unregister with `regsvr32 /u icns-shellext.dll`.
package main

import "C"

import (
	"unsafe"

	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/com"
	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/provider"
)

// main is required by -buildmode=c-shared but never runs.
func main() {}

// factory builds thumbnail providers for the shell.
var factory = com.NewClassFactory(provider.Create)

// DllGetClassObject hands out the class factory for our CLSID.
//
//export DllGetClassObject
func DllGetClassObject(rclsid, riid unsafe.Pointer, ppv *unsafe.Pointer) uint32 {
	if ppv == nil {
		return uint32(com.E_POINTER)
	}
	*ppv = nil
	if !com.IsEqualGUID((*com.GUID)(rclsid), provider.CLSID) {
		return uint32(com.CLASS_E_CLASSNOTAVAILABLE)
	}
	return uint32(factory.QueryInterface((*com.GUID)(riid), ppv))
}

// DllCanUnloadNow always refuses: a Go runtime cannot be torn down and
// unloaded from a host process, so the DLL must stay resident until the host
// exits. Thumbnail providers run in a dedicated surrogate process that exits
// when idle, so this costs nothing in Explorer itself.
//
//export DllCanUnloadNow
func DllCanUnloadNow() uint32 {
	return uint32(com.S_FALSE)
}

// DllRegisterServer registers the provider for the current user.
//
//export DllRegisterServer
func DllRegisterServer() uint32 {
	if err := register(); err != nil {
		logError("registering", err)
		return uint32(com.E_FAIL)
	}
	return uint32(com.S_OK)
}

// DllUnregisterServer removes the current user's registration.
//
//export DllUnregisterServer
func DllUnregisterServer() uint32 {
	if err := unregister(); err != nil {
		logError("unregistering", err)
		return uint32(com.E_FAIL)
	}
	return uint32(com.S_OK)
}

// DllInstall supports `regsvr32 /n /i[:cmdline]`. Registration is always
// per-user, so the command line is ignored.
//
//export DllInstall
func DllInstall(install int32, cmdline unsafe.Pointer) uint32 {
	if install != 0 {
		return DllRegisterServer()
	}
	return DllUnregisterServer()
}
