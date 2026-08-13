# UI 生产就绪审查台账

本台账用于持续记录逐页面生产就绪审查。页面只有同时完成桌面、平板、手机、交互与流转、加载/空/错误/陈旧状态、权限与危险操作、无障碍、i18n、前后端数值核对、自动化测试和不同审查者复审后，才能标记为 `通过`。

状态定义：`待审`、`审查中`、`待修复`、`待复审`、`通过`、`阻塞`。阶段性浏览或仅阅读源码不得标记为通过。

| 页面簇 | 路由 | 状态 | 当前证据/问题 |
|---|---|---|---|
| 认证 | `/sign-in` | 通过 | Round16 独立终审复跑登录成功、强制改密分流与密码存储隔离；成功导航使用 history replace，后退不恢复登录页。390/768/1024/1440 网络断开矩阵均验证“无法连接服务器”、密码保留供重试、恢复按钮、Enter 提交、axe 与无页面级横向溢出；空提交、错误凭据、真实第 6 次失败 429 和 401 会话清理已有独立证据。 |
| 认证 | `/change-password` | 通过 | Round16 独立终审复跑强制改密成功流：成功跳转使用 replace，已完成改密用户直接访问或后退到该路由时强制返回工作台；旧/新/确认密码不进入 localStorage，也不污染后续共享密码。390/768/1024/1440 已验证三字段错误关联、axe 和无页面级横向溢出。 |
| 全局 | `/dashboard` | 通过 | 固定历史窗口 DB/API/UI 已核对请求 75、额度 2,680,256、Token 4,913,981、活跃账户 1；当前真实 today API 为请求/额度/Token/活跃账户均 0，站点 1/在线 1、客户 1、账户 1、实例 1/在线 1，数据状态 complete，页面不会混用历史窗口与今日值。Round17 独立复跑完整、partial、空趋势/排行/站点/告警及四个端点独立失败 4 类用例全部通过；既有 390/1440 axe、溢出、移动导航和详情弹窗焦点证据完整。告警比较语义按 `<=`“达到或低于”、`>=`“达到或超过”。 |
| 实体 | `/sites` | 通过 | Round21 在 Round18 真实 DB/API/UI 证据上独立复审：当前仅站点 1，active/online/authorized/ready/ok、实例 1/1，今日额度/Token/请求真实 0。发现列表已有成功数据后刷新失败没有陈旧提示，已按同 scope `useRetainedQueryData` 保留最近成功列表、显示“站点列表后台刷新失败”并提供单一重试入口；首次错误仍为阻断态。四尺寸筛选、分页、URL、viewer 只读、admin 生命周期、详情 cached stale、axe/overflow 选定矩阵初跑 22/24，其余通过；移动/768 唯一失败暴露资源百分比动态色仅约 3.8:1，已降低浅色主题指标色亮度，失败视口复跑 2/2 通过。接入秘密仍不进入 URL/storage/GET/console/页面错误。 |
| 实体 | `/sites/:siteId` | 通过 | 站点 1 真实状态 active/online/authorized/ready/ok、实例 1/1、当日用量 0；费率 `500000.0000000000`/`6.8200000000` 正确显示 500,000/6.82。viewer 只读，admin 编辑先 preflight，停用/生命周期/受限删除边界、401 清身份、首次错误/404 与后台刷新失败保留最近成功详情均有证据；四尺寸 axe/无溢出通过。Round18 修复新建站点完成后错误留在 `/sites?runId=...` 的流转，现按任务归属进入 `/sites/:siteId/collection-runs?runId=...`。 |
| 实体 | `/sites/:siteId/status` | 通过 | Round18 独立分片验证四尺寸，宽实例表仅组件内横滚、页面无横向溢出。真实站点实例 1、在线 1；最新 DB 采样为 CPU 8.9628%、内存 66.8021%、磁盘 83.7265%，页面/API 使用同一最新采样语义；确定性 E2E 另覆盖完整/缺失资源、全量实例一次加载、聚合、筛选与安全空值。viewer 只读且无危险操作入口。 |
| 实体 | `/sites/:siteId/collection-runs` | 通过 | Round18 当前 DB 再核对站点 1 共 42,187 条：success 41,550、failed 637、running 1；真实失败 run 33453 为 total 1/completed 0/failed 1，错误为稳定 `DATA_UPSTREAM_UNAVAILABLE`，错误参数仅按白名单安全翻译。四尺寸通过列表/窗口分页、URL 深链、仅活动态轮询、终态停止、失败窗口重试新 run、bigint、axe 与无页面级 overflow；敏感诊断字段不暴露。新建接入完成后的 run 深链已修复并四尺寸 4/4。 |
| 实体 | `/customers` | 通过 | Round22 最终独立复核消除上轮环境噪声：真实 `/customers` 在 390/768/1024/1440 四档 4/4，通过客户 1“测试客户”、签约 1,000,000、回款 1,000 与 API/UI 一致且无页面级 overflow。cached stale、viewer 只读、禁用客户仅允许 enable run 恢复、admin 状态请求体和 401 清身份最小矩阵四档 16/16 一次通过，覆盖 axe 与危险操作边界。Round21 修复的同 scope retained 列表、明确陈旧提示和单一重试入口无回归；首次错误仍保持阻断。真实角色层 POST 403 与既有字段错误/筛选/分页/URL 证据继续有效。 |
| 实体 | `/customers/:customerId` | 通过 | Round22 真实 `/customers/1` 与列表同批四尺寸 4/4：签约 1,000,000、回款 1,000，档案展示与 API 一致且无页面级 overflow。viewer 详情只读、客户/账户无 mutation 入口、按 `customer-detail:<id>` 隔离的 cached stale、axe、disabled→enable run、admin 状态边界和 401 分流在四档最小矩阵 16/16 一次通过；上轮三次 Web 连接重置未复现。无效 ID、首次 404/错误、深链/账户分页和跨功能 URL 的既有证据保持有效。 |
| 实体 | `/accounts` | 通过 | Round19 第二审查者按项目独立复核真实 DB→API→UI，390/768/1024/1440 四档 4/4：当前仅账户 1，quota `4929887`、used quota `70113`、request `8`、remote user ID `2`，API bigint 使用十进制字符串；列表紧凑显示 493万/7万且 `title` 精确保留 4929887/70113。危险与错误态四项目分片 20/20：真实 viewer 无新增/编辑/归档/恢复/删除入口，写请求角色层 403；400 字段错误不暴露服务端原文，编辑 pending 禁止 Escape/取消/关闭和重复提交并恢复焦点，管理员状态请求体、最终确认漂移阻断和 401 清身份跳转通过。四档 axe、URL 与无页面级 overflow 通过，本轮未出现连接重置。 |
| 实体 | `/accounts/:accountId` | 通过 | Round19 四档真实详情逐项一致：额度 4,929,887、累计已用 70,113、请求数 8、remote_user_id 2；failed 回填同时显示失败状态、100% 终态进度及“共 812 个窗口，已完成 807 个，失败 5 个”。不同审查者复跑 viewer 只读、编辑 pending、字段错误、管理员状态边界和 401 共 20/20；详情陈旧数据警告/重试、无效 ID 不重试、错误边界、URL、axe 与无页面级 overflow 均有既有隔离证据。本轮四尺寸真实值均一次通过，无环境重跑豁免。 |
| 统计 | `/statistics/global` | 通过 | Round17 再次请求真实 2026-08-11 13:00~15:00 半开窗口，API 为请求 75、额度 2,680,256、Token 4,913,981、活跃账户 1，逐小时 44/31，与既有 DB/UI 对账一致；原始额度模式不重复第五汇总或金额列。首次 503 阻断、已有成功响应后新范围失败保留旧数据、URL back/forward、axe 与无页面溢出在 390/768/1024/1440 四档独立复跑 4/4 通过。 |
| 统计 | `/statistics/sites` | 通过 | 同一真实窗口单站 API 与全局一致：请求 75、额度 2,680,256、Token 4,913,981、活跃账户 1，逐小时 44/31；站点状态 active/online/authorized/ready/ok 与费率 500000/6.82 正确返回。筛选、URL、完整/partial/不可用/真实空和 cached stale 状态由九 scope 与实体状态矩阵覆盖，四尺寸主流程、axe 和无页面溢出通过。 |
| 统计 | `/statistics/customers` | 通过 | Round17 第二批重新核对真实固定窗口：客户汇总请求/额度/Token/活跃均为 0，两个小时明细均为 0，客户 1、账户数 1、站点数 1，complete；这与当前日志用户归属无法映射到纳管账户的生产事实一致。确定性非零 fixture 覆盖汇总、趋势、分页、导出和显示格式；独立五状态/cached stale/URL reload 测试通过。真实生产库仍无非零客户统计，但页面不会把不可用或缺失状态冒充为 0。 |
| 统计 | `/statistics/accounts` | 通过 | Round17 第二批重新核对真实固定窗口：账户汇总请求/额度/Token/活跃均为 0，两个小时明细均为 0，账户 test、客户 1、remote_user_id 2，complete；与当前生产日志身份不匹配事实一致。确定性非零 fixture 覆盖汇总、趋势、分页、导出和显示格式；独立五状态/cached stale/URL reload 测试通过。真实生产库仍无非零账户统计，但不可用或缺失状态不会显示为真实 0。 |
| 统计 | `/statistics/models` | 通过 | Round17 真实 API 复核请求 75、额度 2,680,256、Token 4,913,981；gpt-5.4 请求 40、gpt-5.6-sol 请求 35，四条逐小时明细与既有 DB/UI 对账一致。对象筛选、分页、URL、五类数据状态、cached stale、axe 和四尺寸主流程有矩阵覆盖，原始额度重复列已移除。 |
| 统计 | `/statistics/channels` | 通过 | Round17 真实 API 复核 channel_id=1 的 75 请求、2,680,256 额度、4,913,981 Token，逐小时 44/31；另有 3 个真实零值渠道，两个小时合计 8 行。筛选、URL、五类数据状态、cached stale、axe 和四尺寸主流程有矩阵覆盖，原始额度重复列已移除。 |
| 统计 | `/statistics/groups` | 通过 | 真实窗口 default 分组请求 44/31、汇总 75 已对账。生产库当前没有空 group 身份；Round17 用确定性空字符串 option/breakdown 独立复核稳定 sentinel 的 API 边界还原、筛选、URL、表格、导出、移动布局、axe 和无溢出，四项目初跑 3/4，唯一移动失败为 Web `ERR_CONNECTION_RESET`，服务 healthy 后单独重跑通过。UI 显示“未知分组”且不泄露 sentinel。 |
| 统计 | `/statistics/tokens` | 通过 | 真实窗口 Token 1/test 请求 44/31、汇总 75 已对账。生产库当前没有已删除 Token；确定性 token_id=0 fixture 独立验证 option、URL、表格、导出均显示“未知/已删除 Token”，不会冒充未命名存量 Token；与 group/node 同一四尺寸矩阵通过 axe、移动布局和无溢出。 |
| 统计 | `/statistics/nodes` | 通过 | 真实 200-master 请求 44/31、汇总 75 已对账。生产库当前没有空 node 身份；确定性空字符串 fixture 独立验证稳定 sentinel 边界、筛选、URL、表格、导出均显示“未知节点”且不泄露 sentinel；与 group/token 同一四尺寸矩阵通过 axe、移动布局和无溢出。 |
| 统计 | `/sites/:siteId/stats` | 通过 | Round17 第二批真实固定窗口 API 再次核对请求 75、额度 2,680,256、Token 4,913,981、活跃账户 1，逐小时 44/31，与全局/站点统计及既有 UI 对账一致；强制 site ID=1 边界、详情加载、URL 范围、权限与筛选剥离有契约覆盖。五类完整性状态、刷新保留旧数据及 reload URL 独立复跑通过，既有四尺寸响应式/axe/无溢出证据完整。 |
| 统计 | `/customers/:customerId/stats` | 通过 | 真实固定窗口 customer 1 为完整的真实 0：两个小时及汇总请求/额度/Token/活跃均 0，客户账户数 1、站点数 1；与生产身份无法映射事实一致。强制客户边界、详情加载、URL 与权限有契约覆盖；确定性非零 fixture 和五类状态/cached stale/reload 独立复跑通过。台账明确真实生产库没有非零客户统计。 |
| 统计 | `/accounts/:accountId/stats` | 通过 | 真实固定窗口 account 1 为完整的真实 0：两个小时及汇总请求/额度/Token/活跃均 0，账户 test、remote_user_id 2；与生产身份不匹配事实一致。强制账户边界、详情加载、URL 与权限有契约覆盖；确定性非零 fixture 和五类状态/cached stale/reload 独立复跑通过。台账明确真实生产库没有非零账户统计。 |
| 分析 | `/rankings` | 通过 | Round18 独立终审复核真实月窗口：主模型 token `314028142`、请求 `2549`、quota `137736688`、占比 `0.9318067319`，API bigint/decimal 字符串与页面百分比语义一致；模型/厂商筛选、分页、精确值、导出、首次错误、cached stale 及 global/site + models/vendors 缓存隔离 E2E 通过。390/768/1024/1440 均通过 axe 与页面无横向溢出。真实环境当前只有站点 1，未声称完成真实多站对账；确定性多站 fixture 已覆盖不跨站合计和强制 scope。 |
| 分析 | `/sites/:siteId/rankings` | 通过 | Round18 站点 1 真实排行逐项与全局单站点结果一致；站点页隐藏站点筛选，忽略 URL 伪造 `site_ids`，模型/厂商读取与导出均强制当前站点，bigint 精确展示及缓存按站点和维度隔离。四尺寸 E2E、axe 与无页面溢出通过。真实库无第二站点；多站隔离和站点边界由确定性 fixture/请求契约证明。 |
| 分析 | `/performance-history` | 通过 | Round18 真实数据库 66 条均为 `official_average`，计数器字段非空计数均为 0；当前约 30 天 API 页面窗口 21 条与数据库一致，汇总四项为 null，正确拒绝对上游平均值二次平均。TTFT/延迟毫秒转秒、成功率比例转百分比、TPS 原值、bigint 分页、筛选、首次错误、cached stale 和 global/site 缓存隔离通过；390/768/1024/1440 均通过 axe 与无页面溢出。真实库无 counter-based 样本；加权汇总边界由确定性契约覆盖。 |
| 分析 | `/sites/:siteId/performance-history` | 通过 | Round18 站点 1 真实窗口同为 21 条 `official_average`，站点专用 API 与全局单站结果一致；隐藏站点筛选并强制 path site ID，超过 `2^53` 精确值、计数器加权、单位换算、分页和 scope 隔离均由确定性 E2E 覆盖。四尺寸通过 axe 与无页面溢出。真实环境暂无 counter-based 或多站数据，不将 fixture 冒充生产样本。 |
| 分析 | `/financial-operations` | 通过 | Round18 真实库充值 3 条，amount 总计 `0`、money 总计 `0.0300000000`，API 列表/统计与站点 1 结果一致；兑换真实库 0 条，API 返回 complete + total `"0"`，页面按真实完整空结果展示。精确 bigint/decimal、非零且不可对账 fixture、空结果、首次 503、cached stale、导出、权限请求边界、敏感字段不暴露及 global/site + topups/redemptions 缓存隔离 E2E 通过；四尺寸通过 axe 与无页面溢出。真实库无非零兑换，不声称完成真实非零逐行对账。 |
| 分析 | `/sites/:siteId/financial-operations` | 通过 | Round18 真实站点 1 充值 3 条、money `0.0300000000` 与全局单站一致，兑换为 complete 空结果；站点专用 API、隐藏站点筛选、强制 path site ID、非零兑换、decimal/bigint 精确显示、不跨站合计、导出及 `payment_reference`/secret 不暴露由确定性 E2E/请求契约覆盖。390/768/1024/1440 均通过 axe 与无页面溢出。真实环境没有非零兑换或第二站点，台账不将 fixture 记作真实生产数据。 |
| 导出 | `/exports` | 待复审 | Round24 纠正此前按路由整页判定的错误：列表页与“查看详情”抽屉必须拆开审查。真实 DB/API 仍为 owner=1 的过期任务 1：XLSX、订阅计划、1 行、7,091 字节及各生命周期时间一致；但详情独立审查发现后端曾把页码冒充百分比、移动抽屉仅 75% 宽、长内容不可滚动、冻结条件不可核验、文件大小不可读和 HTML 时间语义错误。当前正在修复并补详情四尺寸、状态、数值和错误恢复证据，完成前不得标记通过。 |
| 资源 | `/user-inventory` | 通过 | Round16 独立终审核对真实 DB/API：2 条用户库存，quota 167,309,549、used quota 231,534,216、balance -64,224,667、request 5,785；bigint 均保持十进制字符串并与页面语义一致。四标签、筛选 URL/reload、导出、首次错误阻断、刷新错误保留最近成功数据、global/site retained scope 隔离、不可用不冒充零、axe 与页面级 overflow 在 390/768/1024/1440 四档通过。 |
| 资源 | `/sites/:siteId/user-inventory` | 通过 | 强制站点使用站点 list/statistics API、隐藏站点筛选并剥离 `site_ids`；真实数据仅属于 site 1，站点统计与全局单站事实一致。四档已验证分组筛选 URL/reload、bigint、等待采集/不可用边界、首次错误与 cached stale，且缓存不会跨 scope 泄漏。 |
| 资源 | `/channel-inventory` | 通过 | 本轮独立完成 DB/API/UI 事实链复核：4 条、启用 3、手动停用 1、缺失 0、余额 0、已用额度 231,534,216；四行已用/响应分别为 160,557,590/7,474、62,255,789/976、27,321/0、8,693,516/3,721，页面平均响应 3,042.75 与数据库合计 12,171/4 一致。390/768/1024/1440 全局和强制站点页均无页面级溢出、无裸 i18n key；bigint/decimal/比例语义、首次错误、cached stale 和 scope 隔离契约通过。仍需 viewer 权限态终审 |
| 资源 | `/sites/:siteId/channel-inventory` | 通过 | 强制站点页隐藏站点筛选并调用站点 list/statistics；390/1440 无溢出，API site_ids 拒绝/前端剥离有测试，数值展示已对账修复 |
| 资源 | `/model-catalog` | 通过 | Round16 独立终审核对真实目录：登记模型 8 条；三 Tab、URL 深链/reload、筛选、导出、文本型图标安全边界、长描述/标签、匹配规则、bigint ID、首次 503 与 cached stale 均通过。390/768 使用卡片、1024/1440 使用桌面详情，四档无页面级 overflow、无裸 i18n key且 axe 通过。 |
| 资源 | `/sites/:siteId/model-catalog` | 通过 | 强制站点隐藏站点筛选，登记/覆盖/未登记三接口均使用 path site ID 并剥离 `site_ids`；四尺寸覆盖布局、错误态、缓存 scope 隔离和 URL 流转，站点数据不会与全局 retained data 混用。 |
| 资源 | `/pricing-groups` | 通过 | Round16 独立终审核对真实 API：定价目录 363 条，其中 missing 261；配置分组 6 条且 missing 0。decimal string 仅格式化、不换算；定价与分组 Tab、complete 空态、首次 503、刷新失败保留最近成功数据、global/site scope 隔离、权限边界、axe 与无页面级 overflow 在 390/768/1024/1440 四档通过，768 正确采用卡片形态。 |
| 资源 | `/sites/:siteId/pricing-groups` | 通过 | 强制站点隐藏站点筛选并剥离 `site_ids`；定价/分组两视图、decimal 展示、首次错误/cached stale、URL 与 retained scope 在四尺寸通过，站点失败不会回退到全局成功数据。 |
| 资源 | `/subscription-plans` | 通过 | Round20 独立复跑 390/768/1024/1440：价格 decimal、额度/自定义秒 bigint、无限额度、四种重置周期及“目录不代表订单/收入/已购买订阅”语义保持精确；站点分析 Tab、筛选、XLSX 导出、axe、敏感字段不进入页面/存储/导出请求和无页面溢出通过。complete 空态、首次 503 阻断、筛选刷新失败保留最近成功计划及 global/site retained scope 隔离通过。订阅计划业务路由只注册四个列表/统计 GET，不提供计划 mutation；viewer 可读取并创建受 owner scope 管理的统计导出任务。当前生产库样本仅证明现存目录，不把确定性超大数和缺失状态 fixture 冒充真实数据。 |
| 资源 | `/sites/:siteId/subscription-plans` | 通过 | Round20 四尺寸复核站点页隐藏站点筛选，剥离 URL 伪造 `site_ids`，列表、统计和 CSV 导出均强制 path site ID；站点失败不会回退到全局成功缓存。站点分析、精确 decimal/bigint、目录状态、viewer 只读权限、axe、敏感字段边界与无页面溢出均通过；后端无订阅计划写路由。 |
| 日志 | `/logs` | 通过 | Round14 独立终审重新核对 retained hook 仅在同 scope 错误时回退，global/site query key 与 retained scope 双重隔离；采集入库前覆盖 password/passwd/authorization/bearer/access_token/api_key/apikey/webhook/secret/cookie/private_key 整条脱敏，查询 DTO 固定 `ip=""`。固定 24 小时窗口 DB→API 精确一致：529 条、quota 23,878,502、prompt 53,265,335、completion 322,668，最新五行 ID/时间/类型/用户/模型/quota/tokens/双 Request ID 逐项一致，所有 bigint 均为十进制字符串；真实全库总量仍为 4,873。viewer 只读、导出请求过滤、URL/reload、详情脱敏、首次 503 阻断、刷新 503 保留安全旧数据均有自动化证据；390/768/1024/1440 独立重跑 16/16，通过 axe 与页面级 overflow 检查。聚焦单测 10/10、前端全检查、开发栈健康均通过 |
| 日志 | `/sites/:siteId/logs` | 通过 | Round14 独立终审确认强制站点 API 与导出使用 path site ID，不允许页面 `site_ids` 覆盖；页面无站点筛选控件，URL reload 保留，unavailable 与错误态分离，retained 数据不跨 global/site。真实数据只属于 site 1，同一固定窗口站点 API 与 DB/全局 API 均为 529 条、quota 23,878,502，最新五行逐字段一致；四视口独立重跑通过 viewer、axe、overflow 与强制站点契约 |
| 任务 | `/upstream-tasks` | 通过 | Round16 终审确认真实 DB/API 当前为 complete 空结果 0 条，页面未把空数据误报为失败或不可用。非空行的 7 状态、进度、时间、Task ID、bigint total/分页、六标签、导出及敏感字段边界使用确定性 fixture 与 API 契约验证；页面保持只读。首次列表/统计错误阻断，筛选刷新错误保留最近成功列表/统计且显示陈旧提示，retained data 按 global/site 隔离；390/768/1024/1440 均无页面级 overflow、无裸 i18n key且 axe 通过。此结论不声称已完成真实非空任务对账。 |
| 任务 | `/sites/:siteId/upstream-tasks` | 通过 | 强制站点隐藏站点筛选并使用 path site ID，后端拒绝未知/重复 scalar、非规范正时间戳及 `site_ids` 范围覆盖；真实站点结果同为 0。四尺寸通过空结果、确定性非空 fixture、bigint、URL/reload、首次错误、cached stale 与 scope 隔离；不声称存在可供真实非空逐行对账的生产样本。 |
| 任务 | `/system-tasks` | 通过 | Round15 独立终审重新核对当前 DB/API：总数 950，860 条 model_update、90 条 log_detail_cleanup，全部 succeeded；API 汇总 950/活动 0/成功 950/失败 0/错误标志 0 与 DB 一致。最新首行 remote_id 2499、进度 100 与 3/3。API 明确返回 `data_status=partial`、`truncated=true`、`source_limit=100`，页面不会把本地 950 条事实误报为完整上游库存；路由仅注册列表与统计 GET，无逐任务详情或 mutation。cached stale 按 global/站点 scope 隔离；筛选刷新 503 后保留最近成功任务行、展示陈旧数据提示、URL 后退/前进、axe 和页面溢出 E2E 在 390/768/1024/1440 四档 4/4 通过。移动任务卡身份/时间文字及状态、类型、数据完整性徽标已使用高对比语义样式，四档复验 axe 零违规。 |
| 任务 | `/sites/:siteId/system-tasks` | 通过 | Round20 真实 DB 动态复核站点 1 当前总数 952：862 条 model_update、90 条 log_detail_cleanup，全部 succeeded，活动/失败/错误标志均 0；最新 remote_id 2501、进度 100、processed/total 3/3。新增真实已认证 E2E 从站点列表与统计 API 捕获动态 total 和最新任务，并与页面逐项比较通过，避免把会增长的总数写死在测试。站点页隐藏站点筛选，忽略 URL 伪造 `site_ids`，列表/统计强制 path site ID；加载/真实空/首次错误/unavailable、筛选刷新失败保留最近成功任务、URL back/forward、bigint 分页、类型分析、导出安全边界在 390/768/1024/1440 四档 20/20 通过，axe 零违规且无页面级横向溢出。路由仅有列表与统计 GET，无详情或 mutation，viewer 页面保持只读。 |
| 告警 | `/alerts` | 通过 | Round15 独立终审重新核对 DB/API：事件 76、firing 3、resolved 73、当前 critical 3/warning 0、今日结束 0，规则 25；真实 firing #75 为 3042.75 >= 3000，#2 为 0.75 <= 0.80。真实渠道可用率规则 warning 0.90、critical 0.80，`>=` 要求 Warning<Critical、`<=` 要求 Warning>Critical、`==` 要求相等，schema 与服务测试覆盖。事件/规则筛选刷新失败时保留最近成功数据并提示重试，首次 503 不显示伪旧数据；四档 4/4 retained/首次错误矩阵同时通过 axe 与页面溢出检查。详情 404 返回稳定 `NOT_FOUND`；resolved 使用中性“已结束”并保留恢复原因。规则更新仅 admin 路由可用，viewer PUT 403 集成测试覆盖；编辑 pending、冲突与服务错误均有 E2E。delivery 诊断由服务层和前端双层脱敏 URL、webhook、Authorization、token、密码等内容，安全 HTTP 诊断保留，Docker `./service` 测试通过；当前真实库无 delivery 行，真实非空投递展示仍无生产 fixture，但不影响隐私和状态契约结论。 |
| 管理 | `/settings/users` | 通过 | Round17 独立终审真实 DB/API/UI 一致：2 个启用用户、唯一启用管理员 1 个、强制改密 viewer 1 个，列表 bigint total `"2"`。viewer 只读，管理员覆盖创建/编辑/重置密码/禁用/启用；最后管理员与当前用户禁用保护、管理员保护计数失败禁写、409 乐观并发、字段错误、强制改密、身份同步、自身 401、URL history、越界分页和 cached stale 禁写均通过。四类写操作使用同步提交锁，pending 期间 Escape/取消/关闭不能中断或重复提交；禁启完成后焦点返回当前触发按钮，390/1440 聚焦用例及四项目复跑 8/8。完整 390/768/1024/1440 矩阵 99 passed/1 条移动端按条件 skipped，覆盖 axe、桌面表格/窄屏卡片、无页面级 overflow。 |
| 管理 | `/settings/system` | 通过 | Round16 独立终审复跑首次 503 阻断态、业务 API 401 清理身份并 replace 到登录、cached stale 只读禁写、admin 完整面、viewer 只读面、原子字段错误和 decrypt-error 明确清除/替换，桌面定向 12/12 通过；核心状态矩阵在 390/768/1024/1440 通过。该轮发现 cached stale 测试仍按旧 768px 导航断点寻找固定侧栏，已同步到产品当前 `<1024px` 抽屉断点，四档复验 4/4。真实 DB 单位再次核对：60 秒→1 分钟、86400 秒→24 小时、2147483648 字节→2048 MB、5368709120 字节→5120 MB，decimal/其余整数原值一致；敏感字段数据库不含明文，keep/clear/decrypt-error 与跨字段约束有隔离覆盖。四档 admin/viewer axe 与页面溢出通过，真实 viewer PUT 403 已有后端证据。 |
| 路由 | `/settings`、`/` | 通过 | 规范化重定向和认证成功跳转使用 history replace；设置 API 401 清理本地身份、replace 到登录且 forward 不恢复设置页。未知路由及详情错误分流、四档错误路由、SPA 主内容焦点、390px 移动抽屉与主题入口、axe/i18n/页面溢出已有独立矩阵证据。 |

## 跨页面阻塞项

- Round11-12 全路由响应式/无障碍矩阵按路由族与四视口拆分执行，避免单次全套超时掩盖结果。已明确通过：认证/壳层/管理/告警/导出 9 路由 × 390/768/1024/1440（36/36）；实体核心 10 路由 × 四视口（40/40）；统计与分析 16 路由 × 四视口（首次 63/64，唯一 `/sites/1/financial-operations`@768 为 Web 连接重置，独立重跑通过）；资源目录/日志/任务全局 8 路由 × 四视口（32/32）；对应强制站点 8 路由 × 四视口（32/32）。矩阵共 204/204，逐路由检查 main/H1、页面级 overflow、键盘首焦点、axe，390 另检查触摸目标；无未跑路由。该矩阵验证的是故障边界下的结构、响应式和无障碍，不替代各页面真实数据、权限、危险操作与业务状态终审。

- 第 7 轮代表页无障碍/i18n 交叉审查覆盖 `/sign-in`、`/dashboard`、`/sites`、`/rankings`、`/channel-inventory`、`/system-tasks`、`/settings/system` 的 390/1440 失败边界：逐页验证首个键盘焦点可见、页面级无横向溢出、移动端触摸目标不小于 40px、axe 零违规。首次扫描发现共享 EmptyTitle 固定 `h3` 导致 `h1` 后跳级，已按设计改为默认 `h2` 并补契约测试，复验 14/14 通过；i18n source/message registry 聚焦测试 18/18、`i18n:check` 与 `bun run check` 通过。真实数据态、弹窗焦点回归及未抽样页面仍需后续轮次，不据此宣称全项目无障碍通过。

- Round19 独立终审通过跨页面 bigint 分页与共享数据表规范：后端 common `PageData.total` 及模型目录/渠道未登记、模型定价/分组配置、用户库存、渠道库存、订阅计划自定义分页 DTO 均以 JSON 十进制字符串传输；后端 `common` 容器测试验证 marshal/unmarshal 契约。前端共享分页和越界纠正全程使用 BigInt，`total='9007199254740993'` 在 390/768/1024/1440 四档均精确显示为 9,007,199,254,740,993，隐藏无法安全计算的末页跳转，键盘下一页同步 URL 与 API `page=2` 且无页面级横向溢出；五类资源页越界页回退、retained refresh、搜索 history 和移动关键字段矩阵 28 passed、12 条按项目条件 skipped。审查发现原 E2E 将所有非手机项目强制为 1440px、没有真正覆盖平板，已修为尊重各项目 viewport 并四档复验；共享表格焦点环、固定高度纵向滚动、非固定高度横向滚动和 sticky header 聚焦测试 27/27 通过。该全局阻塞项关闭。
- 详情页 completeness 的统计窗口语义和 `expected=0` 表达已修复；站点、客户、账户详情均已完成 DB→API→UI 原值对账、四视口展示和独立复审。
- 数据表布局测试已分别约束焦点样式、固定高度纵向滚动和非固定高度横向滚动；最终单元测试数量与全量检查结果以本台账末次门禁记录为准。
- 逐页、同类页和全局交叉审查已完成多轮闭环；生产就绪结论仍须以末次前端、Docker 后端、文档、全路由矩阵和开发栈健康门禁全部通过为准。
- 完成前必须运行前端完整检查、相关单元/E2E、Docker 后端测试、文档检查、开发栈健康检查，并由不同审查者完成最终复审。

## 最终生产就绪门禁（2026-08-12）

> 2026-08-13 更正：本节原“最终生产就绪”结论已撤回。旧的 208/208 矩阵验证的是直接访问 URL 后的结构、响应式与无障碍，没有验证用户在页面内点击全局导航。真实浏览器因此漏过了 `/model-catalog` 当前匹配异常导致任意导航点击停留原页的问题。现已修复路由形态、全局导航包装与未知路由 H1，并新增四尺寸真实点击矩阵；在新的全项目末次门禁完成前，本项目不得再次宣称生产就绪。

- 页面台账 50/50 均为“通过”，无待审、待修复、待复审、阻塞或未闭环状态；逐页结果已分别经过真实数据/确定性边界、同类页面和全局规范交叉复审。
- `bun run check` 通过：路由生成、TypeScript、oxlint、oxfmt、2,676 个 i18n key 与生产构建一致。
- `bun run test:unit` 通过：148 个文件、563 个测试、2,444 次断言，0 失败。
- 全路由响应式/无障碍测试已修正历史上将非手机项目强制为 1440px 的错误，最终按 Playwright 项目真实 viewport 执行 208/208：1440、390、768、1024 均检查 main/H1、页面级 overflow、键盘首焦点、axe，390 额外检查触摸目标。
- 最新 `go-test-runner` 镜像完整构建通过；Docker 后端 `./...` 全量通过，包含所有包、contract 和 39 个逐测试隔离数据库 integration 场景。
- 正确挂载完整源码执行的 Docker docs-check 通过；未使用缺少 `docs/` 的测试镜像副本结果作为证据。
- 最终开发栈必须保持 API/Web/MySQL/Redis healthy，`http://127.0.0.1:3000/healthz` 与 `http://127.0.0.1:5173` 返回 200；最终交付以前一次干净 Web 重启后的复核结果为准。

## Round23 全局点击导航回归（2026-08-13）

- 根因：`/model-catalog` 由目录索引路由生成 `/model-catalog/` 的匹配身份，而浏览器地址为无尾斜杠形式；TanStack Link 在点击时以当前匹配为 `from`，触发 `Could not find match for from`，事件已被阻止但导航未提交。
- 修复：将模型审计全局路由改为非索引文件路由，使生成路径与公开 URL 均为 `/model-catalog`；侧栏 TooltipTrigger 只在桌面折叠态构造，展开态和移动端不再给链接叠加交互触发器；未知路由“页面不存在”改为页面级 H1。
- 数据保护：全局导航保留系统设置未保存修改确认。定向 `settings.spec.ts` 用例通过，取消后仍停留 `/settings/system`，不会静默丢失编辑。
- 新门禁：`app-navigation.spec.ts` 必须执行真实 `link.click()`，不得用 `page.goto()` 代替。四项目覆盖 1440、390、768、1024，从 `/model-catalog` 点击客户管理、系统任务、系统设置和品牌首页，以及带中文逗号的明确未知路由；本轮 20/20 通过。
- 本轮前端门禁：`bun run check` 通过；单元测试 148 文件、564/564 通过；真实浏览器已复验 `/model-catalog` 点击“客户管理”到 `/customers` 且 H1 为“客户管理”。全路由响应式/无障碍矩阵本次运行超过单命令四分钟上限，尚未形成新的全量通过证据，因此生产就绪状态继续保持撤回。

