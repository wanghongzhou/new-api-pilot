# 多站点运营管理平台 — 详细设计 04B：统计、Dashboard 与导出

> 上级文档：[多站点运营管理平台-概要设计.md](./多站点运营管理平台-概要设计.md)  
> 业务功能索引：[多站点运营管理平台-详细设计-04-业务功能与平台API.md](./多站点运营管理平台-详细设计-04-业务功能与平台API.md)  
> 详细设计索引：[多站点运营管理平台-详细设计.md](./多站点运营管理平台-详细设计.md)

## 18. 详细统计

### 18.1 页面路由

| 页面 | 路由 |
|---|---|
| 全局统计 | /statistics/global |
| 站点统计 | /statistics/sites |
| 客户统计 | /statistics/customers |
| 账户统计 | /statistics/accounts |
| 模型统计 | /statistics/models |
| 通道统计 | /statistics/channels |

### 18.2 通用能力

- 小时、日、月、年粒度；
- 时间范围；
- 站点、客户、账户、模型、通道等适用筛选；
- 趋势图和明细表；
- 排序、分页和下钻；
- Excel、CSV 导出；
- 数据完整状态、缺失范围和更新时间。

统一统计页面布局：

~~~text
┌─ 工具栏 ─────────────────────────────────────────────┐
│ 时间范围 | 粒度 | 站点 | 客户 | 账户 | 模型 | 通道   │
│ 指标选择 | 图表/表格 | 重置 | 导出 Excel | 导出 CSV │
└─────────────────────────────────────────────────────┘
┌─ 汇总卡片 ───────────────────────────────────────────┐
│ 请求数 | quota/金额 | Token | 活跃用户 | 完整率      │
└─────────────────────────────────────────────────────┘
┌─ 趋势图 ─────────────────────────────────────────────┐
│ 当前指标随小时/日/月/年变化                           │
└─────────────────────────────────────────────────────┘
┌─ 排行/占比 ─────────────────┐ ┌─ 数据完整性 ─────────┐
│ Top N、占比、环形图           │ │ 缺失站点和缺失区间    │
└──────────────────────────────┘ └──────────────────────┘
┌─ 明细表 ─────────────────────────────────────────────┐
│ 排序、分页、下钻、按站点分项                          │
└─────────────────────────────────────────────────────┘
~~~

页面差异：

- 全局统计：所有站点合计、站点排行、全局完整率；
- 站点统计：按站点横向对比并可下钻单站；
- 客户统计：仅纳管账户，支持跨站分项；
- 账户统计：仅纳管账户，支持客户和站点过滤；
- 模型统计：全站用户，原始 model_name，保留站点分项；
- 通道统计：全站用户，显示站点和通道名称。

图表规则：

- missing/unavailable 时间点显示断点；
- complete 无记录才显示 0；
- 当前日注明“截至最近完整小时”；
- 月、年活跃用户不能由 daily 直接相加；
- 跨站金额由前端逐站换算；
- hover 同时显示原始 quota、站点汇率和换算金额。
- 全局、客户及其他跨站趋势的每个时间桶都返回自己的 site_breakdown；前端不得用整个查询范围的分项替代逐桶分项。

### 18.3 模型与通道

- 模型按 /api/data/flow 原始 model_name 统计；
- 不做模型别名合并；
- 模型统计覆盖每个站点全部用户；
- 通道统计覆盖每个站点全部用户；
- 通道通过 site_channel 显示名称；
- 不展示通道健康和余额。

P0-A 增加分组、Token、节点三个 flow parity breakdown，并提供与现有统计维度一致的独立前端页面：

- group：按 `(site_id,use_group)` 聚合；空字符串显示“未知分组”；
- token：按 `(site_id,token_id)` 聚合，token_id=0 显示“未知/已删除 Token”，token_name 使用确定性事实快照；
- node：按 `(site_id,node_name)` 聚合；空字符串显示“未知节点”；
- 三类均覆盖站点全部用户，支持站点过滤、服务端选项搜索、小时/日/月/年、完整性与导出；不得与平台纳管账户或实例当前目录混淆。
- 独立路由固定为 `/statistics/groups`、`/statistics/tokens`、`/statistics/nodes`；URL 保存时间范围、粒度、指标、图表/表格视图、站点及本维度筛选，刷新和前进后退必须恢复。
- group/token/node 页面复用统一统计布局、逐桶 `site_breakdown` 金额换算、partial/missing/unavailable/paused 状态和导出入口；375px 下不得横向溢出并满足键盘与无障碍名称要求。
- `use_group`、`node_name` 保留上游原值，空字符串只在展示层分别显示“未知分组”“未知节点”；`token_id` 始终为十进制字符串，0 显示“未知/已删除 Token”，不得经 JavaScript number 转换。

### 18.4 导出

- 同时支持 Excel 和 CSV；
- CSV 使用流式生成；
- Excel 超过单工作表行数时拆分工作表；
- 大数据量导出使用后台任务；
- 导出文件设置过期清理；
- 包含原始 quota；
- 包含当前 quota_per_unit、usd_exchange_rate；
- 临时计算 amount_usd、amount_cny；
- 跨站结果按站点分项输出。
- group/token/node 导出保留 site_id 及原始 use_group、token_id/token_name、node_name；token_id 使用十进制字符串并遵守公式注入防护。

导出资源限制：

- 单用户最多同时存在 3 个 pending/running 任务；
- 全局最多同时存在 10 个 pending/running 任务；
- 单文件默认最大 2 GiB，达到限制后安全终止并删除半成品；
- 创建任务前检查导出目录剩余空间；
- 相同用户、统计类型和规范化 filters 在已有活跃任务时返回原任务，不重复创建；
- 以上阈值可配置，但不得配置为非正数或无上限。
- Header 和 `/exports` 页面提供持久任务入口，刷新或离开原统计页后仍可查看、下载和按同条件重新创建。

### 18.5 API

#### 18.4.1 站点管理数据看板

站点管理列表承载每个站点的三层运营概览：站点身份与管理/健康/授权状态、实时实例与资源状态、北京时间今日的请求/quota/Token/活跃账户/平均 RPM/平均 TPM，以及性能摘要中的成功率、平均延迟和吞吐量。列表不展示实时 RPM/TPM，避免与今日平均口径混用；不加载趋势、模型明细或完整统计明细。性能摘要只读取服务端 24 小时缓存，缓存不可用时显示不可用状态，不把 0 解释为成功采样。

站点详情必须内嵌单站数据看板，并复用 `GET /api/sites/:id/stats` 的统计合约和导出能力；不能只提供跳转入口。该区块提供：时间粒度与范围、请求/quota/金额/Token/活跃账户汇总、图表与表格切换的趋势明细、data_status/as_of、完整性说明和 Excel/CSV 导出任务。站点详情中的看板只读取平台 MySQL，不穿透远端站点；独立的站点统计路由保留，作为可深链的全页视图。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/statistics/global | 全局统计 |
| GET | /api/statistics/sites | 站点统计 |
| GET | /api/statistics/customers | 客户统计 |
| GET | /api/statistics/accounts | 账户统计 |
| GET | /api/statistics/models | 模型统计 |
| GET | /api/statistics/channels | 通道统计 |
| GET | /api/statistics/groups | 分组统计 |
| GET | /api/statistics/tokens | Token 统计 |
| GET | /api/statistics/nodes | 节点统计 |
| POST | /api/statistics/export | 创建导出任务 |
| GET | /api/statistics/exports | 当前用户导出任务列表 |
| GET | /api/statistics/exports/:id | 查询导出状态 |
| GET | /api/statistics/exports/:id/download | 下载成功且未过期的文件 |

### 18.6 export_job

| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint PK | 导出任务 ID |
| user_id | bigint | 创建用户 |
| format | varchar(8) | xlsx/csv |
| statistics_type | varchar(32) | global/site/customer/account/model/channel/group/token/node |
| filters | json | 时间范围、粒度和筛选条件 |
| filter_hash | char(64) | 规范化 filters 的 SHA-256 |
| active_key | varchar(192) nullable | 活跃任务幂等键，终态清空 |
| rate_snapshot | json | 导出开始时冻结的各站点汇率 |
| data_snapshot_at | bigint nullable | 本次成功文件的一致性读快照开始时间 |
| status | varchar(16) | pending/running/success/failed/expired |
| progress | int | 0～100 |
| attempt_count | int | 已执行尝试次数 |
| next_attempt_at | bigint | 下次可领取时间 |
| heartbeat_at | bigint nullable | Worker 心跳 |
| file_path | varchar(500) | 临时文件路径 |
| file_name | varchar(255) | 安全下载文件名 |
| file_size | bigint | 文件大小 |
| row_count | bigint | 导出行数 |
| error_code | varchar(64) | 稳定错误码 |
| error_params | json nullable | 本地化参数，不含秘密 |
| error_message | text | 失败原因 |
| expires_at | bigint | 文件过期时间 |
| started_at | bigint nullable | 开始时间 |
| created_at | bigint | 创建时间 |
| finished_at | bigint | 完成时间 |
| updated_at | bigint | 更新时间 |

---

## 19. Dashboard

Dashboard 是登录后的默认首页，只展示最重要的汇总信息，不替代详细统计。

页面布局：

~~~text
┌─ 今日运营 ───────────────────────────────────────────────┐
│ 今日请求 | 今日 quota/金额 | Token | 活跃账户 | 运营实体 │
│ 所有业务指标注明“截至最近完整小时”                       │
└──────────────────────────────────────────────────────────┘
┌─ 实时吞吐 ───────────────────────────────────────────────┐
│ RPM/TPM | 完整站点/预期站点 | 过期站点 | data_status     │
└──────────────────────────────────────────────────────────┘
┌─ 近30天趋势 ───────────────────┐ ┌─ 站点健康墙 ─────────┐
│ 请求数 / quota金额 / Token      │ │ 绿黄红状态块          │
│ 指标切换，点击进入详细统计       │ │ 点击进入站点详情      │
└────────────────────────────────┘ └──────────────────────┘
┌─ 今日快速排行 ────────────────┐ ┌─ 站点健康、完整性与告警 ┐
│ 站点/客户/模型/通道切换          │ │ 完整率、缺失站点       │
│ 按今日请求或 quota 排序           │ │ 回填和校验状态         │
└────────────────────────────────┘ └──────────────────────┘
~~~

### 19.1 五个区块

1. 今日运营
   - 请求数；
   - quota 及当前汇率换算金额；
   - Token；
   - 活跃账户：今日有事实记录的纳管账户去重数；
   - 站点总数及在线、离线数；
   - 客户数；
   - 纳管账户数；
   - 实例数；同时返回资源完整/预期站点数、过期站点和 data_status。存在未知站点时显示已知合计+partial，无任何有效快照时为 null，不能显示 0。

2. 实时吞吐
   - 各站点最新未过期 RPM/TPM 之和；
   - realtime_complete_site_count、realtime_expected_site_count、stale_site_ids 和 data_status；
   - 任一预期站点过期或不可用时为 partial，没有任何有效站点时为 null，不能显示 0。

3. 近 30 天趋势
   - 请求数；
   - quota/金额；
   - Token。

4. 今日快速排行
   - 今日 Top5 站点；
   - 今日 Top5 客户；
   - 今日 Top5 模型；
   - 今日 Top5 通道。

5. 站点健康、完整性与告警
   - 当前资源告警；
   - 授权过期站点；
   - 统计未就绪站点；
   - 昨日校验状态；
   - 缺失窗口；
   - 全局完整率。

### 19.2 交互

- 点击指标跳转对应详细统计页面；
- 在线状态按 60 秒刷新；
- 当前资源和 RPM/TPM 可按 60 秒刷新；
- 业务统计显示截至最近完整小时；
- 所有金额由前端使用站点当前汇率计算。

### 19.3 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/dashboard/summary | 核心指标和实体数 |
| GET | /api/dashboard/trend | 近 30 天趋势 |
| GET | /api/dashboard/top | Top5 排行 |
| GET | /api/dashboard/health | 健康和完整性 |

---

---
# P0-B 日志查询与导出

新增只读接口：`GET /api/logs`（全局）和 `GET /api/sites/:id/logs`（站点），viewer/admin 均可访问已授权站点范围；响应包含 `items,total,page,page_size,data_status`。筛选参数为 `type,start_timestamp,end_timestamp,username,model_name,token_name,channel_id,group,request_id,upstream_request_id`，分页上限 100，时间范围和总桶/总行数在 SQL 前校验。

日志导出复用 `export_job`，导出行必须保留 site_id、created_at、type、用户、模型、token、channel、group、request_id 和 upstream_request_id；content 只能输出脱敏摘要。导出不改变统计汇总，也不暴露上游展示 ID、站点 Token 或密码。
# P0-C 用户库存统计与导出

新增 `GET /api/user-inventory`、`GET /api/user-inventory/statistics`、`GET /api/sites/:id/user-inventory`、`GET /api/sites/:id/user-inventory/statistics`。列表返回当前用户；统计返回 summary、trend、role/status/group breakdown、site_breakdown 和 completeness。quota、used_quota、balance、request_count、用户计数全部为 JSON string。

导出 scope `user_inventory` 复用 export_job，CSV/XLSX 只包含允许落库的库存字段和 site 身份；严禁 email、OAuth、token、密码和 setting。导出 snapshot 与查询使用同一筛选和 repeatable-read 边界。
# P0-D 渠道运营统计与导出

渠道统计提供当前汇总、小时趋势、type/status/group/tag breakdown 和 site_breakdown。当前汇总展示渠道总数、可用数、不可用数、missing 数、余额合计、used_quota 合计、平均/最大响应时间和可用率；趋势只读取 `complete` 小时事实。无成功快照、采集失败、部分站点缺失时必须用 `data_status`、`as_of` 和站点完整性表达，不能把 unavailable/partial/pending 显示为零渠道。

`statistics_type=channel_inventory` 复用 export_job 生成 CSV/XLSX，冻结站点、关键字、type、status、group、tag、remote_state 和余额/响应范围筛选。导出只包含安全运营字段，bigint 保持字符串、balance 保持规范十进制文本，并沿用公式注入防护。任何导出列都不得包含 `key`、多 Key 状态或上游私密配置。
# P0-E 性能历史统计与导出

提供全局与站点性能历史 API，支持整点左闭右开范围、model、group、site 筛选，返回 trend、model/group breakdown、site_breakdown、完整性和 `aggregation_status`。站点范围可精确展示 official_average；跨站只有所有参与行均为 counter_ready 时才返回加权 summary/trend：success=`Σsuccess/Σrequest`，latency=`Σlatency/Σrequest`，TTFT=`Σttft_sum/Σttft_count`，TPS=`Σoutput_tokens*1000/Σgeneration_ms`。缺少任一分母时不得平均各站平均值，总值置空并标记 unavailable，site_breakdown 保留原值。

`statistics_type=performance_history` 使用 export_job 导出单站/逐站 model+group bucket 原值和 source/completeness；只有 counter_ready 才附带 counters。CSV/XLSX 继续防公式注入，bigint 为字符串、decimal 为规范文本。

# F1 充值与兑换码统计、导出

充值统计返回 order_count、按 status/provider/site breakdown，以及每个 `site_id+provider` 独立的 amount/money nominal totals；全局 summary 不含 money/amount 总额。兑换码统计返回 code_count、enabled/disabled/used/expired 数量、quota 名义合计和 status/site breakdown；expired 按查询 `as_of` 派生。complete/partial/pending/missing/unavailable 完整性与逐站 as_of 均保留。

`statistics_type=topup_inventory|redemption_inventory` 复用 export_job，冻结同一安全筛选并输出 CSV/XLSX。导出坚持 bigint string、decimal 文本和公式注入防护，且任何表头、单元格、元数据均不得出现 `trade_no`、兑换码 `key` 或其派生值。

# T1 任务统计与导出

任务统计返回 total/queued/running/success/failure、success_rate、avg_queue_seconds、avg_run_seconds、avg_total_seconds，以及 status/platform/action/model/site breakdown 和 completeness。比率与平均耗时只使用具备相应时间边界的精确计数重算。`statistics_type=upstream_tasks` 导出安全白名单，禁止任何输入、失败原文、结果地址或私有字段。
模型目录 CSV/XLSX 只导出上游白名单字段与派生覆盖数据，支持 site/vendor/status breakdown 和 completeness，不导出 pricing、billing expression、endpoint 或 bound-channel enrich。
### D136 本地模型与供应商排行

模型与供应商排行只从 `usage_fact_hourly`/既有聚合事实和 M1 exact 模型元数据重算，不访问任何官方 rankings endpoint。today/week/month/year 使用 Asia/Shanghai 的自然日、周一、月初和年初边界；token、份额和增长使用整数/Decimal 精确计算，零历史分母的增长为 null。跨站同名模型按站点事实守恒后再展示；供应商仅由同站点 `name_rule=exact` 元数据映射，缺失映射归 `unknown`。响应包含当前 totals/share、前期增长、history、movers/droppers、data_status、site_breakdown 与 as_of，不复制官方 Top20 截断、float、`time.Now` 缓存。
`subscription_plans` CSV/XLSX 仅导出安全计划核心和 missing 状态，不包含 provider product ID、支付 payload 或策略配置。
# D138 定价与分组目录统计和导出

全局与强制站点视图分别提供 pricing/group 两个目录。pricing 支持 site、model、vendor、group、endpoint、remote_state 筛选；group 支持 site、精确/模糊组名和 remote_state 筛选。统计返回 catalog/missing 计数、vendor/group/endpoint/site breakdown、逐站 `data_status/as_of`；所有计数和 ID 为十进制字符串，所有价格与 ratio 为 decimal string，不计算跨站价格总额、平均价格或货币换算。

CSV/XLSX 使用 `statistics_type=pricing_catalog|group_catalog`，冻结除分页外的当前安全筛选。导出只包含 D138 白名单及 completeness，不包含原始响应、凭据、billing expression、override 或 mutation 参数。

# D139 system-task 统计、完整性与导出

列表支持 `site_ids`（仅全局）、`types`、`statuses`、`created_start`、`created_end`、`error_present` 和服务端分页；默认不设置时间边界，避免隐藏长期未终态任务。统计返回 summary=`total/pending/running/succeeded/failed/error_present_count/success_rate`，以及 type/status/site breakdown、`data_status/as_of/completeness`。成功率只以精确 `succeeded/(succeeded+failed)` 重算，分母为零时为 null；所有计数为 bigint string。站点 breakdown 必须保留 partial/unavailable 站点及已有事实。

CSV/XLSX 固定 `statistics_type=system_tasks`，冻结 `site_ids/types/statuses/created_start/created_end/error_present`，排除分页；强制站点 scope 不接受或记录 `site_ids`。导出列只允许列表安全字段和 typed progress/result、`error_present/error_code`、completeness；不得包含 `active_key`、`locked_by`、payload/state/result 原始 JSON、error 原文、远端详情地址或 mutation 参数。导出沿用统一 export owner/轮询/失败/过期/下载闭环，terminal retention 截止点和 `system_task_terminal_retention_days` 作为元数据说明，不把已删除终态补造为空记录。

completeness 导出列固定包含 `data_status/truncated/truncation_reason/source_limit/observed_count`，其中 reason 只允许 `source_limit|id_gap|source_limit_and_id_gap`，source limit 固定为 bigint string `"100"`。
