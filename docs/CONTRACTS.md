# CONTRACTS

## 0. 通用

响应封装：
{ "code": 0, "msg": "", "trace_id": "01J...", "data": {...} }

错误码：
0      成功
40001  参数错误
40101  未登录或 token 无效
40301  IP 被拒绝
40400  不存在
40901  已初始化
42901  请求过于频繁
50001  内部错误
50201  上游超时
50202  上游非 2xx
50203  上游业务错误
50301  接口已关闭
50302  服务未初始化

HTTP 状态码与 code 对应。响应体在任何模式下结构一致，
永远不带堆栈、不带上游原始报文、不带签名密钥。
响应 msg 不携带上游 URL、状态码与 Go 错误细节，对外统一文案，
真实错误只进日志；例外：50203 的上游业务 code 按本节规则写入 msg。

日志级别：
request.start      debug   method, path, query
upstream.request   debug   url, timeout_ms
upstream.response  debug   status, duration_ms, bytes
parse.result       debug   ok, items_count
content.overridden info    book_id, chapter_id, rule_source
frontend.activate  info    app, from, to
auth.revoke        info    actor, ip
response.end       info    status, code, duration_ms
error              error   err, stack（stack 仅 app.mode=debug）

同一请求所有日志行 trace_id 相同。

ID 类型：book_id 和 chapter_id 都是 19 位数字字符串，
全程 string，禁止转数字。

## 1. GET /api/search
入参：q string 必填（URL encode）；page int 1 起，默认 1
上游：novel.snssdk.com/api/novel/channel/homepage/search/search/v1/
      ?device_platform=android&aid=1967&offset=(page-1)*10&q={q}
出参 data：
  items[]      { book_id, title, author, category, abstract, thumb_url }
  page         int
  has_more     bool
  next_offset  int
实现要点：
- q 为空或全是空白返回 40001
- page < 1 按 1 处理
- abstract 需 TrimSpace；category 可能为空串
- ret_data 用指针切片接收：nil 表示上游异常返回 502，
  非 nil 但长度为 0 表示确实没搜到，返回 200 空数组
- 上游 search_id / log_id 只进日志

## 2. GET /api/detail
入参：book_id string 必填，^\d{1,20}$
上游：POST api5-sinfonlinec.novelfm.com/novelfm/bookapi/detail/v1/?aid=3040
      Content-Type: application/json，body {"book_id": "..."}
出参 data：
  { book_id, title, author, word_number, last_chapter, intro, thumb_url }
映射：title 取 book_name，author 取 anchor，
      last_chapter 取 last_chapter_item_id，intro 取 abstract，
      word_number 取 word_number
实现要点：
- word_number 上游是字符串，转 int；解析失败给 0
- intro 必须 TrimSpace
- 上游 code=1001004 表示书不存在，映射成 502 + 50203

## 3. GET /api/catalog
入参：book_id string 必填，^\d{1,20}$
上游：GET fanqienovel.com/api/reader/directory/detail?bookId={book_id}
出参 data：
  { book_id, total, chapters[] }
  chapters: { chapter_id, title, volume, is_vip, is_pay, updated_at }
映射：chapter_id 取 itemId，title 取 title，volume 取 volumeNameList[i]，
      updated_at 取 firstPassTime（Unix 秒），
      is_pay 取 needPay != 0，
      is_vip 取 isChapterLock 或 isPaidPublication 或 isPaidStory
实现要点：
- 先把 chapterListWithVolume 展平成 itemId 到章节的映射，
  再按 allItemIds 的顺序输出
- 找不到的 itemId 跳过并记 debug 日志
- 目录为空返回 200 + total 为 0
- 上一章 / 下一章由前端按数组下标计算

## 4. GET /api/content
入参：book_id string 必填，^\d{1,20}$
      chapter_id string 必填，^\d{1,20}$
上游：GET www.toutiao.com/article/{chapter_id}/?wid={当前毫秒时间戳}
      headers：桌面 Chrome 的 User-Agent、Referer: https://www.toutiao.com/、
      Cookie: __ac_nonce={11 字节随机数的 hex 前 21 位，用 crypto/rand}
出参 data：
  { book_id, chapter_id, title, content }
实现要点：
- book_id 只做格式校验后回填，不验证书是否存在，不参与上游 URL
- FetchContent 只用 chapter_id；ParseContent 保持纯函数
- 正文容器：<article class="syl-article-base ...">
- title 取 <title> 去掉 " - 今日头条"，只取文档第一个 title 元素
- 白名单：p、img、br；img 取 data-src，没有才取 src，
  取值以 "data:" 开头则丢弃；只输出 src
- 其他标签剥标签保留文字
- 上游 404 返回 404 + 40400，msg 为 "该章节无文本内容"
- 找不到 article 容器返回 502
- 超时用 upstream.timeout_seconds
- 启用缓存时：先查库，未命中抓上游后剥掉所有 img 再入库

## 5. init 接口（setup.completed 为 false 时可用）

GET  /init/api/state              → { completed, has_token }
POST /init/api/token              body { token }
  → 200 + init_session cookie（30 分钟）
  → 401；同一 IP 5 次失败锁 15 分钟
POST /init/api/account            body { username, password, admin_path }
POST /init/api/storage            body { enabled }
POST /init/api/database/test      body { driver, dsn }
  → { ok: bool, message: string }
POST /init/api/database           body { driver, dsn }
POST /init/api/confirm            → 写盘，返回 { admin_path }
POST /init/api/cancel             → 清空内存草稿

所有步骤状态存在服务端内存，按 init_session 关联。
confirm 之前磁盘上不得有任何写入。
setup.completed 为 true 之后，上述路径全部返回 404。

## 6. 认证接口（都在 /<secret>/api 下）

POST /<secret>/api/auth/login
  body { username, password }
  → 200 + Set-Cookie token=...，data { username, expires_at, sat }
  → 401 统一 "账号或密码错误"
  → 429 锁定中，带 Retry-After

POST /<secret>/api/auth/logout    → 204，清 cookie

GET  /<secret>/api/auth/me
  → 200 { username, perm, exp, sat }
  → 401

POST /<secret>/api/auth/revoke    → 204
  轮换签名 key，所有已签发 token 立即失效，同时清 cookie

POST /<secret>/api/account/password
  body { old_password, new_password }
  → 200，写新 hash，轮换签名 key，清 cookie，msg 为 "密码已修改，请重新登录"
  → 40001 旧密码错误

## 7. 管理接口（都在 /<secret>/api 下，需认证）

GET  /flags
  → { search: bool, detail: bool, catalog: bool, content: bool }
PUT  /flags/{name}                body { enabled }
  关闭后该接口返回 503 + 50301

GET  /rate-limits                 → 四个接口各 { mode, per_minute, burst }
PUT  /rate-limits                 body 同上，整体替换

GET  /ip-filter                   → { mode, scope, rules[] }
PUT  /ip-filter                   body 同上
  若 mode 为 allow 且当前 IP 不在新名单，返回 40001

GET  /stats/overview
  → { today_requests, today_errors, p95_ms, cache_hit_rate, per_api{},
      upstream_health, uptime_seconds, version, recent_errors[] }
  storage.enabled 为 false 时返回 503 + 50301，
  msg 固定 "统计与审计需要开启文章缓存"

GET  /stats/query?from=&to=&api=
  → { buckets[], p50, p95, p99, error_codes{}, top_queries[], top_books[] }
  storage.enabled 为 false 时同上返回 503 + 50301

GET  /cache?book_id=&chapter_id=&page=&size=
  → { total, total_bytes, hit_rate, items[] }
    items: { item_id, book_id, title, size_bytes, created_at, ttl_left_seconds }
DELETE /cache/{item_id}           → 204
DELETE /cache?book_id=            → { deleted: n }
DELETE /cache/all                 body { confirm: "DELETE" } → { deleted: n }

GET  /content-rules               → { default{}, books{} }
PUT  /content-rules               body 同上，整体替换
POST /content-rules/preview       body { book_id, chapter_id }
  → { original: string|null, result: string, applied: [{ rule, hits }] }

GET  /config/log
  → { level, max_size_mb, max_backups, max_age_days, compress }
PUT  /config/log                  body 同上，立即生效

GET  /config/upstream             → { timeout_seconds, max_concurrency }
PUT  /config/upstream             body 同上

GET  /frontends
  → { reader: { current, previous, versions[] },
      admin:  { current, previous, versions[] } }
    versions[]: { version, size, files, created_at }

POST /frontends/{app}/upload      multipart，字段名 file
  → { version, size, files }
  → 40001 缺少 index.html、超过大小限制、路径穿越

POST /frontends/{app}/activate    body { version, confirm_within_seconds }
  → 200；未在时限内确认则自动回滚

POST /frontends/{app}/confirm     body { version }   → 200

POST /frontends/{app}/rollback    → 200

DELETE /frontends/{app}/{version} → 204

app 取 reader 或 admin。

GET  /audit?event=&from=&to=&page=&size=
  → { total, items[] }
    items: { ts, event, actor, ip, ua, detail, result }
  storage.enabled 为 false 时返回 503 + 50301，
  msg 固定 "统计与审计需要开启文章缓存"