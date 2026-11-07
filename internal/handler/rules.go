package handler

import (
	"context"
	"regexp"
	"strconv"

	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"gopkg.in/yaml.v3"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/rules"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/store"
	"novel-proxy/internal/upstream"
)

const (
	booksMax   = 100
	replaceMax = 50
)

// GetContentRules GET /<secret>/api/content-rules
func GetContentRules(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, cfg.ContentRules)
	}
}

// PutContentRules PUT /<secret>/api/content-rules body 整体替换，热更并写盘。
// 校验：按书最多 100 本、每个规则集 replace 最多 50 条、正则可编译。
func PutContentRules(cfg *config.Config, instancePath string, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body config.ContentRulesConfig
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if len(body.Books) > booksMax {
			api.Fail(c, api.CodeBadRequest, "按书规则最多 100 本")
			return
		}
		if err := validateRuleSet(body.Default, "default"); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		for id, rs := range body.Books {
			if err := validateRuleSet(rs, "book "+id); err != nil {
				api.Fail(c, api.CodeBadRequest, err.Error())
				return
			}
		}

		cfg.ContentRules = body
		err := config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			// 经 JSON 往返转成通用 map，让递归 node 转换器写树。
			raw, jerr := json.Marshal(body)
			if jerr != nil {
				return jerr
			}
			var generic map[string]any
			if err := json.Unmarshal(raw, &generic); err != nil {
				return jerr
			}
			return config.SetMappingKey(doc, []string{"content_rules"}, generic)
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		au.Audit("rules_change", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"books="+strconv.Itoa(len(body.Books)), "ok")
		api.OK(c, cfg.ContentRules)
	}
}

func validateRuleSet(rs config.ContentRuleSet, scope string) error {
	if len(rs.Replace) > replaceMax {
		return &simpleError{scope + ": replace 最多 50 条"}
	}
	for _, r := range rs.Replace {
		if r.Regex {
			if _, err := regexp.Compile(r.Pattern); err != nil {
				return &simpleError{scope + ": 非法正则 " + r.Pattern + ": " + err.Error()}
			}
		}
	}
	return nil
}

// contentFetcher 预览用的取文适配：复用上游 Fetch/Parse。
type contentFetcher struct {
	client *resty.Client
}

func (f contentFetcher) Fetch(ctx context.Context, chapterID string) ([]byte, error) {
	return upstream.FetchContent(ctx, f.client, nil, chapterID)
}

func (f contentFetcher) Parse(raw []byte) (string, error) {
	parsed, err := upstream.ParseContent(raw)
	if err != nil {
		return "", err
	}
	return parsed.Content, nil
}

// ContentRulesPreview POST /<secret>/api/content-rules/preview
// body { book_id, chapter_id } → { original, result, applied }。
// 接管命中时 original 为 null（Q4）。
func ContentRulesPreview(cfg *config.Config, client *resty.Client, st store.ArticleStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			BookID    string `json:"book_id"`
			ChapterID string `json:"chapter_id"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		bookID, err := parseID("book_id", body.BookID)
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		chapterID, err := parseID("chapter_id", body.ChapterID)
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}

		rs := rules.Resolve(cfg.ContentRules, bookID)
		bookCfg, hasBook := cfg.ContentRules.Books[bookID]
		bookOverridden := hasBook && bookCfg.Override != nil
		if source, content, ok := rules.TakeOver(rs, bookOverridden); ok {
			api.OK(c, gin.H{
				"original": nil,
				"result":   content,
				"applied":  []rules.Applied{{Rule: source, Hits: 1}},
			})
			return
		}

		// 原文：先查缓存，未命中抓上游（预览不入库）。
		var original string
		if st != nil {
			if art, gerr := st.Get(c.Request.Context(), bookID, chapterID); gerr == nil {
				original = art.Content
			}
		}
		if original == "" {
			raw, ferr := contentFetcher{client}.Fetch(c.Request.Context(), chapterID)
			if ferr != nil {
				failUpstream(c, ferr)
				return
			}
			original, ferr = contentFetcher{client}.Parse(raw)
			if ferr != nil {
				failUpstreamBody(c, ferr)
				return
			}
		}
		result, applied := rules.Assemble(original, rs, "g")
		api.OK(c, gin.H{"original": original, "result": result, "applied": applied})
	}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
