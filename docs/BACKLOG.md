# BACKLOG

> 未勾选的项按执行顺序排列。

## v0.3 阅读端

- [ ] R1 阅读端骨架
      新建 web-reader/：Vite + React + TS，纯 CSS，不装任何组件库
      三条路由：/、/books/:bookId、/books/:bookId/read/:chapterId
      CSS 变量层（唯一配色与字号来源）：
        字号 16 / 20 / 24 / 28 / 32
        行高 1.85，正文最大宽 38em，段间距 0.8em，不做首行缩进
        三套主题：
          day   bg #ffffff fg #1a1a1a muted #8a8a8a line #e8e8e8
          sepia bg #f5f0e6 fg #3a3226 muted #8a8172 line #e0d8c8
          night bg #1a1a1a fg #c8c8c8 muted #6a6a6a line #2a2a2a
        字体两种：系统无衬线 / 系统衬线，不加载 webfont
      骨架屏组件、错误重试组件、localStorage 读写封装
      .gitignore 补 web-reader/dist、web-reader/node_modules，
      并检查 web-init 的两条是否已在
      验收：
        三页之间能互相跳转
        data-theme 切三套配色都生效
        字号从 16 切到 32，正文列宽随之变化
        go run ./cmd/verify 通过（含 tsc 与 vite build）
      完成后 git tag R1-done

- [x] R2 搜索页
      输入框回车触发，q 与 page 同步到 URL（useSearchParams）
      结果卡片：封面 60x80 + 标题 / 作者 · 分类 / 摘要两行省略
      「加载更多」用接口返回的 next_offset
      三种状态分开：骨架屏、空结果、上游失败，文案必须不同
      验收：
        搜「斗破苍穹」出 10 条
        刷新后结果不丢（URL 带 q）
        断网时显示「搜索服务暂时不可用」，不是空列表
      完成后 git tag R2-done

- [x] R3 详情 + 目录
      并行请求 detail 与 catalog
      封面、标题、作者、字数、开始阅读、简介 5 行折叠 + 展开
      目录按 volume 分组，每页 200 章，?toc=N 直接跳页并高亮当前章
      「开始阅读」跳 chapters[0]
      跨页的卷头显示「（续）」
      验收：
        上千章的书能翻页
        从正文页点目录回来，定位到当前章所在页且高亮
        目录为空时显示「暂无目录」，不是空白
      完成后 git tag R3-done

- [x] R4 正文页
      并行请求 content 与 catalog（后者用于算上下章）
      顶栏：返回（固定跳 /books/:bookId，不用 history.back）、
            章节名、目录
      底栏：上一章、字号、夜间、下一章
      设置面板：字号五档、三主题、两种字体
      章末：本章字数（前端去标签后统计）、本章完、下一章按钮
      顶栏底栏自动隐藏：
        position:fixed + transform，不用 display:none
        滚动监听 passive + requestAnimationFrame 节流
        scrollY <= 8 或距底 <= 8 时强制显示
        向下滚超过 8px 隐藏，向上滚超过 8px 显示
        点击正文区切换显示，与自动隐藏并存
      章节切换时 scrollTo(0,0) 且强制显示菜单
      document.title 同步为「章节名 - 书名」
      图片 onerror 时隐藏，不显示裂图
      正文上下内边距永久保留，菜单显隐不得引起重排
      底栏高度计入 env(safe-area-inset-bottom)
      桌面端：方向键左右翻章、空格下翻一屏
      不做左右滑动翻章
      验收：
        带图章节图片正常显示，图挂了不出现裂图图标
        字号 16 与 32 都不破排版
        向下滚菜单收起，向上滚菜单出现，点击正文也能切换
        上一章 / 下一章 URL 与内容同步变化
        章节切换后位置在顶部且菜单可见
      完成后 git tag R4-done

- [x] R5 管理端前端管理页
      /frontends 页面，后端由 W11 完成
      列出 reader / admin 的当前版本与历史版本
      上传 zip、激活（10 秒确认浮层）、确认、回滚、删除
      验收：
        上传一个 zip 后列表出现新版本
        激活后 10 秒内不确认自动回滚，界面提示已恢复
        删除版本后列表里消失
      完成后 git tag R5-done

- [x] R6 部署与冒烟
      构建 web-reader → 打 zip → 通过 R5 页面上传并激活
      首版同时放到 data/frontends/reader/default/ 作为出厂默认
      写三条 Playwright 冒烟：
        页面能加载且标题正确
        正文图片 src 不以 data: 开头
        点下一章后 URL 里的 chapterId 变化
      验收：从零部署后浏览器能完整读一章
      完成后 git tag R6-done