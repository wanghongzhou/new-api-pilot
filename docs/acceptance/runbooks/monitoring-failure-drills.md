# 监控与故障演练记录模板

适用用例：A19、A42、A43、A48、A53、A78、A84、A85 及发布门禁故障场景。此文件是模板，不是已完成的验收证据。

## 1. 演练信息

| 项目 | 记录 |
|---|---|
| 环境/commit/镜像 digest | `<environment>` / `<commit>` / `<digest>` |
| F02/F04/F05 版本与 checksum | `<fixtures>` |
| 可控 Clock/随机 seed | `<clock>` / `<seed>` |
| 告警 fake server | `<endpoint-id-no-secret>` |
| 开始/结束时间 | `<start>` / `<end>` |
| 执行人/复核人/值班责任人 | `<operator>` / `<reviewer>` / `<on-call>` |

## 2. 安全边界和停止条件

- 仅在隔离或批准的演练环境注入故障；记录受影响站点、队列、数据库和通知目标，确保不会向真实钉钉群发送。
- 演练前保存健康基线、待处理任务、活动告警、磁盘余量和恢复点；所有外部请求携带 request ID 和有界超时。
- 预先定义恢复命令、最大影响窗口和人工停止开关。影响越界、恢复手段失效、数据不变量破坏或秘密泄露时立即中止并升级。

## 3. 故障矩阵

| 场景 | 注入方法 | 预期系统行为 | 恢复证明 |
|---|---|---|---|
| 断网/连接超时 | `<method>` | 有界超时；状态 unknown/partial，不伪造 0 或健康 | 网络恢复后新样本收敛 |
| 上游 401 | `<method>` | 授权失效、停止不安全采集，适用告警 resolved 为 `scope_inactive` | 显式重新授权后重新累计 |
| 上游 429 | `<method>` | 按有界退避重试，不形成重试风暴 | 限流解除后任务完成 |
| 上游 5xx | `<method>` | 按预算重试；实例状态 unknown，不触发“全部不可用”假告警 | 成功样本恢复状态 |
| Worker 进程重启 | `<method>` | heartbeat/reaper 恢复父子任务，attempt 不重复或重置 | 无永久 running，任务终态正确 |
| 磁盘不足 | `<method>` | 导出安全失败并删除半成品，其他队列继续 | 释放空间后新任务成功 |
| 指标 unknown | `<method>` | 不增加连续次数，也不恢复 firing 告警 | 明确健康样本才 resolved |
| Warning/Critical 切换 | `<method>` | 同 target 仅一个活跃事件；降级重新累计 `for_times` | 状态和投递次数一致 |
| 规则 scope 停用 | `<method>` | 自动 resolved，原因 `scope_inactive` | 恢复后从新样本累计 |
| Webhook 失败/重定向 | `<method>` | 拒绝 HTTP、非白名单 host 和跨 host 重定向，URL 参数不泄露 | 合法 fake server 单次投递 |
| Prometheus 抓取/服务发现缺失 | `<method>` | target down、absent 和 scheduler metric missing 在规定 `for` 后 firing，并走独立基础设施 receiver | 恢复抓取后规则自动恢复 |
| Scheduler 停止推进 | `<method>` | 心跳年龄超过 180 秒后持续 2 分钟 firing | 成功调度后心跳推进并恢复 |
| 队列/小时窗口停滞 | `<method>` | oldest age 或全局最大 collection lag 超过 900 秒持续 5 分钟 firing；不产生 site_id 序列 | Worker 恢复后 age/lag 收敛，stale 总数归零 |
| 数据库连接池压力 | `<method>` | 使用率超过 90% 持续 5 分钟 firing；max=0/缺失按指标异常处理 | 连接释放且池指标有效后恢复 |
| Redis 不可达 | `<method>` | healthz 仍存活；readyz=503 且 failed_checks 含 redis；快速任务继续完成但历史写入失败，fast-tasks 返回统一内部错误 | Redis 恢复后 readyz=200，新执行记录可查询且不伪造停机期间历史 |
| 文件系统容量不可读 | `<method>` | total/free 写 0，容量异常 firing；不会因除零静默通过 | 挂载恢复后容量值与系统命令一致 |
| 备份/时钟指标缺失 | `<method>` | absent、备份超 RPO、备份失败或 MySQL/应用偏移超过 5 秒通过独立 receiver firing | backup textfile/数据库或应用时间同步恢复后规则恢复 |
| Metrics recorder panic | `<method>` | HTTP、上游、任务提交、告警投递和导出业务结果不改变，下一采样轮仍运行 | recorder 恢复后 counter/gauge 继续推进 |

## 4. 调度、阈值和通知验证

1. 用可控 Clock 推进 61 分钟，验证 probe/realtime/resource 每 60 秒、user/channel 每小时触发一次且不重复。
2. 把每个队列并发压到配置上限加一，确认单队列不超限、不同队列互不占用，回填分片让出后实时任务优先。
3. 在默认 seed 下依次输入 CPU/内存/磁盘 84%、85%、95% 和健康样本；CPU/内存连续三个成功分钟才 firing。
4. 校验默认 Warning/Critical=85/95，只产生设计约定规则；Warning、Critical 及每次 resolved 各投递一次，重复重评不重复通知。
5. 验证 `healthz`/`readyz` 语义分离，metrics 仅批准网络可达且标签无 Token、Webhook、密钥或高基数敏感值。
6. 执行 `make check-prometheus`，保存 6 条 recording rules 和 23 条 alert rules 的 `promtool` 检查输出；再从 Prometheus `/api/v1/rules` 保存实际加载状态，二者缺一不可。
7. 检查 `/api/v1/series` 和一次原始 `/metrics` 导出，确认不存在 site/user/model/channel/request_id、URL、错误文本、Token、Webhook 或 hash ID label；未知枚举只能归为 `other`。
8. 验证 `receiver_scope="infrastructure"` 的抓取、备份、时钟和指标异常告警不经过本平台钉钉 Worker；模拟钉钉 Worker 故障时仍能送达独立接收端。

## 5. 通过标准与证据

- 故障矩阵全部执行且结果符合预期；每个失败能被监控发现、被限界并按既定步骤恢复，数据状态不被错误美化。
- 调度频率、并发隔离、告警唯一性/降级、恢复语义、通知次数、规则 `for` 时长和独立 receiver 路由均有确定性断言。
- 证据目录包含注入/恢复时间线、命令及退出码、request/event/delivery ID、脱敏日志、指标导出、fake server 请求、数据库状态前后对比和双人签字。

最终结论：`<PASS/FAIL>`；未恢复事项：`<none-or-list>`；证据目录：`<evidence-path>`。
