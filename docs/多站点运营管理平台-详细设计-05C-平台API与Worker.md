# 多站点运营管理平台 — 详细设计 05C：平台 API 与 Worker

> 上级文档：[多站点运营管理平台-概要设计.md](./多站点运营管理平台-概要设计.md)  
> 后端实现索引：[多站点运营管理平台-详细设计-05-后端实现.md](./多站点运营管理平台-详细设计-05-后端实现.md)  
> 详细设计索引：[多站点运营管理平台-详细设计.md](./多站点运营管理平台-详细设计.md)

## 33. 完整平台 API 契约

本节是路由、DTO、权限和错误码的权威定义；前文各业务章节的 API 表是页面级摘要。

### 33.1 通用 DTO

ApiResponse<T>：

| 字段 | 类型 | 说明 |
|---|---|---|
| success | boolean | 是否成功 |
| message | string | 安全兜底消息；前端优先按 code 使用 i18next |
| code | string | 稳定机器码，成功为空 |
| data | T/null | 响应数据 |
| request_id | string | 请求追踪 ID |
| field_errors | object/null | 可选的字段级错误，key 为请求字段名 |

PageData<T>：

| 字段 | 类型 |
|---|---|
| page | number |
| page_size | number |
| total | number |
| items | T[] |

ListQuery：

| 参数 | 类型 | 默认/约束 |
|---|---|---|
| p | int | 默认 1，最小 1 |
| page_size | int | 默认 20，1～100 |
| keyword | string | trim，最大 128 |
| sort_by | string | 每个接口使用白名单 |
| sort_order | string | asc/desc，默认 desc |

所有时间范围使用 start_timestamp、end_timestamp，左闭右开，必须为 Unix 秒并与 granularity 对应的北京时间桶边界对齐。ID path 参数是十进制字符串，解析后必须在 int64 正整数范围内。数据库 BIGINT 字段以及 quota、token_used、request_count 等用量指标 DTO 一律使用字符串；受容量约束的小型实体计数、Unix 秒时间戳、百分比、进度、页码和分页 total 使用 JSON number。

平台不接收文件上传。JSON 请求体默认上限 1 MiB，登录/密码接口上限 16 KiB；超过返回 HTTP 413、code=PAYLOAD_TOO_LARGE。Controller 使用严格 JSON 解码，未知字段、重复字段和尾随 JSON 均返回 VALIDATION_ERROR。

### 33.2 登录和平台用户

| 方法 | 路径 | 权限 | 请求 | data |
|---|---|---|---|---|
| POST | /api/user/login | public | LoginRequest | LoginUser |
| POST | /api/user/logout | UserAuth | 无 | null |
| GET | /api/user/self | UserAuth | 无 | LoginUser |
| PUT | /api/user/password | UserAuth | ChangePasswordRequest | null |
| GET | /api/user/ | UserAuth | ListQuery + role + status | PageData<PlatformUserItem> |
| POST | /api/user/ | AdminAuth | CreatePlatformUserRequest | PlatformUserItem |
| PUT | /api/user/:id | AdminAuth | UpdatePlatformUserRequest | PlatformUserItem |
| POST | /api/user/:id/enable | AdminAuth | 无 | null |
| POST | /api/user/:id/disable | AdminAuth | 无 | null |
| POST | /api/user/:id/reset-password | AdminAuth | ResetPasswordRequest | null |

~~~text
LoginRequest {
  username: lowercase ASCII string(3..64)
  password: non-empty string
}

LoginUser {
  id: string
  username: string
  display_name: string
  role: "admin" | "viewer"
  status: 1 | 2
  must_change_password: boolean
}

ChangePasswordRequest {
  original_password: string
  new_password: Password
}

CreatePlatformUserRequest {
  username: lowercase ASCII string(3..64)
  display_name: string(1..128)
  role: "admin" | "viewer"
  password: Password
}

UpdatePlatformUserRequest {
  username: lowercase ASCII string(3..64)
  display_name: string(1..128)
  role: "admin" | "viewer"
}

ResetPasswordRequest {
  new_password: Password
}

Password = 至少 8 个 Unicode 字符且 UTF-8 编码不超过 72 字节；仅创建、重置和修改密码使用该约束，登录仅要求 password 非空。
~~~

新建和重置后的用户 must_change_password=true。用户名唯一；不能禁用或降级最后一个启用 admin；用户不能禁用自己，admin 重置自己的密码应使用自助修改密码接口。用户名不存在、密码错误以及禁用用户提交错误密码均返回通用登录失败；仅在密码校验成功后发现账户禁用时返回 USER_DISABLED。自助改密的 original_password 错误返回 original_password 字段错误。

### 33.3 站点

| 方法 | 路径 | 权限 | 请求/查询 | data |
|---|---|---|---|---|
| GET | /api/sites | UserAuth | ListQuery + 五组状态筛选 | PageData<SiteListItem> |
| POST | /api/sites/refresh | AdminAuth | SiteBatchRefreshRequest | CollectionRunItem[] |
| POST | /api/sites | AdminAuth | SiteCreateRequest | SiteDetail |
| GET | /api/sites/:id | UserAuth | 无 | SiteDetail |
| GET | /api/sites/:id/performance | UserAuth | hours=1..720，默认 24 | SitePerformanceSummary |
| POST | /api/sites/:id/base-url-preflight | AdminAuth | SiteBaseUrlPreflightRequest | SiteBaseUrlPreflightResult |
| PUT | /api/sites/:id | AdminAuth | SiteUpdateRequest | SiteDetail |
| DELETE | /api/sites/:id | AdminAuth | 无 | null |
| POST | /api/sites/:id/authorize | AdminAuth | SiteAuthorizeRequest | SiteAuthorizationResult |
| POST | /api/sites/:id/recheck-capabilities | AdminAuth | 无 | SiteAuthorizationResult |
| POST | /api/sites/:id/probe | AdminAuth | 无 | SiteProbeResult |
| POST | /api/sites/:id/refresh | AdminAuth | 无 | CollectionRunItem[] |
| POST | /api/sites/:id/backfill | AdminAuth | SiteBackfillRequest | CollectionRunItem |
| POST | /api/sites/:id/disable | AdminAuth | 无 | SiteDetail |
| POST | /api/sites/:id/enable | AdminAuth | 无 | CollectionRunItem |
| POST | /api/sites/:id/end-statistics | AdminAuth | SiteStatisticsEndRequest | SiteDetail |
| DELETE | /api/sites/:id/statistics-end | AdminAuth | 无 | SiteDetail |
| GET | /api/sites/:id/status | UserAuth | ResourceQuery | SiteResourceResponse |
| GET | /api/sites/:id/instances | UserAuth | 无 | SiteInstanceItem[] |
| GET | /api/sites/:id/collection-runs | UserAuth | ListQuery + task_type/status | PageData<CollectionRunItem> |
| GET | /api/collection-runs/:id | UserAuth | 无 | CollectionRunItem |
| GET | /api/collection-runs/:id/windows | UserAuth | ListQuery + status | PageData<CollectionRunWindowItem> |

~~~text
SiteCreateRequest {
  name: string(1..128)
  base_url: absolute http/https URL(<=255)
  remark?: string(<=500)
}

SiteUpdateRequest {
  name: string
  base_url: absolute http/https URL
  remark?: string
  base_url_preflight_token?: string
  confirm_same_site?: boolean
}

SiteBaseUrlPreflightRequest {
  base_url: absolute http/https URL(<=255)
}

SiteAuthorizeRequest {
  mode: "existing_token" | "login_generate_token"
  root_user_id?: IdString
  access_token?: string(1..4096)
  username?: string(1..128)
  password?: string(1..1024)
  confirm_token_rotation?: boolean
}

SiteBatchRefreshRequest {
  site_ids: string[](1..100)
}

SiteBackfillRequest {
  start_timestamp?: int64
  end_timestamp?: int64
  only_missing?: boolean = true
}

SiteStatisticsEndRequest {
  statistics_end_at: int64 aligned to Beijing hour
}
~~~

SiteAuthorizeRequest 条件校验：existing_token 必须且只允许 root_user_id/access_token；login_generate_token 必须且只允许 username/password/confirm_token_rotation=true，root_user_id 从登录响应取得。模式字段混用返回 field_errors。密码和 Cookie 永不保存；Token 只有在 `/api/user/self`、精确 root 读取以及完整用户快照的首用户证明均通过后，才与 root_created_at/statistics_start_at 原子保存。完整快照要求 root 为最小正 ID 且 created_at 不晚于任一用户，并把软删除用户纳入证明。授权事务使用 §31 BumpSiteFence，成功保存新 Token 时恰好递增一次 site.config_version、终止旧父子任务并使旧凭据响应失效。后续版本/接口能力检查失败不回滚已验证凭据，而是保留 authorized 并把 statistics_status 置 pending_config/error，允许修复后重试。

手动补采默认只处理 missing/pending 事实窗口；未传范围时处理该站点全部可重试缺口。显式范围最大天数实时读取 `collector.manual_backfill_max_days`（默认 366），超过返回 VALIDATION_ERROR，admin 需要拆分提交。新 run 为目标小时创建新的 collection_run_window 并使用独立尝试预算。相同 active_key 已存在时 HTTP 200 返回已有任务、success=true、code=""，并在 DTO 中标记 deduplicated=true；成功响应不得携带错误码。

SiteAuthorizationResult 返回 root_user_id、root_created_at、自动计算的 statistics_start_at、版本、system_name、data_export_enabled、接口能力检查项、flow/data 校验结果和回填任务 ID，不返回 Access Token。授权事务只有在 root 身份与 created_at 校验成功后才保存 Token 和历史起点。

`POST /api/sites/:id/authorize` 的响应语义固定如下：`/api/user/self`、精确 root 或完整首用户证明失败时返回失败响应，不保存 Token 和起点；身份已验证并原子保存后，版本、导出开关、必要接口或 flow/data 能力检查失败仍返回 HTTP 200、success=true、code="" 的 SiteAuthorizationResult，对应 capability 标记 failed、backfill_run_id=null。active 站点保持 pending_config/error；disabled 站点无论结果都保持 statistics_status=paused，失败详情从 auth_status/site_capability 读取。后续重试授权会核对不可变 root_created_at；enable/backfill 等要求能力已就绪的操作再按 SITE_EXPORT_DISABLED 或 SITE_INCOMPATIBLE 拒绝。

`POST /api/sites/:id/recheck-capabilities` 使用已保存 Token，重复 `/self`、精确 root、完整首用户证明和 §30.11 全部能力检查并 upsert site_capability；不旋转 Token。能力可用性发生转换时执行 §31 BumpSiteFence，同一结果重复检查不 bump。active 站点身份/首用户失败保持 error 并停止采集；required 能力全部通过后创建新 config_version 的缺口/首次回填 run，状态进入 backfilling，无 expected 窗口时立即 ready。disabled 或 statistics_end_at 非空时只持久化 auth/capability/fence，statistics_status 保持 paused、backfill_run_id=null；身份失败也不改写 paused，后续 enable 根据 auth/capability 拒绝。pending_config/error/paused 页面必须提供该动作，不能要求重新录入同一 Token 才恢复配置类故障。

base_url 变化必须先调用 preflight。服务端对候选地址执行完整 SSRF 校验和不携带 Token 的 `/api/status` 请求，返回新旧 origin/path、system_name、version、变更类型和 10 分钟有效的 HMAC 预检凭证；凭证绑定 site_id、当前 config_version 和规范化候选 URL。PUT 修改 URL 时必须提交该凭证及 `confirm_same_site=true`，事务内再次核对版本并执行 §31 BumpSiteFence，恰好递增一次 config_version、终止旧父子任务并清 active_key。scheme、host 或 port 变化时清除授权可用状态并要求重新授权，不能向新地址发送旧 Token；仅路径变化可在确认后沿用 Token。所有旧 config_version 任务写入时失败为 SITE_CONFIG_CHANGED。statistics_start_at 只在首次成功授权时由 root.created_at 自动写入，此后不可修改；statistics_end_at 只能通过专用接口设置或清除。statistics_end_at 仅允许在站点 disabled 时设置，必须处于 `[coalesce(last_complete_hour+3600, statistics_start_at), disabled_at]`；清除后仍需调用 enable 才恢复采集，禁止未来终止时间制造人为 paused 区间。

`POST /api/sites/refresh` 和 `POST /api/sites/:id/refresh` 只排队探活、RPM/TPM 和实例资源三类当前状态任务，不触发历史用量、用户或通道同步。重复 disable/archive 返回当前对象；重复 enable/restore 若已有同范围任务则返回该任务并标记 deduplicated。

性能健康使用上游已有 `GET /api/perf-metrics/summary?hours=N`，pilot 不修改该接口且不把滚动范围摘要增量累加。详情 `GET /api/sites/:id/performance?hours=N` 每次使用已存凭据实时请求单站上游，返回 `hours/sampled_at/data_status/request_count/success_rate/avg_latency_ms/avg_tps/models`；bigint 计数为字符串，总体比率和均值按模型 request_count 加权。`hours=24` 的成功详情结果同时写入 `site_id + config_version` 进程内缓存；站点列表只读取该缓存，缺失时先返回 `data_status=unavailable` 并受全局 4 并发限制异步刷新。成功 TTL 1 小时，失败 TTL 5 分钟；缓存不落 MySQL/Redis、进程重启后丢失。上游失败不影响站点列表、详情的本地用量或资源数据，也不触发历史补采。

SiteListItem、SiteDetail 及其嵌套结构以 §33.13 为准；§15.6 只用于页面示意。

### 33.4 客户

| 方法 | 路径 | 权限 | 请求/查询 | data |
|---|---|---|---|---|
| GET | /api/customers | UserAuth | ListQuery + status | PageData<CustomerListItem> |
| POST | /api/customers | AdminAuth | CustomerCreateRequest | CustomerDetail |
| GET | /api/customers/:id | UserAuth | 无 | CustomerDetail |
| PUT | /api/customers/:id | AdminAuth | CustomerUpdateRequest | CustomerDetail |
| DELETE | /api/customers/:id | AdminAuth | 无 | null |
| POST | /api/customers/:id/disable | AdminAuth | 无 | CustomerDetail |
| POST | /api/customers/:id/enable | AdminAuth | 无 | CollectionRunItem |
| GET | /api/customers/:id/accounts | UserAuth | ListQuery | PageData<AccountListItem> |

~~~text
CustomerCreateRequest {
  name: string(1..128)
  contact?: string(<=255)
  remark?: string(<=500)
  status: "communicating" | "signing" | "using"
}

CustomerUpdateRequest {
  name: string
  contact?: string
  remark?: string
  status: "communicating" | "signing" | "using"
}
~~~

disabled 不能通过通用 PUT 直接设置，必须调用 disable；从 disabled 恢复必须调用 enable，以确保回填任务不会被绕过。

### 33.5 账户

| 方法 | 路径 | 权限 | 请求/查询 | data |
|---|---|---|---|---|
| GET | /api/accounts | UserAuth | ListQuery + site_id/customer_id/remote_status/remote_state/managed_status | PageData<AccountListItem> |
| GET | /api/accounts/site/:siteId/remote-users | AdminAuth | keyword + p/page_size | PageData<RemoteUserItem> |
| POST | /api/accounts | AdminAuth | AccountCreateRequest | AccountDetail |
| GET | /api/accounts/:id | UserAuth | 无 | AccountDetail |
| PUT | /api/accounts/:id | AdminAuth | AccountUpdateRequest | AccountDetail |
| DELETE | /api/accounts/:id | AdminAuth | 无 | null |
| POST | /api/accounts/:id/archive | AdminAuth | 无 | AccountDetail |
| POST | /api/accounts/:id/restore | AdminAuth | 无 | CollectionRunItem |
| POST | /api/accounts/:id/refresh | AdminAuth | 无 | AccountDetail |

~~~text
AccountCreateRequest {
  site_id: string
  customer_id: string
  remote_user_id: string
  remark?: string(<=500)
}

AccountUpdateRequest {
  remark: string(<=500)
}
~~~

POST 时后端重新从远端读取 remote_user_id，不能信任前端提交的用户名、额度、分组等快照；created_at 必须在 (0,now] 并固化为 remote_created_at。site_id、customer_id、remote_user_id、remote_created_at 不出现在更新 DTO 中。

### 33.6 统计

| 方法 | 路径 | 权限 | 查询/业务范围 | data |
|---|---|---|---|---|
| GET | /api/statistics/global | UserAuth | 全站全部用户 | StatisticsResponse |
| GET | /api/statistics/sites | UserAuth | 全站全部用户，按站点 | StatisticsResponse |
| GET | /api/statistics/customers | UserAuth | 指定客户的纳管账户 | StatisticsResponse |
| GET | /api/statistics/accounts | UserAuth | 指定纳管账户 | StatisticsResponse |
| GET | /api/statistics/models | UserAuth | 各站全部用户，按原始模型 | StatisticsResponse |
| GET | /api/statistics/channels | UserAuth | 各站全部用户，按通道 | StatisticsResponse |
| GET | /api/statistics/options/models | UserAuth | keyword + site_ids + p/page_size | PageData<ModelOption> |
| GET | /api/statistics/options/channels | UserAuth | keyword + site_ids + p/page_size | PageData<ChannelOption> |
| GET | /api/statistics/options/groups | UserAuth | keyword + site_ids + p/page_size | PageData<GroupOption> |
| GET | /api/statistics/options/tokens | UserAuth | keyword + site_ids + p/page_size | PageData<TokenOption> |
| GET | /api/statistics/options/nodes | UserAuth | keyword + site_ids + p/page_size | PageData<NodeOption> |
| GET | /api/statistics/groups | UserAuth | 各站全部用户，按 use_group | StatisticsResponse |
| GET | /api/statistics/tokens | UserAuth | 各站全部用户，按 token_id | StatisticsResponse |
| GET | /api/statistics/nodes | UserAuth | 各站全部用户，按 flow node_name | StatisticsResponse |
| GET | /api/sites/:id/stats | UserAuth | 站点统计便捷入口，复用 statistics/sites | StatisticsResponse |
| GET | /api/customers/:id/stats | UserAuth | 客户统计便捷入口，复用 statistics/customers | StatisticsResponse |
| GET | /api/accounts/:id/stats | UserAuth | 账户统计便捷入口，复用 statistics/accounts | StatisticsResponse |

StatisticsQuery：

在既有 site_ids/customer_ids/account_ids/model_names/channel_keys 之外增加 `use_groups`、`token_keys`、`node_names`，每类最多 100 项。token_key 固定为 `site_id:token_id`，token_id 允许 0；use_group/node_name 按原始字符串精确匹配并受 site_ids 约束。group/token/node scope 仅允许与 site_ids 及自身筛选组合，非法跨 scope 参数返回 VALIDATION_ERROR。

GroupOption={site_id,site_name,use_group}；TokenOption={token_key,site_id,site_name,token_id,token_name}，所有 ID 为字符串；NodeOption={site_id,site_name,node_name}。三个选项接口从事实身份 DISTINCT 查询，空身份合成为未知项，稳定按 site/name/id 排序。

| 参数 | 类型 | 说明 |
|---|---|---|
| start_timestamp | int64 | 必填 |
| end_timestamp | int64 | 必填，必须大于 start |
| granularity | hour/day/month/year | 必填 |
| site_ids | comma-separated IDs | 适用时，去重后最多 100 个 |
| customer_ids | comma-separated IDs | 适用时，去重后最多 100 个 |
| account_ids | comma-separated IDs | 适用时，去重后最多 100 个 |
| model_names | repeated string | 模型页，最多 100 个，每项最长 255，大小写敏感 |
| channel_keys | repeated `site_id:remote_channel_id` | 通道页，去重后最多 100 个；按站点内通道身份精确匹配，禁止拆成独立数组形成笛卡尔积 |
| p/page_size | int | breakdown 明细分页 |
| sort_by | request_count/quota/token_used/active_users/name/bucket_start | 按 scope 白名单；时间桶明细允许 bucket_start |
| sort_order | asc/desc | 默认 desc |

StatisticsResponse：

~~~text
{
  scope: "global" | "site" | "customer" | "account" | "model" | "channel"
  granularity: "hour" | "day" | "month" | "year"
  range: { start_timestamp, end_timestamp, timezone: "Asia/Shanghai", as_of }
  summary: {
    request_count: string | null
    quota: string | null
    token_used: string | null
    active_users: string | null
    data_status: DataStatus
    is_partial: boolean
  }
  trend: TrendPoint[]
  breakdown: PageData<StatisticsBreakdownItem>
  site_breakdown: SiteQuotaBreakdown[]
  completeness: Completeness
}
~~~

TrendPoint 的完整字段以 §33.13 为准。跨站 scope 的每个时间桶必须返回该桶自己的 site_breakdown，不能只返回全范围分项，否则前端无法逐桶换算金额。SiteQuotaBreakdown 包含 site_id、site_name、quota、quota_per_unit、usd_exchange_rate、rate_source、rate_updated_at、data_status；rate_source 为 site/fallback/unavailable，后端不返回落库金额。

### 33.7 Dashboard

| 方法 | 路径 | 权限 | 查询 | data |
|---|---|---|---|---|
| GET | /api/dashboard/summary | UserAuth | 无 | DashboardSummary |
| GET | /api/dashboard/trend | UserAuth | days=30，最大 90 | TrendPoint[] |
| GET | /api/dashboard/top | UserAuth | type + metric + limit<=20 | RankingItem[] |
| GET | /api/dashboard/health | UserAuth | 无 | DashboardHealth |

summary、trend、top 是小时业务数据，health 是 60 秒当前数据。DashboardSummary.active_accounts_today 只统计纳管账户，不等同于 global_stat 的全部远端 active_users。DashboardSummary 的 RPM/TPM 同时返回 realtime_complete_site_count、realtime_expected_site_count、stale_site_ids 和 data_status；存在过期站点时为 partial，没有有效站点时数值为 null。前端并行请求四个接口；任一失败不阻塞其他区块。

### 33.8 导出

| 方法 | 路径 | 权限 | 请求/查询 | data |
|---|---|---|---|---|
| POST | /api/statistics/export | UserAuth | ExportCreateRequest | ExportJobItem |
| GET | /api/statistics/exports | UserAuth | ListQuery | PageData<ExportJobItem> |
| GET | /api/statistics/exports/:id | UserAuth | 无 | ExportJobItem |
| GET | /api/statistics/exports/:id/download | UserAuth | 无 | 文件流 |

~~~text
ExportCreateRequest {
  format: "xlsx" | "csv"
  statistics_type: "global" | "site" | "customer" | "account" | "model" | "channel"
  filters: ExportFilters
}
~~~

普通用户只能查看和下载自己创建的任务；admin 也不通过该接口读取其他人的文件。下载必须 status=success 且未过期，Content-Disposition 使用安全文件名。过期返回 410 EXPORT_EXPIRED；元数据成功但文件丢失时返回 410 EXPORT_FILE_MISSING，并把任务改为 failed 供重新导出。创建任务前校验单用户最多 3 个、全局最多 10 个活跃导出；相同 active_key 返回已有任务。阈值从受保护配置读取，不能配置为非正数或无上限。

### 33.9 告警和规则

| 方法 | 路径 | 权限 | 请求/查询 | data |
|---|---|---|---|---|
| GET | /api/alerts/summary | UserAuth | 无 | AlertSummary |
| GET | /api/alerts | UserAuth | ListQuery + status/level/target_type/site_id/start_timestamp/end_timestamp | PageData<AlertEventItem> |
| GET | /api/alerts/:id | UserAuth | 无 | AlertEventDetail |
| GET | /api/alert-rules | UserAuth | scope_type/scope_id | AlertRuleItem[] |
| PUT | /api/alert-rules/:id | AdminAuth | AlertRuleUpdateRequest | AlertRuleItem |
| POST | /api/alert-rules/overrides | AdminAuth | AlertRuleOverrideRequest | AlertRuleItem |
| DELETE | /api/alert-rules/:id | AdminAuth | 仅 site override | null |
| POST | /api/notifications/dingtalk/test | AdminAuth | 无 | NotificationTestResult |

AlertRuleUpdateRequest 仅允许 enabled、threshold_value、for_times。AlertRuleOverrideRequest 包含 base_rule_id、site_id、enabled、threshold_value、for_times。rule_key、metric、compare_operator、level、scope 创建后不可修改；单站覆盖由复制默认规则生成。全局默认规则不能删除，删除 site override 后自动回落全局规则。只有 CPU/内存/磁盘/stale 等数值规则允许修改 threshold_value/for_times；授权、身份、开关和布尔规则只允许 enabled，其固定阈值/比较符提交修改时返回字段错误。

资源百分比阈值范围 1～100，同一 rule_key 下 Warning 必须小于 Critical；for_times 范围 1～60。规则更新事务成功后从下一采样周期生效，不追溯改写历史事件。

告警列表时间区间筛选 first_observed_at，使用左闭右开 Unix 秒；默认服务端复合排序固定为 `status(firing,pending,resolved) → level(critical,warning,info) → COALESCE(last_fired_at,first_observed_at) desc → id desc`。显式 sort_by 只改变主排序字段，id desc 始终作为稳定尾排序。顶部计数只读取 `/api/alerts/summary`，不允许前端用当前分页推算。

`GET /api/alert-rules?scope_type=global` 返回全局行。`scope_type=site&scope_id=<site>` 返回每个 global 基础规则的一条“有效规则”：有 override 时 id/effective_rule_id=override、override_rule_id 非空、inherited=false；否则 id/effective_rule_id=base、override_rule_id=null、inherited=true。base_rule_id 始终指向对应全局规则。POST overrides 只接收 base_rule_id；PUT 修改 id 指向的实际行；DELETE 只接收 override_rule_id，删除后下次 GET 自动回落全局。

### 33.10 系统配置

| 方法 | 路径 | 权限 | 请求 | data |
|---|---|---|---|---|
| GET | /api/settings | UserAuth | 无 | SettingGroup[] |
| PUT | /api/settings | AdminAuth | SettingPatchRequest | SettingGroup[] |

SettingPatchRequest 是 key/value 数组，后端按白名单验证类型和范围。collector.probe_interval_seconds、collector.realtime_interval_seconds、collector.resource_interval_seconds 允许管理员 PUT 60～3600 的整分钟秒数；调度器每分钟重新加载 platform_setting，保存后最迟在下一次调度检查时采用新周期，无需重启。敏感项 GET 只返回 configured、masked_value、decrypt_error，不返回 setting_value；PUT 中空字符串表示“不修改”，显式 clear=true 才清空。SettingGroup 只返回分组元数据和设置项，不返回 H+15 发布资格或原因码；production 与 development/test 使用相同的字段级及跨字段校验规则。

GET 按 collector、export、rate、notification、upstream、system 固定顺序返回 37 个 platform_setting 以及只读虚拟项 `system.public_origin`；`logs.retention_days`、`performance.retention_days`、`task.retention_days`、`system_task_terminal_retention_days` 均位于 collector 分组，范围 1～3650，其中 `logs.retention_days` 默认 90 天。虚拟项来自运行环境、updated_at=null，PUT 必须拒绝。配置范围固定为：probe_interval_seconds/realtime_interval_seconds/resource_interval_seconds 60～3600 且必须为 60 的整数倍，usage_delay_minutes 1～59，minute_retention_days/logs.retention_days/performance_retention_days/task.retention_days/system_task_terminal_retention_days 1～3650，六个队列 concurrency 1～100，manual_backfill_max_days 1～3660，export.file_ttl_hours 1～168，两个活跃导出上限 1～100 且单用户不得超过全局，max_file_bytes/min_free_disk_bytes 为正 int64；fast_task.history_retention_seconds 60～31536000、fast_task.history_count 1～1000；upstream.connect_timeout_seconds 1～60、response_header_timeout_seconds 1～300、request_timeout_seconds 1～600、export_timeout_seconds 1～3600，并满足 connect<=request、response_header<=request<=export；upstream.rate_limit_requests 1～10000、rate_limit_window_seconds 1～3600、max_inflight_per_origin 1～100。普通 int 只接受规范的正整数 JSON number，不接受字符串、小数、指数、符号或超过 `9007199254740991` 的值；两个字节阈值只接受规范的正 int64 十进制 JSON string，不接受 JSON number。两个 decimal 只接受 ASCII 固定点 JSON string，语法为整数位数字加可选的非空小数位，拒绝空白、符号、指数、非 ASCII 数字和零；小数位最多 10 位、总精度最多 30 位、规范化后整数位最多 20 位，并规范化整数前导零和小数尾零。CPU/内存/磁盘阈值、instance_stale 和任务重试预算不属于 platform_setting，继续由 alert-rules 或固定任务策略管理。

`upstream.allowed_host_suffixes` 与 `upstream.allowed_cidrs` 使用逗号分隔存储，前端以多行文本编辑并在提交时规范化；两者允许同时为空，此时只允许 DNS 解析结果全部属于安全公网地址。IP 字面量必须命中显式 CIDR，私网地址也只有命中显式 CIDR 才允许；loopback、link-local、multicast、云元数据和其他特殊用途地址始终拒绝，不能通过系统设置放行。上述 11 个 fast_task/upstream 设置在保存事务成功后原子刷新运行时快照，新建上游请求和后续 Redis 历史写入立即使用新值，无需重启服务。

费率兜底仅用于站点从未成功同步过费率的情况，不能覆盖已经同步到的站点费率。`rate.fallback_quota_per_unit` 默认 `500000`，表示 500,000 quota = 1 USD，`amount_usd = quota / quota_per_unit`；`rate.fallback_usd_exchange_rate` 默认 `6.8`，表示 1 USD = 6.8 CNY，`amount_cny = amount_usd * usd_exchange_rate`。两项必须同时为正数才会生效，任一被清空时金额状态为 unavailable。

PUT 在进入事务前完成 items 数量、重复 key、白名单、read_only、JSON 表示、类型、单字段范围以及新 webhook 的 HTTPS/allowlist 校验。随后单事务按 setting_key 顺序 `SELECT ... FOR UPDATE` 锁定全部 37 行，构造最终状态并校验活跃导出上限、上游超时关系和钉钉完整配置，全部通过后才统一 UPDATE；任一错误必须零写入。事务采用原子 last-write-wins，不接收未文档化的 CAS/version 字段；只有实际变化的行更新 updated_at，值为 `max(now, previous_updated_at+1)`，未变化和敏感 keep 保持原时间。

敏感 value 缺失或空字符串表示 keep，非空字符串表示替换，clear=true 表示显式清除；clear 与非空 value 冲突。替换值使用 AES-256-GCM 和 AAD=`setting:<key>` 后写库。GET 的 value 永远为 null，configured 只表示密文非空，masked_value 使用固定长度掩码；密文无法解密时仍返回其他非敏感配置，并置 configured=true、decrypt_error=true，不返回密文、明文或解密详情。notification.dingtalk.enabled=true 的最终状态要求 webhook 和 secret 均非空、可解密，且 webhook 为 allowlist 内 HTTPS 地址；停用与两项 clear 可以在同一原子 PUT 中完成。

### 33.11 资源查询

ResourceQuery：

| 参数 | 约束 |
|---|---|
| start_timestamp/end_timestamp | 必填 |
| granularity | minute/hour/day |
| node_name | 可选，最大 128，大小写敏感 |

minute 只能查询最近 retention_days；hour 最大 1 年；day 最大 5 年。响应同时返回 CPU/内存 max 与 avg、磁盘 max 与期末值，前端按指标选择，不使用含糊的统一 aggregation 参数；站点聚合没有磁盘期末值时返回 null。minute 的 max=avg=当前样本、磁盘 max=last=当前样本。时间点使用持久化 sample_count/expected_sample_count/data_status，遇到采集缺口时指标为 null 或已知部分并标记 partial，不补 0。

三种粒度都只接受已闭合桶，end_timestamp 不得晚于当前分钟/北京时间整点/北京时间当日 00:00；minute 的 start 和总跨度必须同时落在实时读取的 retention_days 内。服务在任何数据库查询和切片分配前计算桶数并拒绝超过 200,000 个桶的请求；hour/day 仍分别受 1 年/5 年上限约束，支持的日历范围为 1970～9999。极远未来、时间差溢出或超量桶统一返回 VALIDATION_ERROR，不进入资源表查询。

`GET /api/sites/:id/instances` 在一次查询中把 site_instance 与每个 node 最新分钟样本关联，返回 SiteInstanceItem 当前资源；不得要求前端为 N 个实例再请求 N 次状态接口。超过有效 stale 阈值或当前分钟缺失时按 DTO 返回 stale/offline/unknown 与 sampled_at，不复用旧值伪装新鲜。

快速任务历史不是资源时间序列，但与站点采集记录同屏。统一只读接口：

| 方法 | 路径 | 权限 | 请求 | data |
|---|---|---|---|---|
| GET | /api/fast-tasks | UserAuth | site_id、task_type、status?、offset=0、limit=50 | FastTaskHistoryPage |

`task_type` 仅允许 `site_probe|realtime_stat|resource_snapshot`，`status` 仅允许空、`success|failed`，offset>=0，limit=1..100。`FastTaskHistoryPage={items,offset,limit,total,has_more}`；item 字段为 site_id（IdString）、task_type、started_at、finished_at、status、duration_ms、error、request_id。Redis key 固定为 `new-api-pilot:fast-task:{site_id}:{task_type}`，每次 LPUSH/LTRIM 到系统设置 `fast_task.history_count` 并刷新 `fast_task.history_retention_seconds` TTL；状态筛选在服务端完成后再分页。参数非法返回 VALIDATION_ERROR，Redis 读取失败返回 INTERNAL_ERROR，不回退到伪造空列表。

### 33.12 HTTP 与业务错误

| HTTP | code | 场景 |
|---:|---|---|
| 400 | VALIDATION_ERROR | 参数格式、范围、枚举错误 |
| 401 | AUTH_REQUIRED / AUTH_INVALID | 未登录、Session 失效、用户名或密码错误 |
| 403 | FORBIDDEN | 角色不足 |
| 403 | USER_DISABLED | 密码校验成功但平台用户已禁用 |
| 403 | ORIGIN_FORBIDDEN | 浏览器写请求 Origin 缺失或不匹配 |
| 403 | PASSWORD_CHANGE_REQUIRED | 首次密码未修改 |
| 404 | NOT_FOUND | 资源不存在 |
| 410 | EXPORT_EXPIRED / EXPORT_FILE_MISSING | 导出已过期或文件丢失 |
| 413 | PAYLOAD_TOO_LARGE | 请求体超过接口上限 |
| 409 | CONFLICT | 状态冲突或唯一键冲突 |
| 409 | DELETE_RESTRICTED | 存在关联数据 |
| 409 | LAST_ADMIN | 最后一个启用 admin |
| 409 | BACKFILL_RUNNING | 状态操作要求新建任务但对象已有不同范围的回填任务 |
| 409 | TASK_OVERLAP | 存在非同键但时间范围冲突的任务 |
| 409 | SITE_CONFIG_CHANGED | 任务或预检凭证对应的站点配置版本已变化 |
| 409 | BASE_URL_PREFLIGHT_REQUIRED | 修改地址缺少、过期或不匹配的候选预检凭证 |
| 409 | EXPORT_LIMIT_REACHED | 单用户或全局活跃导出达到配置上限 |
| 422 | SITE_EXPORT_DISABLED | 站点未开启数据导出 |
| 422 | SITE_INCOMPATIBLE | root 身份/created_at 非法，或要求能力已就绪的操作发现上游接口契约/DTO 不兼容 |
| 422 | UPSTREAM_USER_NOT_FOUND | 提交账户时远端用户已不存在 |
| 422 | UPSTREAM_ADDRESS_FORBIDDEN | 上游地址不在网络白名单或解析结果非法 |
| 429 | LOGIN_RATE_LIMITED | 登录失败次数超过限制 |
| 502 | UPSTREAM_ERROR | 上游错误 |
| 502 | TOKEN_ROTATION_RESULT_UNKNOWN | 生成 Token 请求在可能已生效后中断；不自动重试，要求 admin 显式重新授权 |
| 503 | UPSTREAM_UNAVAILABLE | 网络、超时、站点离线 |
| 500 | INTERNAL_CONTRACT_ERROR | 数据库持久值或内部枚举违反程序契约 |
| 500 | INTERNAL_ERROR | 未分类内部错误 |

数据库唯一键错误必须转换为 CONFLICT，不把 SQL 文本返回前端。所有 5xx message 使用安全通用提示，详细错误只写后端日志。

后台任务终态 error.code 使用以下稳定 registry；HTTP 创建接口如同步发现同类问题复用同一 code：

| 域 | code | 含义/恢复 |
|---|---|---|
| collection | COLLECTION_RETRY_EXHAUSTED | 当前 run 重试耗尽；admin 可创建新 run |
| collection | DATA_VALIDATION_MISMATCH | flow/data 不一致；窗口隔离并补采 |
| collection | UPSTREAM_RESPONSE_INVALID | 上游 DTO/字段非法；修复版本或数据后重试 |
| collection | UPSTREAM_RESPONSE_TOO_LARGE | 响应超过 64 MiB；容量评审后调整契约 |
| collection | SITE_CONFIG_CHANGED | 任务冻结版本失效；按新配置重新排队 |
| collection | DEPENDENCY_WINDOWS_MISSING | 本地账户/客户重建等待站点窗口 |
| collection | WORKER_LEASE_LOST | Worker 租约丢失；按当前 run-window 已用预算恢复或终止 |
| export | EXPORT_DISK_LOW | 剩余空间低于阈值；清理空间后重新导出 |
| export | EXPORT_FILE_TOO_LARGE | 文件超过配置上限；缩小范围 |
| export | EXPORT_SNAPSHOT_FAILED | 一致性快照或查询失败；可重试 |
| export | EXPORT_WRITE_FAILED | 文件写入/原子改名失败；可重试 |
| export | EXPORT_EXPIRED | 文件已过期；重新导出 |
| export | EXPORT_FILE_MISSING | 元数据存在但文件丢失；重新导出 |
| delivery | NOTIFICATION_DISABLED | 通知已关闭；不补发旧事件 |
| delivery | NOTIFICATION_NOT_CONFIGURED | Webhook 未配置或 secret 解密失败 |
| delivery | NOTIFICATION_TEST_SUCCEEDED | 测试消息已通过真实投递链路发送成功 |
| delivery | DINGTALK_ADDRESS_FORBIDDEN | Webhook 协议/主机/重定向不合法 |
| delivery | DINGTALK_REJECTED | 钉钉返回非零 errcode；按策略重试 |
| delivery | DELIVERY_RETRY_EXHAUSTED | 投递重试耗尽；保留记录供排查 |
| delivery | DELIVERY_RETRY_SCHEDULED | 测试消息本次同步发送失败，delivery 仍为 pending 且已安排下次重试 |
| internal | INTERNAL_CONTRACT_ERROR | 内部组件读取到不支持的持久值；停止当前操作并修复数据或版本契约 |

其他 MessageRef.code 也不是任意字符串，固定目录如下：

| 类别 | 稳定 code |
|---|---|
| 数据状态原因 | DATA_PENDING、DATA_BACKFILLING、DATA_WINDOW_MISSING、DATA_UPSTREAM_UNAVAILABLE、DATA_SCOPE_PAUSED、DATA_PARTIAL_SITES、DATA_VALIDATION_FAILED |
| 能力检查 | CAPABILITY_OK、CAPABILITY_UPSTREAM_UNAVAILABLE、CAPABILITY_RESPONSE_INVALID、CAPABILITY_EXPORT_DISABLED、CAPABILITY_IDENTITY_FAILED、CAPABILITY_FIRST_USER_PROOF_FAILED、CAPABILITY_NO_TRAFFIC_SKIPPED |
| 告警主文案 | ALERT_SITE_OFFLINE、ALERT_AUTH_EXPIRED、ALERT_EXPORT_DISABLED、ALERT_COLLECTION_MISSING、ALERT_BACKFILL_FAILED、ALERT_VALIDATION_FAILED、ALERT_INSTANCE_STALE、ALERT_INSTANCE_OFFLINE、ALERT_NO_INSTANCE、ALERT_CPU_HIGH、ALERT_MEMORY_HIGH、ALERT_DISK_HIGH、ALERT_ACCOUNT_MISSING、ALERT_ACCOUNT_IDENTITY_MISMATCH、ALERT_ACCOUNT_DISABLED、ALERT_ACCOUNT_QUOTA_EMPTY、ALERT_CHANNEL_BALANCE_LOW、ALERT_CHANNEL_RESPONSE_TIME_HIGH、ALERT_CHANNEL_AVAILABILITY_LOW、ALERT_SCOPE_INACTIVE |

params 的 key 由 code 固定并进入 OpenAPI fixture。下表是 `MessageParamsByCode` 的权威 schema；“同组 code”共享完全相同的 required params，未列 optional 时禁止额外字段。所有 `*_id` 为 IdString，时间为 Timestamp，计数为 number，字节/指标值为十进制 string，名称为不含秘密的安全显示名。

| code | required params | optional params |
|---|---|---|
| COLLECTION_RETRY_EXHAUSTED | site_id、run_id | - |
| DATA_VALIDATION_MISMATCH、DATA_WINDOW_MISSING、DATA_VALIDATION_FAILED、ALERT_COLLECTION_MISSING | site_id、start_timestamp、end_timestamp | - |
| ALERT_VALIDATION_FAILED | site_id、start_timestamp、end_timestamp、failure_kind:`data_mismatch\|execution_failed` | - |
| UPSTREAM_RESPONSE_INVALID | site_id | capability_key:string |
| UPSTREAM_RESPONSE_TOO_LARGE | site_id、response_bytes:string、limit_bytes:string | - |
| SITE_CONFIG_CHANGED | site_id、expected_config_version:number、actual_config_version:number | - |
| DEPENDENCY_WINDOWS_MISSING | site_id、run_id、start_timestamp、end_timestamp | - |
| WORKER_LEASE_LOST | site_id、run_id、hour_ts | - |
| EXPORT_DISK_LOW | export_id、free_bytes:string、threshold_bytes:string | - |
| EXPORT_FILE_TOO_LARGE | export_id、file_bytes:string、limit_bytes:string | - |
| EXPORT_SNAPSHOT_FAILED、EXPORT_WRITE_FAILED、EXPORT_EXPIRED、EXPORT_FILE_MISSING | export_id | - |
| NOTIFICATION_DISABLED、NOTIFICATION_NOT_CONFIGURED、DINGTALK_ADDRESS_FORBIDDEN | alert_event_id:IdString\|null、delivery_id:IdString\|null | - |
| NOTIFICATION_TEST_SUCCEEDED | delivery_id | - |
| DELIVERY_RETRY_EXHAUSTED | alert_event_id:IdString\|null、delivery_id | - |
| DELIVERY_RETRY_SCHEDULED | delivery_id、next_retry_at:NonNegativeIntegerString | - |
| DINGTALK_REJECTED | alert_event_id:IdString\|null、delivery_id、errcode:string | - |
| DATA_PENDING、DATA_BACKFILLING | scope_type:string、scope_id:IdString\|null、progress:number | - |
| DATA_UPSTREAM_UNAVAILABLE | site_id、start_timestamp、end_timestamp | - |
| DATA_SCOPE_PAUSED | scope_type:string、scope_id、start_timestamp、end_timestamp | - |
| DATA_PARTIAL_SITES | complete_site_count:number、expected_site_count:number | - |
| CAPABILITY_OK、CAPABILITY_NO_TRAFFIC_SKIPPED | site_id、capability_key:string | - |
| CAPABILITY_UPSTREAM_UNAVAILABLE、CAPABILITY_RESPONSE_INVALID、CAPABILITY_EXPORT_DISABLED、CAPABILITY_IDENTITY_FAILED、CAPABILITY_FIRST_USER_PROOF_FAILED | site_id、capability_key:string | - |
| ALERT_SITE_OFFLINE、ALERT_AUTH_EXPIRED、ALERT_EXPORT_DISABLED、ALERT_NO_INSTANCE | site_id、site_name:string | - |
| ALERT_BACKFILL_FAILED | site_id、run_id | - |
| ALERT_INSTANCE_STALE、ALERT_INSTANCE_OFFLINE | site_id、instance_name:string | - |
| ALERT_CPU_HIGH、ALERT_MEMORY_HIGH、ALERT_DISK_HIGH | site_id、target_type:string、target_name:string、value:string、threshold:string | - |
| ALERT_CHANNEL_BALANCE_LOW、ALERT_CHANNEL_RESPONSE_TIME_HIGH、ALERT_CHANNEL_AVAILABILITY_LOW | site_id、site_name:string、value:string、threshold:string | - |
| ALERT_ACCOUNT_MISSING、ALERT_ACCOUNT_IDENTITY_MISMATCH、ALERT_ACCOUNT_DISABLED、ALERT_ACCOUNT_QUOTA_EMPTY | account_id、account_name:string | site_id |
| ALERT_SCOPE_INACTIVE | scope_type:string、scope_id、scope_name:string | - |
| INTERNAL_CONTRACT_ERROR | component:string | value:string |

投递参数中的 null 仅用于测试消息或 delivery 尚未创建前的同步配置失败：测试 delivery 的 alert_event_id 固定 null，配置预检失败时 delivery_id 也为 null；测试发送成功使用 `NOTIFICATION_TEST_SUCCEEDED` 和非空 delivery_id；真实告警投递两个 ID 均非空。`INTERNAL_CONTRACT_ERROR.value` 只能携带不含秘密的安全枚举值，无法安全回显时省略。`make docs-check` 必须证明 registry 中每个稳定 code 在本表恰好出现一次，OpenAPI 生成 `code` 判别联合，`zh-CN` 文案和 fixture 覆盖该联合；不得放 Token、URL query、密码或 Webhook。

前端以 code + params 通过 i18next 生成简体中文主文案，本表之外的 error_message/technical_detail 只进入“技术详情”并可复制 request_id，不作为流程分支或已本地化主文案。新增稳定码必须同时修改本 registry、`zh-CN` 文案、契约 fixture 和验收用例。

### 33.13 DTO 字段闭环

以下字段表是前后端生成类型、OpenAPI 契约测试和 fixture 的依据。未列出的数据库字段不得直接透传。

基础类型：

~~~text
IdString = 十进制正整数字符串
NonNegativeIdString = 十进制非负整数字符串，首期仅 channel_id 允许 "0"
MetricString = 十进制有符号整数字符串
Timestamp = Unix 秒 number，可空字段使用 null
DataStatus = "complete" | "partial" | "pending" | "missing" |
             "unavailable" | "paused" | "backfilling"

MessageCode = 上述 registry 的字符串字面量联合
MessageParamsByCode = 上述 code -> params 精确映射
MessageRef<C extends MessageCode> {
  code: C
  params: MessageParamsByCode[C]
  technical_detail: string
}
AnyMessageRef = { [C in MessageCode]: MessageRef<C> }[MessageCode]

MissingRange {
  site_id: IdString
  status: DataStatus
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  reason: AnyMessageRef
}

Completeness {
  data_status: DataStatus
  complete_site_count: number
  expected_site_count: number
  unit_type: "site_hour" | "hour" | "customer_site_hour"
  complete_unit_count: number
  expected_unit_count: number
  completeness_rate: number
  missing_site_ids: IdString[]
  missing_ranges: MissingRange[]
  missing_range_total: number
  missing_ranges_truncated: boolean
  last_verified_at: Timestamp | null
}

RateInfo {
  quota_per_unit: string | null
  usd_exchange_rate: string | null
  source: "site" | "fallback" | "unavailable"
  updated_at: Timestamp | null
}

UsageSummary {
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  as_of: Timestamp | null
  data_status: DataStatus
  is_final?: boolean
}

UpstreamPerformanceModel {
  model_name: string
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  request_count: MetricString
}

UpstreamPerformanceSummary {
  models: UpstreamPerformanceModel[]
}

BackfillSummary {
  status: "none" | "pending" | "running" | "failed"
  progress: number
  total_windows: number
  completed_windows: number
  failed_windows: number
  start_timestamp: Timestamp | null
  end_timestamp: Timestamp | null
  latest_error: AnyMessageRef | null
  run_id: IdString | null
}
~~~

平台用户和任务：

~~~text
PlatformUserItem {
  id: IdString
  username: string
  display_name: string
  role: "admin" | "viewer"
  status: 1 | 2
  must_change_password: boolean
  last_login_at: Timestamp | null
  created_at: Timestamp
  updated_at: Timestamp
}

CollectionRunItem {
  id: IdString
  site_id: IdString | null
  site_config_version: number
  task_type: string
  target_type: "site" | "account" | "customer"
  target_id: IdString
  trigger_type: "schedule" | "manual" | "recovery" | "dependency"
  start_timestamp: Timestamp | null
  end_timestamp: Timestamp | null
  status: "pending" | "running" | "success" | "failed"
  priority: number
  progress: number
  windows_initialized: boolean
  total_windows: number
  completed_windows: number
  failed_windows: number
  created_request_id: string
  last_request_id: string
  fetched_rows: MetricString
  written_rows: MetricString
  retry_count: number
  error: AnyMessageRef | null
  next_attempt_at: Timestamp | null
  started_at: Timestamp | null
  finished_at: Timestamp | null
  created_at: Timestamp
  deduplicated: boolean
}

CollectionRunWindowItem {
  id: IdString
  run_id: IdString
  site_id: IdString
  hour_ts: Timestamp
  status: "pending" | "running" | "success" | "failed" | "unavailable"
  fact_status: "pending" | "complete" | "missing" | "unavailable"
  fetched_rows: MetricString
  written_rows: MetricString
  attempt_count: number
  next_retry_at: Timestamp | null
  verified_at: Timestamp | null
  error: AnyMessageRef | null
  started_at: Timestamp | null
  finished_at: Timestamp | null
  updated_at: Timestamp
}
~~~

站点：

~~~text
SiteListItem {
  id: IdString
  name: string
  base_url: string
  management_status: "active" | "disabled"
  online_status: "unknown" | "online" | "offline"
  auth_status: "unauthorized" | "authorized" | "expired"
  statistics_status: "pending_config" | "backfilling" | "ready" |
                     "partial" | "error" | "paused"
  health_status: "ok" | "warning" | "critical" | "unavailable"
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  rate: RateInfo
  realtime: {
    rpm: MetricString
    tpm: MetricString
    updated_at: Timestamp | null
    expired: boolean
  }
  resource: {
    instance_count: number
    online_instance_count: number
    cpu_max_percent: number
    memory_max_percent: number
    disk_max_used_percent: number
    updated_at: Timestamp | null
    data_status: DataStatus
  }
  today: {
    request_count: MetricString
    quota: MetricString
    token_used: MetricString
    active_users: MetricString
    as_of: Timestamp | null
    data_status: DataStatus
    is_final: boolean
  }
  completeness_rate: number
  disabled_at: Timestamp | null
  updated_at: Timestamp
}

SiteDetail = SiteListItem + {
  remark: string
  config_version: number
  root_user_id: IdString | null
  root_created_at: Timestamp | null
  statistics_start_at: Timestamp | null
  statistics_start_source: "root_created_at" | null
  statistics_end_at: Timestamp | null
  monitoring_start_at: Timestamp | null
  last_probe_at: Timestamp | null
  last_probe_success_at: Timestamp | null
  backfill: BackfillSummary
  completeness: Completeness
}

SiteAuthorizationResult {
  root_user_id: IdString
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  first_user_proof: {
    snapshot_total: number
    min_user_id: IdString
    earliest_created_at: Timestamp
    passed: boolean
  }
  capabilities: { key: string, status: "passed" | "failed" | "skipped", message: AnyMessageRef }[]
  flow_data_validation: "passed" | "failed" | "skipped"
  root_created_at: Timestamp
  statistics_start_at: Timestamp
  backfill_run_id: IdString | null
}

SiteProbeResult {
  probe_success: boolean
  online_status: "unknown" | "online" | "offline"
  contract_status: "compatible" | "incompatible" | "unavailable"
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  probed_at: Timestamp
}

SiteBaseUrlPreflightResult {
  normalized_base_url: string
  change_type: "none" | "path" | "origin"
  old_public: { base_url: string, system_name: string, version: string }
  candidate_public: { base_url: string, system_name: string, version: string }
  contract_status: "compatible" | "incompatible"
  preflight_token: string
  expires_at: Timestamp
}
~~~

未授权站点的 statistics_start_at/root_created_at/statistics_start_source 均为 null；仅首次授权事务成功后，statistics_start_source 才固定为 `root_created_at`。

客户和账户：

~~~text
CustomerListItem {
  id: IdString
  name: string
  contact: string
  remark: string
  status: "communicating" | "signing" | "using" | "disabled"
  account_count: number
  active_account_count: number
  archived_account_count: number
  site_count: number
  today: UsageSummary & { site_breakdown: SiteQuotaBreakdown[] }
  backfill: BackfillSummary
  updated_at: Timestamp
}

CustomerDetail = CustomerListItem + {
  statistics_paused_at: Timestamp | null
  completeness: Completeness
  created_at: Timestamp
}

AccountListItem {
  id: IdString
  site_id: IdString
  site_name: string
  customer_id: IdString
  customer_name: string
  remote_user_id: IdString
  remote_created_at: Timestamp
  username: string
  display_name: string
  remote_group: string
  remote_status: number
  remote_state: "normal" | "missing" | "identity_mismatch"
  managed_status: "active" | "archived"
  quota: MetricString
  used_quota: MetricString
  request_count: MetricString
  rate: RateInfo
  last_synced_at: Timestamp | null
  today: UsageSummary
  backfill: BackfillSummary
  updated_at: Timestamp
}

AccountDetail = AccountListItem + {
  remark: string
  remote_missing_count: number
  last_remote_seen_at: Timestamp | null
  statistics_paused_at: Timestamp | null
  completeness: Completeness
  created_at: Timestamp
}

RemoteUserItem {
  id: IdString
  username: string
  display_name: string
  role: number
  status: number
  group: string
  quota: MetricString
  used_quota: MetricString
  request_count: MetricString
  created_at: Timestamp
  last_login_at: Timestamp | null
  already_managed: boolean
  managed_account_id: IdString | null
  managed_customer_name: string
}
~~~

统计、Dashboard 和资源：

~~~text
SiteQuotaBreakdown {
  site_id: IdString
  site_name: string
  quota: MetricString | null
  quota_per_unit: string | null
  usd_exchange_rate: string | null
  rate_source: "site" | "fallback" | "unavailable"
  rate_updated_at: Timestamp | null
  data_status: DataStatus
}

TrendPoint {
  bucket_start: Timestamp
  bucket_end: Timestamp
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  data_status: DataStatus
  is_final: boolean
  as_of: Timestamp | null
  complete_site_count: number
  expected_site_count: number
  site_breakdown: SiteQuotaBreakdown[]
  reason: AnyMessageRef | null
}

StatisticsBreakdownBase {
  dimension_id: string
  dimension_name: string
  site_id: IdString | null
  site_name: string | null
  bucket_start: Timestamp
  bucket_end: Timestamp
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  data_status: DataStatus
  is_final: boolean
  as_of: Timestamp | null
  site_breakdown: SiteQuotaBreakdown[]
  completeness_rate: number
}

GlobalStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "global"
  site_id: null
  site_name: null
  complete_site_count: number
  expected_site_count: number
}

SiteStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "site"
  site_id: IdString
  site_name: string
  management_status: "active" | "disabled"
  online_status: "unknown" | "online" | "offline"
  auth_status: "unauthorized" | "authorized" | "expired"
  statistics_status: "pending_config" | "backfilling" | "ready" | "partial" | "error" | "paused"
  health_status: "ok" | "warning" | "critical" | "unavailable"
  rate: RateInfo
}

CustomerStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "customer"
  site_id: null
  site_name: null
  account_count: number
  site_count: number
}

AccountStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "account"
  site_id: IdString
  site_name: string
  customer_id: IdString
  customer_name: string
  remote_user_id: IdString
}

账户 breakdown、trend 和 summary 的 `active_users` 不是不适用字段：账户统计表不物化它，读取时按相同范围的事实和完整性派生；有事实返回 `"1"`，expected 全部 complete 且无事实返回 `"0"`，expected 不完整返回 `null`。

ModelStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "model"
  site_id: IdString
  site_name: string
  model_name: string
}

ChannelStatisticsBreakdown = StatisticsBreakdownBase & {
  dimension_type: "channel"
  site_id: IdString
  site_name: string
  remote_channel_id: NonNegativeIdString
  remote_missing: boolean
}

StatisticsBreakdownItem = GlobalStatisticsBreakdown | SiteStatisticsBreakdown |
  CustomerStatisticsBreakdown | AccountStatisticsBreakdown |
  ModelStatisticsBreakdown | ChannelStatisticsBreakdown

ChannelOption {
  key: string // canonical `site_id:remote_channel_id`
  site_id: IdString
  site_name: string
  remote_channel_id: NonNegativeIdString
  name: string
  remote_missing: boolean
}

ModelOption {
  key: string // canonical `site_id:model_name`，仅用于选项行身份
  site_id: IdString
  site_name: string
  model_name: string
}

DashboardSummary {
  today: UsageSummary & { site_breakdown: SiteQuotaBreakdown[] }
  active_accounts_today: MetricString | null
  site_count: number
  online_site_count: number
  offline_site_count: number
  customer_count: number
  managed_account_count: number
  instance_count: number | null
  online_instance_count: number | null
  resource_complete_site_count: number
  resource_expected_site_count: number
  resource_stale_site_ids: IdString[]
  resource_data_status: DataStatus
  rpm: MetricString | null
  tpm: MetricString | null
  realtime_complete_site_count: number
  realtime_expected_site_count: number
  stale_site_ids: IdString[]
  realtime_data_status: DataStatus
}

RankingItem {
  dimension_type: "site" | "customer" | "model" | "channel"
  dimension_id: string
  dimension_name: string
  site_id: IdString | null
  value: MetricString | null
  data_status: DataStatus
  site_breakdown: SiteQuotaBreakdown[]
}

SiteInstanceItem {
  site_id: IdString
  node_name: string
  hostname: string
  is_master: boolean
  runtime_version: string
  goos: string
  goarch: string
  upstream_status: "online" | "stale" | "unknown"
  upstream_stale_after_seconds: number | null
  current_status: "online" | "stale" | "offline" | "unknown"
  effective_stale_after_seconds: number
  cpu_percent: number | null
  memory_percent: number | null
  disk_used_percent: number | null
  disk_total_bytes: MetricString | null
  disk_used_bytes: MetricString | null
  sampled_at: Timestamp | null
  data_status: DataStatus
  first_seen_at: Timestamp
  started_at: Timestamp | null
  last_seen_at: Timestamp | null
  last_synced_at: Timestamp
}

ResourcePoint {
  bucket_start: Timestamp
  bucket_end: Timestamp
  cpu_max_percent: number | null
  cpu_avg_percent: number | null
  memory_max_percent: number | null
  memory_avg_percent: number | null
  disk_max_used_percent: number | null
  disk_last_used_percent: number | null
  instance_count: number | null
  online_instance_count: number | null
  sample_count: number
  expected_sample_count: number
  health_status: "ok" | "warning" | "critical" | "unavailable"
  data_status: DataStatus
}

SiteResourceResponse {
  site_id: IdString
  node_name: string | null
  granularity: "minute" | "hour" | "day"
  summary: ResourcePoint | null
  trend: ResourcePoint[]
}
~~~

channel_key 解析要求 site_id 为正 IdString、remote_channel_id 为 NonNegativeIdString。options/channels 为每个适用站点合成 `site_id:0` 的“未知通道”选项，remote_missing=false；该项不写 site_channel，但可筛选和展示事实中的 channel_id=0。

导出、告警和配置：

~~~text
ExportFilters {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  granularity: "hour" | "day" | "month" | "year"
  site_ids: IdString[]
  customer_ids: IdString[]
  account_ids: IdString[]
  model_names: string[]
  channel_keys: string[] // `site_id:remote_channel_id`
  sort_by: "request_count" | "quota" | "token_used" | "active_users" | "name"
  sort_order: "asc" | "desc"
}

ExportJobItem {
  id: IdString
  format: "xlsx" | "csv"
  statistics_type: "global" | "site" | "customer" | "account" | "model" | "channel"
  filters: ExportFilters
  status: "pending" | "running" | "success" | "failed" | "expired"
  progress: number
  file_name: string
  file_size: MetricString
  row_count: MetricString
  error: AnyMessageRef | null
  data_snapshot_at: Timestamp | null
  expires_at: Timestamp | null
  created_at: Timestamp
  started_at: Timestamp | null
  finished_at: Timestamp | null
  deduplicated: boolean
}

AlertRuleConstraints {
  value_kind: "boolean" | "percentage" | "seconds" | "count" | "quota"
  threshold_editable: boolean
  threshold_min: string | null
  threshold_max: string | null
  threshold_step: string | null
  for_times_editable: boolean
  for_times_min: number
  for_times_max: number
  paired_rule_id: IdString | null
  relation: "warning_lt_critical" | null
}

AlertRuleItem {
  id: IdString
  effective_rule_id: IdString
  base_rule_id: IdString
  override_rule_id: IdString | null
  rule_key: string
  name: string
  enabled: boolean
  level: "info" | "warning" | "critical"
  metric: string
  compare_operator: ">=" | "<=" | "=="
  threshold_value: string | null
  for_times: number
  scope_type: "global" | "site"
  scope_id: "0" | IdString
  inherited: boolean
  editable_fields: ("enabled" | "threshold_value" | "for_times")[]
  constraints: AlertRuleConstraints
  updated_at: Timestamp
}

AlertEventItem {
  id: IdString
  rule_id: IdString
  rule_key: string
  site_id: IdString | null
  site_name: string
  target_type: "site" | "instance" | "account" | "collection"
  target_key: string
  target_name: string
  level: "info" | "warning" | "critical"
  status: "pending" | "firing" | "resolved"
  current_value: string | null
  threshold_value: string | null
  message: AnyMessageRef
  first_observed_at: Timestamp
  first_fired_at: Timestamp | null
  last_fired_at: Timestamp | null
  resolved_at: Timestamp | null
}

AlertSummary {
  firing_count: number
  critical_count: number
  warning_count: number
  resolved_today_count: number
  updated_at: Timestamp
}

AlertEventDetail = AlertEventItem + {
  consecutive_count: number
  deliveries: {
    id: IdString
    event_type: "firing" | "resolved" | "test"
    status: "pending" | "success" | "failed"
    attempt_count: number
    error_code: string
    response_code: number | null
    response_message: string
    next_retry_at: Timestamp | null
    sent_at: Timestamp | null
  }[]
}

SettingItem {
  key: string
  value_type: "int" | "decimal" | "bool" | "string" | "json"
  value: string | number | boolean | object | null
  read_only: boolean
  secret: boolean
  configured: boolean
  decrypt_error: boolean
  masked_value: string
  constraints: object
  updated_at: Timestamp | null
}

SettingGroup {
  key: string
  label_key: string
  items: SettingItem[]
}

SettingPatchRequest {
  items: { key: string, value?: string | number | boolean | object, clear?: boolean }[]
}

max_file_bytes、min_free_disk_bytes 和 decimal 设置的 value 仅使用 string 分支，其他 int 仅使用 number 分支；请求 item 不接受 updated_at/version 字段。system.public_origin 仅出现在 SettingItem 响应中。SettingSLOReasonCode 是机器标志，不等同于缺少 params 的 MessageRef。

DashboardHealth {
  firing_alert_count: number
  critical_alert_count: number
  warning_alert_count: number
  auth_expired_site_ids: IdString[]
  statistics_not_ready_site_ids: IdString[]
  yesterday_validation_status: DataStatus
  completeness: Completeness
  latest_alerts: AlertEventItem[]
}

NotificationTestResult {
  delivery_id: IdString | null
  status: "success" | "failed"
  response_code: number | null
  message: AnyMessageRef
}
~~~

测试接口通过 AdminAuth 和请求校验后，发送成功、远端发送失败以及本地配置预检失败都返回 HTTP 200、ApiResponse.success=true/code=""，由 NotificationTestResult.status/message 表达测试结果；成功时 message.code=`NOTIFICATION_TEST_SUCCEEDED`；首次 429/5xx/网络失败且 delivery 仍为 pending 时 status=failed、message.code=`DELIVERY_RETRY_SCHEDULED`，params 携带 delivery_id 和 Unix 秒十进制字符串 next_retry_at；仅真正耗尽时使用 `DELIVERY_RETRY_EXHAUSTED`。只有认证、权限、请求格式或未分类服务故障使用失败 envelope。这样页面可在没有 delivery 行时展示确定的本地化失败，而不会把“测试未通过”误当成接口传输失败。

列表排序白名单：sites=`priority,name,today_quota,updated_at`；customers=`updated_at,name,today_quota,account_count`；accounts=`updated_at,username,today_quota,quota`；platform users=`created_at,username,last_login_at`；alerts=`last_fired_at,first_fired_at,level,status`；collection runs=`created_at,started_at,priority,status`；exports=`created_at,finished_at,status,file_size`。未知 sort_by 返回 VALIDATION_ERROR，禁止拼接任意 SQL 列名。

---

## 34. 调度、导出和通知 Worker

### 34.1 调度启动和停机

- 进程启动顺序：配置与安全边界校验（含 Redis DSN）→ 日志 → MySQL 版本检查 → 版本化 SQL migration → 幂等 seed → Redis 客户端 → 恢复超时任务 → HTTP Router → Scheduler；
- Scheduler 启动后立即执行一次探活、资源和 RPM/TPM，不等待第一个 ticker；
- 调度器检查 ticker 固定为 60 秒；站点状态检查、实时统计和资源采集分别使用对应系统设置周期，每站点加入 0～10 秒稳定抖动，抖动由 site_id 计算，重启后保持稳定；`performance_sync/topup_sync/redemption_sync/upstream_task_sync/model_meta_sync/plan_sync/pricing_group_sync/system_task_sync` 八个 durable task 统一复用资源采集周期；
- 小时用量按 usage_delay_minutes 生成上一个完整小时任务，默认 H+5；生产发布要求 delay<=5 且 usage_concurrency>=5；
- 持久化任务入队前生成 active_key、priority 和 next_attempt_at；唯一键冲突时复用已有任务 ID；任务进入 success/failed 终态时清空 active_key。周期 probe/realtime/resource 使用有界内存队列与 per-site 锁，不逐次插 collection_run；每次完成后尽力写 Redis 有界历史，写入失败只记录脱敏日志和 readiness/metrics，不改写快速任务已经得到的结果。手动触发、失败状态变化和全部窗口任务仍持久化；
- Worker 只领取 `status=pending AND next_attempt_at<=now` 且窗口型任务 windows_initialized_at 非空的父任务，按 priority 降序、created_at 升序选择；事务按 §31 全局锁序先锁 scope 对象、再 `SELECT collection_run FOR UPDATE SKIP LOCKED` 并改 running。窗口型父任务再按相同锁序以短事务领取 eligible run-window：父行先于子行，原子改 running、attempt_count+1、写 started_at/heartbeat 后提交，才允许在事务外请求上游或重建本地桶；
- collection_run 创建时保存 created_request_id 和当前 site_config_version；每次领取/重试生成新 attempt/CAS token 写 last_request_id，心跳、进度和结果提交都必须匹配该 token。collection 没有独立 lease 列，以 heartbeat_at 判定过期；普通窗口任务单次领取最多处理 24 个窗口或 30 秒后让出；站点首次 `usage_backfill` 为避免千级历史窗口被切成大量批次，单次领取可覆盖该 run 的全部窗口，但窗口执行并发使用热加载的 `collector.backfill_concurrency`，不再存在绕过配置的固定 10 并发。所有远端 HTTP 还必须经过 §30.1.1 的 origin governor；窗口不设置包含 governor 排队时间的外层短超时，每个真正发出的 HTTP 仍使用 §30.1 的固定 30 秒超时，claim context 取消和持续心跳负责生命周期边界，避免 180 秒或最长一小时的 429 冷却在尚未发送 HTTP 时消耗窗口重试次数。历史只读采集按 HTTP 结果分类：收到 2xx（包括空集合、数据不一致或 2xx 响应结构错误）立即结束窗口；非 2xx 响应、连接失败和请求超时才允许重试。
- 单实例启动时先扫描 status=pending 且 windows_initialized_at=null 的窗口 run，并按 §32.9 的 scope→parent 锁序与 config/lifecycle CAS 继续物化，再无条件接管全部 collection/export running 任务（旧进程已不存在）；运行期间每 60 秒执行相同补偿。collection 以 heartbeat 超过 5 分钟判定过期，回收严格按 scope 对象→父 run→run-window 锁序，窗口型只按 attempt_count、非窗口只按父 retry_count 判断预算，均不重复增加领取时已计入的尝试。正常 Quiesce/Stop、父 context 取消、心跳更新失败或任一窗口提交失败时，当前 owner 必须先停止派发并以 run_id + last_request_id CAS 把自己仍持有的 running 窗口释放为 pending；只有进程崩溃、强杀、数据库持续不可用或 owner 已丢失时才允许等待 5 分钟 Reaper 回收。心跳失败、释放失败和提交失败必须记录稳定错误分类、run/window/site/request ID，禁止只记录 Go error 类型。export 启动接管全部 running，运行期只接管 lease_expires_at 已过期的 running，删除半成品后按已计入的 attempt_count 决定 pending/failed。alert_delivery 始终保持 pending，fresh claim 和过期 lease takeover 由 claim_token/lease_expires_at 区分。这样创建/物化/执行任一断点都不会留下孤儿或永久占用，且旧 token 不能覆盖新 owner 或 fence 后终态；
- 优雅停机先停止创建新任务，再等待运行任务最多 30 秒；超过 drain deadline 后取消执行 context，当前 owner 在 handler 返回时把仍持有的 running 工作 CAS 释放为 pending；只有进程没有机会执行释放时才由下次启动无条件接管；
- 所有 ticker 使用 time.Ticker，不使用无限 time.Sleep 循环；
- 单实例进程内互斥键按任务的逻辑 family 和实际 scope 派生，不能统一写成 task_type:site_id：站点任务按 site、用量任务按 site+hour、account/customer 重建按 customer 串行。逻辑 family 用于幂等/互斥，executor queue 用于并发隔离，两者不是同一概念；数据库行锁仍是最终正确性边界。

任务类型是常量包、重试、API 筛选和 F12 的共同权威枚举。产品分类、触发资格和历史语义以 02 §7.1.1 为准；下表只重述 Worker 所需的 target/window 契约，不再把 executor queue 与逻辑锁 family 混为一列：

| task_type | category | target | 窗口/游标 | 持久策略 |
|---|---|---|---|---|
| site_probe | fast | site | 否/否 | 正常周期 Redis 有界历史；恢复/手动/诊断 durable |
| realtime_stat | fast | site | 否/否 | 正常周期 Redis 有界历史；恢复/手动/诊断 durable |
| resource_snapshot | fast | site | 分钟/否 | 正常周期 Redis 有界历史；业务资源事实持久化 |
| performance_sync | durable | site | 最近窗口/无游标 | collection_run + 性能事实 |
| topup_sync | durable | site | 快照/无游标 | collection_run + 充值事实 |
| redemption_sync | durable | site | 快照/无游标 | collection_run + 兑换事实 |
| upstream_task_sync | durable | site | 重叠窗口/无游标 | collection_run + 上游任务事实 |
| model_meta_sync | durable | site | 快照/无游标 | collection_run + 模型目录 |
| plan_sync | durable | site | 快照/无游标 | collection_run + 计划目录 |
| pricing_group_sync | durable | site | 双资源快照/无游标 | collection_run + 定价/分组目录 |
| system_task_sync | durable | site | list/current 快照/无游标 | collection_run + 任务事实/resource state |
| user_sync | hourly | site | 小时快照/无游标 | collection_run + 用户库存/汇总 |
| channel_sync | hourly | site | 小时快照/无游标 | collection_run + 渠道库存/汇总 |
| log_sync | hourly | site | 重叠窗口/完整性游标 | collection_run + 日志事实 |
| usage_hour | usage | site | 小时/共享 usage 游标推进 | 父任务 + run-window |
| usage_backfill | usage | site | 小时/共享 usage 游标推进 | 父任务 + run-window |
| usage_validation | usage | site | 小时/回退或连续 complete 推进 usage 游标 | 父任务 + run-window |
| account_rebuild | rebuild | account | 小时 run-window/无站点游标 | 本地父任务 + run-window |
| customer_rebuild | rebuild | customer | site+小时 run-window/无站点游标 | 本地父任务 + run-window |

site disable/enable/manual/schedule/recovery 通过 trigger_type 和 priority 区分，不再发明新的 task_type。export_job 和 alert_delivery 使用各自表与队列，不写入 collection_run。未知 task_type 在数据库读取或 API 筛选时视为 INTERNAL_CONTRACT_ERROR/VALIDATION_ERROR，不能静默执行。

### 34.2 队列优先级

下表只用于共享的用量/回填/校验/本地重建队列。探活、RPM/TPM、资源、用户/通道元数据、导出和钉钉各自有独立并发队列，不与该表比较数值。

| 优先级 | 任务 |
|---:|---|
| 100 | 上一完整小时实时采集 |
| 90 | 站点恢复缺口补采 |
| 80 | 手动补采 |
| 70 | 次日校验 |
| 60 | 每周校验 |
| 50 | 新站点历史回填 |
| 40 | 新账户/客户本地重建 |

同优先级按窗口时间倒序执行，使最新数据优先可见；首次全历史回填的站点内部仍按时间正序推进连续游标。长范围任务分片让出后重新参与优先级排序，不能持续占住 Worker。

### 34.3 重试

| 类型 | 最大尝试 | 间隔 |
|---|---:|---|
| 小时采集 | 4 | 立即、1m、5m、15m |
| 历史回填 | 5 | 立即、1m、5m、15m、60m |
| 次日/每周校验 | 5 | 立即、5m、15m、60m、6h |
| 导出 | 2 | 立即、1m |
| 钉钉 | 5 | 立即、1m、5m、15m、60m |

401/403 权限错误不重试并使站点 auth_status=expired；429 优先使用 Retry-After，但不超过 1 小时；参数或接口契约不兼容不重试；网络、超时和 5xx 可重试。NewApiClient 单次调用无内部重试，因此表中“最大尝试”就是实际 HTTP 尝试上限。窗口任务在每次 claim 时增加 collection_run_window.attempt_count，失败只更新 next_retry_at/status；非窗口 collection_run 在 claim 时增加 collection_run.retry_count 并更新 next_attempt_at；export_job 在 claim 时增加 attempt_count。租约回收不重复增加已领取尝试次数；不为一次任务的每次尝试创建新 run。

重试排期不得只依赖相同 attempt 的固定延迟。持久化 `next_retry_at` 在既有分级退避基础上增加由 task_type、site_id、run_id、window hour 和 attempt 计算的确定性抖动，抖动范围为基础延迟的 20%，同一窗口重放结果稳定，不同窗口不得形成同秒重试波峰。429 即使没有 `Retry-After`，origin governor 也会先完成 §30.1.1 的整窗冷却；任务到期只表示可以重新进入公平队列，不表示可以立即发出 HTTP。首次历史回填的千级窗口在任何 attempt 都受同一匀速门控，禁止一次失败后同批窗口同步冲击上游。

### 34.4 导出 Worker

- 单实例运行 1 个导出 Worker；
- 创建任务使用进程内 export-create mutex 包住“事务内统计单用户/全局活跃数 + active_key 去重 + INSERT”，防止并发请求同时突破上限；单实例约束下不引入分布式锁；
- 事务中选择 `status=pending AND next_attempt_at<=now` 的最早任务 FOR UPDATE，fresh claim 改 running、attempt_count+1，写入随机 claim_token 和 5 分钟 lease_expires_at 后提交，再生成文件；
- 导出再次执行 StatisticsService 的同一查询构造器，禁止复制另一套统计口径；
- Worker 首次尝试时一次性读取并写入 export_job.rate_snapshot，冻结本任务涉及站点的有效/兜底汇率；同一 job 重试不得覆盖该汇率快照；
- 每次生成尝试开启 MySQL `REPEATABLE READ READ ONLY` 一致性事务，在首次查询后写 data_snapshot_at；全部统计页都在同一事务内读取。失败重试会建立新的数据快照并覆盖 data_snapshot_at，但仍使用原 rate_snapshot；成功文件只对应一个完整快照；
- export_job 的 progress、heartbeat/lease、attempt/status 和 data_snapshot_at 通过独立短事务控制连接更新，不在长只读快照事务中写；所有更新都必须匹配 running 状态和 claim_token，旧 owner 的迟到更新返回 claim lost。第一次 retryable 失败删除半成品后回 pending 并保留 active_key，只有预算耗尽/不可重试才 failed 并清 active_key；
- 查询按稳定唯一复合键做 keyset 分页，每批 5,000 行；禁止使用会因并发重建产生重复/遗漏的 OFFSET 分页。CSV 流式写入 UTF-8 BOM 文件；
- XLSX 单工作表最多 1,000,000 数据行，超过后创建 Data-2、Data-3；
- 数字精确列以文本写入 request_count、quota、token_used，避免 Excel 科学计数损失；
- 所有用户或上游可控文本列以文本单元格写入；CSV 对去除前导空白后以 `= + - @` 开头的值前置单引号，防止公式注入；XLSX 明确设置字符串类型，不生成公式；
- 每个站点分项输出 site_id、site_name、quota_per_unit、usd_exchange_rate、amount_usd、amount_cny、rate_time；
- amount_usd = quota / quota_per_unit；
- amount_cny = amount_usd × usd_exchange_rate；
- 金额列使用高精度 decimal，显示 6 位小数，原始内部计算至少 10 位；
- 文件名格式 statistics-{type}-{start}-{end}-{job_id}.{ext}；
- 文件只写配置的 EXPORT_DIR，下载通过 job ID 映射，禁止接受任意路径；
- EXPORT_DIR 启动时要求仅服务账户可写；Linux 目录权限建议 0700、文件 0600，不跟随目录内符号链接；
- 开始生成和每次刷新进度时检查磁盘剩余空间与最大文件大小；低于 min_free_disk_bytes 或超过 max_file_bytes 时终止并删除半成品；
- 每处理一页更新 progress、heartbeat_at 和 lease_expires_at；启动无条件接管所有 running 导出，运行期只接管 lease_expires_at 已过期的 running。接管先清 claim_token/lease、删除旧临时文件，并按 fresh claim 已预留的 attempt_count 置 pending/failed，不重复递增；
- 成功后 expires_at=finished_at+export.file_ttl_hours（默认 24h）；每日 03:30 清理过期文件；
- 生成失败删除半成品临时文件，最终重命名必须是同文件系统原子操作；
- 页面轮询任务状态时 progress 为 0～100，success/failed/expired 后停止轮询。

导出固定列：

| 顺序 | 列 |
|---:|---|
| 1～4 | scope_type、scope_id、scope_name、site_id/site_name |
| 5～7 | bucket_start、bucket_end、timezone |
| 8～11 | request_count、quota、token_used、active_users |
| 12～16 | quota_per_unit、usd_exchange_rate、rate_source、amount_usd、amount_cny |
| 17～21 | data_status、is_final、as_of、data_snapshot_at、exported_at |

不适用字段留空，不改变列顺序。

### 34.5 钉钉 Worker

- 单实例运行 1 个投递 Worker；
- alert_delivery 不使用 running 状态：fresh claim 时状态仍为 pending，attempt_count+1 并写随机 claim_token/lease_expires_at；只有租约过期后才允许 takeover，takeover 替换 token 但复用已经预留的 attempt_count。完成、失败和重试排期必须匹配 claim_token，旧发送者不得提交；
- alert_event 与 alert_delivery 在同一事务内创建；
- 同一事务还写入不可变 payload_snapshot；所有重试和 takeover 都发送该快照，不重新拼装已经变化的事件、规则或站点名称；
- 文本消息使用 Markdown，标题包含 [Critical] 或 [Warning]；
- 配置 secret 时按钉钉规范使用 timestamp + HMAC-SHA256 生成 sign；
- Webhook 只允许 HTTPS 且 host 必须为 `oapi.dingtalk.com` 或部署配置的钉钉域名白名单；禁止重定向到其他 host；
- HTTP 超时 10 秒，响应 HTTP 2xx 且 JSON errcode=0 才算成功；
- Webhook、secret、签名串和完整响应不得写普通日志；
- response_message 最多保存 2,000 字符并移除 URL 查询参数；
- 相同 alert_event_id + event_type + channel 唯一键防止重复发送；
- resolved 通知必须包含持续时长和恢复时间；
- 详情链接由 PUBLIC_ORIGIN + 前端告警路由生成；非生产环境未配置时省略链接；
- 测试消息 alert_event_id=NULL、event_type=test，每次点击生成独立 delivery；
- 测试接口先同步校验 enabled/configured/地址安全；预检失败返回 HTTP 200、status=failed、delivery_id=null 和对应 AnyMessageRef，不插入 delivery。预检通过后插入 alert_event_id=NULL 的独立 delivery 并同步等待第一次发送结果，此后的发送失败返回非空 delivery_id 且保留记录供排查。

### 34.6 告警评估

每次快速周期的探活、资源、RPM/TPM 提交，以及每次完整 `user_sync`/`channel_sync` 小时快照提交后评估对应规则。连续次数由 alert_event.consecutive_count 保存，进程重启不丢失。阈值从单站覆盖回落全局默认；Warning 和 Critical 同时满足时只保留 Critical，已有 Warning firing 先 resolved，再创建 Critical firing。恢复条件使用阈值本身，不额外引入迟滞区间。

规则适用条件：disabled 站点不评估在线、实例和采集告警；未授权站点不评估需授权的资源/采集告警，只有 expired 触发授权告警；data_export_enabled=false 时保留导出开关告警，不评估用量采集/回填/校验告警；archived 账户或 disabled 客户不评估账户告警；实例接口请求失败按 unknown 处理，不以 0 个实例触发告警；remote_state=missing/identity_mismatch 的账户不重复触发 quota 或禁用告警。site_offline 直接依据 probe_fail_count>=3 触发一次，不在 online_status 变为 offline 后再次累计三次。目标变为不适用时自动 resolved 并记录 scope_inactive。

内置规则评估矩阵：

| rule_key | canonical target | 命中/恢复 | 触发时机 |
|---|---|---|---|
| site_offline | site_id | probe_fail_count>=3 / 任一次成功探活 | 探活事务后 |
| site_auth_expired | site_id | auth_status=expired / authorized；unauthorized 从未授权不命中 | 授权状态变化后 |
| site_export_disabled | site_id | 已授权 active 站点开关=false / true | status 探活后 |
| collection_missing | site_id/hour_ts | fact window=missing / complete 或范围退出 expected | 窗口事务后 |
| backfill_failed | site_id/run_id | run=failed / 原 run 范围内所有失败事实窗口后续均 complete/unavailable | run 终态及修复事务后 |
| validation_failed | site_id/hour_ts | DATA_VALIDATION_MISMATCH（failure_kind=data_mismatch）或 usage_validation run-window 终态 failed（failure_kind=execution_failed） / 后续同小时 validation run-window success 且事实窗口 complete | 每次验证尝试终态事务后 |
| instance_stale | site_id/node_name | 距 last_seen 超有效阈值 / 明确 online 样本 | 资源成功样本后 |
| instance_offline | site_id/node_name | 连续 3 个成功样本非 online / 明确 online 样本 | 资源成功样本后 |
| site_no_instance | site_id | 成功快照 instance_count=0 / >0；请求失败为 unknown | 资源成功样本后 |
| cpu_high/memory_high/disk_high | site_id/node_name | 数值阈值与 for_times / 明确低于阈值 | 资源成功样本后 |
| account_missing | account_id | remote_state=missing / normal | 完整用户同步后 |
| account_identity_mismatch | account_id | identity_mismatch / 对象归档或删除后 scope_inactive | 完整用户同步后 |
| account_disabled/account_quota_empty | account_id | 明确属性命中 / 明确健康属性 | 完整用户同步后 |
| channel_balance_low | site_id | balance_total<=有效阈值 / >有效阈值 | complete channel_sync 小时快照后 |
| channel_response_time_high | site_id | response_time_avg_ms>=有效阈值 / <有效阈值 | complete channel_sync 小时快照后 |
| channel_availability_low | site_id | availability_rate<=有效阈值 / >有效阈值；Warning 默认连续 3 个 complete 小时快照 | complete channel_sync 小时快照后 |

成功且完整的实例快照是实例 scope 的权威目录。本轮未出现的既有 active node 写 retired_at，并在同一状态收敛中以 resolution_reason=retired 结束其 pending/firing 实例告警，不发送 recovered 通知；失败、非法或 unknown 轮次不退役节点，也不结束告警。已退役 node 重新出现时清 retired_at，从新样本重新累计；不能复活旧事件。

探活、资源、用户、渠道小时快照、窗口、授权和 lifecycle 的源事务成功提交后，post-commit coordinator 使用独立的有界 context 按本次 observed_at/row identity 读取精确 committed snapshot，再调用统一 evaluator；不得在源事务提交前发送通知，也不得把“最新一行”误当成触发本次 hook 的样本。post-commit 失败只记录脱敏错误，不回滚已经提交的业务事务。渠道缺行、config_version 不匹配、超过 2 小时未更新或非 complete 样本为 unknown；unknown 不推进 cursor、不累计、不恢复。

规则状态变更、alert_event/alert_delivery 以及 alert_evaluation_cursor 的推进必须在同一事务内完成；Warning 与 Critical 切换时先把旧 active_key 置 NULL，再创建新事件。cursor 以 canonical active_key 为主键，保存 last_sample_at + last_sample_key：同 key 重放幂等忽略，更早样本忽略，同一时间戳但不同 key 视为契约冲突并拒绝；unknown 样本不推进 cursor，避免未知状态吞掉随后可确认的同一业务样本。另有每 5 分钟扫描当前 committed 状态的兜底评估，用于收敛提交后崩溃或 hook 失败；扫描使用同一 evaluator 和同一 cursor 规则，不实现第二套状态机。

### 34.7 数据清理

02:00 次日业务校验和 03:30 资源分钟 retention 延续既有调度。D141 另外冻结授权触发与四类数据维护操作；`operation_id/category/trigger_class` 是 F13 和 docscheck 使用的稳定功能标识，不是 Worker 函数名：

<!-- DATA_MAINTENANCE_CATALOG_START -->
| operation_id | category | trigger_class | 功能/默认触发 | 幂等、水位与批次 | 失败/fence | 完成结果 |
|---|---|---|---|---|---|---|
| `authorize_pricing_group_sync` | `authorization_trigger` | `authorize_post_commit` | 授权成功且 required capability ready 后，事务提交后立即幂等 enqueue `pricing_group_sync` | active_key 去重；同一 site/config_version 重放复用已有 run | 创建或执行失败不回滚授权；只记录稳定安全诊断；任务领取/提交复核 config fence | 返回或诊断可追踪 run_id，目录同步独立收敛 |
| `resource_daily_finalize` | `resource_maintenance` | `beijing_daily` | 默认 03:00，以北京时间前一完整自然日为范围，重建 instance/site resource daily 并 finalize | 可控 Clock；持久日水位；按 site/date 稳定批次；同日重放结果一致 | 单 site/batch 失败保留水位未完成，不把该日整体标 complete/final；生命周期/config fence 变化时丢弃旧 scope | 所有应处理 scope 成功且 sample/expected/data_status 固化后才推进日水位并置 final |
| `resource_rollup_gap_repair` | `resource_maintenance` | `beijing_daily` | 默认 03:20，扫描存在 expected/分钟事实但缺失或未固化的 resource hourly | 可控 Clock；复合游标严格为 `(resource_kind,site_id,node_name,bucket_start)`；kind 顺序固定 `instance_hourly` 后 `site_hourly`，site kind 的 node_name 固定空串，其他字符串按 `utf8mb4_bin` 字节序；F13 每批 2；逐桶短事务 | 部分失败不推进该 item 游标；不伪造零，不改正确 complete 的任何字段/时间戳，不改 daily/final；提交复核 config/lifecycle revision | 仅缺失或 sample/expected/status 未固化的 hourly 被幂等修复 |
| `collection_run_error_redaction` | `retention` | `retention_scan` | 默认 04:00，处理 finished_at 超过 90 天且仍含 error_message 的 collection_run | cutoff 来自可控 Clock；按 id 游标有界批次；重复清空幂等 | 单批失败不推进该批游标；不得删除 run、窗口、稳定 code/params、状态、范围或计数 | 仅 error_message 详细文本置空，审计骨架永久保留 |
| `metadata_diagnostic_run_cleanup` | `retention` | `retention_scan` | 自动清理 finished_at 超过 30 天的成功 schedule `user_sync/channel_sync` 诊断 run | cutoff 来自可控 Clock；按 id 游标有界批次；重复扫描幂等；windowless 同时要求 parent range 为空且 `NOT EXISTS collection_run_window` | 失败、手动、任何存在 child window 的异常/旧数据、窗口型、本地重建、非目标 task_type 或非 success run 永不由该操作删除；不得先删 child 再使 parent 命中 | 仅符合全部谓词的自动成功诊断 run 被清理 |
<!-- DATA_MAINTENANCE_CATALOG_END -->

上述 daily 操作按北京时间日期建立持久水位，不能仅用进程内 `lastRunDate`；进程在默认时点后启动必须补跑未完成日期，同日崩溃恢复从持久游标继续。每个批次在短事务中提交，任何部分失败都不得提前推进日水位或把日/桶标记 complete/final。F13 使用可注入 Clock 固定日期、cutoff、批次边界和重启点。

daily finalize 采用两阶段原子发布：阶段 1 按 site 短事务重建两类 daily，但当日所有目标行保持 `is_final=0`；只有全 expected scope 重建/守恒校验通过且 global scope_revision 稳定后，阶段 2 才在单一发布事务中复核目标行数与 revision，并把当日全部 instance/site daily 原子置 `is_final=1` 后推进 complete 水位。任一站点或批次失败时，包括此前已成功重建的站点在内，当日全部 daily 仍为 unfinalized。

resource `scope_revision` 按 03 §14.3 的规范元组计算。每个 item 提交采用 `site -> 相交 pause/lifecycle -> maintenance state -> aggregate row` 锁序并复核 site slice revision；全扫描完成时重算 global revision，若与持久值不同则清空 `(cursor_kind,cursor_site_id,cursor_node_name,cursor_bucket_start)` 从头重扫。该规则覆盖前一日结束后补录 pause、retire/reappear 或 config 变化，禁止以“历史日期已结束”为由跳过 fence。

调度相位冻结为：02:59 不运行四个 daily/retention operation；03:00 只可运行 `resource_daily_finalize`；03:19 仍不得运行 gap；03:20 增加 gap；04:00 再增加两个 retention。五个 operation 状态和失败相互独立，本轮任一失败仍须尝试其余 due operation，最终以 `errors.Join` 返回；仅失败者不推进自己的水位。启动恢复最多处理 20 个 authorization intent，普通 wake/tick 每轮最多 100 个，剩余 backlog 由后续 wake/tick 继续，不能无限阻塞 ready。starting 阶段即建立 cancel/done；parent cancel、Quiesce 或 Stop 后不得 ready 反弹、double-close 或悬挂，即使 processor 延迟响应 cancel 也必须在释放后稳定收敛。

过期导出文件、日志事实和 terminal system-task 继续由各自自动维护收敛，清理失败可重试；D141 不冻结这三者的具体墙钟分钟，只冻结 TTL/retention、终态适用条件与最终清理结果。

业务 hourly/daily、collection_window、collection_run_window（随其永久 run）、告警事件和平台实体永久保留。

---
#### 授权后立即探测要求

授权事务提交成功后必须触发一次可追踪的 immediate site_probe；探测不得在授权事务内执行，避免事务回滚时产生外部副作用。探测失败不回滚已保存的授权或阻塞 `usage_backfill`，站点保留 `online_status=unknown`（或按既有失败阈值转为 offline），并记录可查询的失败诊断；探测成功后更新在线状态和 `last_probe_success_at`。

所有采集任务类型使用独立并发配额和同类型租约。`usage_backfill`、`usage_validation`、`account_rebuild`、`customer_rebuild` 各自拥有独立 limiter/metrics key；未配置专属值时沿用 `collector.backfill_concurrency` 作为各队列默认上限。站点首次授权产生的 `usage_backfill` 保留独立 initial-backfill limiter，避免占用普通 backfill 队列槽位，但其 run 并发和窗口并发都必须使用热加载的 `collector.backfill_concurrency`，默认 2；首次回填 claim 一次领取该 run 的全部窗口，避免 24 窗口分片导致单个慢窗口阻塞后续批次，远端请求再受 §30.1.1 的 origin 匀速、公平与在途上限约束。领取查询必须按 `CollectionPriorityInitialBackfill` 过滤，避免首次回填槽位耗尽时误领取普通回填。站点级 `usage_backfill` 运行时不得阻塞 `user_sync`、`channel_sync`、`site_probe`、`realtime_stat` 或 `resource_snapshot`；同一任务类型仍由 `active_key` 去重并受站点配置版本栅栏约束。
# P0-B 日志 API 与 Worker

Worker 新增 `log_sync` 任务类型，按跨整点的 hourly cadence 使用 site/config_version fence、有限重叠时间窗口和稳定去重；进程在小时中途启动时等待下一整点，任务失败遵循现有 retry/lease 规则。API 新增 `GET /api/logs` 与 `GET /api/sites/:id/logs`，筛选字段与 `/api/log/` 对齐但采用平台 snake_case DTO，权限沿用站点范围授权。新增日志导出 scope `logs`，复用 `export_job` 生命周期和文件 TTL。
# P0-C 用户库存 API 与 Worker

`user_sync` 保持 metadata 队列、小时 cadence、active_key、config fence 和重试预算；handler 的 fetched_rows 为完整远端用户数，written_rows 包含库存/汇总/account 受影响行。API 统一使用 UserAuth，viewer 可读、写接口不存在；列表 page_size 最大 100、查询跨度最大 1 年，排序列白名单固定。

export_job 新增 `user_inventory`，与日志/统计导出共享 owner、claim、heartbeat、lease、文件 TTL、大小和磁盘门槛。
# P0-D 渠道运营 API 与 Worker

P0-C 补充：用户库存列表支持 `remote_user_id` 规范正 bigint 字符串精确筛选。该参数与 keyword（仅 username/display_name 模糊匹配）相互独立，适用于全局和站点列表接口；`statistics_type=user_inventory` 导出必须冻结并透传同一 `remote_user_id`，不得退化为未筛选导出。

只读接口：`GET /api/channel-inventory`、`GET /api/channel-inventory/statistics`、`GET /api/sites/:id/channel-inventory`、`GET /api/sites/:id/channel-inventory/statistics`。列表支持分页及 site/type/status/group/tag/state/keyword/balance/response_time 筛选；统计支持左闭右开整点范围和站点/type/status/group/tag 筛选。所有接口使用统一响应 envelope、viewer 可读、强制改密期间禁止，并返回稳定完整性状态。

周期 `channel_sync` 调用完整分页 SnapshotChannels 后通过原子仓储提交，不再只替换 name/status 元数据。成功后可发布渠道告警采样触发；采集失败沿用任务分类、重试、租约和 config fence，不写部分事实。导出 worker 新增 `channel_inventory` scope，使用 repeatable-read 数据快照、现有 claim/heartbeat/lease/TTL/publish 生命周期。

成功 channel_sync 提交后以 `site_id + hour_ts + collected_at` 读取精确小时行并发布 `channel` post-commit 样本；五分钟扫描读取每站最新小时行作为兜底。渠道 evaluator 只消费 `channel.balance_total`、`channel.response_time_avg_ms`、`channel.availability_rate`，规则、event、cursor 和 delivery 与其他内置告警在同一事务收敛。全局规则可被站点 override；同一事件的 firing/resolved delivery 继续由 `(alert_event_id,event_type,channel)` 唯一键去重，详情链接仍为 `/alerts?alertId=<id>`。
# P0-E 性能历史 API、Worker 与保留

新增任务 `performance_sync`，调度周期复用可热更新的资源采集周期，独立 active_key/lease/concurrency/retry；每轮采集最近 24 小时官方 buckets 并按唯一键幂等覆盖。`performance.retention_days` 默认 90、范围 1～3650，成功提交后删除 cutoff 前历史；清理失败不得撤销已提交快照，但进入可观测错误与下轮重试。

API：`GET /api/performance-history`、`GET /api/performance-history/statistics`、`GET /api/sites/:id/performance-history`、`GET /api/sites/:id/performance-history/statistics`。viewer 可读、强制改密禁止，统一 envelope；单站 official_average 可用，跨站无 counters 时 summary 明确 unavailable，绝不返回伪加权值。statistics 单次候选事实超过 100,000 行时返回 HTTP 413 `PAYLOAD_TOO_LARGE`，不得截断后标记 complete；导出继续在 repeatable-read 数据快照内按冻结的 site/model/group/range 筛选生成，仅受文件大小和磁盘门槛约束，不设置未文档化的事实行数硬上限。

# F1 充值/兑换码 API、Worker 与导出

新增 `topup_sync`、`redemption_sync` 周期任务，独立 required task type、队列、lease、max attempts=3 和 resource concurrency。API 为 `GET /api/topups`、`GET /api/topups/statistics`、`GET /api/sites/:id/topups`、`GET /api/sites/:id/topups/statistics` 以及对应 `/redemptions` 四个接口；统一 envelope，viewer 可读，站点接口强制 site filter。列表支持 site、remote/user ID、status、provider/method、时间、remote_state 等安全筛选。

统计必须返回 completeness/site_breakdown；充值只允许逐站/provider nominal totals，禁止跨站金额 summary。导出 runtime 接受 `topup_inventory` 与 `redemption_inventory`，校验冻结筛选并生成安全 CSV/XLSX。OpenAPI/MessageRef、日志、错误 params、数据库模型、DTO 和导出均执行敏感字段 absence scan。

# T1 任务 API 与 Worker

新增 `upstream_task_sync` required task、metadata queue、resource concurrency、独立 lease/active_key/max attempts=3。API：`GET /api/upstream-tasks`、`/statistics` 及对应站点四接口；支持 site/task/platform/user/group/channel/action/status/model/submit time 筛选。export runtime 接受 `upstream_tasks`。所有 bigint 为字符串，统一 envelope，viewer 可读。
平台新增 `/api/model-catalog`、`/api/model-catalog/coverage`、`/api/model-catalog/missing` 及对应 `/api/sites/:id/...` 只读接口；导出类型 `model_catalog` 复用异步导出运行时。
本地排行提供 `/api/rankings/models`、`/api/rankings/vendors` 与 `/api/sites/:id/rankings/...`，支持 `period=today|week|month|year`；`statistics_type=model_rankings|vendor_rankings` 支持 CSV/XLSX。
新增 `/api/subscription-plans`、`/api/subscription-plans/statistics` 与对应站点接口；无用户订阅/订单全局聚合端点。
# D138 定价与分组目录 API/Worker

提供 `GET /api/pricing-catalog`、`GET /api/pricing-catalog/statistics`、`GET /api/group-catalog`，以及三个对应的 `/api/sites/:id/...` 强制站点端点。全局端点可接收 `site_ids`，站点端点拒绝 `site_ids`；列表统一服务端分页，pricing statistics 与 pricing list 使用同一安全筛选语义，group 的计数与 completeness 随 catalog 响应返回。pricing item 仅包含 site identity、model/vendor、exact decimal pricing/group ratio、usable groups、supported endpoints、remote state 与采集元数据；group item 仅包含 site identity、group name、remote state 与采集元数据。

顶层返回 `data_status/as_of/site_breakdown`，partial/unavailable 保留已知 facts；不存在记录只有在完整快照证明后才是 missing，不能以空数组代替不可用。导出创建复用 `pricing_catalog|group_catalog`，请求体禁止携带分页或任何远端 mutation 字段。

Scheduler 按设置周期 enqueue `pricing_group_sync`，授权通过后立即执行一次；BumpSiteFence 终止旧版本任务，恢复扫描接管超时 running 任务。Token 全站库存、随机进程 runtime stats、进程本地 affinity、可能刷新 OAuth 的 Codex GET 和全部远端 mutation 均无平台路由、DTO、Worker 或导出类型。

# D139 system-task API/Worker

只读 API 为 `GET /api/system-tasks`、`GET /api/system-tasks/statistics`、`GET /api/sites/:id/system-tasks`、`GET /api/sites/:id/system-tasks/statistics`。统一 envelope、UserAuth、Viewer/Admin 可读、强制改密期间禁止。全局列表/统计接受 `site_ids/types/statuses/created_start/created_end/error_present`；站点接口强制 path site 并拒绝 `site_ids`。不得注册 POST/PUT/PATCH/DELETE system-task 路由，也不得注册 pilot 单任务详情代理。

列表和统计顶层必须显式返回 `truncated:boolean`、`truncation_reason:null|source_limit|id_gap|source_limit_and_id_gap`、`source_limit:"100"`、`observed_count` bigint string；这些字段与 `data_status/as_of` 一起构成 typed completeness，禁止仅返回无法解释原因的 `partial`。

`SystemTaskItem` 固定为 `id/site_id/site_name/remote_id/task_id/type/status/created_at/updated_at/progress/result/error_present/error_code/data_status/as_of`。progress 是 nullable `{total,processed,progress,remaining}`；result 是按五个 type 的判别联合。ID 和全部计数为 JSON decimal string，progress 为 0..100 integer。statistics 返回精确 summary、type/status/site breakdown 和 completeness；API、日志、错误 params 均不得出现 active_key、locked_by、raw JSON 或 raw error。

Worker 注册 `system_task_sync` required metadata queue task，独立 lease、active_key、resource concurrency、max attempts=3、site/config fence 和持久 failure state。平台设置 `system_task_terminal_retention_days` 为正整数，变更在下一次清理生效；清理只作用终态。export runtime 接受 `statistics_type=system_tasks` 和同名安全筛选，不接受 raw 字段或远端 mutation 参数。
