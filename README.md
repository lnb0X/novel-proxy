# novel-proxy

自用小说中转站。Go 后端代理上游小说站的搜索、详情、目录与正文四个公开查询，
提供参数校验、正文白名单过滤、文章缓存、请求统计与审计，以及一套可整体替换
的前端托管机制。三个前端（阅读端 / 管理端 / 初始化向导）均为纯静态产物，
可独立开发、独立替换。

## 功能

- 四个公开查询接口：搜索、书籍详情、章节目录、章节正文
- 正文白名单过滤（只保留 p / br / img），入库时剥掉全部图片
- 文章缓存：sqlite / postgres / mysql / mongo 四种驱动
- 请求统计与审计日志（与文章缓存同库，未启用缓存时相关接口返回 50301）
- 内容拼接规则：全局与按书两级（prepend / append / replace / override）
- 管理端：接口开关、限速、IP 过滤、缓存管理、统计查询、日志与上游配置、
  修改密码、前端版本管理
- 前端版本托管：上传 zip、激活、10 秒确认窗口、自动回滚
- init 向导：首次启动在浏览器完成全部配置，确认后热挂载，无需重启
- 阅读端：日 / 棕 / 夜三主题，两种字体，字号五档，顶栏底栏自动隐藏，
  目录多列网格与正 / 倒序，章末导航

## 快速开始

构建前端（三个目录各一次；也可以直接使用已有的 dist 产物）：

```bash
cd web && npm install && npm run build && cd ..
cd web-init && npm install && npm run build && cd ..
cd web-reader && npm install && npm run build && cd ..
```

把产物部署到前端托管目录：

```bash
mkdir -p data/frontends
cp -r web/dist data/frontends/admin/default
cp -r web-init/dist data/frontends/init/default
cp -r web-reader/dist data/frontends/reader/default
```

构建并启动后端（默认监听 127.0.0.1:8080）：

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o app.exe ./cmd/server
./app.exe
```

浏览器打开 `http://127.0.0.1:8080/init/`，按四步向导完成初始化。
初始化令牌在启动日志与 `data/init-token.txt` 中；确认提交后立即生效，
无需重启。阅读端入口为根路径 `/`，管理端在初始化时设置的管理端路径下。

需要能访问上游小说站的公开接口；全部测试与运行数据都落在 `data/`
目录内，删除该目录即重置。

## 测试

```bash
go run ./cmd/verify          # vet + 单测 + 依赖检查 + 前端构建 + 文字规范
go run ./cmd/verify -race    # 竞态检测（测试子进程内开 CGO）
```

阅读端另有 Playwright 冒烟（需要本地服务在 8080 运行）：

```bash
cd web-reader && npx playwright test
```

单测全部使用写死的上游 JSON 样本，断网状态下可通过。

## 文档

- docs/API-HANDOFF.md —— 后端接口交接文档：面向前端开发者的接口事实
  （响应样本原样抓取自运行实例）
- docs/CONTRACTS.md —— 接口契约与日志事件
- docs/DECISIONS.md —— 设计决策记录
- docs/QUESTIONS.md —— 规格缺口与裁决记录
- docs/NOTES.md —— 实现细节备忘
- docs/SAMPLES.md —— 验收样本与预期

## 说明

- 仓库中的测试样例包含上游公开接口的少量真实数据片段，仅用于回归测试。
- 面向公众部署时请在初始化向导中设置足够强度的管理员密码，
  并按需配置 IP 过滤。
