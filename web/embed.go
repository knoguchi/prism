package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded dist filesystem
// Returns nil if dist directory doesn't exist (dev mode)
func DistFS() fs.FS {
	distDir, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	return distDir
}