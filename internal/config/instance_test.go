package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUpdateInstanceCreates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data", "instance.yaml")
	err := UpdateInstance(p, func(doc *yaml.Node) error {
		return SetMappingKey(doc, []string{"admin", "username"}, "boss")
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); len(b) == 0 {
		t.Fatal("文件未写入")
	}
	// 走 Load 验证能被读到。
	dir := filepath.Dir(filepath.Dir(p))
	cp := writeFile(t, dir, "config.yaml", "")
	if _, err := Load(cp, p); err != nil {
		t.Fatalf("写出的 instance.yaml 应可被 Load 解析: %v", err)
	}
}

func TestUpdateInstanceMergesAndKeepsComments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "instance.yaml")
	os.WriteFile(p, []byte("# 注释保留\nadmin:\n  username: old\nstorage:\n  enabled: true\n"), 0o644)

	err := UpdateInstance(p, func(doc *yaml.Node) error {
		return SetMappingKey(doc, []string{"admin", "password_hash"}, "$2a$12$abc")
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	for _, want := range []string{"username: old", "$2a$12$abc", "enabled: true", "注释保留"} {
		if !strings.Contains(s, want) {
			t.Fatalf("实例文件缺 %q:\n%s", want, s)
		}
	}
	// 再改一次覆盖旧值。
	err = UpdateInstance(p, func(doc *yaml.Node) error {
		return SetMappingKey(doc, []string{"admin", "username"}, "new")
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if strings.Contains(string(b), "username: old") {
		t.Fatalf("旧值应被覆盖:\n%s", b)
	}
}

func TestUpdateInstanceNoTmpLeft(t *testing.T) {
	p := filepath.Join(t.TempDir(), "instance.yaml")
	if err := UpdateInstance(p, func(doc *yaml.Node) error {
		return SetMappingKey(doc, []string{"setup", "completed"}, "true")
	}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}
