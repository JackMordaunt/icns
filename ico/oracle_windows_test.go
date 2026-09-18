//go:build windows

package ico

// Windows decides what an ico file may contain, so these tests hand it files
// this package wrote and read back a file it wrote itself. They run wherever
// Windows PowerShell is present, which covers the Windows CI runner.

import (
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// readFrames lists every frame the Windows Imaging Component finds, as
// "width height r g b a". This is the decoder behind Explorer's preview.
const readFrames = `
param([string]$Path)
Add-Type -AssemblyName PresentationCore
$stream = [System.IO.File]::OpenRead($Path)
$decoder = [System.Windows.Media.Imaging.BitmapDecoder]::Create($stream, 'None', 'OnLoad')
foreach ($frame in $decoder.Frames) {
    $converted = New-Object System.Windows.Media.Imaging.FormatConvertedBitmap($frame, [System.Windows.Media.PixelFormats]::Bgra32, $null, 0)
    $pixel = New-Object byte[] 4
    $at = New-Object System.Windows.Int32Rect ([int]($converted.PixelWidth/2)), ([int]($converted.PixelHeight/2)), 1, 1
    $converted.CopyPixels($at, $pixel, 4, 0)
    "{0} {1} {2} {3} {4} {5}" -f $converted.PixelWidth, $converted.PixelHeight, $pixel[2], $pixel[1], $pixel[0], $pixel[3]
}
$stream.Close()
`

// writeIcon has Windows write an ico file of one solid colour.
const writeIcon = `
param([string]$Path, [int]$Size, [int]$R, [int]$G, [int]$B)
Add-Type -AssemblyName System.Drawing
$bitmap = New-Object System.Drawing.Bitmap($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.Clear([System.Drawing.Color]::FromArgb(255, $R, $G, $B))
$graphics.Dispose()
$icon = [System.Drawing.Icon]::FromHandle($bitmap.GetHicon())
$stream = [System.IO.File]::Create($Path)
$icon.Save($stream)
$stream.Close()
`

// powershell runs a script and returns what it printed, failing the test
// with whatever it wrote if it did not run.
func powershell(t *testing.T, script string, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oracle.ps1")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	args = append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}, args...)
	out, err := exec.Command("powershell.exe", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("powershell %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestOracleWindowsReads checks that Windows finds every icon this package
// writes, at the size and colour it was given.
func TestOracleWindowsReads(t *testing.T) {
	var (
		images = map[uint]image.Image{}
		want   = map[int]color.NRGBA{}
	)
	for i, size := range Sizes() {
		c := color.NRGBA{R: uint8(20 + i*30), G: uint8(200 - i*20), B: 0x40, A: 0xFF}
		images[size] = solid(int(size), c)
		want[int(size)] = c
	}
	path := filepath.Join(t.TempDir(), "icon.ico")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewEncoder(f).EncodeSizes(images); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	out := powershell(t, readFrames, "-Path", path)
	seen := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 6 {
			continue
		}
		n := make([]int, 6)
		for i, field := range fields {
			if n[i], err = strconv.Atoi(field); err != nil {
				t.Fatalf("the oracle printed %q", line)
			}
		}
		width, height := n[0], n[1]
		if width != height {
			t.Errorf("Windows read a %dx%d icon, which is not square", width, height)
		}
		got := color.NRGBA{R: uint8(n[2]), G: uint8(n[3]), B: uint8(n[4]), A: uint8(n[5])}
		if got != want[width] {
			t.Errorf("Windows read the %d pixel icon as %v, want %v", width, got, want[width])
		}
		seen[width] = true
	}
	for size := range want {
		if !seen[size] {
			t.Errorf("Windows did not read the %d pixel icon at all:\n%s", size, out)
		}
	}
}

// TestOracleWindowsWrites decodes an icon Windows wrote. It writes them
// indexed, so this covers the colour table against a real writer.
func TestOracleWindowsWrites(t *testing.T) {
	const size = 48
	// Red is in the palette Windows picks, so the colour survives exactly.
	want := color.NRGBA{R: 0xFF, A: 0xFF}
	path := filepath.Join(t.TempDir(), "windows.ico")
	powershell(t, writeIcon,
		"-Path", path,
		"-Size", strconv.Itoa(size),
		"-R", strconv.Itoa(int(want.R)),
		"-G", strconv.Itoa(int(want.G)),
		"-B", strconv.Itoa(int(want.B)),
	)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != size {
		t.Errorf("decoded a %d pixel icon, want %d", got, size)
	}
	if got := centre(img); got != want {
		t.Errorf("centre = %v, want %v", got, want)
	}
}
