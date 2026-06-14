//go:build ignore

// Generates the tray icon: a paw mark in TokenDog orange, anti-aliased via 4x
// supersampling. Writes tokendog.png (Linux/macOS) and tokendog.ico (Windows,
// a PNG-in-ICO container supported by modern Windows). Run with `go generate`.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	out = 32 // final icon size
	ss  = 4  // supersample factor
	big = out * ss
)

func main() {
	fg := color.RGBA{0xF5, 0xA6, 0x23, 0xFF} // TokenDog orange

	// Paw: one pad (ellipse) + four toes (circles), in 32px coordinate space.
	type circle struct{ cx, cy, r float64 }
	toes := []circle{
		{8.5, 12, 3.3},
		{13.5, 8.5, 3.3},
		{18.5, 8.5, 3.3},
		{23.5, 12, 3.3},
	}

	hi := image.NewRGBA(image.Rect(0, 0, big, big))
	for y := 0; y < big; y++ {
		for x := 0; x < big; x++ {
			px := (float64(x) + 0.5) / ss
			py := (float64(y) + 0.5) / ss
			inside := inEllipse(px, py, 16, 21, 7.5, 6.5) // pad
			for _, c := range toes {
				dx, dy := px-c.cx, py-c.cy
				if dx*dx+dy*dy <= c.r*c.r {
					inside = true
				}
			}
			if inside {
				hi.SetRGBA(x, y, fg)
			}
		}
	}

	// Box-downsample to out×out, averaging the alpha for smooth edges.
	small := image.NewRGBA(image.Rect(0, 0, out, out))
	for y := 0; y < out; y++ {
		for x := 0; x < out; x++ {
			var r, g, b, a int
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := hi.RGBAAt(x*ss+dx, y*ss+dy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
				}
			}
			n := ss * ss
			small.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err != nil {
		panic(err)
	}
	mustWrite("tokendog.png", buf.Bytes())
	mustWrite("tokendog.ico", wrapICO(buf.Bytes(), out))
}

func inEllipse(px, py, cx, cy, rx, ry float64) bool {
	dx := (px - cx) / rx
	dy := (py - cy) / ry
	return dx*dx+dy*dy <= 1
}

// wrapICO packs a PNG into a single-image .ico container.
func wrapICO(pngBytes []byte, size int) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w(uint16(0))             // reserved
	w(uint16(1))             // type: icon
	w(uint16(1))             // image count
	b.WriteByte(byte(size))  // width
	b.WriteByte(byte(size))  // height
	b.WriteByte(0)           // palette size (0 = none)
	b.WriteByte(0)           // reserved
	w(uint16(1))             // color planes
	w(uint16(32))            // bits per pixel
	w(uint32(len(pngBytes))) // image data size
	w(uint32(6 + 16))        // offset to image data
	b.Write(pngBytes)
	return b.Bytes()
}

func mustWrite(name string, data []byte) {
	if err := os.WriteFile(name, data, 0o644); err != nil {
		panic(err)
	}
}
