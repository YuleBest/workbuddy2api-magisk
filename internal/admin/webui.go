// webui.go 内嵌前端资源与 SPA 路由。
//
// dist 由 web/ 前端构建产出（pnpm build，见 DEVELOPMENT.md）；编译期经 go:embed
// 打进二进制，部署只带一个可执行文件。dist 不存在时 embed 会编译失败，因此仓库
// 里始终保留一个占位 index.html（未构建时页面会提示如何构建）。
package admin

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// redirectIndex /admin → /admin/（相对路径解析需要尾斜杠）。
func (h *Handler) redirectIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
}

// serveSPA 提供静态资源；未命中的路径回退 index.html（前端 history 路由接管）。
//
// 缓存策略：Vite 产物 assets/ 下是内容哈希文件名，可长缓存；index.html 必须
// no-cache，否则前端发新版后用户仍拿旧壳去请求已删除的 assets。
func (h *Handler) serveSPA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/admin/")
	if name == "" {
		name = "index.html"
	}
	if strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	if f, st, ok := openAsset(name); ok {
		defer f.Close()
		writeAsset(w, r, name, f, st)
		return
	}
	// SPA 回退：/admin/accounts 这类前端路由交给 index.html。
	idx, st, ok := openAsset("index.html")
	if !ok {
		http.Error(w, "前端资源未构建：请在 web/ 目录执行 pnpm install && pnpm build", http.StatusServiceUnavailable)
		return
	}
	defer idx.Close()
	writeAsset(w, r, "index.html", idx, st)
}

// openAsset 打开内嵌资源；目录与不存在都返回 ok=false（交给 SPA 回退）。
func openAsset(name string) (fs.File, fs.FileInfo, bool) {
	f, err := distFS.Open("dist/" + name)
	if err != nil {
		return nil, nil, false
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		return nil, nil, false
	}
	return f, st, true
}

// writeAsset 定 Content-Type/缓存头并写响应。
func writeAsset(w http.ResponseWriter, r *http.Request, name string, f fs.File, st fs.FileInfo) {
	if strings.HasPrefix(name, "assets/") {
		// 内容哈希文件名：一年长缓存。
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if strings.HasSuffix(name, ".html") {
		ctype = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	seeker, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		http.Error(w, "unsupported asset", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), seeker)
}
