//go:build !embed_web

package webui

import "io/fs"

// Assets returns nil for local builds, where Rsbuild serves the frontend.
func Assets() fs.FS {
	return nil
}
