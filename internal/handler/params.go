package handler

import (
	"fmt"
	"regexp"
)

// idRe ID 参数白名单：1 到 20 位纯数字。
// book_id / chapter_id 全程 string，只校验格式，不转数字。
var idRe = regexp.MustCompile(`^\d{1,20}$`)

// parseID 校验 ID 参数：空串或格式不匹配都返回错误，调用方统一映射成 40001。
func parseID(name, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("参数 %s 必填", name)
	}
	if !idRe.MatchString(value) {
		return "", fmt.Errorf("参数 %s 必须是 1 到 20 位数字", name)
	}
	return value, nil
}
