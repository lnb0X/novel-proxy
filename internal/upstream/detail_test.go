package upstream

import (
	"errors"
	"testing"
)

// 样本取自 2026-11-07 对上游的实测结构（字段有节选）。
const detailSampleJSON = `{
  "code": 0,
  "message": "success",
  "log_id": "20261107012740A884A687DEC5B9AB77F4",
  "data": {
    "book_id": "7579985241087937560",
    "book_name": "天龙殿",
    "anchor": "山海星辰",
    "word_number": "0",
    "last_chapter_item_id": "7579988207375240217",
    "abstract": "  世界至高神殿天龙殿殿主林阳  ",
    "thumb_url": "https://p6-novelfm-sign.novelfmpic.com/novel-pic/abc.jpeg?x-expires=1794590829",
    "album_type": "0",
    "audio_duration": "8325.834"
  }
}`

func TestParseDetailNormal(t *testing.T) {
	d, err := ParseDetail([]byte(detailSampleJSON))
	if err != nil {
		t.Fatal(err)
	}
	if d.BookID != "7579985241087937560" {
		t.Errorf("book_id = %q", d.BookID)
	}
	if d.Title != "天龙殿" {
		t.Errorf("title 应取 book_name: %q", d.Title)
	}
	if d.Author != "山海星辰" {
		t.Errorf("author 应取 anchor: %q", d.Author)
	}
	if d.WordNumber != 0 {
		t.Errorf("word_number = %d", d.WordNumber)
	}
	if d.LastChapter != "7579988207375240217" {
		t.Errorf("last_chapter 应取 last_chapter_item_id 且保持字符串: %q", d.LastChapter)
	}
	if d.Intro != "世界至高神殿天龙殿殿主林阳" {
		t.Errorf("intro 应 TrimSpace: %q", d.Intro)
	}
	if d.ThumbURL == "" {
		t.Error("thumb_url 缺失")
	}
}

func TestParseDetailWordNumber(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{`"12345"`, 12345},
		{`"0"`, 0},
		{`"abc"`, 0},
		{`""`, 0},
	}
	for _, c := range cases {
		raw := `{"code":0,"data":{"book_id":"1","word_number":` + c.raw + `}}`
		d, err := ParseDetail([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if d.WordNumber != c.want {
			t.Errorf("word_number %s: got %d want %d", c.raw, d.WordNumber, c.want)
		}
	}
}

func TestParseDetailIntroWhitespace(t *testing.T) {
	raw := `{"code":0,"data":{"book_id":"1","abstract":" "}}`
	d, err := ParseDetail([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if d.Intro != "" {
		t.Fatalf("纯空白 intro 应为空串: %q", d.Intro)
	}
}

func TestParseDetailBookMissing(t *testing.T) {
	// 实测：书不存在时上游 HTTP 200，code=1001004。
	raw := `{"code":1001004,"data":{},"message":"书籍不存在或者已经下架"}`
	_, err := ParseDetail([]byte(raw))
	var biz *BizError
	if !errors.As(err, &biz) {
		t.Fatalf("应返回 *BizError: %v", err)
	}
	if biz.Code != 1001004 {
		t.Fatalf("BizError.Code = %d", biz.Code)
	}
}

func TestParseDetailNilData(t *testing.T) {
	_, err := ParseDetail([]byte(`{"code":0,"data":null}`))
	if err == nil {
		t.Fatal("data 为 null 应报错")
	}
}

func TestParseDetailBadJSON(t *testing.T) {
	_, err := ParseDetail([]byte("not json"))
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}
