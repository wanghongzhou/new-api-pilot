//go:build embed_web

package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedAssets embed.FS

// Assets returns the production frontend embedded by the Docker build.
func Assets() fs.FS {
	assets, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	return assets
}
