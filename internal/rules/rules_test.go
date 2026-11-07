package rules

import (
	"testing"

	"novel-proxy/internal/config"
)

func strPtr(s string) *string { return &s }

func TestResolveInheritance(t *testing.T) {
	cfg := config.ContentRulesConfig{
		Default: config.ContentRuleSet{
			Prepend: strPtr("全局头"),
			Append:  strPtr("全局尾"),
			Replace: []config.ReplaceRule{{Pattern: "a", Replacement: "b"}},
		},
		Books: map[string]config.ContentRuleSet{
			"100": {
				Prepend:  nil, // 继承全局
				Append:   strPtr(""),
				Override: strPtr("接管"),
			},
			"200": {
				Prepend: strPtr("书头"),
			},
		},
	}

	rs := Resolve(cfg, "300") // 无按书配置
	if rs.Prepend == nil || *rs.Prepend != "全局头" {
		t.Fatalf("无书配置应继承全局 prepend: %v", rs.Prepend)
	}
	if len(rs.Replace) != 1 {
		t.Fatalf("replace 应为全局的: %+v", rs.Replace)
	}

	// 100：prepend 继承、append 抑制、override 取书级。
	rs = Resolve(cfg, "100")
	if rs.Prepend == nil || *rs.Prepend != "全局头" {
		t.Fatalf("null 应继承: %v", rs.Prepend)
	}
	if rs.Append == nil || *rs.Append != "" {
		t.Fatalf("空串应抑制全局: %v", rs.Append)
	}
	if rs.Override == nil || *rs.Override != "接管" {
		t.Fatalf("override 应取书级: %v", rs.Override)
	}

	// 200：prepend 覆盖。
	rs = Resolve(cfg, "200")
	if rs.Prepend == nil || *rs.Prepend != "书头" {
		t.Fatalf("按书 prepend 应覆盖: %v", rs.Prepend)
	}
	if rs.Append == nil || *rs.Append != "全局尾" {
		t.Fatalf("未配置的 append 应继承: %v", rs.Append)
	}
}

func TestTakeOver(t *testing.T) {
	// 无 override。
	rs := Resolve(config.ContentRulesConfig{}, "1")
	if _, _, ok := TakeOver(rs, false); ok {
		t.Fatal("无 override 不应接管")
	}
	// 全局接管。
	g := config.ContentRulesConfig{Default: config.ContentRuleSet{Override: strPtr("全站接管")}}
	source, content, ok := TakeOver(Resolve(g, "1"), false)
	if !ok || source != "global.override" || content != "全站接管" {
		t.Fatalf("全局接管不符: %s %s %v", source, content, ok)
	}
	// 书级优先于全局。
	b := config.ContentRulesConfig{
		Default: config.ContentRuleSet{Override: strPtr("全站接管")},
		Books:   map[string]config.ContentRuleSet{"1": {Override: strPtr("本书接管")}},
	}
	source, content, ok = TakeOver(Resolve(b, "1"), true)
	if !ok || source != "book.override" || content != "本书接管" {
		t.Fatalf("书级接管不符: %s %s %v", source, content, ok)
	}
	// 其他书仍走全局。
	source, content, ok = TakeOver(Resolve(b, "2"), false)
	if !ok || source != "global.override" || content != "全站接管" {
		t.Fatalf("其他书应走全局: %s %s %v", source, content, ok)
	}
}

func TestApplyReplaceCascade(t *testing.T) {
	// 级联：前一条结果参与后一条匹配。
	rules := []config.ReplaceRule{
		{Pattern: "蛇", Replacement: "龙"},
		{Pattern: "龙群", Replacement: "群龙"},
	}
	out, applied := ApplyReplace("蛇群蠕动。蛇群飞舞。", rules, "g")
	if out != "群龙蠕动。群龙飞舞。" {
		t.Fatalf("级联结果 = %q", out)
	}
	if applied[0].Hits != 2 || applied[0].Rule != "g[0]" {
		t.Fatalf("第一条命中: %+v", applied[0])
	}
	if applied[1].Hits != 2 || applied[1].Rule != "g[1]" {
		t.Fatalf("第二条命中: %+v", applied[1])
	}

	// 顺序不同结果不同（验收项）。
	r1 := []config.ReplaceRule{
		{Pattern: "ab", Replacement: "x"},
		{Pattern: "x", Replacement: "y"},
	}
	r2 := []config.ReplaceRule{
		{Pattern: "x", Replacement: "y"},
		{Pattern: "ab", Replacement: "x"},
	}
	out1, _ := ApplyReplace("ab", r1, "g")
	out2, _ := ApplyReplace("ab", r2, "g")
	if out1 != "y" || out2 != "x" {
		t.Fatalf("顺序语义破坏: %q vs %q", out1, out2)
	}

	// 正则替换与命中数。
	r3 := []config.ReplaceRule{{Pattern: `\d+`, Replacement: "N", Regex: true}}
	out, applied = ApplyReplace("第12章 第345页", r3, "g")
	if out != "第N章 第N页" || applied[0].Hits != 2 {
		t.Fatalf("正则替换: %q %+v", out, applied[0])
	}
	// 非法正则跳过。
	r4 := []config.ReplaceRule{{Pattern: "([", Replacement: "x", Regex: true}}
	out, applied = ApplyReplace("abc", r4, "g")
	if out != "abc" || applied[0].Hits != 0 {
		t.Fatalf("非法正则应跳过: %q %+v", out, applied)
	}
}

func TestAssemble(t *testing.T) {
	rs := config.ContentRuleSet{
		Prepend: strPtr("<p>头</p>"),
		Append:  strPtr("<p>尾</p>"),
		Replace: []config.ReplaceRule{{Pattern: "蛇", Replacement: "龙"}},
	}
	out, applied := Assemble("<p>蛇。</p>", rs, "g")
	if out != "<p>头</p><p>龙。</p><p>尾</p>" {
		t.Fatalf("assemble = %q", out)
	}
	if len(applied) != 1 || applied[0].Hits != 1 {
		t.Fatalf("applied = %+v", applied)
	}
}
