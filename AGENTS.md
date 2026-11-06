# AGENTS.md

## 项目
自用小说中转站。Go 后端（gin + resty）代理上游接口并统一封装。
v0.1 只有后端：一个 Go 服务 + 四个 API。不涉及任何前端。
这是薄壳：只做请求和字段映射。不要写防御性代码、不要做兼容层、不要为"以后可能"预留抽象。

## 环境
Windows。所有命令必须是纯 go 调用，不要用 make、sed、awk。
构建产物是 bin/app.exe。

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
4. 全绿后 commit
5. 在 BACKLOG 勾掉该任务
6. 回到第 1 步

## 完成定义
完成 = go run ./cmd/verify 全绿 + BACKLOG 已勾选 + 有对应测试。
未通过 verify 时禁止声称完成，也禁止开始下一个任务。

## 输出纪律
每个任务结束只写三行：做了什么（不超过 3 句）、verify 结果、commit hash。
不要复述代码、不要贴 diff、不要解释设计取舍。
理由：自述内容会挤占上下文，上下文压缩时规则会丢失。

## 契约规则
docs/CONTRACTS.md 是唯一真相。
允许：新增接口、新增可选参数、新增返回字段
禁止：删除或重命名已有字段/接口；修改已有字段的类型或语义；修改已有接口的必填参数
需要上述禁止项才能完成的任务：写进 QUESTIONS.md，跳过，做下一个

## 上游适配
每个上游接口一个文件，放在 internal/upstream/ 下：
search.go / detail.go / catalog.go / content.go
每个文件必须拆成两个函数：
  FetchXxx(ctx, client, ...) ([]byte, error)    — 只发请求，用统一的 resty client
  ParseXxx(raw []byte) (Result, error)          — 纯函数，只做解析和字段映射
不掺业务逻辑、不做缓存、不写错误分类。
handler 层只依赖这两个函数，换源时只改对应文件。

## 测试
测试只覆盖 ParseXxx，喂写死的 JSON 字符串，禁止在测试里访问网络。
内部包（config、logx、api）的纯逻辑直接单测。
go test ./... 必须能在断网状态下全绿。

## 依赖
### 第一层 · 免申请
标准库，以及 golang.org/x/*（准官方扩展）。直接 go get。
### 第二层 · 记录即可
单一职责、纯 Go、无 CGO、不改变项目结构的库。
可以加，但 commit message 必须有一行：dep: <模块名> — <用途>
### 第三层 · 必须申请
ORM（gorm/ent/sqlx）、DI（wire/fx）、其他 Web 框架（echo/fiber/chi）、
需要改目录结构才能用的、带 CGO 的。
写进 QUESTIONS.md：用途 / 候选 / 为什么现有依赖做不到 / 依赖树多大 / 最近一次提交。
然后跳过该任务。
### 第二层也要过的门槛
未 archived、最近 12 个月有 push、CGO_ENABLED=0 可编译、
许可证 MIT/Apache-2.0/BSD-3/ISC、依赖树里不拉进额外的 Web 框架或 ORM。
### 不要过度手写，也不要滥用依赖
不要因为"自己写更可控"就手写：手写代码同样没人 review，也没有别人替你踩坑。
也不要为了省几十行就引一个新依赖。
判断标准是"这个库会不会改变项目结构"，不是"这段代码我能不能写"。

## 文字规范
全项目禁止使用 emoji：代码、注释、字符串字面量、日志、错误信息、文档、
commit message、变量名、测试数据里自己写的内容，一律禁止。

例外（重要）：从上游取回的正文、书名、作者、简介、章节标题，
必须原样透传，不许过滤、替换、删除其中任何字符，包括 emoji。
那是数据，不是你的表达。

禁止用 emoji 或符号代替图标和状态标记，不要用打勾、叉号、警告三角这类字符。
需要区分状态时用文字或颜色，不要造符号。

## 仓库卫生
- 必须存在 .gitignore，至少忽略：bin/、logs/、*.exe、.env、tmp/
- 必须存在 .gitattributes，内容为 * text=auto eol=lf
- 这两个文件必须在 T1 第一次运行服务之前建好。
  否则 logs/app.log 会被 git add 进去，之后再补 ignore 也洗不掉历史
- 不许提交：编译产物、日志、.env、tmp/、任何含凭据的文件
- 仓库统一 LF 换行

## Git
- main 单分支，不走 PR
- commit 必须原子：每完成一个可运行的小步就提交，整任务只提交一次视为违规
- 提交前必须 go run ./cmd/verify 全绿，历史里不允许出现红点
- commit message：第一行 `<任务号>: <做了什么>`，空行，之后写为什么
- 每个任务全绿后打 tag：`git tag <任务号>-done`
- 禁止：git commit --amend、git reset --hard、git checkout -- .、
  git restore .、git clean -fd、git push --force、rebase。要撤销用 git revert
- .env 和任何含凭据的文件不得提交

## 红线
- 上游地址、参数、任何凭据只出现在服务端，绝不下发前端
- 上游 HTTP 恒为 200，业务错误在 body.code：必须检查 code != 0，
  映射成 502 + 50203 并透出上游 code。不能只看 HTTP 状态码
- 错误不得吞成空结果：解析失败返回 502，不能返回空数组或空 content
- book_id / chapter_id 全程 string：Go string，禁止转 int64
- content 必须过白名单：只保留 p / img / br，img 只保留 src 且必须是 http(s)，
  其他标签剥标签保留文字。禁止用正则剥 HTML
- 不得修改 cmd/verify/main.go，也不得修改已有测试来让检查通过
- v0.1 不建数据库、不做缓存、不碰前端、不提供静态文件服务
- 禁止使用 gin.Default()、gin.Logger()、gin.Recovery()，原因见 DECISIONS
- 禁止 resty 的 SetDebug(true)

## 后端约束
- 统一响应封装、日志级别、trace_id 见 CONTRACTS.md 第 0 节
- 配置只能从 config.yaml 读，不许硬编码地址、超时、日志参数
- 日志必须同时写 stdout 和文件，不许只打 stdout
- 未匹配路由（NoRoute）统一返回 JSON 404，格式与 /api/* 一致，不返回 HTML
- 所有路径拼接用 filepath.Join，不要手写斜杠