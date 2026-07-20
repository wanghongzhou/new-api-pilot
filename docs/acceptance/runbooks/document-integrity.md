# 文档完整性验收记录模板

适用用例：A83。此文件是模板，不是已完成的验收证据。

## 1. 检查信息

| 项目 | 记录 |
|---|---|
| commit/工作树状态 | `<commit>` / `<clean-or-diff-reference>` |
| 文档检查工具版本 | `<version>` |
| fixture manifest/checksum | `<version>` / `<checksum>` |
| 开始/结束时间 | `<start>` / `<end>` |
| 执行人/复核人 | `<operator>` / `<reviewer>` |

## 2. 前置条件

- `make docs-check` 已按本目录 README 的首批基础设施要求实现；工具和依赖版本已锁定。
- 负向测试在临时 worktree 或临时副本中执行，不直接改写设计基线；每次只引入一种缺陷并从同一干净 commit 重建环境。
- 检查输出稳定、可机器读取，错误消息能定位文件、编号或 key；i18n 配置固定且只能加载 `zh-CN`，不得用另一份列表掩盖额外 locale、语言检测或切换入口。

## 3. 正向检查

执行 `make docs-check` 并保存命令、退出码和完整报告。至少验证：

- D01～D141、R01～R10、A01～A102 连续且唯一；每个 D 的 R/A 引用合法，需求域、详细决策、验收项之间不存在孤儿。
- `docs/acceptance/manifest.yaml` 恰有 A01～A102，每项必填字段完整，R/fixture/layer 合法，运行手册路径存在；`planned:` 测试路径只允许用于尚未实现的功能。
- F01～F13 固定路径均在 fixture manifest 中，文件 checksum、版本和引用一致；F12、02 §7.1.1 与 constant 恰有同一组 19 个 task_type/category/trigger_class/purpose_key，F13 与 05C §34.7 恰有同一组五个 operation_id/category/trigger_class。
- MessageRef registry、精确 params 和单一 `zh-CN` 资源一致，无缺键、重复或空值；不存在其他 locale、语言检测或切换入口。
- 全部 Markdown 内部锚点、相对文件链接和受控外部链接有效。

## 4. 确定性负向检查

在五个相互独立的临时副本中分别执行：

1. 删除一个 D 到 R/A 的映射，检查必须非零退出并定位该 D。
2. 删除一个 A manifest 项，检查必须报告断号或缺项。
3. 删除一个 MessageRef 参数或 `zh-CN` key，或增加额外 locale/语言切换配置，检查必须定位 registry/i18n 差异。
4. 制造一个失效 Markdown 链接，检查必须定位源文件和目标。
5. 修改 fixture 内容但不更新 checksum，检查必须报告 checksum 不一致。

每个负向检查后从干净 commit 重新创建副本；不得用“恢复后仍有本地残留”的环境执行下一项。最后在原始工作树再次运行正向检查并要求零退出。

## 5. 通过标准与证据

- 正向检查通过；五类缺陷均稳定失败且错误可定位；恢复基线后全部检查再次通过。
- 报告确认编号范围、逐 D 映射、manifest、fixture/checksum、MessageRef/params、单一 `zh-CN` i18n 和 Markdown 链接全部覆盖。
- 证据目录包含 commit、依赖版本、每次命令/退出码/完整输出、临时变更 diff、最终正向报告和复核签字。

最终结论：`<PASS/FAIL>`；证据目录：`<evidence-path>`。
