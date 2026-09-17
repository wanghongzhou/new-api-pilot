# A52 生产站点接入验收手册

A52 只验证生产站点的只读接入前置条件，不执行任何上游写操作。正式运行使用一个已脱敏的受控材料 ZIP，并通过唯一入口执行；不要求具名责任人、复核人、批准人或身份分离：

```powershell
$env:A52_CONTROLLED_INPUT = 'C:\absolute\controlled\a52-input.json'
go run ./scripts/acceptance run -case A52 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a52.ps1
```

输入 JSON 必须声明 `schema_version=1`、`acceptance_id=A52`、`status=passed`、`passed=true`、`evidence_class=formal`、`acceptance_eligible=true`、`scope=controlled_production_read_only`、脱敏 `target_identity`、64 位小写十六进制 `immutable_reference`、RFC3339Nano 起止时间、全部 A52 assertions 为 true，并用绝对 `controlled_material_path` 指向 ZIP。A52 不再要求各站点版本相同，也不把仓库当前没有执行的 quota 保留策略检查伪装成代码验收。ZIP 精确包含：

- `site-inventory.json`：逐站脱敏身份、逐站实际版本和只读范围，checks 精确为 `per_site_identity_recorded`、`per_site_version_recorded`、`readonly_scope`；
- `readonly-verification.json`：与当前 `SiteAuthorizationResult.capabilities` 完全一致，checks 精确为 `status_contract`、`self_identity`、`root_identity`、`first_user_proof`、`user_pagination`、`channel_pagination`、`data_export_enabled`、`flow_contract`、`data_contract`、`flow_data_consistency`、`instance_contract`、`realtime_contract`；其中 `flow_data_consistency` 可记录当前代码允许的无流量 skipped 证明，但不得缺项。

每个 ZIP JSON 都必须使用 schema 1、对应 acceptance ID/文件名 artifact type、`passed=true`、`sanitized=true`、RFC3339Nano `observed_at`、64 位小写十六进制 `reference_sha256` 及精确 checks 集合；多项、少项或 false 均失败。runner 不接受相对路径，不读取或修改上游 new-api，不保存 Token、密码、DSN 或原始主机秘密。缺少材料时 runner 只写 blocked 报告并失败，manifest 必须继续保持 `planned:`。

正式证据由专用 validator 校验 canonical command、封闭文件集合、ZIP 内精确技术文件、当前代码对应的用例断言和逐文件 SHA-256；材料不得包含或要求 `approvals.json`、operator、reviewer、approver。任意通用 `exit 0`、blocked 报告、额外文件或篡改均不得通过。
