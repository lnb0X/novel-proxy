# BACKLOG

> 未勾选的项按执行顺序排列。工作循环取最上面一个未勾选的，所以顺序即执行顺序。

## v0.2 管理端与基础设施

- [x] W1 配置分层
      config.yaml 基础结构 + data/instance.yaml 覆盖 + env 覆盖
      KnownFields 严格模式，enum 非法值报错退出
      确认 .gitignore 含 data/
      启动日志打印实际监听地址与实际使用的前端目录
      验收：
        instance.yaml 覆盖 config.yaml 生效
        APP_MODE 覆盖两者
        未知键启动报错，enum 非法值启动报错
        git status 里看不到 data/ 下的任何文件
      完成后 git tag W1-done

- [x] W2 ArticleStore + SQLite + content 接缓存
      store 接口 + SQL 实现（先只接 sqlite）+ articles 表
      content：查库 → 未命中抓上游 → 剥 img → 入库 → 返回
      验收：
        同一章第二次请求不打上游，返回纯文字
        sqlite 里 content 不含 img 标签
        store 用内存库测试通过
      完成后 git tag W2-done

- [x] W3 init 向导（按 Q1 裁决：init 是 frontends 第三个 app）
      令牌门禁（生成 / 打印 / 写文件 / 复用 / 常量时间比较 / 失败限流）
      四步流程 + 内存草稿 + confirm 一次性写盘
      完成前任何路径 302 到 /init/（/api/* 返回 503 + 50302）
      完成后 /init/* 全部 404
      init 页面：web-init/ 独立 Vite + React + TS 应用，base '/init/'，
      产物放 data/frontends/init/default/（或版本目录 + init-current.json），
      重启生效；data/frontends/init/ 缺失时 /init/ 返回 Go 固定说明页
      验收：
        未初始化时 /api/search 返回 50302
        / 302 到 /init/
        错误令牌 5 次后锁定
        confirm 之后 data/instance.yaml 出现且 setup.completed 为 true
        重启后令牌不变
      完成后 git tag W3-done

- [x] W4 静态服务
      可换 handler（atomic 持有 http.Handler）
      路径解析：-static > 指针文件 > default/ > 404
      目录由 internal/frontend 的 manager 提供，不要在 handler 里写死路径
      固定 404 页面；/api/* 的未知路径返回 JSON 404
      验收：
        -static 指向临时目录时能正常返回文件
        不带 -static 且无指针时返回 404，日志说明原因
        修改指针文件并触发重新加载后目录切换生效
      完成后 git tag W4-done

- [x] W5 JWT 认证
      内存 key（atomic.Pointer）、签发、校验、滑动续期、绝对上限
      login / logout / me / revoke
      登录失败限流
      管理端写操作校验 X-Admin-Request
      验收：
        Set-Cookie 含 HttpOnly 和 SameSite=Strict
        剩余不足 10 分钟时响应带新 cookie
        sat 过期后 401
        密码错 5 次后返回 429
        revoke 之后旧 token 立即 401
        重启后旧 token 返回 401
      完成后 git tag W5-done

- [x] W6 接口开关 + 限速
      四个开关 + 热更 + 管理接口
      令牌桶（global / per_ip / off），per_ip 回收和上限
      验收：
        关闭 search 后返回 503 + 50301，重启后状态保持
        per_minute 设为 2 时第 3 次返回 429 带 Retry-After
        per_ip 模式下两个不同来源互不影响
      完成后 git tag W6-done

- [x] W7 IP 过滤
      off / allow / block + scope + CIDR
      /init/* 豁免，API 403，管理端 404
      保存时锁死检查
      验收：
        allow 模式下非名单 IP 访问 /api/search 返回 403 + 40301
        同一 IP 访问管理端路径返回 404
        /init/ 不受影响
        把自己排除在外的保存被拒绝
      完成后 git tag W7-done

- [x] W8 统计与审计
      request_log 与 audit_log 两张表 + 写入钩子 + 聚合查询
      概览、查询、审计三组接口
      验收：
        发 10 次请求后概览计数为 10
        按时间范围筛选结果正确
        记录一次登录和一次开关变更后，审计接口能查到两条
      完成后 git tag W8-done

- [x] W9 内容规则
      全局 + 按书，prepend / append / replace / override
      渲染时拼接，不入库
      预览接口
      验收：
        全局 prepend 对所有书生效
        某书设为空字符串后抑制全局 prepend
        override 命中时不请求上游且日志有 content.overridden
        replace 顺序不同结果不同
        预览的 applied 带命中次数
      完成后 git tag W9-done

- [x] W11 前端版本管理
      internal/frontend：版本目录、指针读写、上传解压、切换、回滚
      五个管理接口
      验收：
        上传一个合法 zip 后出现新版本目录
        上传缺少 index.html 的 zip 返回 40001
        含 ../ 路径的 zip 被拒绝且不留下文件
        activate 后未确认时自动回滚
        删除版本后目录消失
      完成后 git tag W11-done

- [x] W12 管理端骨架与登录
      Vite + React + TS + AntD，Hash 路由，base './'
      布局、侧边栏分组、认证拦截、401 跳登录
      /login 登录页
      / 概览页
      完成后在 375px 和 1280px 各截一张图存 tmp/shots/，
      对照 AGENTS.md 视觉红线自查
      验收：登录后能看到概览数据，登出后访问任何页都跳登录
      完成后 git tag W12-done

- [x] W14 补三个后端缺口
      1. GET/PUT /config/log
         - logx 增加 SetLevel，内部用 slog.LevelVar，立即生效
         - GET 返回 { level, max_size_mb, max_backups, max_age_days, compress }
         - PUT：level 立即生效；其余四项写回 data/instance.yaml，
           返回 msg 说明「其余四项重启后生效」
         - 理由：lumberjack 的字段在它自己的互斥锁里读，外部改是数据竞争，
           而锁不导出；重建 writer 要换掉 slog handler，代价大于收益
         - DECISIONS 把日志滚动四项从「可热更」移到「需重启」
      2. GET/PUT /config/upstream
         - GET 返回 { timeout_seconds, max_concurrency }
         - PUT 写回 instance.yaml，两项都立即生效
      3. 让 max_concurrency 真正生效
         - upstream 包加全局信号量，包住四个 FetchXxx 的 HTTP 调用
         - 用 golang.org/x/sync/semaphore.Weighted（第一层依赖），
           它支持 SetLimit，运行中改并发数不用重建
         - 获取时带 context，超时按 upstream.timeout_seconds 算
      4. 埋 config_change 审计事件（DECISIONS 列了 15 类，实际只有 14 类）
      5. DECISIONS 明写「文章缓存永不失效，/cache 的 ttl_left_seconds
         恒为 0」，或把该字段从 CONTRACTS 删掉，二选一写进文档
      验收：
        四个 config 端点 curl 通
        PUT level=debug 后立即可见 debug 日志，不需要重启
        PUT max_size_mb 返回「重启后生效」的 msg
        max_concurrency 设为 1，并发发 5 个 catalog，
        日志里 upstream.response 的时间戳串行错开、不重叠
        审计能查到 config_change
      完成后 git tag W14-done

- [x] W13a 管理端七个轻页面
      /flags          四个开关，切换带二次确认
      /rate-limits    四行 × (模式 / 每分钟 / 突发)
      /access         模式 + 范围 + 名单增删，含锁死提示
      /account        只读：用户名、权限、会话起止
      /account/password 旧密码 + 新密码 + 确认
      /config/log     五项，level 与其余四项要标明生效方式
      /config/upstream  timeout 与 max_concurrency
      每个页面：独立路由，不得用 Tab / Modal / Drawer 承载另一页内容；
      完成后在 375px 和 1280px 各截一张图存 tmp/shots/，
      对照 AGENTS.md 视觉红线自查
      验收：
        每页能完成 CONTRACTS 第 7 节对应的全部操作
        表单校验失败有明确提示，不静默
      完成后 git tag W13a-done

- [x] W13b 管理端四个重页面
      /stats   时间范围筛选、请求量趋势、P50/P95/P99、错误码分布、
               热门搜索词与热门书
      /cache   按 book_id / chapter_id 搜索、分页、单条删、按书清空、
               全清（二次确认 + 手输 DELETE）
      /rules   全局三块 + 按书配置、replace 列表可上下移动、预览对比
      /audit   表格 + 按事件类型与时间筛选 + 分页
      验收：
        /rules 调顺序后保存，预览结果随之变化
        /cache 全清必须手输 DELETE 才放行
        /audit 能筛出 login_success 与 flag_change
        四页双宽度截图自查
      完成后 git tag W13b-done

- [ ] W10 三个数据库驱动
      DSN 见 docs/QUESTIONS.md 的 Q5
      1. dialect.go 增加 postgresDialect 与 mysqlDialect
         - TEXT vs LONGTEXT、TIMESTAMPTZ vs DATETIME
         - 占位符 $1/$2 vs ?
         - 建表与建索引都必须幂等
      2. sql.go 的 Open() 增加
         case "postgres"（驱动名 pgx）与 case "mysql"
      3. 新建 mongo.go，实现 ArticleStore 的九个方法
         - collection articles，唯一索引 {item_id}，普通索引 {book_id}
         - SumSize 用 $group + $sum 聚合
         - Init 幂等：collection 已存在不算错，索引已存在也算正常
      4. 新依赖（均第二层，commit message 写 dep: 行）
         github.com/jackc/pgx/v5
         github.com/go-sql-driver/mysql
         go.mongodb.org/mongo-driver
      5. 注意 sqlite 的 _pragma 拼接只对 sqlite 生效，
         不要套到 postgres / mysql 上
      验收（每个驱动各跑一遍，用 live，不进 go test）：
        Init → Put 两条 → Get 比对内容 → Count 与 SumSize
        → Delete 一条 → DeleteByBook → DeleteAll 归零
        10 个 goroutine 并发 Put 不报错且 Count 正确
      单测只测方言 DDL 的字符串输出，纯字符串断言，不连库
      某个实例不可达时，把实际输出写进 QUESTIONS 标为环境不可达，
      不要反复重试
      完成后 git tag W10-done
