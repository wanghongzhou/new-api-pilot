# A75 PITR 与密钥恢复受控演练

A75 只在隔离环境或 Pilot-owned 恢复目标上执行，不切换生产流量，不修改上游 new-api。正式运行唯一入口：

```powershell
$env:A75_CONTROLLED_INPUT = 'C:\absolute\controlled\a75-input.json'
go run ./scripts/acceptance run -case A75 -- powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/acceptance/run-a75.ps1
```

输入 JSON 必须为 schema 1 的 formal passing 报告，`scope=controlled_pilot_owned_isolated`，绑定脱敏目标身份、不可变备份/恢复引用、起止时间和全部 A75 assertions，并用绝对路径引用受控材料 ZIP。ZIP 精确包含：

- `backup.json`：全量备份、binlog 位点和保留密钥版本证明；
- `restore.json`：隔离目标身份、恢复步骤和不切生产证明；
- `verify-restore.json`：full verify-restore、密文、外键、任务、事实与汇总验证；
- `rpo-rto.json`：实测 RPO 不超过 1 小时、RTO 不超过 4 小时；
- `cleanup.json`：Pilot-owned 临时资源清理且无残留；
- `approvals.json`：互不相同的具名 operator、reviewer、approver。

所有材料必须脱敏并标记 passed。缺少备份/binlog/密钥、隔离目标、全量验证、RPO/RTO 或审批时 runner 只生成 blocked 失败证据；不得用 A22、A51 或手工报告代替 A75。
