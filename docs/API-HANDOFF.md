# novel-proxy 后端接口交接文档

面向为本服务定制前端的第三方开发者（人或 AI）。本文档只陈述服务端事实：
提供什么接口、返回什么数据、有什么行为边界；不涉及任何前端设计。

示例中的 JSON 全部原样抓取自 2026-11-14 运行中的服务实例
（版本字段见 stats/overview 样本），每一条都能用文内标注的 curl 复现。

## 一、这是什么

服务端是一个小说内容聚合代理：代理上游小说站的搜索、书籍详情、章节目录
与章节正文四个公开查询，提供响应规整、参数校验、正文白名单过滤、
文章缓存、请求统计与审计、以及一套可整体替换的前端托管机制。

三个可替换前端与角色：

- reader，挂载于 `/`，面向读者：搜索、详情、目录、正文阅读。
- admin，挂载于 `/<secret>`，面向站点管理员：运行配置、缓存、统计、
  审计、前端版本管理。`<secret>` 是初始化时设置的管理端路径段。
- init，挂载于 `/init`，只在服务尚未初始化时使用：设置管理员账号、
  管理端路径与存储。初始化完成后永久 404。

能力边界（事实）：

- 没有用户体系：只有一名管理员（初始化时设置），读者匿名。
- 没有阅读进度存储。
- 对外只读接口只有下文 3.1 的四个。
- 内容来自上游公开小说站接口，服务端不持有上游凭据。

## 二、三个前端与各自的部署方式

reader 与 admin 支持在管理端上传替换：

1. 构建纯静态产物，根目录必须有 index.html。
2. 打包 zip：压缩包不超过 50MB，解压后不超过 200MB。
3. 登录管理端，在前端管理页选择 reader 或 admin，上传并激活。
4. 激活后 10 秒内必须调用确认接口，否则服务端自动回滚到上一版本。
5. 支持随时回滚或删除历史版本；当前版本与回滚点由服务端持久化。

免上传路径：

- 把产物放到服务端的 `data/frontends/<app>/default/` 目录后重启。
- 启动参数 `-static <目录>` 直接指定 reader 的产物目录，优先级最高，
  仅对 reader 生效。

init 不能通过界面上传：只能在未初始化状态下把产物放到
`data/frontends/init/default/` 后重启。通常不需要替换它。

路由注意事项：

- reader 在根路径，负责 `/` 下所有页面路由；admin 在自己的路径前缀下。
- reader 对未知路径的回落规则：路径不含点号时回落 index.html，
  History 与 Hash 路由都能用；路径含点号时按静态资源处理，未命中
  返回空 body 的 404。因此页面路由段不要带点号。
- admin 的服务端不做回落：未知页面路径直接 404。admin 前端的页面
  路由需要放在 `#` 之后（Hash 路由），服务端只会看到 `/<secret>/`。
- `/api/` 与 `/<secret>/api/` 开头的请求永远归后端，前端不会收到。
- 所有静态文件未命中都是空 body 的 404。

安全提示（事实陈述）：上传自定义 admin 前端存在把管理端改到
打不开的风险。恢复手段：删除服务端的
`data/frontends/admin-current.json` 并重启，会回落到上一版本。

## 三、接口契约

所有响应都是同一封装：

```json
{"code":0,"msg":"","trace_id":"01M7CHECNW3YSX4T2XA1DRJGK6","data":{}}
```

code 为 0 表示成功；非 0 见第五节。每个响应都带 trace_id，
同一请求的所有日志共用它。响应 JSON 中 `<`、`>`、`&` 会被转义为
`\u003c`、`\u003e`、`\u0026`（标准 JSON 转义，解析后即原字符）。

### 3.1 公开接口（无鉴权，四个，均为 GET）

#### GET /api/search

参数：

| 参数 | 必填 | 说明 |
|---|---|---|
| q | 是 | 搜索词；空或全空白返回 40001 |
| page | 否 | 从 1 开始；缺省按 1 处理 |

```bash
curl "http://127.0.0.1:8080/api/search?q=%E6%96%97%E7%A0%B4&page=1"
```

```json
{"code":0,"msg":"","trace_id":"01M7CHEAMWMFKRC7VA8B5YJRD0","data":{"items":[{"book_id":"7579450348599790616","title":"炎狱仙尊","author":"爱吃豆腐叶的李隐蔽","category":"东方仙侠","abstract":"火焰谷弟子{炎辰}天生身怀狂暴的炎狱灵体，能驭九天玄火，却因力量难以掌控，被谷中长老视为宗门隐患。一次灵体暴走，他被打入凶险万分的炎狱秘境，意外觉醒上古火帝残魂，获传至高功法《炎狱火典》，自此掌控了体内肆虐的力量。在秘境深处，炎辰发现了能压制魔界邪火的炎狱火种，也得知魔界正欲借邪火入侵修仙界，而火焰谷的叛徒长老早已通魔，妄图夺取火种开启两界通道。为守护苍生，炎辰拼死逃出秘境，集结宗门忠诚弟子与正道势力，重返炎狱秘境展开终极之战。最终他融合炎狱灵体与火种，催动《炎狱火典》焚尽邪火，封印两界通道，斩杀叛徒长老，登顶火焰谷谷主之位，更突破至仙尊境，被三界尊为“炎狱仙尊”。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2oe72c627cb1af5ef1091a47ccb535e1eb~tplv-shrink:640:0.image"},{"book_id":"7338338730211822654","title":"开局斗破当配角","author":"关长卿","category":"男频衍生","abstract":"穿越到了斗破的世界，给我绑定了最强配角系统。 觉醒了混沌焱炎体，混沌火作为伴生火焰，能够吞噬异火本源，完美复制天下异火。 辛苦修炼大半年，还不如系统一个任务奖励丰富以后，萧凡的斗破之旅，画风开始变了。 萧凡：师弟，看我创造的佛怒火莲！ 萧凡：莎莎，看我给你买的鞋子。 萧凡：小医仙，还记得星空下的约定吗？ 双帝之战，魂天帝身边的第一高手，脱下了自己的面具：族长对不起， 曾经没得选，现在我想做个好人！","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/f65d83155427e539f5a3816874a9bf7b~tplv-shrink:640:0.image"},{"book_id":"6542710247130139655","title":"绝世武神","author":"净无痕","category":"传统玄幻","abstract":"九霄大陆，武道为尊，弱者庸碌，受人欺凌，强者一怒，血流成河。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2o748da6e0628ab2afd8c54631f41c6c68~tplv-shrink:640:0.image"},{"book_id":"7373421185389825049","title":"穿越诸天，从斗气大陆开始","author":"斗破小迷弟","category":"男频衍生","abstract":"【诸天+同人+无敌+多女主】蓝星牛马林飞穿越异界，成为乌坦城萧家二长老的孙子萧烬，觉醒充值变强系统，从此开始无敌人生。\n萧火火：老大，十年了我终于突破斗圣了。\n萧烬：小老弟，不行啊，你的嫂子们都已经斗帝了，你还得练。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/213ab591edcb0e72937a86c07194a653~tplv-shrink:640:0.image"},{"book_id":"7639648524077894681","title":"斗气：我送萧炎一把屠龙刀","author":"凌寒小哥","category":"男频衍生","abstract":"萧炎这辈子最幸运的事，不是从母亲那里继承了药老，而是遇到了师父林锋。\n他老妈活得好好的，老爹也没被人抓走过，他身边一起长大的除了青梅竹马的薰儿，还多了一个小师姐——小医仙。\n等等，这个剧本怎么跟说好的不一样？\n\n纳兰嫣然来退婚？可以。\n但她带着云岚宗长老来以势压人？那不好意思，我师父说要去云岚宗串个门。\n云山：“阁下究竟是什么人？”\n林锋：“教你做人的人。”\n\n林锋带了一把倚天剑和一把屠龙刀来到斗气大陆，收了萧炎和小医仙做徒弟。\n既然为师是用剑的，那这把自己重新祭炼过的刀，就送给小徒弟吧。\n\n日更2章，每天早上7:00，7:01各一章。\n\nPS：不跟萧炎抢资源，也不跟萧炎抢女人，希望得到读者朋友的喜欢，不喜欢也没关系，请勿喷，祝大家都能看到自己喜欢的小说。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2oadc0b2a22b25c27774e492112fc1d6f9~tplv-shrink:640:0.image"},{"book_id":"6857384164207889421","title":"诸天万界无限融合系统","author":"芥末加葱","category":"动漫衍生","abstract":"（已写斗破、火影、海贼、一拳、斗罗、狐妖、超神、一人）一觉醒来，高凡穿越异界。获得系统，助他融合诸天。钓鱼的姜太公和电鱼的雷神索尔在河边争执，“这条银河该由我为陛下守护！”美杜莎和纲手在后宫打架，“今后的帝宫我才是皇后！”守门的白胡子、宇智波斑、琦玉三人在宫门前斗地主，“宇智波斑，你不准用瞳术作弊！”后花园织毛线的的女娲和天使彦一脸惆怅，“入宫都200年了，陛下何时才能临幸我俩？”","thumb_url":"https://p6-tt.bytecdn.cn/novel-images/a6842cab5989cef030977ce5581f9723~tplv-shrink:640:0.image"},{"book_id":"7429588038528814104","title":"斩神：炎帝萧炎代理人，开局焚诀","author":"招财小汪汪","category":"男频衍生","abstract":"【斩神+斗破+开局获得焚诀，无尽火域+斗技+异火+佛怒火莲+丹药+前期不无敌，中后期越阶杀敌】\n穿越斩神世界，别人觉醒精神力，我开局获得炎帝血脉，拥有无尽火域修炼斗气，别人用禁墟，我用斗技。\n我手搓佛怒火莲！\n“守夜人？不，我斩神用斗气。”  \n身负炎帝血脉，手握焚诀吞噬万千“神秘”。\n古神教会视他为渎神天灾，境外神明惧他作焚世灾星。  \n而他玄重尺一横，异火焚天！\n“神明？不过是异火的养料罢了，境外诸神……皆为我火下亡魂！”\n多种异火融成火莲，瞬杀邪神！\n林七夜瞳孔骤缩：“你管这叫代理人技能？！”  \n二十三异火焚穿奥林匹斯，全球神系在他脚下哀嚎！  \n当至高神降临大夏，他屈指成莲，身后火海化作四个焚天大字：\n——无尽火域！！\n“大夏神明由我守护，境外诸神？唯死而已！”  \n“精神力修炼体系？抱歉，我斗气大陆的功法……专吞神明！”  \n斩神世界唯一真神？不，我即炎帝！！","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2oa8095026ea0a76a926756d7d74c6a4ff~tplv-shrink:640:0.image"},{"book_id":"7666036464840363032","title":"我有一个修为返还系统","author":"因果与共","category":"男频衍生","abstract":"这是小作者第一本斗破同人文，全程不保姆也不窝囊，有爽点，女主不收配角和萧薰儿。\n小作者在这感谢各位读者支持，也希望读者大大们能够评论给出建议，小作者在此跪谢。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2off3aa134bffee351998b3c3526d6bd62~tplv-shrink:640:0.image"},{"book_id":"7628179050665610265","title":"开局绑定炼药系统，萧炎慌了","author":"星野宗介","category":"男频衍生","abstract":"我叫萧尘，穿越成了萧炎的哥哥。\n\n我弟是未来的炎帝。\n我绑定了炼药系统。\n\n他抢异火，我用来炼丹。他打架，我提供丹药。他受伤，我帮他疗伤。\n\n本来我们是乌坦城最好的兄弟。\n\n直到小医仙敲开了我的门。\n然后是云韵。美杜莎。薰儿。\n\n萧炎终于忍不住了：“哥，你是不是故意的？”\n\n我说：“各凭本事。”","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/p2o49e3d1876778c6ca362e1e6d06ede9dc~tplv-shrink:640:0.image"},{"book_id":"7556555977751792702","title":"穿越成斗气世界萧炎","author":"十年有鱼","category":"动漫衍生","abstract":"大千世界的主宰萧炎，在一次突破瓶颈时遭遇时空乱流，竟被卷入回到斗气大陆的少年时代——恰是他与纳兰嫣然三年之约将满的前夕。\n此刻的他，斗气修为跌落至少年水准，昔日纵横大千的无上威能被封印，唯有那颗历经万载沉浮的主宰之心依旧璀璨。重回云岚宗下，面对曾经的轻视与即将到来的挑战，萧炎眼中再无少年的青涩与愤懑，唯有洞悉全局的从容。\n他知晓未来的每一处机遇与陷阱，熟悉每一位伙伴的命运轨迹。这一次，他不仅要赢下三年之约，更要弥补前世的遗憾：护薰儿周全，助药老早登巅峰，让萧家真正屹立于大陆之巅。当主宰级的智慧与布局融入少年的身躯，斗气大陆的命运齿轮，已悄然转向一个更辉煌的未来。","thumb_url":"https://p6-tt.bytecdn.cn/novel-pic/a7e24620daae1a8972a04d7881a21691~tplv-shrink:640:0.image"}],"page":1,"has_more":true,"next_offset":10}}
```

错误：q 为空或全空白 → 40001「参数 q 必填」。

#### GET /api/detail

| 参数 | 必填 | 说明 |
|---|---|---|
| book_id | 是 | 1 到 20 位数字字符串 |

```bash
curl "http://127.0.0.1:8080/api/detail?book_id=7579985241087937560"
```

```json
{"code":0,"msg":"","trace_id":"01M7CHEBQR45WQG5W81NGFVPQ7","data":{"book_id":"7579985241087937560","title":"天降萌娃，我的老婆是美女总裁","author":"山海星辰","word_number":0,"last_chapter":"7579988207375240217","intro":"世界至高神殿天龙殿殿主林阳，为寻回失散多年的妹妹，选择隐姓埋名化身普通代驾。一次偶然的机会，他撞见被人下药的唐氏总裁唐清曼，出手将其救下。时光匆匆过了六年，唐清曼带着一个可爱的女儿找到林阳，见面后便立刻告知他这是两人的孩子，还满心期待地向林阳提出，想要再添一个二胎的想法。","thumb_url":"https://p9-novelfm-sign.novelfmpic.com/novel-pic/60975d45bc83be4909d893e72d260711~tplv-y3bzr8ilui-shortplay_comic_v3:600:840.jpeg?lk3s=bfb7c328\u0026scene=book_page_shortplay_comic_v3\u0026x-expires=1795248338\u0026x-signature=y4aN2Ub7AmgW3Yr%2BlHgSmkBF5NY%3D"}}
```

错误：book_id 非法 → 40001；上游无此书 → 502 + 50203（样本见第六节）。

#### GET /api/catalog

参数同 detail 的 book_id。返回完整章节数组，可能为空。

```bash
curl "http://127.0.0.1:8080/api/catalog?book_id=7637198840910580798"
```

```json
{"code":0,"msg":"","trace_id":"01M7CHEC1S1D1B98YDVD9DG67M","data":{"book_id":"7637198840910580798","total":0,"chapters":[]}}
```

有章节时的 chapters[] 元素结构（58 章书的完整响应，7143 字节，
对应 `curl "http://127.0.0.1:8080/api/catalog?book_id=7579985241087937560"`）：

```json
{"code":0,"msg":"","trace_id":"01M7CHECD39D64RA8AR0E6CW3G","data":{"book_id":"7579985241087937560","total":58,"chapters":[{"chapter_id":"7579988172734467097","title":"第1集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853574},{"chapter_id":"7579988190329588761","title":"第2集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853569},{"chapter_id":"7579988114697899033","title":"第3集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853544},{"chapter_id":"7579988149766474777","title":"第4集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853560},{"chapter_id":"7579988078782073881","title":"第5集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853547},{"chapter_id":"7579988120611867673","title":"第6集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853558},{"chapter_id":"7579988175615970329","title":"第7集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988196948200473","title":"第8集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988089607556121","title":"第9集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853536},{"chapter_id":"7579988192363809817","title":"第10集","volume":"","is_vip":false,"is_pay":false,"updated_at":1764853553},{"chapter_id":"7579988084029131801","title":"第11集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853529},{"chapter_id":"7579988183098592281","title":"第12集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853553},{"chapter_id":"7579988142963313689","title":"第13集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988216996957209","title":"第14集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853567},{"chapter_id":"7579988180145802265","title":"第15集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988219115097113","title":"第16集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853562},{"chapter_id":"7579988199406062617","title":"第17集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988117340294169","title":"第18集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853539},{"chapter_id":"7579988127989648409","title":"第19集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853539},{"chapter_id":"7579988081147644953","title":"第20集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853545},{"chapter_id":"7579988170578611225","title":"第21集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853563},{"chapter_id":"7579988202203663385","title":"第22集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853566},{"chapter_id":"7579988145077226521","title":"第23集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853565},{"chapter_id":"7579988107085220889","title":"第24集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853547},{"chapter_id":"7579988178094787609","title":"第25集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853571},{"chapter_id":"7579988100588243993","title":"第26集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853592},{"chapter_id":"7579988109488557081","title":"第27集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853551},{"chapter_id":"7579988125141699609","title":"第28集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853549},{"chapter_id":"7579988105168440345","title":"第29集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853533},{"chapter_id":"7579988122818055193","title":"第30集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853677},{"chapter_id":"7579988165448977433","title":"第31集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853567},{"chapter_id":"7579988140929076249","title":"第32集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853577},{"chapter_id":"7579988156896775193","title":"第33集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853585},{"chapter_id":"7579988209463987225","title":"第34集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853612},{"chapter_id":"7579988130271349785","title":"第35集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853573},{"chapter_id":"7579988095278255129","title":"第36集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853598},{"chapter_id":"7579988161082706969","title":"第37集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853564},{"chapter_id":"7579988092812020761","title":"第38集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853540},{"chapter_id":"7579988168473054233","title":"第39集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853562},{"chapter_id":"7579988154241780761","title":"第40集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853565},{"chapter_id":"7579988211796020249","title":"第41集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853579},{"chapter_id":"7579988087527181337","title":"第42集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853544},{"chapter_id":"7579988185292229657","title":"第43集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853552},{"chapter_id":"7579988102949653529","title":"第44集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853539},{"chapter_id":"7579988204510530585","title":"第45集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853561},{"chapter_id":"7579988076269685785","title":"第46集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853567},{"chapter_id":"7579988137783331865","title":"第47集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853558},{"chapter_id":"7579988111791229977","title":"第48集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853563},{"chapter_id":"7579988158821977113","title":"第49集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853540},{"chapter_id":"7579988163154693145","title":"第50集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853571},{"chapter_id":"7579988135472286745","title":"第51集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853568},{"chapter_id":"7579988188140145689","title":"第52集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853586},{"chapter_id":"7579988147652529177","title":"第53集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853567},{"chapter_id":"7579988152111074329","title":"第54集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853545},{"chapter_id":"7579988098273004569","title":"第55集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853546},{"chapter_id":"7579988214279048217","title":"第56集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853557},{"chapter_id":"7579988132574022681","title":"第57集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853548},{"chapter_id":"7579988207375240217","title":"第58集","volume":"","is_vip":true,"is_pay":false,"updated_at":1764853638}]}}
```

#### GET /api/content

| 参数 | 必填 | 说明 |
|---|---|---|
| book_id | 是 | 只做格式校验并回填，不参与上游请求 |
| chapter_id | 是 | 1 到 20 位数字字符串 |

```bash
curl "http://127.0.0.1:8080/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"
```

```json
{"code":0,"msg":"","trace_id":"01M7CHECNW3YSX4T2XA1DRJGK6","data":{"book_id":"7526452300462705689","chapter_id":"7526766660229808665","title":"第1章 重生","content":"\u003cp\u003e“蝼蚁，吾为天，见吾，如何不跪！”\u003c/p\u003e\u003cp\u003e无尽魂域中，天道降临，俯视魂域众生\u003c/p\u003e\u003cp\u003e“不是，我没听错吧，背后顶个光，就自称为天了，那我岂不是天祖宗？”魂天帝无情的嘲笑着。\u003c/p\u003e\u003cp\u003e天道见此，摇了摇头，叹道：“吾来此，只为送你们魂族一场机缘。”\u003c/p\u003e\u003cp\u003e说罢，便不等众人反应，手指一点，一股强大的毁灭力量从指尖迸发而出，直击魂天帝。\u003c/p\u003e\u003cp\u003e魂天帝虽有防备，但还是不敌，半跪在地上，吐了一大口血。\u003c/p\u003e\u003cp\u003e那一股毁灭力量化成一束激光，贯穿了魂天帝的身体，同时也贯穿了整个魂域，似流星一般，带着毁灭的力量穿透过去。\u003c/p\u003e\u003cp\u003e众人见此，不敢上前。\u003c/p\u003e\u003cp\u003e‘竟如此之强’魂天帝想，他抬头看向天道，感觉异常无力。\u003c/p\u003e\u003cp\u003e“听着，天道无私，吾不管你是好是坏，但是这个世界容不下异世之人，你需要重新有一个身份，你可明白？”\u003c/p\u003e\u003cp\u003e听到这里，魂天帝哪还不明白，他本不是这个世界的人，听闻天道意思，无心杀他，但是他不敢赌。\u003c/p\u003e\u003cp\u003e“管你天道什么的，纵使你为天道，也不可能屠尽一方小世界，你可别忘了，我们这些强者也是有小天道之力加持，我们团结起来你未必是我族对手”一个头铁的人站了出来。\u003c/p\u003e\u003cp\u003e毕竟魂天帝是这方小世界的主心骨，他若死了，魂族就败落了，剩下的那些人未必不会卸磨杀驴。\u003c/p\u003e\u003cp\u003e他们也得罪了不少各方小世界的强者，若战，成功几率不大。\u003c/p\u003e\u003cp\u003e“这对我没有好处，循界多一方强者对于天道来说是有利的，只是，异世之魂，得死！”\u003c/p\u003e\u003cp\u003e说罢，天道不再废话，一掌拍出，铺天盖地的威压落下，同时滚滚雷云蓄势待发，直击魂天帝。\u003c/p\u003e\u003cp\u003e“此后，魂族宣布退隐，无尽魂域被封锁，百年之后，再也没了声息，有人怀疑，魂族众人早已被天道除掉，也有人怀疑，魂族在酝酿什么经天纬地的巨大阴谋，但真实情况，不得而知，成为各界强者心中的一个共同的谜团。”\u003c/p\u003e\u003cp\u003e风水仙界，一处名为水城的地方中，一家小饭馆生意异常火爆，正因其中的说书先生。\u003c/p\u003e\u003cp\u003e“先生，这是真正发生的事吗？”一个人问道。\u003c/p\u003e\u003cp\u003e那说书先生年过花甲，听完之后，抚了抚花白的胡须，眯着眼道：“其实真正情况我并不知道，只是幼时听闻长辈谈论过，思及过去，才偶尔拿出来讲讲”\u003c/p\u003e\u003cp\u003e饭馆边上靠窗的位子中，一名约莫有十五六岁的少年看着窗外，来来往往，车水马龙，不知在思考什么？\u003c/p\u003e\u003cp\u003e‘不知何时，已经过去了十五六年了吗？时间过得真快，想当年，一个闭关动辄就几百上千年，而如今，仅仅一年两载就如此漫长…’\u003c/p\u003e\u003cp\u003e这人正是魂天帝，16年前，魂天帝重生成婴儿，出生在一个小家族当中，说来也巧，这个家族也姓魂，要知道，无论过去了多长时间，魂族都等于禁忌的一个词汇。\u003c/p\u003e\u003cp\u003e然而在这样的情况下，魂家也能混的风生水起，虽说是一个小家族，但是在水城中也算是一股不小的势力。\u003c/p\u003e\u003cp\u003e此次外出，他准备重新踏上修行之路，体验够了凡人的生活，接下来的，就要回归了，不过在回归之前，还需要有足够的实力才对。\u003c/p\u003e\u003cp\u003e所以他现在唯一的目标，就是找到一所宗门加入，宗门不要太大，毕竟还是有一些老怪物认识他的，万一认出来，那就真的Shit了。\u003c/p\u003e\u003cp\u003e听着说书先生滔滔不绝的讲述，魂天帝不动声色的站了起来，走了出去。\u003c/p\u003e\u003cp\u003e风水宗，是风水仙界一不错的势力，既没有太弱小，也没有太强大，正好适合现在的魂天帝。\u003c/p\u003e\u003cp\u003e而这个宗门离水城也不是很远，走上一两个星期就能到。\u003c/p\u003e\u003cp\u003e而风水宗的招徒大典刚好也在两星期后举行，时间刚刚好。\u003c/p\u003e\u003cp\u003e路上，魂天帝特别选了山中小路走，毕竟魂天帝还真不习惯人多的地方。\u003c/p\u003e\u003cp\u003e走着走着，突然听到了一声虎啸，只见一名和他差不多大的少年向他奔来，身后跟着一头白毛老虎，魂天帝大感不妙，他现在可没有修为，若是硬刚，恐怕不到发育时间就shit了。\u003c/p\u003e\u003cp\u003e几乎没有思考时间，魂天帝拔腿就跑，而那少年也紧追着他不放。\u003c/p\u003e\u003cp\u003e二人跑到一处山洞洞口，那山洞洞口窄小，魂天帝一个眼尖，一个滑铲就滑到了洞里，而那少年也紧追其后，那白虎进不去，只是一直用爪子在那洞口抓着，在洞口外走来走去。\u003c/p\u003e\u003cp\u003e似是知道自己进不去，在洞口徘徊了几圈就走了。\u003c/p\u003e\u003cp\u003e二人见此，稍稍放松了下来。\u003c/p\u003e\u003cp\u003e魂天帝放松之后，看向那少年，生气的几乎想要让他挂起来滋滋放血，放出生魂喂狗。\u003c/p\u003e\u003cp\u003e而那少年也没啥好脸色，贱乎乎的模样。\u003c/p\u003e\u003cp\u003e“喂，你谁呀？我认识你吗？你就跟着我”魂天帝生气的说道。\u003c/p\u003e\u003cp\u003e那少年：“那咋了？你咋就知道我跟着你？别自恋了好吗？”\u003c/p\u003e\u003cp\u003e那人也累的气喘吁吁，而魂天帝见着感觉莫名有些眼熟，但始终想不起来她是谁。\u003c/p\u003e\u003cp\u003e“你大白天穿黑衣服，不知道的还以为你是魔道中人，话说，我们是不是在哪见过？”魂天帝问道，他行事一般都是直接刨根问底的。\u003c/p\u003e\u003cp\u003e少年闻此也反应了过来，仔细观察下魂天帝，摸着下巴思考不知是想到了什么，吓了一跳，结结巴巴道：“没，没有吧”\u003c/p\u003e\u003cp\u003eos:怎么那么像我家大人？\u003c/p\u003e\u003cp\u003e少年其实就是虚无吞炎，他是刚来这个世界，随机夺舍了个人，但是没有修为，又刚好被一头白虎盯上，跑的时候遇见了魂天帝。\u003c/p\u003e\u003cp\u003e本想卸磨杀驴的，但发现这是自家大人，不得不放下了那个念头。\u003c/p\u003e\u003cp\u003e魂天帝：“你结巴什么？”\u003c/p\u003e\u003cp\u003e“累，累的”说完，就假装很累的大口呼吸起来。\u003c/p\u003e\u003cp\u003e魂天帝将信将疑的“哦”了一声。\u003c/p\u003e\u003cp\u003e而虚无吞炎则是试探的问道：“道友，我还不知道你的名字，咋也算是生死一回了，算半个兄弟，你看这光叫道友多不礼貌，对吧？”\u003c/p\u003e\u003cp\u003e魂天帝闻此，翻了个白眼，嘲讽道：“别了吧，咱们素不相识，这位道友还是不要自恋好了，咱都不认识你”\u003c/p\u003e\u003cp\u003e能跟他魂天帝称兄道弟的，都去地底见阎王了。\u003c/p\u003e\u003cp\u003e“啊哈哈”虚无吞炎尴尬的笑了笑，道：“先前多有对不住，还望道友见谅哈，那我先介绍一下我自己，我叫虚无…言，无背景无门派无修为，道友你呢？”\u003c/p\u003e\u003cp\u003e魂天帝闻此，眼神中带了点探究的意味看向虚无吞炎。\u003c/p\u003e\u003cp\u003e“你的名字让我想到了一个故人”\u003c/p\u003e"}}
```

错误：章节无文本 → 404 + 40400「该章节无文本内容」；上游 404 同此；
book_id / chapter_id 非法 → 40001。

### 3.2 管理接口（需要 JWT）

鉴权流程：

1. `POST /<secret>/api/auth/login`，body `{"username":"...","password":"..."}`，
   且必须带请求头 `X-Admin-Request: 1`（所有非 GET 请求都要，登录也不例外；
   缺失时返回 40101「缺少 X-Admin-Request 请求头」）。
2. 成功返回 200，服务端通过 Set-Cookie 下发会话：

   ```
   Set-Cookie: token=<JWT>; Path=/<secret>; Max-Age=1800; HttpOnly; SameSite=Strict
   ```

   HTTPS 部署下另有 Secure 属性。响应体：

   ```json
   HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Sat, 14 Nov 2026 08:05:38 GMT
Content-Length: 127

{"code":0,"msg":"","trace_id":"01M7CHECS0P9JCJ9XQDZ8C04MX","data":{"expires_at":1794645338,"sat":1794686738,"username":"boss"}}
   ```

   expires_at 是本次 token 过期时间（签发后 30 分钟），sat 是绝对上限
   （首次登录后 12 小时），均为 Unix 秒。
3. 会话续期是自动的：token 剩余有效期不足 10 分钟时，任何已认证响应都会
   带一个新的 Set-Cookie。调用方不需要主动续期，但必须接收并保存响应里
   的最新 cookie。
4. 到达 12 小时绝对上限后必须重新登录。
5. 所有非 GET 请求必须带 `X-Admin-Request: 1`，缺失按未认证处理（40101）。
6. 登出（logout）与强制登出（revoke）都通过轮换服务端签名密钥实现，
   之后所有旧 token 立即失效；logout 额外清除 cookie（204，Set-Cookie
   的 Max-Age=0）。修改密码成功同样轮换密钥并清除 cookie。
7. 401 只有一种含义：无有效会话（未登录、过期或密钥已轮换），
   前端此时引导重新登录。

浏览器场景下 cookie 由浏览器自动携带；非浏览器调用方需要自行保存
Set-Cookie 并在后续请求回传 Cookie 头。

管理接口清单（31 个，全部在 `/<secret>/api/` 下）：

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/auth/login | 登录，见上 |
| POST | /api/auth/logout | 登出，204 并清 cookie |
| GET | /api/auth/me | 当前会话信息 |
| POST | /api/auth/revoke | 强制登出，轮换密钥，204 |
| POST | /api/account/password | body {old_password,new_password}；改后轮换密钥并清 cookie |
| GET | /api/ip-filter | 当前 IP 过滤配置 |
| PUT | /api/ip-filter | body {mode,scope,rules[]} 整体替换；mode=allow 且当前 IP 不在名单时拒绝 |
| GET | /api/frontends | 前端版本列表（reader/admin 各一组） |
| POST | /api/frontends/:app/upload | multipart 字段 file；app 只接受 reader/admin |
| POST | /api/frontends/:app/activate | body {version,confirm_within_seconds}；10 秒内未确认自动回滚 |
| POST | /api/frontends/:app/confirm | body {version}；确认切换 |
| POST | /api/frontends/:app/rollback | 回滚到上一版本 |
| DELETE | /api/frontends/:app/:version | 删除历史版本，204 |
| GET | /api/flags | 四个接口的开关状态 |
| PUT | /api/flags/:name | body {enabled}；关闭后对应公开接口返回 503 + 50301 |
| GET | /api/rate-limits | 四个接口的限速配置 |
| PUT | /api/rate-limits | body 同 GET 的 data，整体替换 |
| GET | /api/stats/overview | 今日请求、错误率、p95、缓存命中率、上游健康 |
| GET | /api/stats/query | ?from=&to=&api= 时间段聚合 |
| GET | /api/audit | 审计日志，分页 |
| GET | /api/cache | 文章缓存列表，?book_id=&chapter_id=&page=&size= |
| DELETE | /api/cache/:item_id | 删除单条，204 |
| DELETE | /api/cache?book_id= | 按书删除，返回 {deleted:n} |
| DELETE | /api/cache/all | body {confirm:"DELETE"}，全清 |
| GET | /api/content-rules | 内容拼接规则（全局 + 按书） |
| PUT | /api/content-rules | 整体替换 |
| POST | /api/content-rules/preview | body {book_id,chapter_id} → {original,result,applied} |
| GET | /api/config/log | 日志级别与滚动参数 |
| PUT | /api/config/log | 立即生效 |
| GET | /api/config/upstream | 上游超时与并发上限 |
| PUT | /api/config/upstream | body {timeout_seconds,max_concurrency} |

当 storage.enabled 为 false 时，stats / audit / cache 三组接口一律返回
503 + 50301「统计与审计需要开启文章缓存」。

GET 样本（对应 `GET /<secret>/api/auth/me`，带 cookie 与
`X-Admin-Request: 1`）：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8SR8KM3VD8CGZ0S42WG","data":{"exp":1794645338,"perm":"admin","sat":1794686738,"username":"boss"}}
```

对应 `GET /api/flags`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8TM9CDC76DW4ZQPR5BN","data":{"catalog":true,"content":true,"detail":true,"search":true}}
```

对应 `GET /api/rate-limits`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8VG3PB4JR438WFYG52B","data":{"search":{"mode":"off","per_minute":0,"burst":0},"detail":{"mode":"off","per_minute":0,"burst":0},"catalog":{"mode":"off","per_minute":0,"burst":0},"content":{"mode":"off","per_minute":0,"burst":0}}}
```

对应 `GET /api/ip-filter`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8WB5P8JC89MATTT3T6C","data":{"mode":"off","rules":[],"scope":"all"}}
```

对应 `GET /api/stats/overview`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8XAR6P74XH6ZA9JWC1X","data":{"today_requests":12,"today_errors":0,"p95_ms":1046,"cache_hit_rate":0,"per_api":{"catalog":{"errors":0,"requests":2},"content":{"errors":0,"requests":3},"detail":{"errors":0,"requests":1},"other":{"errors":0,"requests":5},"search":{"errors":0,"requests":1}},"upstream_health":"ok","recent_errors":[],"version":"v0.3.2-0.20261114071752-13a234008e49","uptime_seconds":50}}
```

对应 `GET /api/stats/query?api=search`（时间段内无数据时 buckets 为空数组）：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8Y7JAGA4TTK3S75D4MK","data":{"buckets":[],"p50":0,"p95":0,"p99":0,"error_codes":{},"top_queries":[],"top_books":[]}}
```

对应 `GET /api/audit?page=1&size=5`（登录动作会产生一条 login_success）：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8Z3YMTKZYHAGDVHTFKJ","data":{"total":1,"items":[{"ts":"2026-11-14T08:05:38.8858024Z","event":"login_success","actor":"boss","ip":"127.0.0.1","ua":"curl/8.12.1","detail":"","result":"ok"}]}}
```

对应 `GET /api/cache?page=1&size=5`（缓存过两章正文）：

```json
{"code":0,"msg":"","trace_id":"01M7CHF8ZZB3N8VRWWET90WRVG","data":{"total":2,"total_bytes":14392,"hit_rate":0,"items":[{"item_id":"7173216089122439711","book_id":"7526452300462705689","title":"第1章 空屋","size_bytes":7502,"created_at":1794643520,"ttl_left_seconds":0},{"item_id":"7526766660229808665","book_id":"7526452300462705689","title":"第1章 重生","size_bytes":6890,"created_at":1794643519,"ttl_left_seconds":0}]}}
```

对应 `GET /api/frontends`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF94BHR3CGHJ5ASXSZ60N","data":{"admin":{"current":"default","previous":"20261107231216-0ce8","versions":[{"version":"20261107231216-0ce8","size":843240,"files":2,"created_at":1794643516},{"version":"20261107232936-0ef3","size":1205697,"files":2,"created_at":1794643516}]},"reader":{"current":"default","previous":"20261108093902-aba8","versions":[]}}}
```

对应 `GET /api/config/log`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF92KGP064QWCDKB1FS1Q","data":{"compress":true,"level":"info","max_age_days":14,"max_backups":14,"max_size_mb":50}}
```

对应 `GET /api/config/upstream`：

```json
{"code":0,"msg":"","trace_id":"01M7CHF93FQCEXBRT1BN8E6BQF","data":{"max_concurrency":8,"timeout_seconds":10}}
```

对应 `PUT /api/flags/search` body `{"enabled":false}`（响应含被改的开关名）：

```json
{"code":0,"msg":"","trace_id":"01M7CHNGAVE4N0081JVHGN9V7V","data":{"enabled":false,"name":"search"}}
```

### 3.3 初始化接口（只在未初始化时可用）

这些接口存在的前提是服务尚未初始化（setup.completed 为 false）。
流程固定为：令牌 → 账号 → 存储 → 确认。全部流程状态保存在服务端内存，
确认之前磁盘上没有任何写入。存储确认失败时返回 40001 并保持向导可用，
此时同样没有磁盘写入。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /init/api/state | {completed, has_token} |
| POST | /init/api/token | body {token}；成功下发 init_session cookie；同 IP 5 次失败锁 15 分钟 |
| POST | /init/api/account | body {username,password,admin_path} |
| POST | /init/api/storage | body {enabled} |
| POST | /init/api/database/test | body {driver,dsn} → {ok,message}；只做连通性检查，不建表 |
| POST | /init/api/database | body {driver,dsn} |
| POST | /init/api/confirm | 依内存草稿写盘并热挂载，返回 {admin_path} |
| POST | /init/api/cancel | 清空服务端内存草稿 |

初始化完成后，以上路径全部返回 404。

## 四、数据语义

1. book_id 与 chapter_id 都是 19 位数字字符串。原因：数值超出
   JavaScript Number 的安全整数范围，转 Number 会丢精度，必须按
   字符串透传。
2. content 是已过滤的 HTML 片段，只含 p / br / img 三种标签，
   img 的 src 一定是 http(s) 绝对地址。原因：服务端对上游做了
   白名单过滤，其余标签已剥除。
3. catalog 一次性返回完整数组。原因：服务端不提供上一章 / 下一章，
   相邻章节由调用方按数组下标自行计算。
4. 图片 URL 带上游签名，约 7 天过期。原因：签名是上游防盗链机制，
   不要持久化图片地址。
5. 启用缓存后，缓存命中返回纯文字（入库时剥掉全部 img），未命中返回
   带图 HTML。原因：入库时按规则剥图，同一章两次请求的结果可能不同，
   这是刻意取舍。
6. 搜索 q 为空返回 40001。原因：上游把空 q 当全量列表返回，
   服务端拦截为参数错误，不是空列表。
7. page 从 1 开始。原因：上游 offset 由 (page-1)*10 计算。
8. 目录可能为空，此时是 200 且 total 为 0。原因：上游部分书确实
   没有可展示的目录。

## 五、服务端行为约束

| code | HTTP | 触发 | 处置分类 |
|---|---|---|---|
| 40001 | 400 | 参数缺失或非法 | 不可重试，改参数后重发 |
| 40101 | 401 | 未登录 / token 失效 / 缺 X-Admin-Request 头 | 引导登录 |
| 40301 | 403 | 请求 IP 被过滤规则拒绝 | 不可重试 |
| 40400 | 404 | 资源不存在（章节无文本、未知管理 API） | 不可重试 |
| 40901 | 409 | 服务已初始化时再次访问 init 流程 | 引导离开 init |
| 42901 | 429 | 触发限速或登录失败锁定 | 按 Retry-After 延时后可重试 |
| 50001 | 500 | 服务端内部错误 | 可重试 |
| 50201 | 502 | 上游请求超时 | 可重试 |
| 50202 | 502 | 上游返回非 2xx | 可重试 |
| 50203 | 502 | 上游业务错误（书不存在、已下架等） | 不可重试 |
| 50301 | 503 | 接口被管理端开关关闭 / 统计需要缓存 | 引导在管理端开启 |
| 50302 | 503 | 服务未初始化 | 引导进入 /init/ |

公开接口开关关闭后返回 503 + 50301；触发限速返回 429 + 42901 并带
Retry-After 响应头；未初始化时 /api/* 返回 503 + 50302、其余路径 302
到 /init/；初始化完成后 /init/* 返回 404。

## 六、测试数据与自检

以下样本在 2026-11-07 与 2026-11-14 实测有效。base 指服务根地址。

| 用途 | curl（相对 base） |
|---|---|
| 搜索 | `curl "$base/api/search?q=%E6%96%97%E7%A0%B4&page=1"` |
| 纯文本正文 | `curl "$base/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"` |
| 带图正文 | `curl "$base/api/content?book_id=7526452300462705689&chapter_id=7173216089122439711"` |
| 语音转写正文（无 p 标签正文，验证标签剥除） | `curl "$base/api/content?book_id=7526452300462705689&chapter_id=7587759954107304984"` |
| 无文本章节（404） | `curl "$base/api/content?book_id=7526452300462705689&chapter_id=7579988172734467097"` |
| 空目录书 | `curl "$base/api/catalog?book_id=7637198840910580798"` |
| 大目录书（508 章） | `curl "$base/api/catalog?book_id=7542602438201576472"` |
| 常规详情（word_number=0） | `curl "$base/api/detail?book_id=7579985241087937560"` |
| 上游无此书（50203） | `curl "$base/api/detail?book_id=7645235352245667856"` |
| 参数错误 | `curl "$base/api/detail?book_id=abc"` |
| 空 q | `curl "$base/api/search?q="` |

## 七、已知限制

- 对外只读接口只有 3.1 的四个；管理接口只为运维服务。
- 搜索直接来自上游，服务端没有本地搜索索引。
- 部分书没有可展示目录（空目录为正常响应）。
- 有声书章节没有文本内容（404 + 40400）。
- 没有用户体系与阅读进度存储；读者匿名，管理员仅一名。
- 上游图片 URL 约 7 天过期，不适合持久化。
- 上游偶发超时与非 2xx，重试是预期内的调用方式。
