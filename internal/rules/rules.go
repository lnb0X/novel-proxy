// Package rules 实现内容拼接规则：全局一套 + 按书覆盖，
// override 接管、replace 级联、prepend/append 继承与抑制。
// 拼接只发生在响应时，结果不入库（DECISIONS 内容规则节）。
package rules

import (
	"regexp"
	"strconv"
	"strings"

	"novel-proxy/internal/config"
)

// Resolve 计算某本书的生效规则：
//   - prepend / append：按书为 null 继承全局，为空字符串抑制全局
//   - replace：全局规则按顺序执行完，再执行按书的规则
//   - override：按书非 null 用按书，否则用全局
func Resolve(cfg config.ContentRulesConfig, bookID string) config.ContentRuleSet {
	effective := config.ContentRuleSet{
		Prepend:  cfg.Default.Prepend,
		Append:   cfg.Default.Append,
		Replace:  append([]config.ReplaceRule{}, cfg.Default.Replace...),
		Override: cfg.Default.Override,
	}
	book, ok := cfg.Books[bookID]
	if !ok {
		return effective
	}
	if book.Prepend != nil {
		effective.Prepend = book.Prepend
	}
	if book.Append != nil {
		effective.Append = book.Append
	}
	effective.Replace = append(effective.Replace, book.Replace...)
	if book.Override != nil {
		effective.Override = book.Override
	}
	return effective
}

// Applied 预览接口里每条规则的命中记录。
type Applied struct {
	Rule string `json:"rule"`
	Hits int    `json:"hits"`
}

// TakeOver 返回接管内容与来源；rs.Override 为 nil 表示无接管（Q4：
// 非空即接管，不做任何匹配）。来源：该书配置了 override 记
// book.override，否则记 global.override。
func TakeOver(rs config.ContentRuleSet, bookHasOverride bool) (source, content string, ok bool) {
	if rs.Override == nil {
		return "", "", false
	}
	if bookHasOverride {
		return "book.override", *rs.Override, true
	}
	return "global.override", *rs.Override, true
}

// ApplyReplace 级联执行替换：前一条的结果参与后一条匹配。
// 非 regex 用子串全量替换，regex 用正则全量替换；返回结果与每条命中数。
// 非法正则跳过该条（PUT 时已校验，此处不中断整章）。
func ApplyReplace(content string, ruleList []config.ReplaceRule, prefix string) (string, []Applied) {
	applied := make([]Applied, 0, len(ruleList))
	for i, r := range ruleList {
		hits := 0
		if r.Regex {
			if re, err := regexp.Compile(r.Pattern); err == nil {
				ms := re.FindAllStringIndex(content, -1)
				hits = len(ms)
				if hits > 0 {
					content = re.ReplaceAllString(content, r.Replacement)
				}
			}
		} else if r.Pattern != "" {
			hits = strings.Count(content, r.Pattern)
			if hits > 0 {
				content = strings.ReplaceAll(content, r.Pattern, r.Replacement)
			}
		}
		applied = append(applied, Applied{Rule: prefix + "[" + strconv.Itoa(i) + "]", Hits: hits})
	}
	return content, applied
}

// Assemble 完整拼接：replace（prefix 区分来源段）→ prepend → append。
func Assemble(content string, rs config.ContentRuleSet, replacePrefix string) (string, []Applied) {
	content, applied := ApplyReplace(content, rs.Replace, replacePrefix)
	if rs.Prepend != nil {
		content = *rs.Prepend + content
	}
	if rs.Append != nil {
		content += *rs.Append
	}
	return content, applied
}
