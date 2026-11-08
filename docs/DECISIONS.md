# DECISIONS
已冻结。不得推翻，有异议写 QUESTIONS.md。

## 模块
module 名：novel-proxy

cmd/server/main.go
cmd/verify/main.go              — 验收程序，项目所有者维护，agent 不得修改
internal/config/                — 配置加载、分层、热更
internal/logx/                  — slog + lumberjack
internal/api/                   — 统一响应封装、错误码
internal/middleware/            — trace_id、recover、ipfilter、initguard、
                                  secret、auth、flags、ratelimit
internal/handler/               — 公开接口 + init + admin
internal/upstream/              — 四个上游适配
internal/store/                 — ArticleStore 接口与四种实现
internal/rules/                 — 内容拼接规则
internal/stats/                 — 请求统计
internal/audit/                 — 审计日志
internal/frontend/              — 前端版本管理
web/                            — 管理端 Vite 应用
web-init/                       — 初始化前端 Vite 应用
web-reader/                     — 阅读端前端 Vite 应用

## 构建
CGO_ENABLED=0，-trimpath -ldflags="-s -w"，输出 bin/app.exe。

## 中间件顺序（固定，不得调整）
trace_id → recover → ipfilter → initguard → secret → auth
        → flags → ratelimit → handler

## 认证
- JWT HS256，签名 key 启动时 crypto/rand 生成 32 字节
- key 只在内存，用 atomic.Pointer 持有，可并发安全地替换
- claims：sub（用户名）、perm（"admin"）、iat、exp、sat
- exp = iat + 30 分钟，滑动续期
- sat = 首次登录 + 12 小时，绝对上限，续期不得越过
- 续签时机：剩余有效期 < 10 分钟时才签新 token
- 存放：HttpOnly Cookie，SameSite=Strict，Path=/<secret>，
  Max-Age=1800，HTTPS 环境下加 Secure
- 管理端所有非 GET 请求要求请求头 X-Admin-Request: 1
- 强制登出即轮换签名 key，轮换后所有已签发 token 立即失效
- 改密码成功后自动轮换签名 key，等同于一次强制登出
- 服务重启同样使所有 token 失效
- 不引入 token 版本号字段

## 密码
- bcrypt，cost 12
- 长度 >= 12
- 小写、大写、数字、符号四类中至少满足三类
- 不得与用户名相同，不得包含用户名（忽略大小写）
- 拒绝常见弱口令：password、qwerty、123456、admin、letmein
- 前后端规则必须一致，后端为最终裁判

## 配置
优先级 env > data/instance.yaml > config.yaml
- config.yaml：进 git，带注释和默认值，程序不写它
- data/instance.yaml：不进 git，init 和运行期写入
- 环境变量：APP_MODE / APP_ADDR / APP_LOG_LEVEL / APP_STORAGE_DSN

可热更（立即生效，写回 instance.yaml）：
  log.level、flags.*、storage.enabled、rate_limit.*、ip_filter.*、
  content_rules.*、upstream.timeout_seconds、upstream.max_concurrency
需重启：
  server.addr、server.admin_path、storage.driver、storage.dsn、log.file、
  log.max_size_mb、log.max_backups、log.max_age_days、log.compress
  （lumberjack 的滚动参数在它自己的互斥锁里读，运行期不可变，见 W14）

## 配置 schema
config.yaml，所有键可省略取默认值。未知键必须报错（KnownFields）。

app.mode            string  release   debug / release
server.addr         string  127.0.0.1:8080
server.shutdown_timeout_seconds  int  10
log.level           string  info      debug / info / error
log.file            string  ./logs/app.log
log.max_size_mb     int     50
log.max_backups     int     14
log.max_age_days    int     14
log.compress        bool    true

setup.completed     bool    false
admin.username      string  ""
admin.password_hash string  ""
server.admin_path   string  ""

flags.search        bool    true
flags.detail        bool    true
flags.catalog       bool    true
flags.content       bool    true

storage.enabled     bool    false
storage.driver      string  ""        sqlite / postgres / mysql / mongo
storage.dsn         string  ""

upstream.timeout_seconds  int  10
upstream.max_concurrency  int  8

rate_limit.<api>.mode        string  off     global / per_ip / off
rate_limit.<api>.per_minute  int     0
rate_limit.<api>.burst       int     0

ip_filter.mode   string  off     off / allow / block
ip_filter.scope  string  all     all / api / admin
ip_filter.rules  []string []

content_rules.default.prepend  *string  null
content_rules.default.append   *string  null
content_rules.default.replace  []Rule   []
content_rules.default.override *string  null
content_rules.books.<book_id>.{prepend,append,replace,override}

enum 字段取到非法值必须报错退出，不许兜底成默认值。

## 内容规则
- 全局一套 + 按书覆盖，按书最多 100 本，每条 replace 最多 50 条
- prepend / append：按书为 null 时继承全局；为空字符串时抑制全局
- replace：全局规则按顺序执行完，再执行按书的规则；顺序在管理端可调
- replace 是级联的，前一条的结果参与后一条匹配；支持勾选正则
- override（接管）：非空即接管，不做任何匹配。按书配置了 override，
  这本书所有章节都被接管；全局 override 非空，全站所有章节都被接管。
  接管时不抓上游、不查缓存、不写缓存、不执行其余规则，直接以
  override 字符串作为正文返回。预览接口中接管时 original 为 null，
  applied 记 { "rule": "book.override" 或 "global.override", "hits": 1 }
- 拼接在响应时进行，结果不入库
- 拼接内容不做安全过滤，管理员可写入任意 HTML，风险由 token 保管承担

## 执行流程（/api/content）
1. 检查接管规则，命中则直接返回自定义内容
2. 查缓存，命中取原文；未命中抓上游、剥图、入库
3. 应用 replace（全局 → 按书）
4. 应用 prepend / append
5. 返回

## 限速
- 每个接口独立配置：mode（global / per_ip / off）、per_minute、burst
- 用 golang.org/x/time/rate 令牌桶
- per_ip 的 limiter 每 5 分钟扫描，10 分钟无访问的删除
- per_ip 的 map 上限 10000 条，超过后新 IP 按全局限流处理
- 超限返回 429 + 42901 + Retry-After
- 管理端 API 用固定的内部限额，不进配置界面
- 登录失败锁定是独立机制：IP 5 次 / 账号 10 次

## IP 过滤
- mode：off / allow / block；scope：all / api / admin
- 支持 CIDR，用 net/netip
- /init/* 永不受影响
- 公开 API 被拒返回 403 + 40301；管理端路径被拒返回 404
- 保存时若当前 IP 不在新白名单内，拒绝保存
- 名单最多 200 条
- 不解析 X-Forwarded-For，IP 取 RemoteAddr（net.SplitHostPort）
- 部署在反向代理后时 per_ip 会退化为全局限速

## 存储
ArticleStore 接口：
  Init(ctx) error            — 建表/建索引，幂等
  Ping(ctx) error
  Get(ctx, bookID, itemID) (*Article, error)
  Put(ctx, *Article) error   — upsert
  Delete(ctx, bookID, itemID) error
  DeleteByBook(ctx, bookID) (int64, error)
  DeleteAll(ctx) (int64, error)
  Count(ctx) (int64, error)
  SumSize(ctx) (int64, error)

articles 表（SQL）：
  item_id     varchar(32)  主键
  book_id     varchar(32)  NOT NULL，索引
  title       varchar(512)
  content     text
  size_bytes  bigint
  created_at  timestamp
  updated_at  timestamp

Mongo：collection articles，唯一索引 {item_id}，普通索引 {book_id}

content 入库时剥掉所有 img 标签，只留 p / br / 文字。

文章缓存永不失效：没有 TTL 概念，/cache 管理接口的
ttl_left_seconds 恒为 0（W14 裁决）。

## 明文约定
命中缓存返回纯文字，未命中返回带图 HTML，同一章两次看不同，这是刻意取舍。

## 静态服务与前端版本
- 版本目录：data/frontends/<app>/<version>/，app 取 reader / admin / init
- 兜底目录：data/frontends/<app>/default/
- 指针文件：data/frontends/<app>-current.json，内容 {"version": "..."}
- 解析顺序：-static 参数 > 指针文件 > default/ > 返回 404
- 切换：重写指针文件（先写 .tmp 再 os.Rename），并替换内存中的 handler
- 运行期通过 atomic 持有 http.Handler 实现可换，不使用符号链接
- 上传 zip：压缩包上限 50MB，解压后上限 200MB，根目录必须有 index.html
- 解压必须防 zip slip
- 切换管理端后 10 秒内未确认则自动回滚到上一版本
- 回滚点记在 data/frontends/<app>-previous.json
- 恢复手段：删除 data/frontends/admin-current.json 并重启，回落到 previous

## 前端应用（三个，各自独立）
- web/          管理端：React + AntD + Hash 路由，base './'
- web-init/     初始化：React + AntD，base '/init/'
- web-reader/   阅读端：React，纯 CSS，不使用任何组件库
- 三者各自独立的 package.json 和 node_modules，不使用 workspaces
- 构建产物不进 git
- web-reader 参与 frontends 的上传流程（app 名 reader）
- web-init 不参与，只能通过文件系统放到 data/frontends/init/ 更换

## init 页面
- init 是 frontends 的第三个 app，复用同一套版本解析与目录约定；
  -static 参数不作用于 init（它只替换根站点挂载）
- 源码：web-init/，独立的 Vite + React + TS 应用，base '/init/'，
  与管理端是两个构建产物
- 部署：把构建产物放到 data/frontends/init/default/（或放到版本目录
  再写 data/frontends/init-current.json），重启生效
- 不重新编译 Go 就能换 init 前端
- 兜底：data/frontends/init/ 不存在时，/init/ 返回一个写死在 Go 里的
  固定说明页，写明应该把产物放到哪个绝对路径；不用 go:embed 塞第二套
  init UI，避免与 React 那套分叉
- data/frontends/init/ 不进 git，每个部署环境自己放一次，写进部署说明
- 管理端的上传/切换接口仍然只接受 reader 和 admin；init 只能通过
  文件系统更换，不走 web

## 日志
- slog JSON，同时写 stdout 和文件
- 级别映射见 CONTRACTS 第 0 节
- trace_id 贯穿，secret 路径在日志中脱敏为 /<secret>/

## 统计与审计
- request_log：每请求一行，字段 time、path、status、code、
  duration_ms、upstream_ms、bytes、trace_id、query（varchar(256)，
  存 search 的 q，其余接口为空）
- audit_log：time、event、actor、ip、ua、detail、result
- 审计事件：login_success、login_fail、logout、password_change、
  force_logout、flag_change、rate_limit_change、ip_filter_change、
  cache_clear、rules_change、frontend_upload、frontend_activate、
  frontend_rollback、frontend_delete、config_change
- 两张表都保留 90 天，超期清理
- storage.enabled 为 false 时，stats 与 audit 三组接口一律返回
  503 + 50301，msg 固定为「统计与审计需要开启文章缓存」
- upstream_health 取最近 100 条 request_log 推导：5xx 或超时占比
  大于 20% 为 down，大于 5% 为 degraded，否则 ok
- top_queries 从 request_log.query 列聚合
- cache_hit_rate 由 store 层维护两个 atomic 计数器（hits / misses），
  stats 接口读取后计算比值；store 关闭时比率为 0

## 安全基线
- secret 路径存在性差异是已知取舍：错误路径 404、正确路径未登录 401。
  401 是前端跳登录所必需，无法与 404 伪装同时消除
- 响应 msg 不携带上游 URL、状态码、原始报文与 Go 错误细节，
  真实错误只进日志；例外：50203 的上游业务 code 按契约写入 msg

## 404
- 固定 HTML，零动态内容，状态码 404，Cache-Control: no-store
- /api/* 的未知路径返回 JSON 404，不返回这个 HTML
- 静态资源找不到返回空 body 404

## 管理端页面（14 个，每个独立路由）
/login
/
/flags
/rate-limits
/access
/stats
/cache
/rules
/frontends
/config/log
/config/upstream
/account
/account/password
/audit

## 阅读端（v0.3）
路由三条：
  /                              搜索页
  /books/:bookId                 详情 + 目录（合并，无独立目录页）
  /books/:bookId/read/:chapterId 正文页

数据：
- 全部走 TanStack Query + 原生 fetch
- 详情页并行请求 detail 与 catalog
- 正文页并行请求 content 与 catalog（后者用于算上一章 / 下一章）
- 上一章 / 下一章按 catalog 数组下标计算，后端不提供

排版（CSS 变量，唯一配色与字号来源）：
- 字号可选值：16 / 20 / 24 / 28 / 32
- 行高 1.85，正文最大宽度 38em，段间距 0.8em，不做首行缩进
- 主题三套：
    day   bg #ffffff  fg #1a1a1a  muted #8a8a8a  line #e8e8e8
    sepia bg #f5f0e6  fg #3a3226  muted #8a8172  line #e0d8c8
    night bg #1a1a1a  fg #c8c8c8  muted #6a6a6a  line #2a2a2a
- 字体两种：系统无衬线 / 系统衬线，不加载任何 webfont
- 阅读设置的三项（字号、主题、字体）存 localStorage，键名前缀 reader.

交互：
- 顶栏底栏用 position:fixed + transform 隐藏，不用 display:none
- 自动隐藏规则：scrollY <= 8 或距底 <= 8 时强制显示；
  向下滚动超过 8px 隐藏；向上滚动超过 8px 显示
- 点击正文区切换显示状态，与自动隐藏并存
- 章节切换时 scrollTo(0,0)，且强制显示顶栏底栏
- 滚动监听必须 passive + requestAnimationFrame 节流
- 不做左右滑动翻章（与滚动冲突）
- 桌面端：方向键左右翻章、空格下翻一屏
- 正文上下内边距永久保留，菜单显隐不得引起重排
- 底栏高度计入 env(safe-area-inset-bottom)

行为：
- 搜索的 q 与 page 同步到 URL
- 目录每页 200 章，页码体现在 ?toc=N
- 正文页「返回」固定跳 /books/:bookId，不用 history.back()
- document.title 同步为「章节名 - 书名」
- 章末字数由前端去掉标签后统计，不请求接口
- 图片 onerror 时隐藏，不显示裂图占位
- 详情页简介超过 5 行折叠
- 上游失败与空结果必须显示不同文案

部署：
- 构建产物打 zip，通过管理端 /frontends 上传并激活
- 首版放到 data/frontends/reader/default/ 作为出厂默认

## 明确不用
Next.js / Nuxt / Tailwind / shadcn / MUI / Zustand / Redux / Axios /
viper / zap / logrus / echo / fiber / chi / goquery / GORM / ent