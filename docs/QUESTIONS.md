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
