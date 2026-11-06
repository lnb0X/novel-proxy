package upstream

import (
	"reflect"
	"testing"
)

// 样本取自 2026-11-07 对上游的实测结构（两卷，字段有节选）。
const catalogSampleJSON = `{
  "code": 0,
  "message": "success",
  "data": {
    "allItemIds": ["7579988190329588761", "7579988172734467097", "8888888888888888888", "7579988114697899033"],
    "volumeNameList": ["第一卷", "第二卷"],
    "chapterListWithVolume": [
      [
        {"itemId": "7579988172734467097", "needPay": 0, "title": "第1集", "isChapterLock": false, "isPaidPublication": false, "isPaidStory": false, "firstPassTime": "1764853574"},
        {"itemId": "7579988190329588761", "needPay": 2, "title": "第2集", "isChapterLock": true, "isPaidPublication": false, "isPaidStory": false, "firstPassTime": "1764853569"}
      ],
      [
        {"itemId": "7579988114697899033", "needPay": 0, "title": "第3集", "isChapterLock": false, "isPaidPublication": true, "isPaidStory": false, "firstPassTime": "1764853560"}
      ]
    ]
  }
}`

func TestParseCatalogNormal(t *testing.T) {
	c, err := ParseCatalog([]byte(catalogSampleJSON))
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 3 {
		t.Fatalf("total = %d，缺失章节应被跳过", c.Total)
	}
	if len(c.Missing) != 1 || c.Missing[0] != "8888888888888888888" {
		t.Fatalf("missing = %v", c.Missing)
	}
	// 顺序必须与 allItemIds 一致，不是 chapterListWithVolume 的展开顺序。
	wantOrder := []string{"7579988190329588761", "7579988172734467097", "7579988114697899033"}
	for i, want := range wantOrder {
		if c.Chapters[i].ChapterID != want {
			t.Fatalf("chapters[%d].chapter_id = %q, want %q", i, c.Chapters[i].ChapterID, want)
		}
	}
	first := c.Chapters[1]
	if first.Title != "第1集" || first.Volume != "第一卷" {
		t.Errorf("第1集映射不符: %+v", first)
	}
	if first.IsPay || first.IsVip {
		t.Errorf("第1集应为免费非 vip: %+v", first)
	}
	if first.UpdatedAt != 1764853574 {
		t.Errorf("updated_at = %d", first.UpdatedAt)
	}
	second := c.Chapters[0]
	if !second.IsPay || !second.IsVip {
		t.Errorf("needPay=2 且 isChapterLock 应映射为付费 vip: %+v", second)
	}
	if second.Volume != "第一卷" {
		t.Errorf("volume = %q", second.Volume)
	}
	third := c.Chapters[2]
	if third.Volume != "第二卷" {
		t.Errorf("第二卷卷名不符: %q", third.Volume)
	}
	if !third.IsVip || third.IsPay {
		t.Errorf("isPaidPublication 应映射 is_vip: %+v", third)
	}
	if !reflect.DeepEqual(c.Missing, []string{"8888888888888888888"}) {
		t.Errorf("missing = %v", c.Missing)
	}
}

func TestParseCatalogEmpty(t *testing.T) {
	// allItemIds 为空 = 目录空，200 + total 0，不报错。
	raw := `{"code":0,"data":{"allItemIds":[],"volumeNameList":[],"chapterListWithVolume":[]}}`
	c, err := ParseCatalog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 0 || c.Chapters == nil || len(c.Chapters) != 0 {
		t.Fatalf("应得到 total 0 和非 nil 空数组: %+v", c)
	}
}

func TestParseCatalogMissingVolumeName(t *testing.T) {
	// volumeNameList 比卷数短时，超出部分卷名为空串。
	raw := `{"code":0,"data":{"allItemIds":["1"],"volumeNameList":[],"chapterListWithVolume":[[{"itemId":"1","title":"t","needPay":0,"isChapterLock":false,"isPaidPublication":false,"isPaidStory":false,"firstPassTime":"10"}]]}}`
	c, err := ParseCatalog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Chapters[0].Volume != "" {
		t.Fatalf("volume = %q", c.Chapters[0].Volume)
	}
}

func TestParseCatalogBadFirstPassTime(t *testing.T) {
	raw := `{"code":0,"data":{"allItemIds":["1"],"volumeNameList":[],"chapterListWithVolume":[[{"itemId":"1","title":"t","needPay":0,"isChapterLock":false,"isPaidPublication":false,"isPaidStory":false,"firstPassTime":""}]]}}`
	c, err := ParseCatalog([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Chapters[0].UpdatedAt != 0 {
		t.Fatalf("firstPassTime 解析失败应给 0: %d", c.Chapters[0].UpdatedAt)
	}
}

func TestParseCatalogBizError(t *testing.T) {
	raw := `{"code":1001004,"message":"书籍不存在","data":{}}`
	_, err := ParseCatalog([]byte(raw))
	if err == nil {
		t.Fatal("code != 0 应报错")
	}
	biz, ok := err.(*BizError)
	if !ok || biz.Code != 1001004 {
		t.Fatalf("应返回 BizError: %v", err)
	}
}

func TestParseCatalogNilData(t *testing.T) {
	_, err := ParseCatalog([]byte(`{"code":0,"data":null}`))
	if err == nil {
		t.Fatal("data 为 null 应报错")
	}
}

func TestParseCatalogBadJSON(t *testing.T) {
	_, err := ParseCatalog([]byte("not json"))
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
