package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDetailRejectsEmptyBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/detail", Detail(nil)) })
	target := "/api/detail"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestCatalogRejectsEmptyBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/catalog", Catalog(nil)) })
	target := "/api/catalog"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestContentRejectsEmptyChapterID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(nil)) })
	target := "/api/content"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}
