# icns

Easily convert `.jpg` and `.png` to `.icns` with the command line tool `icnsify`, or use the library to convert from any `image.Image` to `.icns`.

`go get github.com/jackmordaunt/icns`

`icns` files allow for high resolution icons to make your apps look sexy. The most common ways to generate icns files are 1. use `iconutil` which is a Mac native cli utility, or 2. use tools that wrap `ImageMagick` which adds a large dependency to your project for such a simple use case.

Note: All icons within the `icns` are sized for high dpi retina screens, using the appropriate `icns` OSTypes.

## Library Usage

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
  dest, err := os.Open("path/to/icon.icns")
  if err != nil {
    log.Fatalf("opening destination file: %v", err)
  }
  defer dest.Close()
  if err := icns.Encode(dest, srcImg); err != nil {
    log.Fatalf("encoding icns: %v", err)
  }
}
```

## Roadmap

* [x] Encoder: `image.Image -> .icns`
* [x] Command Line Interface
  * [x] Encoding
  * [x] Pipe support
  * [ ] Decoding
* [ ] Implement Decoder: `.icns -> image.Image`
* [ ] Symmetric test: `decode(encode(img)) == img`
* [ ] Encode based on input image format (jpg -> jpg, png -> png) to avoid lossy conversions
