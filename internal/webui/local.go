//go:build !with_frontend

package webui

import (
	"io/fs"
	"os"
)

func defaultFiles() fs.FS { return os.DirFS("web/dist") }
