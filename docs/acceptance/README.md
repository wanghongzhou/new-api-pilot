# 设计基线验收清单

本目录是概要设计和详细设计的**设计基线实施清单**，用于把 R01～R10 与 A01～A100 转换为开发阶段可执行、可追踪、可审计的验收项。它证明需求已有实施落点，但不代表功能、测试、演练或发布已经完成。

## 目录内容

- `manifest.yaml`：A01～A100 的唯一执行索引；每项指定主需求域、固定 fixture、验收层、测试或运行手册、责任角色和证据目录。
- `../../testdata/design/message-ref-openapi.json`：由 MessageRef catalog 生成的 OpenAPI 3.1 判别联合契约；`make contract-generate` 会同步更新该文件和完整 fixture checksum 清单，`make docs-check` 会拒绝 catalog、HTTP code、MessageRef code、params schema 或 checksum 的漂移。
- `runbooks/`：只能在隔离或受控环境执行的部署、恢复、容量、故障和文档完整性演练模板。
- `planned:` 路径：实现尚未开始时允许不存在。实现对应功能的同一开发任务必须创建该路径并移除 `planned:` 前缀，不能把它留到发布前处理。
- F01～F11 及 `testdata/design/manifest.sha256` 是固定的实施契约路径，不表示当前文件已经存在；首批基础设施任务必须将它们版本化落盘。fixture 不存在或 checksum 不匹配时，任何引用它的用例都不可执行、不可判定通过。
- 运行手册文件当前只是模板；只有完成执行、复核并将完整记录写入对应 `evidence_path` 后，相关 A 用例才算通过。

## 首批开发基础设施

进入开发阶段后，以下自动化必须作为第一波基础设施任务落地：

1. `make docs-check`：检查 Markdown 链接、D01～D139/R01～R10/A01～A100 连续性与映射、MessageRef catalog/params、单一 `zh-CN` i18n 资源、fixture manifest/checksum，以及本清单的结构完整性；额外 locale、语言检测或切换配置必须失败。
2. `cd web && bun run test:e2e`：执行 Playwright 桌面端和移动端页面验收。
3. `make acceptance`：串联 `make docs-check`、Docker 后端测试镜像（含 gofmt/go vet/go test 与独立 `new_api_pilot_test` 集成库）、`cd web && bun run check`、E2E 和受控演练证据检查；宿主机 Go 结果不构成发布证据。
4. `make check-prometheus`：使用固定版本 `promtool` 校验 recording/alert rules；它只证明规则文件可解析，受控环境仍需保存规则加载 API 和实际告警路由证据。

这些命令在实际创建并通过之前只属于计划，不得作为“实现完成”或“验收通过”的证据。

## Manifest 约定

每个 `acceptance_cases` 项必须且只能对应一个 A 编号，并包含以下字段：

| 字段 | 约束 |
|---|---|
| `acceptance_id` | `A01`～`A100`，连续、唯一 |
| `requirement_id` | 一个主需求域 `R01`～`R10`；跨域覆盖仍由设计追踪矩阵保留 |
| `fixture` | 非空 F01～F11 列表；执行时记录实际版本和 checksum |
| `layer` | `integration`、`contract`、`e2e`、`static-analysis` 或 `runbook` |
| `test_or_runbook_path` | 向后兼容的单个可执行测试路径或本目录下的受控演练模板；与 `test_or_runbook_paths` 互斥；`planned:` 表示首批实现待建 |
| `test_or_runbook_paths` | 推荐的非空测试路径数组；路径必须唯一且逐条存在；与 `test_or_runbook_path` 互斥 |
| `owner_role` | `backend`、`frontend`、`sre`、`security` 或 `qa`；对实现和证据负责，不替代独立复核人 |
| `evidence_path` | 每次执行的不可覆盖证据目录；计划阶段使用 `planned:` |

固定 fixture 的内容和路径以 `manifest.yaml` 及详细设计 §51.1 为准。测试必须使用可注入 Clock，不依赖运行当天时间。

## 从计划到通过

1. 功能任务开始时，责任人确认关联 A 项、fixture 和断言，创建计划测试路径，并移除该项路径的 `planned:` 前缀。
2. 自动化用例按 Given/When/Then 实现，同时断言 HTTP 状态与 `code`/DTO、数据库不变量、用户可见状态及外部副作用。运行手册按模板执行并由另一角色复核。
3. 每次验收写入独立证据目录，至少包含 commit、镜像 digest、fixture 版本/checksum、命令或操作记录、开始/结束时间、结果以及日志/报告路径；秘密和 Webhook 查询参数必须脱敏。
4. `make acceptance` 汇总结果。失败、缺证据、证据过期、路径仍为 `planned:` 或 required 用例被 skipped，均阻断发布。

## 发布与变更规则

A01～A100 全部是 required。发布不得跳过、降级为口头确认，或以另一个相似用例替代；受环境限制的用例必须通过对应运行手册在受控环境完成。任何失败先修复并重跑，再由非执行人复核证据。

A89～A100 必须使用 `test_or_runbook_paths` 同时绑定四类可执行资产：`tests/integration/` 后端集成测试、`tests/contract/` 或 `*.test.ts(x)`/非 integration Go `*_test.go` contract/unit 测试、在 Playwright `chromium-desktop` 与 `chromium-mobile` 双项目下执行的 `web/e2e/` 用例，以及名称明确标识 privacy/absence/security/safety 或 fixture consumption 的安全边界测试。任一类别、任一路径或桌面/移动项目配置缺失都必须由 docscheck 阻断。

新增或修改 D、R、A、fixture、MessageRef 或外部行为时，必须在同一变更中同步设计追踪矩阵、`manifest.yaml`、测试/运行手册和证据检查。只有设计评审、自动化结构检查和语义验收均通过，才能更新发布结论。
