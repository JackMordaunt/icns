// Package icns implements an encoder for Apple's `.icns` file format.
// Reference: "https://en.wikipedia.org/wiki/Apple_Icon_Image_format".
//
// icns files allow for high resolution icons to make your apps look sexy.
// The most common ways to generate icns files are 1. use `iconutil` which is
// a Mac native cli utility, or 2. use tools that wrap `ImageMagick` which adds
// a large dependency to your project for such a simple use case.
//
// With this library you can use pure Go to create icns files from any source
// image, given that you can decode it into an `image.Image`, without any
// heavyweight dependencies or subprocessing required. You can also use this
// library to create icns files on windows and linux.
//
// A small CLI app `icnsify` is provided to allow you to create icns files
// using this library from the command line. It supports piping, which is
// something `iconutil` does not do, making it substantially easier to wrap.
//
// Note: icns files are written with an icon at every size macOS draws, the
// retina OSTypes for the larger ones and the colour and mask pair Apple still
// uses at 16 and 32 pixels.
package icns
