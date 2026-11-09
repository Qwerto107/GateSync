package webui

import (
	"io/fs"
	"os"
)

// 配置了外部目录时优先使用该目录，否则使用当前构建提供的页面。
func Files(root string) fs.FS {
	if root != "" {
		return os.DirFS(root)
	}
	return defaultFiles()
}
