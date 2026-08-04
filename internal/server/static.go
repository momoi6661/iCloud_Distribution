// static.go - 内嵌前端构建产物。
//
// web/dist 由 `cd web && npm run build` 生成,编译时通过 embed.FS 打包进二进制,
// 实现单文件部署。开发模式下前端由 Vite dev server 代理,后端只需 API。
package server

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var webDist embed.FS

// StaticFS 返回 web/dist 子目录的文件系统。
// web/dist 不存在有效构建时,占位 index.html 会提示先构建前端。
func StaticFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}
