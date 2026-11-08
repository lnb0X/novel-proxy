# QUESTIONS

格式：问题 / 证据 / 影响哪个任务。

写完跳过该任务，继续做下一个，不要停下来等回复。

## Q1 W3 init 页面的承载与伺服方式
问题：init 页面（React 应用，base /init/）放在哪里、由谁伺服？
      1. 它是 web/ 管理端单应用的一部分，还是独立应用？AGENTS 前端约束说
         web/ 是单个 Vite 应用；若 init 页属于它，则 W12 才建骨架，W3 阶段
         无处产出页面。
      2. W4 之前静态服务尚不存在，且 DECISIONS「静态服务与前端版本」的
         data/frontends/<app>/ 只定义了 reader 与 admin 两种 app，init 页面
         不在版本管理范围内。未初始化阶段 302 到 /init/ 之后，页面由什么提供
         （临时静态目录、二进制内嵌、还是别的）？
证据：BACKLOG W3「init 页面为 React 应用，固定 base /init/」；
      DECISIONS「静态服务与前端版本」app 仅 reader / admin；
      AGENTS「前端约束」web/ 为单个 Vite 应用，W12 才搭骨架。
影响：W3（本次跳过）、W12（页面归属与构建方式）。
W3 的后端部分（令牌门禁、四步流程、内存草稿、confirm 写盘、
initguard 门禁行为）规格明确，待上面两点澄清后与页面一并实现。

## Q2 W6 接口开关的持久化位置
问题：flags（search / detail / catalog / content 四个开关）的开关状态
      存在哪里？W6 验收要求「关闭 search 后返回 503 + 50301，重启后状态
      保持」，即必须持久化；但 DECISIONS 配置 schema 没有 flags 键，
      可写文件只有 data/instance.yaml，而它对 Config 结构开 KnownFields
      严格模式，写入 flags 段会被未知键校验拒绝。
      若要新增 flags.<api> 配置键，属于对 DECISIONS 配置 schema 的修改，
      按规则不由实现方自行决定。
证据：BACKLOG W6「四个开关 + 热更 + 管理接口 / 重启后状态保持」；
      DECISIONS「配置 schema」无 flags 相关键；「可热更」清单也不含
      flags；CONTRACTS 第 7 节只定义了 GET/PUT /flags 接口形状。
影响：W6（本次跳过）。W6 的令牌桶部分（global / per_ip / off、
      per_ip 回收与上限、429 + 42901 + Retry-After）规格明确，
      待持久化位置澄清后与开关一并实现。

## Q3 W8 统计与审计的数据来源与可用条件
问题：
      1. request_log / audit_log 两张表建在哪里？AGENTS 存储层写明
         「数据库只在 storage.enabled 时连接」，storage 关闭时统计与
         审计三组接口的行为未定义（50301？50001？还是降级不可用）。
      2. /stats/overview 的 upstream_health 如何判定（健康检查探测？
         从 request_log 推导？）、version 取值来源，均未定义。
      3. /stats/query 的 top_queries[]：request_log 的字段只有
         time/path/status/code/duration_ms/upstream_ms/bytes/trace_id，
         没有查询词，top_queries 的数据来源是什么？
         cache_hit_rate 的命中 / 未命中计数由谁维护（ArticleStore
         接口没有计数方法）。
      4. 验收项「记录一次开关变更」依赖 W6 的开关接口，W6 已因 Q2 跳过。
证据：DECISIONS「统计与审计」「存储」「配置 schema」；
      CONTRACTS 第 7 节 stats / audit 三组接口；AGENTS「存储层」。
影响：W8（本次跳过）。两张表的 DDL、90 天保留、审计事件清单本身
      规格明确，待上面四点澄清后实现。

## Q4 W9 内容规则 override 的命中语义
问题：override（接管）是 *string，无 regex 标志位。「命中时不抓上游、
      不查缓存、不写缓存」的前提下，可用作匹配输入的只有 book_id /
      chapter_id / 路径等请求元数据。那么：
      1. override 按什么匹配（章节 ID 正则？子串？），还是「配置非空
         即接管、其字符串直接作为正文返回」？
      2. 全局 default.override 非空时是否意味着所有书所有章节都被接管？
      3. /content-rules/preview 返回 original: string|null，null 是否
         对应 override 命中（因此未抓取原文）？applied 里 override 的
         hits 如何计数？
证据：DECISIONS「内容规则」「配置 schema」；CONTRACTS 第 7 节
      /content-rules 与 /content-rules/preview。
影响：W9（本次跳过）。prepend / append 的继承与抑制、replace 的
      级联与顺序、管理接口形状均规格明确，待 override 语义澄清后实现。

## Q5 W10 三个外部数据库无可用实例
问题：本机没有可用的 postgres / mysql / mongo 实例（Windows 开发机，
      无 Docker），W10 验收「三种库各走一遍 Init + Put + Get + Delete」
      无法执行。按任务自身约定「没有可用实例的驱动，写 QUESTIONS.md
      后跳过」。sqlite 驱动已在 W2 完成（modernc.org/sqlite v1.58.0，
      纯 Go）。
证据：BACKLOG W10 验收条款；本机环境无三个数据库的可用连接。
影响：W10（本次跳过）。待提供三个库的连接方式（DSN 与实例）后实现
      dialect.go 其余方言与 mongo.go，并跑通验收。

## Q6 W13 十二个页面中八个的后端缺失或规格不全
问题：W13 验收要求「CONTRACTS 第 7 节的每个操作都能在界面上完成」，
      但对应后端大多尚未存在或规格不全：
      1. /flags、/rate-limits 页面依赖 W6 接口（Q2 跳过）；
      2. /stats、/audit 页面依赖 W8 接口（Q3 跳过）；
      3. /rules 页面依赖 W9 接口（Q4 跳过）；
      4. /cache 页面：CONTRACTS 定义了 GET/DELETE /cache*，但无任务实现
         该端点；且 items 的 ttl_left_seconds 无从计算（articles 表与
         配置里都没有 TTL 概念），hit_rate 依赖 Q3-3 的命中计数；
      5. /config/upstream：upstream.max_concurrency 当前没有任何代码
         消费（并发上限从未实现），PUT 后是无副作用的空操作；
         timeout_seconds 热更是可行的。
      现有后端可直接支撑的页面只有 /access、/frontends、/account、
      /account/password 四个；/config/log 规格完整可实现（需 logx 支持
      热更 level 与滚动参数）。
证据：CONTRACTS 第 7 节；DECISIONS「配置 schema」（无 TTL 键）；
      BACKLOG W13 验收条款；QUESTIONS Q2 / Q3 / Q4。
影响：W13（本次跳过）。待 Q2 / Q3 / Q4 澄清并补齐 cache TTL 与
      max_concurrency 语义后，一次性实现缺失端点与全部十二页。

## Q7 x/sync 信号量没有 SetLimit（W14 事实性偏差）
问题：W14 写明「用 golang.org/x/sync/semaphore.Weighted，它支持 SetLimit」。
      实测 go1.25 工具链能用的全部 x/sync 版本（v0.12 到 v0.15，
      含模块图内的 v0.22）都没有 SetLimit——该方法在更新的 x/sync 里，
      而那些版本要求 go >= 1.26，本机工具链装不上。
      已改用互斥锁下重建 Weighted 的方式实现同一语义（运行期热调上限，
      旧名额持有者继续用旧信号量释放），目标行为不变。
证据：golang.org/x/sync@v0.22.0/semaphore/semaphore.go 无 SetLimit 方法；
      go.mod 要求 go 1.25.0 的全部版本均无。
影响：W14 已按替代实现完成，无阻塞；如需严格按 SetLimit 走，
      需升级 Go 工具链到 1.26+。

## Q8 W10 MongoDB 实例认证失败（已解决）
根因：authSource 写错。用户 <user> 建在 <db> 库（面板显示「绑定用户 test，
权限：数据库所有者」），不是 admin 库。DSN 改为
authSource=<as> 后 2026-11-08 验收通过：mongo live 全链路 ALL PASS
（Init/Put x2/Get/Count+SumSize/Delete/DeleteByBook/并发 10 Put/
DeleteAll 归零），验收驱动 tmp/w10mongo.go（验收后删除）。
原始报错记录：authSource=<as> 时 SCRAM-SHA-1 与 SHA-256 均返回
AuthenticationFailed。

## Q9 reader 挂载点缺 SPA 兜底（W4 遗留，本卷补齐）
问题：DECISIONS 明写「阅读端用 BrowserRouter（有 SPA 兜底）」，但 W4
      实现的 reader 挂载点是纯静态服务，刷新 /books/:id 会 404。
      W4 时 reader 尚不存在所以未暴露；v0.3 BrowserRouter 使其成为
      阻塞。已在 reader 挂载点补 SPA 兜底（无扩展名路径回落
      index.html，带扩展名资源仍空 404），不改 /api/* 契约。
证据：internal/frontend/manager.go Reload；BACKLOG R3/R4 验收依赖
      深链接刷新。
影响：W4 已补齐（非 v0.3 新契约），R3/R4 可正常做。
