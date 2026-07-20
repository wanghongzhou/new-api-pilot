# A49 容量与性能验收运行手册

本手册只覆盖 A49 的固定容量画像和 20 个并发只读用户。A17 的导出磁盘与 2 GiB 边界由导出集成测试单独验收。本文描述可执行流程，但在一次正式 full 运行真实通过、复核并登记证据前，不代表 A49 已完成。

## 1. 固定画像

正式运行必须直接读取 `testdata/design/f05-ops-capacity.yaml`，且不得通过命令行缩量：

- 固定 Clock：`2026-01-17T23:59:59+08:00`（Unix `1768665599`），固定 seed `49050117`。
- 50 个站点、1000 个客户、100000 个不同远端用户、10000 个托管账户。
- 每站 100 个有效通道、200 个有效模型。
- 30 天内恰好 15000000 条 `usage_fact_hourly`：`50 × 2000 × 30 × 5`。
- 生成与读取口径一致的 `collection_window`、`usage_fact_daily`，以及账户/客户/站点/全局/模型/通道所需汇总。窗口覆盖固定 31 天小时查询和 Dashboard 当日范围。
- 20 个不同的 viewer 用户。每个用户必须真实执行 `POST /api/user/login`，后续请求同时携带自己的 session cookie 和 `New-Api-User`，不得绕过认证。

seed 工具会核对所有行数、不同远端用户数、每站模型/通道数，以及小时事实、日事实和站点汇总的指标总和。任一不一致立即失败。

## 2. 隔离环境和资源

`scripts/acceptance/run-a49.ps1` 为每次运行创建唯一命名的 internal Docker network、MySQL 数据卷、导出卷、应用镜像和容器，不发布任何宿主端口。MySQL、应用、loader 和 load 只在该 internal network 通信；报告容器使用 `--network none`。失败时先保存脱敏日志和 inspect，再只删除本次运行的精确资源，不执行 `docker system prune` 或其他全局清理。

full 运行前置资源：

| 项目 | 最低值 |
|---|---:|
| Docker 可用 CPU | 8 |
| Docker 可用内存 | 16 GiB |
| Docker 存储可用空间 | 35 GiB |
| 证据盘可用空间 | 5 GiB |

脚本先验证 Docker 版本、资源和空间，再构建当前工作区应用镜像。应用必须以受限命令 `capacity-serve` 启动，并同时满足 `APP_ENV=test`、`ACCEPTANCE_ID=A49`、`A49_FIXED_NOW_UNIX=1768665599`。脚本还会执行一次缺失固定 Clock guard 的负向启动，确认应用在连接数据库前 fail closed。`capacity-serve` 不启动采集、告警投递或导出 worker，因此容量期间数据库保持只读。

## 3. 负载矩阵

三个场景依次运行；每个场景均先预热 120 秒，再采样 600 秒，总负载时间为：

`3 × (120s + 600s) = 2160s = 36 分钟`

| 场景 | 独立测量项 | 调度方式 | P95 上限 |
|---|---|---|---:|
| 普通列表 | `list_sites`、`list_customers`、`list_accounts` | 20 个 viewer 稳定轮询三个固定分页/排序查询 | client 与 server 均 `< 1000 ms` |
| 31 天小时趋势 | `hourly_global_31d` | 20 个 viewer 请求固定 744 小时全局趋势 | client 与 server 均 `< 3000 ms` |
| Dashboard | summary、trend、top-site、top-customer、top-model、top-channel、health | 每个 viewer 每轮并发请求七个接口，并等待整组完成 | 七接口 client/server 及 composite client 均 `< 3000 ms` |

共 11 个 HTTP 测量项；四个 top 即使共享路由，也必须按 `type` 分桶独立统计。`dashboard_composite` 是额外第 12 个测量项，表示同一 viewer 的七个并发请求全部完成的墙钟耗时，不替代七个接口各自的门禁。

每个 sample 测量项必须满足：

- 至少 1000 个成功请求；
- 错误率 `errors / attempts < 0.001`（必须严格低于 0.1%）；
- 20 个不同 viewer 全部参与；
- HTTP 状态、成功 envelope、request ID 和最小 DTO 契约正确。

错误、超时、非 2xx、envelope 错误和 DTO 错误都计入 attempts 和 client 延迟总体，不得从 percentile 中剔除。

## 4. percentile 与服务端日志

P50/P95/P99 使用 nearest-rank：对全部延迟升序排序，取从 1 开始的 `ceil(p × n)` 位。client 延迟包含连接复用、完整响应体读取和契约解码；server 延迟来自应用 access log 的 `duration_ms`。

每个请求使用唯一 `X-Request-ID`，sample 和 warmup 使用不同前缀。报告必须逐条将原始请求与 access log 对账：缺失、重复或状态码不一致都会使对应接口失败。普通列表、小时趋势和七个 Dashboard 接口同时执行 client/server P95 硬门禁；composite 只有 client 墙钟延迟。

## 5. 执行命令

正式运行（预计还需镜像构建和 1500 万行装载时间）：

```powershell
go run ./scripts/acceptance run -case A49 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a49.ps1
```

开发 smoke 使用相同 Clock、接口和契约，但只使用 2 个站点、40 个远端用户、10 个托管账户、2 个 viewer，以及 `1s warmup + 3s sample`。它只能验证编排和报告链路，报告明确写入 `acceptance_eligible=false`，绝不能用于移除 manifest 中 A49 的 `planned:`：

```powershell
go run ./scripts/acceptance run -case A49 -evidence-root artifacts/smoke -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a49.ps1 -Smoke
```

禁止手工设置更小的 full 参数；Go profile 校验会拒绝任何与 F05 正式基数、120/600 秒时长、20 viewer 或阈值不同的 full 配置。

## 6. 证据和复核

每个运行目录至少包含：

- `evidence.json`、`stdout.log`、`stderr.log`；
- `a49-seed-report.json`：画像行数、指标总和和 fixture checksum；
- `a49-load-results.jsonl`：全部脱敏原始请求记录，包括失败和 warmup；
- `a49-load-metadata.json`、`a49-report.json`；
- `a49-app.log`：完整 request ID/access log；
- `a49-environment.json`、应用镜像构建日志、Docker stats 时间线；
- MySQL 状态和关键查询计划；
- `a49-artifacts.json`：固定证据文件的相对路径、字节数和 SHA-256；
- `a49-cleanup.json`：最终 Docker label sweep 的结果，容器、网络、卷和镜像残留必须全部为空。

正式结论只有在 `a49-report.json` 同时满足 `mode=full`、`evidence_class=formal`、`acceptance_eligible=true`、`passed=true`，文件清单逐项复算一致，post-cleanup sweep 通过，统一 acceptance runner 的 `evidence.json` 也为 passed，且无 required skip 时才可记为 PASS。docscheck 在移除 `planned:` 后会重新验证完整正式运行目录，而不是只检查目录是否存在。失败和 smoke 目录保留为不可变调试记录，A49 manifest 继续保持 `planned:artifacts/acceptance/A49/`。
