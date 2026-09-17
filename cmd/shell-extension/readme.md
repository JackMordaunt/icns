# icns shell extension

A Windows shell extension that renders thumbnails for `.icns` files in
Explorer. It is an in-process COM server implementing `IInitializeWithStream`
and `IThumbnailProvider`, written in Go.

## How it works

- `internal/com` has the minimal COM plumbing: GUIDs, `HRESULT`s, the
  `IUnknown`/`IClassFactory` vtables, an `IStream` → `io.Reader` adapter, and a
  generic class factory.
- `internal/provider` is the COM object. Each instance is a Go struct whose
  first two fields are vtable pointers (one per interface); the vtables are
  package-level singletons filled with `syscall.NewCallback` trampolines.
  Instances are reference counted, pinned, and tracked in a global set so the
  GC never frees an object the shell still points at.
- `main.go` exports the entry points COM requires (`DllGetClassObject`,
  `DllCanUnloadNow`, `DllRegisterServer`, `DllUnregisterServer`, `DllInstall`).
  This is the only place cgo appears, and it contains no C code; cgo is needed
  purely because `go build -buildmode=c-shared` requires it.
- `register.go` writes the registry entries and notifies the shell.

Thumbnail selection picks the smallest icon that is at least the requested size
and downsamples it with Lanczos; if none is large enough the largest icon is
returned as-is. The bitmap handed to the shell is a top-down 32bpp DIB with
premultiplied alpha (`WTSAT_ARGB`).

## Building

A GCC-compatible toolchain (mingw-w64) must be on `PATH`; cgo does not support
MSVC. With scoop: `scoop install mingw`.

```powershell
go build -buildmode=c-shared -o icns-shellext.dll .
```

## Installing

Keep the DLL where it is registered from; the absolute path is written to the
registry. There are two modes, chosen by whether `regsvr32` runs elevated:

| | Per-user (`regsvr32` from a normal prompt) | Machine-wide (`regsvr32` from an elevated prompt) |
|---|---|---|
| Registry | `HKCU\Software\Classes` | `HKLM\SOFTWARE\Classes` + Shell Extensions `Approved` list |
| Hosting | Inside `explorer.exe` (`DisableProcessIsolation=1`) | Isolated `dllhost.exe` surrogate (the shell default) |
| Trade-off | No elevation; a crash in the provider would take Explorer down | Safer hosting; needs admin once |

```powershell
regsvr32 icns-shellext.dll      # register
regsvr32 /u icns-shellext.dll   # unregister (run elevated to remove an HKLM registration)
```

Why the per-user mode runs in-process: the shell's thumbnail surrogate only
resolves handler CLSIDs from HKLM. A CLSID registered solely under HKCU fails
there with `REGDB_E_CLASSNOTREG` (observed on Windows 11 23H2), and any HKLM
entry for the CLSID, including a stale one pointing at a DLL that no longer
exists, shadows the per-user entry. Elevated registration removes the per-user
entries so the machine-wide, isolated configuration is the one in effect.

Explorer caches thumbnails, so files viewed before installation may keep their
generic icon until the cache is rebuilt (Disk Cleanup → Thumbnails, or copy the
file). In the isolated mode a replaced DLL only takes effect once the surrogate
exits (a few seconds idle) or you run `taskkill /f /im dllhost.exe`; in the
per-user mode Explorer itself has to be restarted.

## Testing

`go test .` builds the DLL, loads it, and drives it through the raw COM
vtables: class factory, `Initialize` with an in-memory `IStream`, interface
identity, `GetThumbnail` at several sizes, and pixel-level checks of the
returned bitmap (channel order, premultiplied alpha, top-down rows).

`ICNS_SHELLEXT_E2E=1 go test -run TestExplorer .` additionally asks the shell
itself (`IShellItemImageFactory`) for a thumbnail of a temporary `.icns` file,
which exercises the registry entries and whichever hosting mode is registered.
It requires the DLL to be registered first.

## Debugging

Registration failures are logged to the file named by the `ICNS_SHELLEXT_LOG`
environment variable when set. Thumbnail extraction errors go to the host
process's stderr, which is normally discarded; the in-process test above is the
easiest way to reproduce a decoding problem.

Useful `HRESULT`s from `IShellItemImageFactory::GetImage(SIIGBF_THUMBNAILONLY)`:

| Code | Meaning here |
|---|---|
| `0x8004B200` `WTS_E_FAILEDEXTRACTION` | No handler for the extension, or the handler failed to decode |
| `0x80040154` `REGDB_E_CLASSNOTREG` | Handler found but its CLSID is not visible to the host (per-user CLSID in the surrogate) |
| `0x8007007E` `ERROR_MOD_NOT_FOUND` | The registered DLL path does not exist (typically a stale HKLM entry) |
