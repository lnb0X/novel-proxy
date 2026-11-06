package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var emojiRe = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}\x{2B00}-\x{2BFF}]`)

var banned = []string{
	"gorm.io/",
	"go.uber.org/fx",
	"github.com/google/wire",
	"github.com/go-kit/",
}

func run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

func fail(format string, a ...any) {
	fmt.Printf("FAIL: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	fmt.Println("==> go vet")
	if err := run("go", "vet", "./..."); err != nil {
		fail("go vet 未通过")
	}

	fmt.Println("==> go test")
	if err := run("go", "test", "./..."); err != nil {
		fail("go test 未通过")
	}

	fmt.Println("==> 依赖检查")
	out, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		fail("go list -m all 失败: %v", err)
	}
	for _, m := range strings.Fields(string(out)) {
		for _, b := range banned {
			if strings.HasPrefix(m, b) {
				fail("引入了被禁依赖: %s", m)
			}
		}
	}

	dout, err := exec.Command("go", "list", "-m", "-f",
		"{{if and (not .Main) (not .Indirect)}}{{.Path}}{{end}}", "all").Output()
	if err != nil {
		fail("统计直接依赖失败: %v", err)
	}
	direct := 0
	for _, line := range strings.Split(string(dout), "\n") {
		if strings.TrimSpace(line) != "" {
			direct++
		}
	}
	fmt.Printf("    直接依赖 %d 个\n", direct)
	if direct > 10 {
		fail("直接依赖 %d 个，超过预算 10", direct)
	}

	fmt.Println("==> emoji 检查")
	var bad []string
	_ = filepath.Walk(".", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testdata", "bin", "logs":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".go", ".md", ".yaml", ".yml", ".sh", ".ps1":
		default:
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if emojiRe.Match(b) {
			bad = append(bad, p)
		}
		return nil
	})
	if len(bad) > 0 {
		fail("以下文件包含 emoji:\n  %s", strings.Join(bad, "\n  "))
	}

	fmt.Println("==> OK")
}