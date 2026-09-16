# A74 部署与回滚受控演练

本手册只适用于 A74。演练只能操作隔离环境或 Pilot-owned 资源，不得修改上游 new-api 的代码、配置、数据、索引、容器或服务。正式证据通过唯一入口接收已脱敏的受控材料：

```powershell
$env:A74_CONTROLLED_INPUT = 'C:\absolute\controlled\a74-input.json'
go run ./scripts/acceptance run -case A74 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a74.ps1
```

输入 JSON 使用 schema 1、`acceptance_id=A74`、formal passing 状态、`scope=controlled_pilot_owned_isolated`、脱敏目标身份、不可变镜像/发布引用、时间线和全部 A74 assertions，并以绝对路径指向材料 ZIP。ZIP 精确包含 `deployment.json`、`rollback.json`、`monitoring.json`、`cleanup.json`、`approvals.json`；所有文件必须标记 passed/sanitized，审批人必须具名、互异且批准。

缺少不可变镜像 digest、备份、故障注入、旧镜像恢复、监控观察、清理证明或审批时只能产生 blocked 失败证据并保持 `planned:`。

## 1. 演练信息

| 项目 | 记录 |
|---|---|
| 环境/隔离边界 | `<environment>` |
| 变更单/commit | `<change-id>` / `<commit>` |
| 旧/新镜像 digest | `<old-digest>` / `<new-digest>` |
| 部署前/目标 schema 版本 | `<before>` / `<target>` |
| 备份 ID 与 checksum | `<backup-id>` / `<checksum>` |
| 开始/结束时间（Asia/Shanghai） | `<start>` / `<end>` |
| 执行人/复核人/批准人 | `<operator>` / `<reviewer>` / `<approver>` |

## 2. 前置条件和停止条件

- 已确认维护窗口、影响范围、告警通知、当前镜像和配置版本；生产站点接入清单已逐站确认上游精确版本、`enable_data_export`、`quota_data` 保留策略和首个不可删除 root 责任人。
- 全量备份、binlog 位点、当前密钥版本和配置导出均已生成并验证可读；备份不与主库共享故障域。
- 新旧镜像、migration checksum、部署命令、恢复命令、Prometheus 配置版本和冒烟账户已冻结；秘密不出现在命令输出或证据中。
- 任一备份/checksum/密钥验证失败、迁移来源版本不明、审批缺失或监控基线异常时，停止部署。

## 3. 首次部署或升级

1. 记录部署前 `healthz`、`readyz`（含 MySQL/Redis/Scheduler 检查）、schema 版本、当前任务数量、关键指标和上游能力检查结果。
2. 在空库（首次部署）或已有数据测试库（升级）执行 `new-api-pilot migrate`，保存退出码、逐版本 checksum 和 checkpoint 恢复结果；已执行版本不得重复执行，DDL 提交间隙重启必须按 postcondition 续跑。
3. 部署精确镜像 digest，等待 `healthz` 存活和 `readyz` 就绪；验证安全 Header、metrics 仅内网可达且没有敏感标签。执行 `make check-prometheus`，加载 `deploy/prometheus` 中的 scrape/recording/alert 配置，并保存 Prometheus `/api/v1/rules` 成功状态。验证 SPA 只对 GET/HEAD fallback，未知 `/api` 和 health/ready/metrics 子路径不 fallback，index 为 no-cache、hash 资产 immutable，路径穿越与隐藏文件请求返回 404。
4. 执行登录/权限、站点列表、单站探活、统计查询、任务入队与导出下载冒烟，记录 request ID、响应 envelope 和数据库不变量。
5. 观察 `<observation-window>`，确认错误率、延迟、队列、数据库连接、collection lag、runtime readiness、告警/导出失败和磁盘容量无持续异常；验证抓取/备份/时钟缺失告警走独立 receiver，然后记录升级判定。

## 4. 故障注入与默认回滚

故障点：`<migration/deploy/readiness/smoke>`；注入方式：`<method>`；触发时间：`<time>`。

1. 停止新流量和 Worker claim，记录失败现场、最后成功 migration、任务状态和回滚决策人。
2. 停止新镜像。不得直接让旧镜像连接未知或只向前兼容的新 schema。
3. 按已验证的备份恢复流程恢复部署前数据库、binlog 位置、配置和对应密钥版本，并执行 `new-api-pilot verify-restore --mode=full --manifest=/absolute/path/manifest.json`；保存版本化 JSON 报告和退出码，非 0 不得继续。
4. 仅在恢复后的 schema 与旧镜像兼容且校验通过后启动旧镜像；同时恢复与旧镜像指标契约匹配的 Prometheus 配置。恢复流量前再次验证 `healthz`、`readyz`、安全 Header、`/metrics`、规则加载状态和完整冒烟集。
5. 核对事实、汇总、任务 active key、站点授权和配置；确认没有重复任务、半成品导出或丢失的已确认写入。

## 5. 通过标准与证据

- 首次部署/升级路径的 migration、健康/就绪、安全 Header 和冒烟全部通过。
- 故障路径恢复到已知 schema 后旧镜像可用；没有在未知 schema 上启动，也没有绕过恢复步骤。
- 上游版本与生产接入清单逐站满足设计约束；所有失败步骤均有明确判定和处置。
- 证据目录包含命令及退出码、时间线、脱敏日志、数据库版本/checksum、镜像 digest、HTTP 报告、`promtool` 输出、Prometheus 规则 API、监控截图或导出、备份引用及执行/复核签字。

最终结论必须由专用 validator 根据封闭 artifact 和 SHA-256 得出，不接受手工填写 `<PASS>`、通用 wrapper 或其他用例证据替代。
