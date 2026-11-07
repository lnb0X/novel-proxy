package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func wrapBody(article string) string {
	return `<html lang="zh"><head><title>第1章 重生 - 今日头条</title>` +
		`<script>window.a=1;</script></head><body><div>页面杂项</div>` +
		article + `<img src="https://sf6-cdn-tos.toutiaostatic.com/avatar.image" alt="头像"/>` +
		`</body></html>`
}

func TestParseContentPlainText(t *testing.T) {
	// 样本 1：纯文本，只有 p 标签，无标签残留。
	raw := wrapBody(`<article class="syl-article-base syl-page-article tt-article-content syl-device-pc">` +
		`<p>第一段正文。</p><p>第二段正文。</p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "第1章 重生" {
		t.Fatalf("title 应去掉后缀: %q", c.Title)
	}
	if c.Content != "<p>第一段正文。</p><p>第二段正文。</p>" {
		t.Fatalf("content = %q", c.Content)
	}
	if strings.Contains(c.Content, "页面杂项") {
		t.Fatal("article 之外的文本不应进入正文")
	}
}

func TestParseContentImg(t *testing.T) {
	// 样本 2：img 的 src 是 data: 占位符，取 data-src。
	raw := wrapBody(`<article class="syl-article-base"><p>前文。</p>` +
		`<img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" img_width="640" img_height="427" ` +
		`class="syl-page-img" data-src="https://p3-sign.toutiaoimg.com/novel-static/abc~tplv.gif.jpeg?_iz=58558&amp;x-expires=1">` +
		`<p>后文。</p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := `<p>前文。</p><img src="https://p3-sign.toutiaoimg.com/novel-static/abc~tplv.gif.jpeg?_iz=58558&amp;x-expires=1"><p>后文。</p>`
	if c.Content != want {
		t.Fatalf("content = %q\nwant   %q", c.Content, want)
	}
	if strings.Contains(c.Content, "data:") {
		t.Fatal("data: 占位图不应输出")
	}
}

func TestParseContentImgSrcFallback(t *testing.T) {
	// 没有 data-src 时取 src；纯相对路径 src 丢弃。
	raw := wrapBody(`<article class="syl-article-base">` +
		`<img src="https://p3-sign.toutiaoimg.com/x.image">` +
		`<img src="novel-static/relative-only">` +
		`</article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Content != `<img src="https://p3-sign.toutiaoimg.com/x.image">` {
		t.Fatalf("content = %q", c.Content)
	}
}

func TestParseContentStripTagsKeepText(t *testing.T) {
	// 样本 3：tt-audio、span start_time、div.novel-fm-asr 剥标签保留文字。
	raw := wrapBody(`<article class="syl-article-base">` +
		`<tt-audio data-id="v0d56i" data-token="xxx" time="684.24" title="259八翼黑蛇皇" content></tt-audio>` +
		`<div class="novel-fm-asr"><span start_time="150">蛇群蠕动。</span>` +
		`<span start_time="5070">唐三眼中精光一闪。</span></div>` +
		`<strong>加粗文字</strong></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := "蛇群蠕动。唐三眼中精光一闪。加粗文字"
	if c.Content != want {
		t.Fatalf("content = %q, want %q", c.Content, want)
	}
	for _, banned := range []string{"<tt-audio", "<span", "<div", "<strong", "start_time", "data-id"} {
		if strings.Contains(c.Content, banned) {
			t.Fatalf("残留 %q: %s", banned, c.Content)
		}
	}
}

func TestParseContentBr(t *testing.T) {
	raw := wrapBody(`<article class="syl-article-base"><p>行1<br>行2</p><p>行3<br/></p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Content != "<p>行1<br>行2</p><p>行3<br></p>" {
		t.Fatalf("content = %q", c.Content)
	}
}

func TestParseContentTextEntitiesPassThrough(t *testing.T) {
	// 文本 token 原样透传，不做二次转义。
	raw := wrapBody(`<article class="syl-article-base"><p>A&amp;B</p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Content != "<p>A&amp;B</p>" {
		t.Fatalf("content = %q", c.Content)
	}
}

func TestParseContentTitleNoSuffix(t *testing.T) {
	raw := strings.Replace(wrapBody(`<article class="syl-article-base"><p>x</p></article>`),
		" - 今日头条", "", 1)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "第1章 重生" {
		t.Fatalf("title = %q", c.Title)
	}
}

func TestParseContentFirstTitleWins(t *testing.T) {
	// 实测上游页面第二个 <title> 是 aria 占位（Loading...），不能拼进标题。
	raw := `<html><head><title>第1章 重生 - 今日头条</title>` +
		`<title id="author-related-list-loading-aria">Loading...</title></head>` +
		`<body><article class="syl-article-base"><p>x</p></article></body></html>`
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "第1章 重生" {
		t.Fatalf("title 应只取第一个 title 元素: %q", c.Title)
	}
}

func TestParseContentLeavesBookIDEmpty(t *testing.T) {
	// ParseContent 保持纯函数不收 book_id；出参结构必须带 book_id 字段，
	// 由 handler 校验透传后回填。
	raw := wrapBody(`<article class="syl-article-base"><p>x</p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.BookID != "" {
		t.Fatalf("Parse 不应填 book_id: %q", c.BookID)
	}
	b, err := json.Marshal(Content{BookID: "7526452300462705689", ChapterID: "1", Title: "t", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"book_id":"7526452300462705689"`) {
		t.Fatalf("出参 JSON 缺 book_id 字段: %s", b)
	}
}

func TestParseContentNoArticle(t *testing.T) {
	// 页面里找不到 article 必须报错，不得返回空 content。
	raw := wrapBody(`<div class="captcha">请输入验证码</div>`)
	_, err := ParseContent([]byte(raw))
	if err == nil {
		t.Fatal("找不到 article 应报错")
	}
}

func TestParseContentEmptyArticle(t *testing.T) {
	raw := wrapBody(`<article class="syl-article-base"></article>`)
	_, err := ParseContent([]byte(raw))
	if err == nil {
		t.Fatal("article 内过滤后为空应报错")
	}
}

func TestParseContentWrongClassNotCaptured(t *testing.T) {
	// 其他 class 的 article 不是正文容器。
	raw := wrapBody(`<article class="syl-article-other"><p>非正文</p></article>` +
		`<article class="syl-article-base"><p>真正文</p></article>`)
	c, err := ParseContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if c.Content != "<p>真正文</p>" {
		t.Fatalf("content = %q", c.Content)
	}
}
