# A52 生产站点接入验收手册

A52 只验证生产站点的只读接入前置条件，不执行任何上游写操作。正式运行必须由具名责任人、独立复核人和批准人准备一个已脱敏的受控材料 ZIP，并通过唯一入口执行：

```powershell
$env:A52_CONTROLLED_INPUT = 'C:\absolute\controlled\a52-input.json'
go run ./scripts/acceptance run -case A52 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a52.ps1
```

输入 JSON 必须声明 `schema_version=1`、`acceptance_id=A52`、`status=passed`、`passed=true`、`evidence_class=formal`、`acceptance_eligible=true`、`scope=controlled_production_read_only`、脱敏 `target_identity`、64 位小写十六进制 `immutable_reference`、RFC3339Nano 起止时间、全部 A52 assertions 为 true，并用绝对 `controlled_material_path` 指向 ZIP。ZIP 精确包含：

- `site-inventory.json`：逐站脱敏身份和统一版本证明；
- `readonly-verification.json`：状态、身份、只读 API 契约、导出能力、`enable_data_export`、quota 保留和首个不可删除 root 证明；
- `approvals.json`：互不相同的 operator、reviewer、approver 及 `approved=true`。

每个 ZIP JSON 都必须使用 schema 1、对应 acceptance ID/文件名 artifact type、`passed=true`、`sanitized=true`。runner 不接受相对路径，不读取或修改上游 new-api，不保存 Token、密码、DSN 或原始主机秘密。缺少材料时 runner 只写 blocked 报告并失败，manifest 必须继续保持 `planned:`。

正式证据由专用 validator 校验 canonical command、封闭文件集合、ZIP 内精确文件、独立审批、用例断言和逐文件 SHA-256；任意通用 `exit 0`、blocked 报告、额外文件或篡改均不得通过。
