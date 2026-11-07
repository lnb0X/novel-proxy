# BACKLOG

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

- [ ] W3 init 向导
      令牌门禁（生成 / 打印 / 写文件 / 复用 / 常量时间比较 / 失败限流）
      四步流程 + 内存草稿 + confirm 一次性写盘
      完成前任何路径 302 到 /init/（/api/* 返回 503 + 50302）
      完成后 /init/* 全部 404
      init 页面为 React 应用，固定 base /init/
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

- [ ] W5 JWT 认证
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

- [ ] W6 接口开关 + 限速
      四个开关 + 热更 + 管理接口
      令牌桶（global / per_ip / off），per_ip 回收和上限
      验收：
        关闭 search 后返回 503 + 50301，重启后状态保持
        per_minute 设为 2 时第 3 次返回 429 带 Retry-After
        per_ip 模式下两个不同来源互不影响
      完成后 git tag W6-done

- [ ] W7 IP 过滤
      off / allow / block + scope + CIDR
      /init/* 豁免，API 403，管理端 404
      保存时锁死检查
      验收：
        allow 模式下非名单 IP 访问 /api/search 返回 403 + 40301
        同一 IP 访问管理端路径返回 404
        /init/ 不受影响
        把自己排除在外的保存被拒绝
      完成后 git tag W7-done

- [ ] W8 统计与审计
      request_log 与 audit_log 两张表 + 写入钩子 + 聚合查询
      概览、查询、审计三组接口
      验收：
        发 10 次请求后概览计数为 10
        按时间范围筛选结果正确
        记录一次登录和一次开关变更后，审计接口能查到两条
      完成后 git tag W8-done

- [ ] W9 内容规则
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

- [ ] W10 postgres / mysql / mongo 三个驱动
      方言 DDL、连接测试、Init 幂等
      验收：三种库各走一遍 Init + Put + Get + Delete
      没有可用实例的驱动，写 QUESTIONS.md 后跳过
      完成后 git tag W10-done

- [ ] W11 前端版本管理
      internal/frontend：版本目录、指针读写、上传解压、切换、回滚
      五个管理接口
      验收：
        上传一个合法 zip 后出现新版本目录
        上传缺少 index.html 的 zip 返回 40001
        含 ../ 路径的 zip 被拒绝且不留下文件
        activate 后未确认时自动回滚
        删除版本后目录消失
      完成后 git tag W11-done

- [ ] W12 管理端骨架与登录
      Vite + React + TS + AntD，Hash 路由，base './'
      布局、侧边栏分组、认证拦截、401 跳登录
      /login 登录页
      / 概览页
      完成后在 375px 和 1280px 各截一张图存 tmp/shots/，
      对照 AGENTS.md 视觉红线自查
      验收：登录后能看到概览数据，登出后访问任何页都跳登录
      完成后 git tag W12-done

- [ ] W13 管理端其余十二个页面
      /flags /rate-limits /access /stats /cache /rules /frontends
      /config/log /config/upstream /account /account/password /audit
      每个页面完成后双宽度截图并自查
      验收：CONTRACTS 第 7 节的每个操作都能在界面上完成
      完成后 git tag W13-done

## v0.3
阅读端前端