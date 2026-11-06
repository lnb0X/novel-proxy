# CONTRACTS

## 0. 通用

响应封装（所有 /api/* 以及未匹配路由）：
{ "code": 0, "msg": "", "trace_id": "01J...", "data": {...} }

- code：0 成功；50201 上游超时 / 50202 上游非 2xx / 50203 上游业务错误 /
  40001 参数错误 / 40400 不存在 / 50001 内部错误
- data：成功为业务数据，失败为 null
- HTTP 状态码与 code 对应（200 / 502 / 400 / 404 / 500）
- 响应体在任何模式下结构一致，永远不带堆栈、不带上游原始报文

上游错误处理（强制）：
上游 HTTP 恒为 200，业务错误在 body.code。
必须检查 code != 0，映射成 502 + 50203，并把上游 code 写进 msg。
不能只看 HTTP 状态码。

日志：JSON 行，同时写 stdout 和 log.file，由 lumberjack 滚动，trace_id 贯穿。
级别映射（level: info 时每请求只有 1 行）：
  request.start     debug   method, path, query
  upstream.request  debug   url, timeout_ms
  upstream.response debug   status, duration_ms, bytes
  parse.result      debug   ok, items_count
  response.end      info    status, code, duration_ms
  error             error   err, stack（stack 只在 app.mode=debug 时出现）

同一请求的所有日志行 trace_id 必须相同。

ID 类型：book_id 和 chapter_id 都是 19 位数字字符串，
全程 string，禁止转数字（JS Number 会丢精度）。

## 1. GET /api/search
入参：q string 必填（URL encode）；page int 1 起，默认 1
上游：novel.snssdk.com/api/novel/channel/homepage/search/search/v1/
      ?device_platform=android&aid=1967&offset=(page-1)*10&q={q}
出参 data：
  items[]      { book_id, title, author, category, abstract, thumb_url }
  page         int
  has_more     bool
  next_offset  int（上游返回的 offset）
实现要点：
- q 为空或全是空白 → 40001
- page < 1 → 按 1 处理，不报错
- 超出上游可返回范围 → 上游给空数组，照常返回 200 + items 为空
- abstract 需 TrimSpace，空串表示无简介；category 可能为空串
- ret_data 用 *[]T 接收：nil = 上游异常 → 502；非 nil 空 = 真没搜到 → 200 空数组
- 上游 search_id / log_id 只进日志，不下发

## 2. GET /api/detail
入参：book_id string 必填，空值 → 40001
上游：POST api5-sinfonlinec.novelfm.com/novelfm/bookapi/detail/v1/?aid=3040
      Content-Type: application/json，body {"book_id": "..."}
出参 data：
  { book_id, title, author, word_number, last_chapter, intro, thumb_url }
映射：title 取 book_name，author 取 anchor，last_chapter 取 last_chapter_item_id，
      intro 取 abstract，word_number 取 word_number
实现要点：
- word_number 上游是字符串，转 int；解析失败给 0
- intro 必须 TrimSpace（上游可能是 " "）
- 上游 code=1001004 表示书不存在 → 502

## 3. GET /api/catalog
入参：book_id string 必填，空值 → 40001
上游：GET fanqienovel.com/api/reader/directory/detail?bookId={book_id}
出参 data：
  { book_id, total, chapters[] }
  chapters: { chapter_id, title, volume, is_vip, is_pay, updated_at }
映射：chapter_id 取 itemId，title 取 title，volume 取 volumeNameList[i]，
      updated_at 取 firstPassTime（Unix 秒），
      is_pay 取 needPay != 0，
      is_vip 取 isChapterLock || isPaidPublication || isPaidStory
实现要点：
- 先把 chapterListWithVolume 展平成 map[itemId]Chapter，
  再按 allItemIds 的顺序取值输出。不要直接遍历 chapterListWithVolume，
  两者顺序不作保证
- 若某个 itemId 在 map 里找不到，跳过该章节并记一条 debug 日志，不要输出空章节
- chapterListWithVolume 只用来取卷名和章节属性，扁平化后丢掉嵌套
- 目录为空（allItemIds 为空）→ 200 + total: 0，不报错
- 上一章/下一章由前端按数组下标算，后端不提供

## 4. GET /api/content
入参：chapter_id string 必填，空值 → 40001
上游：GET www.toutiao.com/article/{chapter_id}/?wid={当前毫秒时间戳}
      headers：User-Agent（桌面 Chrome）、Referer: https://www.toutiao.com/、
      Cookie: __ac_nonce={11 字节随机数的 hex 前 21 位，用 crypto/rand}
出参 data：
  { chapter_id, title, content }   content 是 HTML 字符串
实现要点：
- 正文容器：<article class="syl-article-base ...">
- title 取 <title>，去掉结尾的 " - 今日头条"
- 白名单：只保留 <p>、<img>、<br>
  img 地址取 data-src（主路径），没有才取 src；
  取到的值若以 "data:" 开头则丢弃该 img（那是 1x1 占位符）
  只输出 <img src="...">，style/class/img_width 全部丢弃
  其他标签（tt-audio、span start_time、div.novel-fm-asr）剥标签保留文字
- 上游 404 = 该章节无文本（有声书）→ 404 + msg "该章节无文本内容"
- 页面里找不到 <article> → 502，不要返回空 content
- 超时用 upstream.timeout_seconds