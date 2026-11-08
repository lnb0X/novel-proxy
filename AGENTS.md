# AGENTS.md

## 项目
自用小说中转站。Go 后端（gin + resty）代理上游接口，
带初始化向导、JWT 认证、多数据库文章缓存、前端版本管理、管理端界面。
v0.3 范围：阅读端前端。后端已完成，本期不改任何 /api/* 契约。

## 环境
Windows。所有命令必须是纯 go 调用，不要用 make、sed、awk。
构建产物是 bin/app.exe。

生产构建一律 CGO_ENABLED=0。只有 `go run ./cmd/verify -race` 会在
测试子进程里设 CGO_ENABLED=1——这是竞态检测的要求，不算违反红线。

## 开始任何任务前
按顺序读 docs/DECISIONS.md、docs/CONTRACTS.md、docs/BACKLOG.md。
这三个文件里没有的信息，不要猜。

## 关于已冻结的决策
DECISIONS.md 里的选择已经讨论过，不要重新评估、不要提替代方案。
只有发现事实性错误时才写 QUESTIONS.md，格式：问题 / 证据 / 影响哪个任务。
写完继续做下一个任务，不要停下来等回复。

## 工作循环
1. 从 BACKLOG 取最上面一个未勾选的任务，只做这一个
2. 实现
3. go run ./cmd/verify —— 红了就继续修，不许改检查让它变绿
4. go run ./cmd/verify -race —— 收尾前必须跑一次
5. 全绿后 commit
6. 在 BACKLOG 勾掉该任务
7. 回到第 1 步

## 完成定义
完成 = go run ./cmd/verify 全绿
     + go run ./cmd/verify -race 全绿
     + BACKLOG 已勾选
     + 有对应测试
未通过任何一项时禁止声称完成，也禁止开始下一个任务。

## 输出纪律
每个任务结束只写三行：做了什么（不超过 3 句）、verify 与 -race 结果、
commit hash。不要复述代码、不要贴 diff、不要解释设计取舍。

## 编辑纪律
- 修改源码一律用精确字符串替换，禁止用 sed、awk、heredoc 批量改写
- 禁止 git add -A，只 add 明确列出的路径
- 禁止把 verify 的输出管道给 head / tail / grep 而不检查退出码
- 这三个禁令来自实际事故：管道吞退出码导致两次红 verify 被提交、
  脚本转义失误导致测试文件被破坏、add -A 把 node_modules 提交进仓库

## 契约规则
docs/CONTRACTS.md 是唯一真相。
允许：新增接口、新增可选参数、新增返回字段
禁止：删除或重命名已有字段/接口；修改已有字段的类型或语义；
      修改已有接口的必填参数
需要上述禁止项才能完成的任务：写进 QUESTIONS.md，跳过，做下一个

## 上游适配
internal/upstream/ 下每个上游接口一个文件，拆成两个函数：
  FetchXxx(ctx, client, lg, ...) ([]byte, error)  — 只发请求，从 lg 记
                                                    upstream.request / response
  ParseXxx(raw []byte) (Result, error)            — 纯函数，只做解析和映射
不掺业务逻辑、不做缓存、不写错误分类。
所有上游 HTTP 调用必须经过全局并发信号量，上限来自 upstream.max_concurrency。

## 存储层
internal/store/ 下：
  store.go        — ArticleStore 接口
  sql.go          — postgres / mysql / sqlite 共用实现
  mongo.go        — MongoDB 实现
  dialect.go      — 三种 SQL 方言的 DDL 差异
数据库只在 storage.enabled 时连接。未启用缓存时程序必须能正常运行。
storage.enabled 为 false 时，统计与审计接口返回 50301。

## 前端版本管理
internal/frontend/ 下：
  manager.go      — 版本目录解析、指针读写、可换 handler
  upload.go       — zip 解压与校验
- 目录：data/frontends/<app>/<version>/，另有 default/ 作为兜底
- app 有三个：reader / admin / init
- 指针：data/frontends/<app>-current.json，内容是 {"version": "..."}
- 解析顺序：-static > 指针 > default/ > 无
- 切换只重写指针并替换内存中的 handler，不使用符号链接
- 解压必须防 zip slip，版本名必须过 validateVersion
- 管理端上传/切换接口只接受 reader 和 admin，init 只能通过文件系统更换

## 测试
- ParseXxx 用写死的 JSON 字符串测试，禁止访问网络
- store 层用 SQLite 内存库（:memory:）测试，禁止连真实外部数据库
- handler 层测试用 httptest，不启动真实端口
- 并发相关代码必须有并发测试，且在 -race 下通过
- go test ./... 必须能在断网状态下全绿

## 依赖
### 第一层 · 免申请
标准库，以及 golang.org/x/*。直接 go get。
### 第二层 · 记录即可
单一职责、纯 Go、无 CGO、不改变项目结构的库。
commit message 必须有一行：dep: <模块名> — <用途>
### 第三层 · 必须申请
ORM、DI、Web 框架、需要改目录结构的、带 CGO 的。
写进 QUESTIONS.md 后跳过该任务。
### 第二层门槛
未 archived、最近 12 个月有 push、CGO_ENABLED=0 可编译、
许可证 MIT/Apache-2.0/BSD-3/ISC。

## 文字规范
全项目禁止使用 emoji：代码、注释、字符串字面量、日志、错误信息、文档、
commit message、变量名、测试数据里自己写的内容，一律禁止。

例外（重要）：从上游取回的正文、书名、作者、简介、章节标题，
以及管理员在内容规则里填写的内容，必须原样透传，
不许过滤、替换、删除其中任何字符。那是数据，不是你的表达。

禁止用 emoji 或符号代替图标和状态标记，用文字或颜色区分。

## 仓库卫生
- .gitignore 至少忽略：bin/、logs/、*.exe、.env、tmp/、data/、
  web*/dist/、web*/node_modules/、*.tsbuildinfo、tmp_*.log
- .gitattributes 内容为 * text=auto eol=lf
- 不许提交：编译产物、日志、.env、tmp/、data/、node_modules
- 仓库统一 LF 换行

## Git
- main 单分支，不走 PR
- commit 必须原子，每个可运行的小步提交一次
- 提交前必须 verify 与 -race 全绿，历史里不允许出现红点
- commit message：第一行 `<任务号>: <做了什么>`，空行，之后写为什么
- commit message 必须与实际改动相符，不许复用别人的标题
- 每个任务全绿后打 tag：`git tag <任务号>-done`
- 禁止：commit --amend、reset --hard、checkout -- .、restore .、
  clean -fd、push --force、rebase。要撤销用 revert

## 红线
- 上游地址、参数、任何凭据只出现在服务端
- 上游 HTTP 恒为 200，业务错误在 body.code：必须检查 code != 0，
  映射成 502 + 50203。不能只看 HTTP 状态码
- 对外响应不得包含：上游 URL、上游状态码、原始报文、堆栈、
  内部错误文本。统一文案，真实错误只进日志
- 错误不得吞成空结果：解析失败返回 502，不能返回空数组或空 content
- book_id / chapter_id 全程 string，禁止转 int64
- content 正文必须过白名单（p / br / img），img 只保留 src 且必须是 http(s)
- 入库的 content 必须剥掉所有 img 标签
- 拼接后的内容不入库，只在响应时生成
- JWT 签名 key 只在内存，禁止落盘、禁止写日志、禁止出现在任何错误信息里
- 强制登出只能通过轮换签名 key 实现
- 不解析 X-Forwarded-For，IP 一律取 RemoteAddr
- 解压 zip 必须做路径穿越检查
- 中间件拒绝请求必须走 api.Fail（内部已 Abort），禁止直接写响应后 return
- 不得修改 cmd/verify/main.go，不得修改已有测试来让检查通过
- 禁止使用 gin.Default() / gin.Logger() / gin.Recovery()
- 禁止 resty 的 SetDebug(true)

## 后端约束
- 统一响应封装、错误码、日志字段见 CONTRACTS.md 第 0 节
- 配置只能从 config.yaml 和 data/instance.yaml 读，不许硬编码
- 日志必须同时写 stdout 和文件
- 中间件顺序固定（见 DECISIONS.md），不得调整
- 未匹配路由：/api/* 返回 JSON 404，其余返回固定 HTML 404
- 所有路径拼接用 filepath.Join；URL 路径清洗用 path.Clean

## 前端约束

三个独立应用：web/（管理端，AntD）、web-init/（初始化，AntD）、
web-reader/（阅读端，纯 CSS）。

通用：
- 一律 Vite + React + TypeScript，strict 开启
- 管理端和初始化用 Hash 路由；阅读端用 BrowserRouter（有 SPA 兜底）
- 禁止把 dist、node_modules 提交进仓库
- 一个路由只承载一个职责，不得用 Tab / Modal / Drawer 承载另一页内容
- 视觉红线：主色不得用 indigo/violet；禁止渐变文字/按钮、玻璃拟态；
  禁止 emoji 当图标；圆角统一 6px

阅读端额外：
- 不引入任何组件库，不引入 Tailwind、CSS-in-JS、UI 框架
- 样式只通过 web-reader/src/reader.css 里的 CSS 变量控制，
  不得在组件里写死颜色、字号、间距
- 正文直接落在页面背景上，禁止卡片/边框/投影/圆角容器包裹
- 禁止背景渐变、纹理、装饰图形、翻页动画、淡入淡出
- 禁止顶部悬浮导航遮挡正文
- 行宽不得超过 38em