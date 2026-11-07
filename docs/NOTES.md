# NOTES

实现细节备忘。改动相关行为前先读这里。

## content 标题提取

content 页里存在第二个 `<title id="author-related-list-loading-aria">Loading`，
ParseContent 只取文档中第一个 title 元素。已有回归测试覆盖
（internal/upstream/content_test.go 的 TestParseContentFirstTitleWins），
以后不要改成遍历所有 title，否则标题会拼上 Loading 尾巴。
