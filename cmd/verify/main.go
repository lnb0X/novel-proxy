package main

import (
	"flag"
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

// extraEnv 只作用于本进程 spawn 的子进程，不改外部 shell 的环境。
// 目前只有 -race 会用到（需要 CGO_ENABLED=1）。
var extraEnv []string

func command(dir, name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	if dir != "" {
		c.Dir = dir
	}
	if len(extraEnv) > 0 {
		c.Env = append(os.Environ(), extraEnv...)
	}
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c
}

func run(name string, args ...string) error {
	return command("", name, args...).Run()
}

func runIn(dir, name string, args ...string) error {
	return command(dir, name, args...).Run()
}

func fail(format string, a ...any) {
	fmt.Printf("FAIL: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	race := flag.Bool("race", false, "开启数据竞争检测（需要 gcc，构建与运行都会变慢）")
	flag.Parse()

	testArgs := []string{"test"}
	if *race {
		extraEnv = []string{"CGO_ENABLED=1"}
		testArgs = append(testArgs, "-race")
		fmt.Println("==> 已开启 -race（CGO_ENABLED=1，仅作用于测试子进程）")
	}
	testArgs = append(testArgs, "./...")

	fmt.Println("==> go vet")
	if err := run("go", "vet", "./..."); err != nil {
		fail("go vet 未通过")
	}

	fmt.Println("==> go test")
	if err := run("go", testArgs...); err != nil {
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
	if direct > 15 {
		fail("直接依赖 %d 个，超过预算 15", direct)
	}

	if _, err := os.Stat("web/package.json"); err == nil {
		if _, err := os.Stat("web/node_modules"); err != nil {
			fail("web/node_modules 不存在，先在 web/ 下执行 npm install")
		}
		fmt.Println("==> 前端类型检查")
		if err := runIn("web", "npx", "tsc", "--noEmit"); err != nil {
			fail("前端类型检查未通过")
		}
		fmt.Println("==> 前端构建")
		if err := runIn("web", "npm", "run", "build"); err != nil {
			fail("前端构建未通过")
		}
	}

	fmt.Println("==> emoji 检查")
	var bad []string
	_ = filepath.Walk(".", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testdata", "bin", "logs", "data":
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