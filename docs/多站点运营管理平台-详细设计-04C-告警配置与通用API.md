# 多站点运营管理平台 — 详细设计 04C：告警、配置与通用 API

> 上级文档：[多站点运营管理平台-概要设计.md](./多站点运营管理平台-概要设计.md)  
> 业务功能索引：[多站点运营管理平台-详细设计-04-业务功能与平台API.md](./多站点运营管理平台-详细设计-04-业务功能与平台API.md)  
> 详细设计索引：[多站点运营管理平台-详细设计.md](./多站点运营管理平台-详细设计.md)

## 20. 告警

### 20.1 告警状态

~~~text
pending → firing → resolved
~~~

- pending：达到条件但尚未满足连续次数；
- firing：正式触发；
- resolved：事件已结束；`resolution_reason` 必须说明结束原因，取值为 `recovered`（同一目标出现明确健康样本）、`remediated`（失败任务的业务结果已修复）、`retired`（目标或监控范围已退役）或 `superseded`（配置栅栏终止且由新任务接管）。`resolved` 不是所有规则的“恢复”同义词；
- 不做人工确认、负责人、处理备注和人工关闭。

### 20.2 告警范围

站点类：

- 站点离线；
- 授权过期；
- 未开启数据导出；
- 小时采集失败或缺口；
- 历史回填失败；
- 次日或每周统计校验失败。

资源类：

- 实例 stale；
- 实例离线；
- CPU、内存、磁盘超过阈值；
- 全部实例不可用。

账户类：

- 纳管账户在远端不存在；
- 远端用户 ID 对应的 created_at 与纳管时不一致；
- 远端账户被禁用；
- account.quota <= 0。

渠道类（仅站点级完整快照聚合，不读取 Key）：

- 渠道余额总和过低；
- 渠道平均响应时间过高；
- 渠道可用率过低，连续次数按相邻 complete 小时快照累计。

不做消费异常、长期无用量、Key 级健康或 Key 级财务告警。

### 20.3 默认阈值

| 指标 | Warning | Critical | 触发 |
|---|---:|---:|---|
| CPU | 85% | 95% | 连续 3 分钟 |
| 内存 | 85% | 95% | 连续 3 分钟 |
| 磁盘 | 85% | 95% | Warning 连续 3 分钟，Critical 立即 |
| 实例 stale | 90 秒未上报 | — | 立即 |
| 实例离线 | 连续 3 次未获取 | — | Critical |
| 渠道余额总和 | <= 100 | <= 0 | 立即 |
| 渠道平均响应时间 | >= 1000 ms | >= 3000 ms | Warning 连续 3 个 complete 小时快照，Critical 立即 |
| 渠道可用率 | <= 0.99 | <= 0.90 | Warning 连续 3 个 complete 小时快照，Critical 立即 |

平台默认值可配置，单站可覆盖。页面 health_status 与告警共用同一套阈值。

实例 stale 的唯一有效阈值来自实际生效的 `instance_stale` 告警规则；上游 `stale_after_seconds` 仅作为原始能力字段记录和诊断，不直接驱动页面状态。默认全局规则为 90 秒，单站覆盖后页面、health_status 和告警同时使用覆盖值，避免出现两套 stale 口径。

告警评估必须遵守适用条件：

- management_status=disabled 的站点不评估在线、实例和采集告警；
- auth_status<>authorized 时不评估需要授权接口的资源与采集告警；公开 status 仅用于可达性和响应契约检查，只有 auth_status=expired 才触发授权过期；
- data_export_enabled=false 时不评估小时采集、回填和校验告警，只保留数据导出未开启告警；
- managed_status=archived 的账户或所属 customer=disabled 时不评估账户类告警；
- 实例接口本轮请求失败表示 unknown，不得按 0 个实例触发全部实例不可用；
- 账户 remote_state=missing/identity_mismatch 时不同时触发 quota 为空或远端禁用告警，只保留对应身份告警；
- site_offline 的连续次数直接使用探活原始失败次数，不在 online_status 已经连续三次失败后再次累计三次。
- 渠道规则只适用于 management_status=active、auth_status=authorized 且 channel_sync 已形成当前 config_version 的 complete 小时快照的站点；缺行、config_version 不匹配、超过 2 小时未更新或非 complete 样本统一为 unknown，不增加连续次数也不恢复事件。

目标从适用变为不适用时，已有 pending/firing 事件自动 resolved，message 记录 `scope_inactive`、`resolution_reason=retired`，不发送“已恢复”通知；重新启用后必须由新样本重新累计，不能复活旧事件。一次成功的实例快照是节点目录的权威来源：此前已知但本次未出现的节点标记退役，结束该节点的 stale/offline/资源阈值事件；接口失败或未知样本绝不退役节点。

### 20.4 alert_rule

- id、rule_key、name；
- enabled、level、metric、compare_operator、threshold_value、for_times；
- scope_type、scope_id；
- created_at、updated_at。

Warning 和 Critical 是同一 rule_key 的两条独立规则。前端按 rule_key + level 使用 i18next 显示名称，数据库 name 仅作为后端和通知兜底文本。scope_type=global 时 scope_id=0；scope_type=site 时 scope_id 为站点 ID。

### 20.5 alert_event

- id、rule_id、rule_key、site_id；
- target_type、target_key、active_key；
- level、status、consecutive_count；
- current_value、threshold_value、message_code、message_params、message；
- first_observed_at、first_fired_at、last_fired_at、resolved_at、resolution_reason；
- created_at、updated_at。

相同 rule_key + target_type + canonical_target_key 只保留一个活跃事件；实例键为 `site_id/node_name`，站点及渠道站点级聚合为 `site_id`，账户为 `account_id`，采集缺口/验证失败为 `site_id/hour_ts`，回填失败为 `site_id/run_id`。Warning/Critical 和 global/site override 共享该逻辑键，不能并存多个活跃事件。

采样结果为 unknown 时不增加连续次数，也不结束 pending/firing；只有明确健康样本才能以 `recovered` 结束。回填失败在窗口事实修复后以 `remediated` 结束；因配置栅栏终止的校验/回填任务以 `superseded` 结束。小时窗口缺口和校验不一致是同一窗口的两个层次：数据校验不一致时只创建 `validation_failed`，不再并发创建泛化的 `collection_missing`。Critical 和 Warning 同时满足时仅保留 Critical；从 Critical 降到仍满足 Warning 时先结束 Critical，再按 Warning 的 for_times 从当前样本重新累计。

### 20.6 钉钉通知

本期仅支持钉钉机器人：

- Warning、Critical 发送；
- Info 仅平台展示；
- 首次 firing 时发送；
- 仅 `resolution_reason=recovered` 时发送恢复通知；退役、修复和替代只保留事件审计，不伪装为恢复；
- 持续异常不重复刷屏；
- 失败自动重试并记录投递结果；
- Webhook 和签名密钥加密保存；
- 支持测试消息；
- 通知包含站点、实例、指标、当前值、阈值、时间和详情链接。

详情链接固定为 `PUBLIC_ORIGIN + /alerts?alertId={event_id}`，前端通过 URL 打开事件详情；复制、刷新和从钉钉进入后仍能定位同一事件。

notification.dingtalk.enabled=false 时仍创建/恢复告警事件，但不创建新 delivery；已 pending 的 delivery 终止为 failed、error_code=NOTIFICATION_DISABLED，重新启用后不补发旧事件，只发送之后的新 firing/resolved。测试接口在未启用、未配置或地址预检失败时返回 HTTP 200 的明确 failed NotificationTestResult，delivery_id=null，不伪造投递记录。

### 20.7 alert_delivery

| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint PK | 投递记录 ID |
| alert_event_id | bigint nullable | 告警事件；测试消息为 null |
| event_type | varchar(16) | firing/resolved/test |
| channel | varchar(16) | dingtalk |
| status | varchar(16) | pending/success/failed |
| attempt_count | int | 已尝试次数 |
| error_code | varchar(64) | 稳定失败码 |
| response_code | int nullable | 尚未收到 HTTP 响应时为 null |
| response_message | text | 钉钉响应或错误 |
| next_retry_at | bigint nullable | 下次重试时间；无待重试时为 null |
| sent_at | bigint nullable | 成功发送时间；未成功时为 null |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

### 20.8 告警中心页面

路由：/alerts

- 事件 Tab 的顶部计数通过 `GET /api/alerts/summary` 获取，列表通过 `GET /api/alerts` 获取；
- 顶部卡片：当前 firing 总数、Critical 数、Warning 数、今日恢复数；
- 列表筛选：状态、级别、目标类型、站点、时间；
- firing 置顶，resolved 进入历史列表；
- 字段：级别、规则、站点、实例/账户、当前值、阈值、首次时间、最近时间、结束时间、结束原因；
- 点击站点、实例或账户进入对应详情；
- 不显示确认、负责人、处理备注和人工关闭按钮；
- 规则配置仅 admin 可编辑；
- 钉钉配置和测试消息仅 admin 可操作；
- viewer 可查看全部告警和规则。
- 详情使用 `/alerts?alertId=<id>` 深链打开，钉钉、刷新和浏览器导航均保持可定位。

---

## 21. 系统配置

admin 可配置：

| 分组 | 配置 |
|---|---|
| 采集 | 探活、资源、RPM/TPM 固定 60 秒只读展示；小时采集延迟可配置 |
| 并发 | 各任务队列并发上限 |
| 回填 | 手动补采单次最大天数，默认 366 |
| 留存 | 分钟资源数据留存天数，默认 90 天 |
| 导出 | 文件有效期、单用户/全局活跃任务上限、最大文件、最小磁盘余量 |
| 通知 | 只读展示 PUBLIC_ORIGIN；配置钉钉 Webhook、签名密钥和开关 |
| 汇率 | 站点汇率获取失败时的展示兜底值 |

Access Token 加密密钥不在页面配置，由环境变量提供。

配置值校验范围：小时采集延迟 1～59 分钟；队列并发 1～100；手动补采上限 1～3660 天；分钟留存 1～3650 天；导出有效期 1～168 小时、活跃任务上限 1～100、文件和磁盘阈值必须为正数；兜底汇率为空或正数。任何一项非法时整批 PUT 不落库。CPU/内存/磁盘阈值和连续次数只通过 alert-rules API 修改，不写 platform_setting。

系统设置不提供 H+15 发布资格判定，也不因 `usage_delay_minutes` 或 `usage_concurrency` 的组合阻止保存；两个字段只按各自范围校验。H+15 仍作为采集监控与容量验收目标，由告警和运维流程观测。手动补采 API 的范围上限必须实时读取 `collector.manual_backfill_max_days`，不得在 Controller 写死 366。

配置更新后，新并发值只影响后续领取，不取消运行任务；留存值在下一次清理任务生效；导出上限在任务领取时读取并在该次尝试内固定。

### 21.1 platform_setting

| 字段 | 类型 | 说明 |
|---|---|---|
| id | bigint PK | 配置 ID |
| setting_key | varchar(128) unique | 配置键 |
| setting_value | text | 配置值 |
| value_type | varchar(16) | int/decimal/bool/string/json |
| is_secret | tinyint | 是否为敏感配置 |
| updated_at | bigint | 更新时间 |

钉钉 Webhook 和签名密钥属于敏感配置，加密后写入 setting_value。Access Token 加密主密钥不得写入该表。

敏感 setting_value 为空字符串表示“未配置”，不执行解密；非空值必须是 v1 AES-GCM 密文，解密失败返回 configured=true + decrypt_error，且禁止发送通知。

value_type=int 按 Go int64 解析和范围校验，不使用平台相关的 Go `int`，因此 2 GiB 等字节阈值不会在 32 位边界溢出。

---

## 22. API 通用规范

### 22.1 响应

~~~json
{
  "success": true,
  "message": "",
  "code": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 100,
    "items": []
  },
  "request_id": "req_xxx"
}
~~~

响应外壳沿用 new-api 的 `success/message/data` 形式。`code` 是本平台新增的稳定机器错误码，成功时为空字符串；`request_id` 用于定位后端日志。message 是安全兜底文本，前端优先按 code 走 i18next，不把后端 message 当作已本地化文案。非分页接口的 `data` 直接为 DTO，分页接口的分页信息全部位于 `data` 内，不再使用顶层 `meta`。

### 22.2 分页与排序

- 前端路由使用 page/pageSize，调用 API 时映射为 p/page_size；
- API 默认 p=1；
- 默认 page_size=20；
- page_size 最大 100；
- 列表统一支持 p、page_size、sort_by、sort_order；
- 表格数据后端分页；
- 图表限制最大时间点数量。
- 数据库 BIGINT 主键、外键和业务计数返回十进制字符串；Unix 秒时间戳、百分比、页码和分页 total 返回 JSON number。

### 22.3 错误码

| HTTP | 含义 |
|---:|---|
| 400 | 参数错误 |
| 401 | 未登录或会话失效 |
| 403 | 无权限 |
| 404 | 对象不存在 |
| 409 | 唯一约束或状态冲突 |
| 500 | 系统错误 |

错误响应同时返回稳定的机器错误码和中文提示。

### 22.4 数据完整性

- data_status 与业务数据同时返回；
- missing_ranges 明确返回缺失区间；
- 不完整数据不能包装为完整零值；
- 当前日返回截至最近完整小时；
- 跨站结果返回 site_breakdown。

---
# P0-B 日志完整性与权限

日志采集失败、分页不完整、授权过期、上游能力关闭和 config_version fence 丢弃均产生可查询的日志完整性状态，不将失败轮次解释为零条日志。全局日志查询必须按调用者站点权限过滤；站点查询只允许目标站点。敏感字段脱敏规则与日志查询、导出共用，任何 5xx 不返回上游响应正文。
# P0-D 渠道指标告警输入

渠道完整快照发布稳定站点级指标：`channel.available_count`、`channel.unavailable_count`、`channel.availability_rate`、`channel.balance_total`、`channel.response_time_avg_ms`、`channel.response_time_max_ms`。余额阈值使用十进制定点比较，响应时间单位固定毫秒；样本必须携带站点、观测时间和完整性状态。非 complete 样本不得触发数值型告警，只能进入采集失败/陈旧数据规则。

本平台不采集渠道 Key，因此不得提供 `key_count`、多 Key 健康、Key 级余额或 Key 级禁用原因等虚构指标。原有任何“不得展示渠道健康和余额”的限制由本节替代：允许展示渠道级状态、余额和响应时间，但绝不读取、存储或展示 Key 及 Key 派生状态。

内置规则固定为 `channel_balance_low`（`channel.balance_total`，`<=`）、`channel_response_time_high`（`channel.response_time_avg_ms`，`>=`）和 `channel_availability_low`（`channel.availability_rate`，`<=`）。三者 target_type 均为 `site`、canonical target 为 site_id，使用现有 global 基础规则与 site override 继承模型。Warning/Critical 默认阈值与连续次数以 §20.3 为准；低方向规则的 Critical 阈值必须小于 Warning，高方向规则相反。恢复使用同一阈值且必须来自后续 complete 快照，不引入迟滞。每个 complete 小时快照只形成一个 canonical sample identity；重放、五分钟兜底扫描与 post-commit hook 共享 cursor，因此不能重复累计或重复投递。
