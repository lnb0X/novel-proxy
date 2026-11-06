# BACKLOG

## v0.1 四个接口

- [x] T1 配置与日志
      严格按这个顺序做：
      1. 建 .gitignore：bin/、logs/、*.exe、.env、tmp/
      2. 建 .gitattributes：* text=auto eol=lf
      3. go mod init novel-proxy
      4. 按 DECISIONS.md 的「配置 schema」生成 config.yaml，不要发明键名
      5. 建目录骨架：cmd/server、internal/config、internal/logx、internal/api
      6. 实现配置加载 + 日志初始化

      注意：第 3 步之前 go run ./cmd/verify 会报 go.mod 不存在，这是正常的，
      不要为此改任何代码。第 3 步之后再跑。

      验收：
        go test ./internal/config ./internal/logx 全绿
        覆盖：默认值、yaml 覆盖、flag 覆盖、env 覆盖、非法 level 报错、未知键报错
        go run ./cmd/server -config ./config.yaml 输出一条 JSON 到 stdout，
        logs/app.log 内容相同
        log.level 改 debug 后 debug 日志出现
        max_size_mb 改 1 后出现轮转文件
        git status 里看不到 logs/ 和 bin/ 下的任何文件
      完成后 git tag T1-done

- [x] T2 gin 骨架与统一响应
      gin.New() + 自写 slog 中间件 + 自写 recovery 中间件 + 统一响应封装
      NoRoute 统一返回 JSON 404；app.mode 映射到 gin.SetMode
      验收：
        curl http://localhost:8080/api/nope → 404，响应体是 JSON，含 code/msg/trace_id
        curl http://localhost:8080/         → 404，同样是 JSON
        上面两次请求之后，logs/app.log 里每次请求都有 request.start 和 response.end 两条，
        且同一次请求的两条 trace_id 相同
        日志里没有出现 gin 自己格式的行（例如 [GIN-debug] 或 [GIN]）
        app.mode 改 debug 后，error 级日志开始带 stack 字段，HTTP 响应体不变
      完成后 git tag T2-done

- [x] T3 internal/upstream/search.go + GET /api/search
      FetchSearch + ParseSearch，测试只测 ParseSearch
      验收：curl 'http://localhost:8080/api/search?q=斗破苍穹&page=1' 返回 10 条，
            book_id 是字符串，abstract 已 trim，has_more / next_offset 有值
            q 为空时返回 40001
      样本：q=斗破苍穹

- [x] T4 internal/upstream/detail.go + GET /api/detail
      FetchDetail + ParseDetail
      验收：book_id=7579985241087937560 返回 7 个字段齐全，word_number 是数字
      样本：7579985241087937560（正常）、7645235352245667856（书不存在 → 502）

- [ ] T5 internal/upstream/catalog.go + GET /api/catalog
      FetchCatalog + ParseCatalog
      验收：book_id=7579985241087937560 返回 58 章，volume 是空串，
            chapters 顺序与 upstrTeam 的 allItemIds 一致
      样本：7579985241087937560（58 章）、7542602438201576472（499 章）、
            7637198840910580798（详情有目录空 → total: 0）

- [ ] T6 internal/upstream/content.go + GET /api/content
      FetchContent + ParseContent（Tokenizer 白名单过滤）
      验收：四个样本全部符合预期，详见 docs/SAMPLES.md
      完成后 git tag T6-done

## v0.2
静态文件服务、阅读端前端、管理端、缓存