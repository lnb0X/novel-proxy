package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// UpdateInstance 修改 data/instance.yaml 并原子写回：
// 以 yaml.Node 打开文档交给 set 增改键，先写 .tmp 再 rename。
// 文件不存在时从空文档创建。只动指定键，其余内容原样保留。
func UpdateInstance(path string, set func(doc *yaml.Node) error) error {
	var doc yaml.Node
	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("解析 instance.yaml: %w", err)
		}
	}
	// Unmarshal 进 Node 得到 DocumentNode，剥掉外壳取真正的值节点。
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = *doc.Content[0]
	}
	// 空文档或纯标量都要归一成映射。
	if doc.Kind == 0 || doc.Kind == yaml.ScalarNode {
		doc = yaml.Node{Kind: yaml.MappingNode}
	}
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("instance.yaml 顶层不是映射")
	}
	if err := set(&doc); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("编码 instance.yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return err
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建数据目录: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("写 instance.yaml: %w", err)
	}
	return os.Rename(tmp, path)
}

// SetMappingKey 按 keyPath 增改嵌套键，中间层不存在时创建。
// value 为 string 时写标量，为 []string 时写序列。
func SetMappingKey(doc *yaml.Node, keyPath []string, value any) error {
	node := doc
	for _, key := range keyPath[:len(keyPath)-1] {
		child := mappingChild(node, key)
		if child == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			valNode := &yaml.Node{Kind: yaml.MappingNode}
			node.Content = append(node.Content, keyNode, valNode)
			child = valNode
		}
		node = child
	}
	last := keyPath[len(keyPath)-1]
	valNode, err := valueNode(value)
	if err != nil {
		return err
	}
	if existing := mappingValue(node, last); existing != nil {
		*existing = *valNode
		return nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: last}
	node.Content = append(node.Content, keyNode, valNode)
	return nil
}

func valueNode(value any) (*yaml.Node, error) {
	switch v := value.(type) {
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}, nil
	case bool:
		s := "false"
		if v {
			s = "true"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: s}, nil
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}, nil
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, s := range v {
			seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
		}
		return seq, nil
	default:
		return nil, fmt.Errorf("不支持的键值类型 %T", value)
	}
}

// mappingChild 返回映射里 key 对应的值节点，不存在返回 nil。
func mappingChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	return mappingChild(m, key)
}
