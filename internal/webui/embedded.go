//go:build with_frontend

package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var content embed.FS

func defaultFiles() fs.FS {
	files, err := fs.Sub(content, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
