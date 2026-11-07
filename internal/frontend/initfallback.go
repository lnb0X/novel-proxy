package frontend

import (
	"fmt"
	"net/http"
	"path/filepath"
)

// InitFallbackPage init 前端未部署时的固定说明页：
// 告诉部署者把 web-init 构建产物放到哪个绝对路径。
// 页面只含部署路径这一项必要信息，不走版本管理（见 DECISIONS init 页面节）。
func InitFallbackPage(targetDir string) http.Handler {
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		abs = targetDir
	}
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh">
<head><meta charset="utf-8"><title>init 前端未部署</title></head>
<body>
<h1>init 前端未部署</h1>
<p>把 web-init/ 的构建产物（dist 内容）放到下面的目录后重启服务：</p>
<p><code>%s</code></p>
<p>也可以放到版本目录并写 data/frontends/init-current.json。详见 DECISIONS.md 的「init 页面」一节。</p>
</body>
</html>
`, abs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	})
}
