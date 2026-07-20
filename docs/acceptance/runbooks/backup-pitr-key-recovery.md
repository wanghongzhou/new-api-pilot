# 备份、PITR 与加密密钥恢复演练记录模板

适用用例：A22、A51、A75。此文件是模板，不是已完成的验收证据。

## A22 自动化受控演练

A22 已提供可执行编排 `scripts/acceptance/run-a22.ps1`。正式演练只能通过统一验收入口执行：

```powershell
go run ./scripts/acceptance run -case A22 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a22.ps1
```

开发验证可直接执行脚本，证据只写入唯一的
`artifacts/smoke/A22-dev-*` 目录，并固定标记
`evidence_class=development`、`acceptance_eligible=false`。开发证据不能替代正式验收，也不能移除 manifest 中 A22 的 `planned:` 前缀。

该编排使用同名数据库、不同 server UUID 的两个 MySQL 8.4 实例和相互独立的数据卷，在无宿主机端口的内部网络中完成以下步骤：

1. 按 F05 固定时钟写入代表性平台用户、站点、密文、任务、运行窗口、游标、告警、导出以及六级 hourly/daily 汇总。
2. 对源库生成确定性全库快照；快照哈希覆盖密文，但证据绝不输出密文。
3. 执行真实备份和 manifest-only 预检，并分别注入 migration checksum 篡改、目标 server UUID 不匹配两条负分支。两条分支都必须在导入前失败，目标保持零表、源快照不变且不得产生 release gate。
4. 恢复到隔离目标后执行 full verify、release gate 校验和第二次全库快照；源/目标业务快照、任务窗口、active key 与六级汇总必须完全相同，同时 server UUID 必须不同。
5. 仅在 release gate 存在后启动恢复目标应用，真实请求 health、ready、login、self 和 site 列表。认证同时要求登录 cookie 与匹配的 `New-Api-User`，并校验应用观察到的数据库 UUID 指纹等于目标而不等于源。
6. 计算 RPO/RTO、扫描全部证据中的 DSN/密钥/URL 凭证，最后只删除本次唯一标签资源，并确认容器、网络、卷和临时镜像均无残留。

每次成功运行生成 19 个受控内部工件和 `a22-artifacts.json`。清单逐项记录路径、大小和 SHA-256；统一入口还会在关闭 stdout/stderr 后再次执行秘密扫描和完整合同校验。无论开发或正式模式，报告范围始终是 `controlled_technical_drill`，并且 `production_release_authorized=false`；该演练本身永远不授权生产切换。

## 1. 演练信息

| 项目 | 记录 |
|---|---|
| 源环境/隔离恢复环境 | `<source>` / `<isolated-target>` |
| 目标恢复时间 T | `<timestamp-with-timezone>` |
| 全量备份/起止 binlog | `<backup-id>` / `<binlog-range>` |
| 备份与 fixture checksum | `<checksums>` |
| `encryption_key_id`/保留版本 | `<key-id>` / `<key-version>` |
| 允许 RPO/RTO | `<= 1h` / `<= 4h` |
| 实际数据损失/恢复耗时 | `<duration>` / `<duration>` |
| 执行人/复核人/批准人 | `<operator>` / `<reviewer>` / `<approver>` |

## 2. 前置条件和停止条件

- 使用 F05 固定画像，并记录 commit、镜像 digest、MySQL 精确版本、时区、备份策略、binlog 保留周期和恢复工具版本。
- 全量备份、binlog 和密钥版本分开保存且均可读取；恢复环境与生产网络、存储和凭证隔离。
- 目标时间、业务停止点、恢复顺序、DNS/流量切换权限和回退负责人均已批准。
- checksum 不符、binlog 缺口、精确密钥版本缺失、隔离边界失效或任何秘密出现在日志时立即停止，不得切换生产。

## 3. 备份与可恢复性检查

1. 使用权限 0600/0400 的 MySQL defaults file，通过绝对 `BACKUP_ROOT` 执行 `bash scripts/backup.sh`；密码不得出现在参数或日志。确认脚本用 `new-api-pilot:migration-runner` advisory lock 覆盖 dump 和 server/migration metadata 采集，保存脚本 JSON 和退出码，确认退出 0 后才接收原子发布的 `backup-<UTC>-<random>` 目录。
2. 检查 `database.sql.gz`、dump sidecar、`manifest.json` 和 manifest sidecar；manifest 必须包含一致性 SOURCE file/position 或单一 GTID 起点、server UUID、完整 schema migration/checksum、镜像 digest、dump 大小/hash 和完整 `encryption_key_id`，脚本 JSON 中密钥指纹只能有 12 位。
3. 备份当前及保留期内所有被引用的密钥版本和 `encryption_key_id` 映射；验证权限最小化和恢复人可按审批取得。
4. 验证备份目录、对象数量、大小、checksum 和保留策略；从日志中删除 Token、Webhook URL、明文密钥、完整密钥指纹和解密后配置。

## 4. 隔离 PITR

1. 在空隔离环境安装与源环境兼容的 MySQL 8.4，设置 utf8mb4/utf8mb4_unicode_ci、READ-COMMITTED 和 Asia/Shanghai；创建存在但零表的目标 schema，不接入生产流量。
2. 先执行 `new-api-pilot verify-backup --mode=manifest-only --manifest=/absolute/path/manifest.json`，验证绝对路径、未知/缺失字段、密钥指纹、SOURCE 坐标、dump 与双 sidecar 的 hash/size；退出非 0 时确认目标仍为零表。随后把全量 dump 导入空目标 schema 并保存导入 JSON/日志和退出码。只做全量恢复且不回放 binlog 时可使用 `scripts/restore.sh` 一体化执行本步、第 3～5 步和 release gate。
3. PITR 从记录的位置顺序应用 binlog，严格停止在目标时间 T；保存每段应用结果和最终位点。在回放完成前不得运行一体化 restore 脚本创建 release gate。
4. 装载与 manifest `encryption_key_id` 精确匹配的 `ENCRYPTION_KEY` 和精确镜像 digest；密钥不得通过命令行参数或日志输出。`DATABASE_DSN` 必须选择隔离目标 schema。
5. 在完成全部回放后执行 `new-api-pilot verify-restore --mode=full --manifest=/absolute/path/manifest.json`。核对 manifest、migration/checkpoint、表/seed/外键、敏感配置全量解密、关键计数、六级 hourly/daily 聚合、窗口/游标/active key、导出任务和告警状态；退出码非 0 或 JSON status 非 success 时立即停止且不得存在 release gate。只有全通过后才原子发布含 `verify-report.json` 的 release 目录。
6. 编排只在 release gate 存在后启动应用。对恢复结果执行只读 API 冒烟，并比较目标时间前后的哨兵记录，证明没有应用 T 之后的事务。

## 5. 密钥轮换失败分支

1. 在停机窗口临时注入 Base64 编码 32 字节的 `OLD_ENCRYPTION_KEY`/`NEW_ENCRYPTION_KEY`，先执行 `new-api-pilot secrets reencrypt --dry-run --batch-size=100`；JSON 报告成功后再以相同密钥对执行 `new-api-pilot secrets reencrypt --batch-size=100`。`batch-size` 只允许 1～1000，并保存两次 JSON 与退出码。
2. 在隔离副本分别注入不可解密密文、暂存中断和暂存后业务行并发变化；证明坏密文不写业务表，同一密钥对可续跑，不同密钥对拒绝接管，任一 source hash CAS 失败会回滚整个最终事务。
3. 清除注入故障后以同一密钥对重跑；只有命令退出 0、JSON status=success、暂存表为空、全部秘密可由新密钥读取且旧密钥不可读取时，才允许把运行时 `ENCRYPTION_KEY` 切换到新版本。完成后移除 OLD/NEW 维护变量。

## 6. 通过标准与证据

- 恢复环境通过全部结构、密文、外键、任务状态、事实与汇总校验，目标时间之后的事务未被应用。
- 实测数据损失不超过 1 小时，端到端恢复不超过 4 小时；任何失败分支均未触发生产切换。
- 密钥轮换全成功才切换，注入失败时仍能用旧密钥恢复；数据库、日志和证据无明文秘密。
- 证据目录包含备份/binlog/密钥引用、checksum、工具与镜像版本、脱敏命令输出、逐项校验报告、RPO/RTO 计算和双人签字。

最终结论：`<PASS/FAIL>`；切换决定：`<NOT-SWITCHED/APPROVED>`；证据目录：`<evidence-path>`。
