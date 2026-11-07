# DECISIONS
已冻结。不得推翻，有异议写 QUESTIONS.md。

## 定位
薄壳。只做请求转发和字段映射。上游改版、反爬升级、多源适配由二开者处理。

## 环境
Windows。验收命令统一为 go run ./cmd/verify，不依赖 make、bash、grep。

## 模块
- module 名：novel-proxy
- 目录骨架（不要多建任何一层）：
    cmd/server/main.go
    cmd/verify/main.go              — 验收程序，项目所有者维护，agent 不得修改
    internal/config/config.go
    internal/logx/logx.go
    internal/api/response.go        — 统一响应封装
    internal/handler/*.go           — 四个 handler
    internal/upstream/*.go          — 四个上游适配（Fetch + Parse）

## 构建
- CGO_ENABLED=0，带 -trimpath -ldflags="-s -w"
- 输出 bin/app.exe
- 不提供 Makefile

## 后端
- Go 1.22+，Web 框架 gin
- HTTP 客户端 go-resty/resty/v2，全局复用一个 client
- 日志 log/slog，JSON handler，同时写 stdout 和文件
- 日志滚动用 lumberjack，四个参数走配置
- gin 的 Logger 和 Recovery 中间件全部禁用（不用 gin.Default()），
  换成自写的 slog 中间件和 recovery 中间件
- app.mode 只影响日志详细程度和 gin 的启动提示，不影响 HTTP 响应体。
  两种模式下响应体完全一致，永远不带堆栈。
  debug 模式下 error 级日志多一个 stack 字段。
- app.mode 通过 gin.SetMode 设置：debug 用 gin.DebugMode，release 用 gin.ReleaseMode
- 配置：YAML 文件 + flag + 环境变量，不引入 viper
- 未匹配路由（NoRoute）统一返回 JSON 404
- v0.1 不建数据库、不做缓存、不做持久化、不服务静态文件
- HTML 解析用 golang.org/x/net/html 的 Tokenizer（流式过滤，不建 DOM）

## 配置 schema
文件 config.yaml，所有键可省略，省略时用下列默认值。
未知键必须报错退出（yaml.v3 的 KnownFields），防止拼写错误被静默忽略。

| 键 | 类型 | 默认 | 取值 |
|---|---|---|---|
| app.mode | string | release | debug / release |
| server.addr | string | :8080 | |
| server.shutdown_timeout_seconds | int | 10 | |
| log.level | string | info | debug / info / error |
| log.file | string | ./logs/app.log | |
| log.max_size_mb | int | 50 | |
| log.max_backups | int | 7 | |
| log.max_age_days | int | 14 | |
| log.compress | bool | true | |
| upstream.timeout_seconds | int | 10 | |
| upstream.max_concurrency | int | 8 | |

enum 类字段取到非法值时必须报错退出，不许兜底成默认值。

## 配置优先级
- 文件路径：-config > APP_CONFIG > ./config.yaml
- 键值覆盖：只有 app.mode、server.addr、log.level 支持环境变量
  （APP_MODE / APP_ADDR / APP_LOG_LEVEL），其余只读 yaml

## 已批准的第三方模块
- github.com/gin-gonic/gin
- github.com/go-resty/resty/v2
- gopkg.in/yaml.v3
- gopkg.in/natefinch/lumberjack.v2
- golang.org/x/net/html
新增按 AGENTS.md 分层规则处理。

## 上游（三域名、三套参数风格，不要统一）
| 接口 | 域名 | 方式 | 参数 |
|---|---|---|---|
| search | novel.snssdk.com | GET | q / offset |
| detail | api5-sinfonlinec.novelfm.com | POST JSON | book_id |
| catalog | fanqienovel.com | GET | bookId（驼峰） |
| content | www.toutiao.com/article/{id}/ | GET | wid（毫秒时间戳）+ __ac_nonce cookie |

- book_id 在 search 和 detail 之间互通，不需要映射表
- 封面图和正文图片都是带 x-expires 的签名 URL，约 7 天过期

## 部署与前端（v0.2 才实现，v0.1 不碰）
v0.1 不服务任何静态文件，也不要为下面这些预先写代码。
- Go 单二进制 + 外置静态目录，不用 go:embed
- 静态目录支持符号链接，切换链接即换前端，不重启
- 解析顺序：-static 参数 > APP_STATIC_DIR 环境变量 > config.yaml
- 静态服务必须实现 SPA 兜底：带扩展名或 /assets/ 前缀的路径找不到时返回 404
- 单个 Vite + React + TypeScript 应用，strict 开启
- 路由 React Router，BrowserRouter
- 服务端状态 TanStack Query，本地状态 useState + Context
- 阅读端纯 CSS + CSS 变量；AntD 仅用于管理端
- 请求用原生 fetch
- 路由表：
    /                              搜索页
    /books/:bookId                 详情页
    /books/:bookId/toc             目录页
    /books/:bookId/read/:chapterId 正文页
- 阅读端视觉红线：正文直接落在页面背景上，禁止卡片/容器/边框/投影包裹；
  禁止背景渐变、纹理、装饰图形、翻页动画；禁止顶部悬浮导航遮挡正文；
  行宽不超过 38 字；中文正文 18px / 行高 1.85

## 明确不用
Next.js / Nuxt / Remix / Astro / Tailwind / shadcn / MUI / Chakra /
Zustand / Redux / Jotai / Axios / styled-components / emotion / monorepo /
viper / zap / logrus / echo / fiber / chi / goquery

## 契约变更
v0.1.1：content 新增必填参数 book_id；detail / catalog 的 book_id
校验升级为格式白名单（^\d{1,20}$）。v0.1.0 的旧调用方会收到 40001，
属于破坏性变更，无兼容期。