package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
)

// envelope 用于测试里解响应封装。
type envelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

func newValidationEngine(register func(*gin.Engine)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	register(r)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, target string) (int, envelope) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("响应体不是 JSON: %v\n%s", err, w.Body.String())
	}
	return w.Code, e
}

// decodeData 解 data 字段到 v。
func decodeData(raw json.RawMessage, v any) error {
	return json.Unmarshal(raw, v)
}

// wantBadRequest 断言 400 + 40001 + data null。
func wantBadRequest(t *testing.T, target string, code int, e envelope) {
	t.Helper()
	if code != http.StatusBadRequest {
		t.Fatalf("%s: HTTP status = %d, want 400", target, code)
	}
	if e.Code != api.CodeBadRequest {
		t.Fatalf("%s: code = %d, want 40001", target, e.Code)
	}
	if string(e.Data) != "null" {
		t.Fatalf("%s: data 应为 null: %s", target, string(e.Data))
	}
}
