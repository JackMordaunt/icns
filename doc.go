// Package icns implements an encoder for Apple's `.icns` file format.
// Reference: "https://en.wikipedia.org/wiki/Apple_Icon_Image_format".
//
// icns files allow for high resolution icons to make your apps look sexy.
// The most common ways to generate icns files are 1. use `iconutil` which is
// a Mac native cli utility, or 2. use tools that wrap `ImageMagick` which adds
// a large dependency to your project for such a simple use case.
//
// Note: All icons within the icns are sized for high dpi retina screens.
//
// Todo(jackmordaunt):
// - Write tests (only manual testing has been done)
// 	- How to test the correctness of a file format?
// - Create Decoder (.icns -> image.Image)
// - Encode based on input image format (jpg -> jpg, png -> png) to avoid
// 	lossy conversions
package icns
