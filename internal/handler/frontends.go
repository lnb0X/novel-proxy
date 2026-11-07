package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/frontend"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
)

func validApp(app string) bool {
	return app == "reader" || app == "admin"
}

// ListFrontends GET /<secret>/api/frontends
func ListFrontends(m *frontend.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		out := gin.H{}
		for _, app := range []string{"reader", "admin"} {
			current := m.CurrentBasis(app)
			previous, _ := m.ReadPrevious(app)
			out[app] = gin.H{
				"current":  current,
				"previous": previous,
				"versions": m.ListVersions(app),
			}
		}
		api.OK(c, out)
	}
}

// UploadFrontend POST /<secret>/api/frontends/:app/upload，multipart 字段 file。
func UploadFrontend(m *frontend.Manager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.Param("app")
		if !validApp(app) {
			api.Fail(c, api.CodeBadRequest, "app 只能是 reader 或 admin")
			return
		}
		fh, err := c.FormFile("file")
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "multipart 缺少 file 字段")
			return
		}
		src, err := fh.Open()
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "读取上传文件失败")
			return
		}
		defer src.Close()
		info, err := m.Upload(app, src)
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		au.Audit("frontend_upload", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"app="+app+" version="+info.Version, "ok")
		api.OK(c, info)
	}
}

// ActivateFrontend POST /<secret>/api/frontends/:app/activate
func ActivateFrontend(m *frontend.Manager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.Param("app")
		if !validApp(app) {
			api.Fail(c, api.CodeBadRequest, "app 只能是 reader 或 admin")
			return
		}
		var body struct {
			Version              string `json:"version"`
			ConfirmWithinSeconds int    `json:"confirm_within_seconds"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Version == "" {
			api.Fail(c, api.CodeBadRequest, "请求体必须含 version")
			return
		}
		within := time.Duration(body.ConfirmWithinSeconds) * time.Second
		if within <= 0 {
			within = 10 * time.Second
		}
		previous, err := m.Activate(app, body.Version, within)
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		api.Logger(c).Info("frontend.activate", "app", app, "from", previous, "to", body.Version)
		au.Audit("frontend_activate", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"app="+app+" from="+previous+" to="+body.Version, "ok")
		api.OK(c, gin.H{"version": body.Version, "previous": previous, "confirm_within_seconds": int(within.Seconds())})
	}
}

// ConfirmFrontend POST /<secret>/api/frontends/:app/confirm
func ConfirmFrontend(m *frontend.Manager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.Param("app")
		if !validApp(app) {
			api.Fail(c, api.CodeBadRequest, "app 只能是 reader 或 admin")
			return
		}
		var body struct {
			Version string `json:"version"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Version == "" {
			api.Fail(c, api.CodeBadRequest, "请求体必须含 version")
			return
		}
		if err := m.Confirm(app, body.Version); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		api.OK(c, gin.H{"version": body.Version})
	}
}

// RollbackFrontend POST /<secret>/api/frontends/:app/rollback
func RollbackFrontend(m *frontend.Manager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.Param("app")
		if !validApp(app) {
			api.Fail(c, api.CodeBadRequest, "app 只能是 reader 或 admin")
			return
		}
		if err := m.Rollback(app); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		au.Audit("frontend_rollback", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"app="+app, "ok")
		api.OK(c, gin.H{"current": m.CurrentBasis(app)})
	}
}

// DeleteFrontend DELETE /<secret>/api/frontends/:app/:version
func DeleteFrontend(m *frontend.Manager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.Param("app")
		if !validApp(app) {
			api.Fail(c, api.CodeBadRequest, "app 只能是 reader 或 admin")
			return
		}
		version := c.Param("version")
		if version == "" || version == "default" {
			api.Fail(c, api.CodeBadRequest, "default 目录不可删除")
			return
		}
		if err := m.DeleteVersion(app, version); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		m.Reload(app)
		au.Audit("frontend_delete", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"app="+app+" version="+version, "ok")
		c.Status(204)
	}
}
