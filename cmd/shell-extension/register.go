//go:build windows

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"unsafe"

	"github.com/jackmordaunt/icns/cmd/shell-extension/internal/provider"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Registry layout, relative to HKEY_CURRENT_USER or HKEY_LOCAL_MACHINE:
//
//	Software\Classes
//	  CLSID\{CLSID}                          (Default) = friendly name
//	    InprocServer32                       (Default) = <path to dll>
//	                                         ThreadingModel = Apartment
//	  .icns\ShellEx\{IID_IThumbnailProvider} (Default) = {CLSID}
//
// Registration goes to HKLM when the process is elevated and to HKCU otherwise.
//
// The two differ in how the handler is hosted. Explorer normally runs
// thumbnail providers in an isolated surrogate process (dllhost.exe), and that
// surrogate only resolves handler CLSIDs from HKLM: a CLSID registered solely
// under HKCU fails there with REGDB_E_CLASSNOTREG, and a stale HKLM entry
// shadows any per-user one. A per-user registration therefore sets
// DisableProcessIsolation so the provider is loaded into Explorer itself,
// which does honour HKCU. Machine-wide registration keeps the default,
// isolated hosting.
const (
	friendlyName         = "ICNS Thumbnail Provider"
	clsidString          = "{E21C95C5-5086-4F9F-8876-7FF4CE4AC6EC}"
	iidThumbnailProvider = "{E357FCCD-A995-4576-B01F-234630154E96}"

	clsidKey    = `Software\Classes\CLSID\` + clsidString
	inprocKey   = clsidKey + `\InprocServer32`
	shellExKey  = `Software\Classes\.icns\ShellEx`
	handlerKey  = shellExKey + `\` + iidThumbnailProvider
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Shell Extensions\Approved`
)

func init() {
	// Keep the string constants and the GUIDs the provider answers to in sync.
	if got := provider.CLSID.String(); got != clsidString {
		panic(fmt.Sprintf("CLSID mismatch: registry uses %s, provider uses %s", clsidString, got))
	}
	if got := provider.IID_IThumbnailProvider.String(); got != iidThumbnailProvider {
		panic(fmt.Sprintf("IID mismatch: registry uses %s, provider uses %s", iidThumbnailProvider, got))
	}
}

// hive is a registry root together with its conventional name for messages.
type hive struct {
	root registry.Key
	name string
}

var (
	currentUser  = hive{registry.CURRENT_USER, "HKCU"}
	localMachine = hive{registry.LOCAL_MACHINE, "HKLM"}
)

func elevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

type regValue struct{ key, name, data string }

func register() error {
	dll, err := modulePath()
	if err != nil {
		return fmt.Errorf("locating dll: %w", err)
	}
	h := currentUser
	values := []regValue{
		{clsidKey, "", friendlyName},
		{inprocKey, "", dll},
		{inprocKey, "ThreadingModel", "Apartment"},
		{handlerKey, "", clsidString},
	}
	if elevated() {
		h = localMachine
		// A per-user registration takes precedence in Explorer and would
		// keep the in-process hosting; make the machine-wide one authoritative.
		if err := unregisterFrom(currentUser); err != nil {
			return err
		}
		// Only consulted when the EnforceShellExtensionSecurity policy is on,
		// but it lives in HKLM so this is the only chance to write it.
		values = append(values, regValue{approvedKey, clsidString, friendlyName})
	}
	for _, v := range values {
		if err := h.setValue(v.key, v.name, v.data); err != nil {
			return err
		}
	}
	if h == currentUser {
		if err := h.setDWord(clsidKey, "DisableProcessIsolation", 1); err != nil {
			return err
		}
	}
	notifyAssocChanged()
	return nil
}

func unregister() error {
	hives := []hive{currentUser}
	if elevated() {
		hives = append(hives, localMachine)
	}
	for _, h := range hives {
		if err := unregisterFrom(h); err != nil {
			return err
		}
	}
	notifyAssocChanged()
	return nil
}

func unregisterFrom(h hive) error {
	if h == localMachine {
		if err := h.deleteValue(approvedKey, clsidString); err != nil {
			return err
		}
	}
	for _, key := range []string{handlerKey, shellExKey, inprocKey, clsidKey} {
		err := registry.DeleteKey(h.root, key)
		switch {
		case err == nil, errors.Is(err, registry.ErrNotExist):
		case key == shellExKey:
			// Other handlers may live under ShellEx; leave it in place.
		default:
			return fmt.Errorf("deleting %s\\%s: %w", h.name, key, err)
		}
	}
	return nil
}

func (h hive) setValue(key, name, data string) error {
	k, _, err := registry.CreateKey(h.root, key, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating %s\\%s: %w", h.name, key, err)
	}
	defer k.Close()
	if err := k.SetStringValue(name, data); err != nil {
		return fmt.Errorf("setting %s\\%s\\%q: %w", h.name, key, name, err)
	}
	return nil
}

func (h hive) setDWord(key, name string, data uint32) error {
	k, _, err := registry.CreateKey(h.root, key, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating %s\\%s: %w", h.name, key, err)
	}
	defer k.Close()
	if err := k.SetDWordValue(name, data); err != nil {
		return fmt.Errorf("setting %s\\%s\\%q: %w", h.name, key, name, err)
	}
	return nil
}

func (h hive) deleteValue(key, name string) error {
	k, err := registry.OpenKey(h.root, key, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening %s\\%s: %w", h.name, key, err)
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("deleting %s\\%s\\%q: %w", h.name, key, name, err)
	}
	return nil
}

// anchor is any symbol that lives inside this DLL's image; its address lets
// us ask the loader which module we are.
var anchor byte

// modulePath returns the absolute path of the loaded DLL.
func modulePath() (string, error) {
	var module windows.Handle
	err := windows.GetModuleHandleEx(
		windows.GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS|windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
		(*uint16)(unsafe.Pointer(&anchor)),
		&module,
	)
	if err != nil {
		return "", fmt.Errorf("GetModuleHandleEx: %w", err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetModuleFileName(module, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", fmt.Errorf("GetModuleFileName: %w", err)
	}
	return windows.UTF16ToString(buf[:n]), nil
}

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procSHChangeNotify = shell32.NewProc("SHChangeNotify")
)

const (
	shcneAssocChanged = 0x08000000
	shcnfIDList       = 0x0000
	shcnfFlush        = 0x1000
)

// notifyAssocChanged tells the shell that file associations changed so
// Explorer picks up the new handler without a restart.
func notifyAssocChanged() {
	procSHChangeNotify.Call(shcneAssocChanged, shcnfIDList|shcnfFlush, 0, 0)
}

// logError reports a failure to stderr, which regsvr32 discards, and to the
// file named by ICNS_SHELLEXT_LOG when set.
func logError(msg string, err error) {
	if path := os.Getenv("ICNS_SHELLEXT_LOG"); path != "" {
		if f, ferr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr == nil {
			defer f.Close()
			slog.New(slog.NewTextHandler(f, nil)).Error(msg, "err", err)
		}
	}
	slog.Error(msg, "err", err)
}
