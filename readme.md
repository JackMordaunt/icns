# icns

[![CI](https://github.com/JackMordaunt/icns/actions/workflows/ci.yml/badge.svg)](https://github.com/JackMordaunt/icns/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jackmordaunt/icns/v4.svg)](https://pkg.go.dev/github.com/jackmordaunt/icns/v4)

Easily convert `.jpg` and `.png` to `.icns` with the command line tool `icnsify`, or use the library to convert from any `image.Image` to `.icns`.

`go get github.com/jackmordaunt/icns/v4`

`icns` files allow for high resolution icons to make your apps look sexy. The most common ways to generate icns files are:

1. `iconutil`, which is a Mac native cli utility.
2. `ImageMagick` which adds a large dependency to your project for such a simple use case.

With this library you can use pure Go to create `icns` files from any source image, given that you can decode it into an `image.Image`, without any heavyweight dependencies or subprocessing required. You can also use it to create icns files on windows and linux (thanks Go).

A small CLI app `icnsify` is provided allowing you to create icns files using this library from the command line. It supports piping, which is something `iconutil` does not do, making it substantially easier to wrap or chuck into a shell pipeline.

Note: `icns` files are written with an icon at every size macOS draws, the retina OSTypes for the larger ones and the colour and mask pair Apple still uses at 16 and 32 pixels. Decoding reaches further back than writing does. Alongside PNG it understands the `is32`, `il32`, `ih32` and `it32` colour and mask elements, the `ARGB` sidebar and toolbar icons, and the 1-, 4- and 8-bit indexed icons of System 7 through Mac OS 8. Where a file holds the same icon at several depths, the richest is returned first. JPEG 2000 icons are identified but not decoded; `Entry.Payload` hands over their bytes.

## GUI

`preview` is a gui for displaying `icns` files cross-platform.

### Go Tool

```
go install github.com/jackmordaunt/icns/cmd/preview@latest
```

### Clone

```
git clone https://github.com/jackmordaunt/icns
cd icns/cmd/preview && go install .
```

Note: Gio cannot be cross-compiled right now, so there are no `preview` builds in releases.
Note: `preview` has its own `go.mod` and therefore is versioned independently (unversioned).

![preview](docs/preview.png)

## Windows Explorer thumbnails

`cmd/shell-extension` is a Windows shell extension that renders `.icns` thumbnails in Explorer. It is a COM server written in Go and built as a DLL; see [its readme](cmd/shell-extension/readme.md) for build and registration steps.

## Command Line

### Go Tool

```
go install github.com/jackmordaunt/icns/v4/cmd/icnsify@latest
```

### [Scoop](https://scoop.sh/)

```powershell
scoop bucket add extras # Ensure bucket is added first
scoop install icnsify
```

Or from my personal bucket:

```powershell
scoop bucket add jackmordaunt https://github.com/jackmordaunt/scoop-bucket 
scoop install jackmordaunt/icns # Name is defaulted to repo name. 
```

### [Winget](https://learn.microsoft.com/en-us/windows/package-manager/)

```powershell
winget install icnsify
```

### [Brew](https://brew.sh)

```sh
brew tap jackmordaunt/homebrew-tap # Ensure tap is added first.
brew install icnsify
```

### Clone

```
git clone https://github.com/jackmordaunt/icns
cd icns && go install ./cmd/icnsify
```

Pipe it

`cat icon.png | icnsify > icon.icns`

`cat icon.icns | icnsify > icon.png`

Standard

`icnsify -i icon.png -o icon.icns`

`icnsify -i icon.icns -o icon.png`

From an iconset directory, which uses each drawing where it is given instead of resizing one image for every size

`icnsify -i MyIcon.iconset -o MyIcon.icns`

## Library

`go get github.com/jackmordaunt/icns/v4`

```go
func main() {
        pngf, err := os.Open("path/to/icon.png")
        if err != nil {
                log.Fatalf("opening source image: %v", err)
        }
        defer pngf.Close()
        srcImg, _, err := image.Decode(pngf)
        if err != nil {
                log.Fatalf("decoding source image: %v", err)
        }
        dest, err := os.Create("path/to/icon.icns")
        if err != nil {
                log.Fatalf("opening destination file: %v", err)
        }
        defer dest.Close()
        if err := icns.Encode(dest, srcImg); err != nil {
                log.Fatalf("encoding icns: %v", err)
        }
}
```

### Per-size artwork

Icons are usually hand tuned at the small sizes rather than reduced from the large one. `EncodeSlots` takes a drawing per slot and resizes only the slots left empty, filling them from the largest image given.

```go
images := map[icns.Slot]image.Image{
        {Points: 16, Scale: 1}:  small, // icon_16x16.png
        {Points: 512, Scale: 2}: large, // icon_512x512@2x.png
}
if err := icns.NewEncoder(dest).EncodeSlots(images); err != nil {
        log.Fatalf("encoding icns: %v", err)
}
```

A slot is a size and a display scale, because 16x16@2x and 32x32 are both 32 pixels of artwork but fill different elements. `icns.Slots()` lists the ten a file holds and `icns.ParseSlot` reads iconset file names.

### Reading one size

`NewDecoder` identifies the icons without decoding any of them, so reading a single size does not pay for the rest.

```go
d, err := icns.NewDecoder(src)
if err != nil {
        log.Fatalf("reading icns: %v", err)
}
for _, icon := range d.Icons() { // Largest first.
        if icon.Size > 128 {
                continue
        }
        img, err := icon.Decode()
        ...
}
```

`Entry.Payload` returns the bytes the file stores, which is how to reach a JPEG 2000 icon: this package identifies that format but cannot decode it.

## Development

The repository is a Go workspace of three modules:

| Module | Contents | Why separate |
|---|---|---|
| `github.com/jackmordaunt/icns/v4` (root) | The library and `cmd/icnsify` | Depends only on `golang.org/x/image`; one tag versions both |
| `github.com/jackmordaunt/icns/cmd/preview` | The Gio GUI | Keeps Gio's dependency tree out of library consumers' module graphs |
| `github.com/jackmordaunt/icns/cmd/shell-extension` | The Windows DLL | Windows-only and needs cgo (mingw) |

`go.work` ties them together, so every build inside the checkout uses the working-tree library and gopls sees the whole repository. Note that `./...` only matches the module you are in; to cover all three from the root, name them:

```
go test ./... ./cmd/preview/... ./cmd/shell-extension/...
```

The two command modules require the library at a published tag. Inside the workspace that requirement only shapes the module graph; the code always comes from the working tree, so the tag can lag behind without affecting development.

CI builds and tests the library on Linux, macOS and Windows, runs the encoder under the race detector, fuzzes the decoder, and reports `gofmt`, `go vet` and `govulncheck`. The shell extension is tested on Windows, where cgo can reach a C compiler, and `preview` is built on macOS and Windows, since Gio needs a long list of X11 and Wayland headers on Linux. Both command modules are also built with `GOWORK=off`, which is what `go install` sees.

Releasing: tag the root module (`vX.Y.Z`), which releases the library and `icnsify` together. When `go install .../cmd/preview@latest`, or a shell extension built outside the checkout, should pick up a newer library, bump that module's requirement and tidy it outside the workspace:

```powershell
$env:GOWORK = 'off'; go get github.com/jackmordaunt/icns/v4@latest; go mod tidy; Remove-Item Env:GOWORK
```

## Roadmap

- [x] Encoder: `image.Image -> .icns`
- [x] Command Line Interface
  - [x] Encoding
  - [x] Pipe support
  - [x] Decoding
- [x] Implement Decoder: `.icns -> image.Image`
- [x] Symmetric test: `decode(encode(img)) == img`
- [x] Windows Explorer thumbnails

## Coffee

If this software is useful to you, consider buying me a coffee!

[https://liberapay.com/JackMordaunt](https://liberapay.com/JackMordaunt)
