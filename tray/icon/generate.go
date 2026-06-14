//go:build ignore

// Regenerates tokendog.ico from tokendog.png. The PNG is the TokenDog brand
// badge (the engraved Samoyed in a bone ring) from the brand kit; this step
// just wraps those exact bytes in a single-image .ico container for Windows
// (modern Windows reads PNG-in-ICO). Run with `go generate ./...`.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/png"
	"os"
)

func main() {
	png, err := os.ReadFile("tokendog.png")
	if err != nil {
		panic(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("tokendog.ico", wrapICO(png, cfg.Width), 0o644); err != nil {
		panic(err)
	}
}

// wrapICO packs a PNG into a single-image .ico container. A width/height byte
// of 0 means 256; sizes ≥256 are stored as 0 per the ICO spec.
func wrapICO(png []byte, size int) []byte {
	dim := byte(size)
	if size >= 256 {
		dim = 0
	}
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	w(uint16(0))         // reserved
	w(uint16(1))         // type: icon
	w(uint16(1))         // image count
	b.WriteByte(dim)     // width
	b.WriteByte(dim)     // height
	b.WriteByte(0)       // palette size (0 = none)
	b.WriteByte(0)       // reserved
	w(uint16(1))         // color planes
	w(uint16(32))        // bits per pixel
	w(uint32(len(png)))  // image data size
	w(uint32(6 + 16))    // offset to image data
	b.Write(png)
	return b.Bytes()
}
