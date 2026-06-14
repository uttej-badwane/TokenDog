// Package icon embeds the tray icon. Windows wants ICO bytes; Linux/macoS use
// PNG. Regenerate the image files with `go generate ./...`.
package icon

import (
	_ "embed"
	"runtime"
)

//go:generate go run generate.go

//go:embed tokendog.png
var pngData []byte

//go:embed tokendog.ico
var icoData []byte

// Data returns the tray icon bytes appropriate for the current OS.
func Data() []byte {
	if runtime.GOOS == "windows" {
		return icoData
	}
	return pngData
}
