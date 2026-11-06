package upstream

import (
	"errors"
	"strings"
	"testing"
)

// 样本取自 2026-11-07 对上游的实测结构（字段有节选）。
const searchSampleJSON = `{
  "code": 0,
  "message": "success",
  "log_id": "202611070120099F1C61FB55993D9F6384",
  "data": {
    "exactly_match": true,
    "has_more": true,
    "offset": 10,
    "search_id": "202611070120099F1C61FB55993D9F6384",
    "ret_data": [
      {
        "abstract": " ",
        "author": "用户9318260224596",
        "book_id": "7637198840910580798",
        "category": "",
        "thumb_url": "https://p6-tt.bytecdn.cn/novel-pic/abc.image",
        "title": "《斗破苍穹》",
        "score": "8.5",
        "creation_status": "0"
      },
      {
        "abstract": "  一段有内容的简介  ",
        "author": "天蚕土豆",
        "book_id": "7579985241087937560",
        "category": "玄幻",
        "thumb_url": "https://p3-novel.byteimg.com/xyz.image",
        "title": "斗破苍穹",
        "score": "9.1"
      }
    ]
  }
}`

func TestParseSearchNormal(t *testing.T) {
	r, err := ParseSearch([]byte(searchSampleJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 2 {
		t.Fatalf("items 数量 = %d", len(r.Items))
	}
	first := r.Items[0]
	if first.BookID != "7637198840910580798" {
		t.Errorf("book_id 必须保持字符串: %q", first.BookID)
	}
	if first.Abstract != "" {
		t.Errorf("纯空白 abstract 应 trim 成空串: %q", first.Abstract)
	}
	if first.Category != "" {
		t.Errorf("category 空串应透传: %q", first.Category)
	}
	second := r.Items[1]
	if second.Abstract != "一段有内容的简介" {
		t.Errorf("abstract 应 TrimSpace: %q", second.Abstract)
	}
	if second.Author != "天蚕土豆" || second.Title != "斗破苍穹" || second.Category != "玄幻" {
		t.Errorf("字段映射不符: %+v", second)
	}
	if r.HasMore != true {
		t.Errorf("has_more = %v", r.HasMore)
	}
	if r.NextOffset != 10 {
		t.Errorf("next_offset = %d，应取上游 offset", r.NextOffset)
	}
	// page 由 handler 填，Parse 不设置。
	if r.Page != 0 {
		t.Errorf("Parse 不应填 page: %d", r.Page)
	}
}

func TestParseSearchEmptyRetData(t *testing.T) {
	// ret_data 非 nil 空数组 = 真没搜到，200 空数组。
	raw := `{"code":0,"data":{"has_more":false,"offset":0,"ret_data":[]}}`
	r, err := ParseSearch([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if r.Items == nil || len(r.Items) != 0 {
		t.Fatalf("应得到非 nil 空数组: %#v", r.Items)
	}
}

func TestParseSearchNilRetData(t *testing.T) {
	// ret_data 为 null = 上游异常，必须报错，不得吞成空结果。
	raw := `{"code":0,"data":{"has_more":false,"offset":0,"ret_data":null}}`
	_, err := ParseSearch([]byte(raw))
	if err == nil {
		t.Fatal("ret_data 为 null 应报错")
	}
}

func TestParseSearchNilData(t *testing.T) {
	raw := `{"code":0,"data":null}`
	_, err := ParseSearch([]byte(raw))
	if err == nil {
		t.Fatal("data 为 null 应报错")
	}
}

func TestParseSearchBizError(t *testing.T) {
	raw := `{"code":1001004,"message":"书不存在","data":{}}`
	_, err := ParseSearch([]byte(raw))
	var biz *BizError
	if !errors.As(err, &biz) {
		t.Fatalf("应返回 *BizError: %v", err)
	}
	if biz.Code != 1001004 || biz.Message != "书不存在" {
		t.Fatalf("BizError 字段不符: %+v", biz)
	}
	if !strings.Contains(err.Error(), "code=1001004") {
		t.Fatalf("错误信息必须带上游 code: %v", err)
	}
}

func TestParseSearchBadJSON(t *testing.T) {
	_, err := ParseSearch([]byte("not json"))
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
