# 多站点运营管理平台 — 详细设计 04：业务功能与平台 API 索引

> 上级文档：[多站点运营管理平台-概要设计.md](./多站点运营管理平台-概要设计.md)  
> 详细设计索引：[多站点运营管理平台-详细设计.md](./多站点运营管理平台-详细设计.md)

业务功能按页面和领域拆分为三个文档：

1. [04A：站点、客户与账户](./多站点运营管理平台-详细设计-04A-站点客户与账户.md)
   - 站点生命周期、授权、状态和资源入口；
   - 纳管账户的固定绑定、归档和恢复；
   - 客户状态、账户列表和跨站统计入口。
2. [04B：统计、Dashboard 与导出](./多站点运营管理平台-详细设计-04B-统计Dashboard与导出.md)
   - 六级统计页面和查询交互；
   - Dashboard 区块和实时完整性；
   - Excel/CSV 导出任务和资源限制。
3. [04C：告警、配置与通用 API](./多站点运营管理平台-详细设计-04C-告警配置与通用API.md)
   - 告警状态、阈值、适用条件和钉钉；
   - 系统配置；
   - API 响应、分页、错误码和完整性规范。

章节编号继续沿用 §15～§22。完整请求 DTO、权限和路由权威定义见后端实现 05C。

# F1 充值与兑换码运营

平台提供只读的全局及站点充值订单、兑换码列表、统计、完整性与导出能力。Viewer/Admin 均可读，所有写操作、补单、生成/查看兑换码密钥、远端财务变更均不属于平台能力。充值金额不得跨站汇总；页面和 API 必须标注 money/amount 为无统一币种与 provider 语义的名义值。

# T1 异步任务运营

提供只读全局/站点任务列表、状态统计、完整性与导出。Viewer/Admin 可读，不提供提交、取消、重试、下载结果或查看失败原文动作。
模型目录提供全局/站点 catalog、coverage、missing，以及 site/vendor/status breakdown 和 completeness；icon URL 保持纯文本，平台不主动加载。
订阅计划目录提供全局/站点 catalog 与统计、completeness/site breakdown；明确不提供跨站订阅收入、订单或用户订阅库存统计。
# D138 定价与分组目录

平台提供只读全局/强制站点 pricing catalog、group catalog、统计、completeness 与 CSV/XLSX 导出。Viewer/Admin 均可读；页面明确区分“站点已配置分组”与“usage 中已观察分组”，并注明 pricing 是 root 视角的配置快照，不是跨站账单或统一货币报价。平台不提供任何远端配置、渠道、Token、兑换码、订阅或任务 mutation。

# D139 system-task 只读运营

平台提供五类 system-task 的全局和强制站点只读列表、统计、完整性和 CSV/XLSX 导出，Viewer/Admin 均可读。页面与 API 不提供创建日志清理、提交、取消、重试、逐任务详情或任何远端 mutation；不得把 D134 的上游异步业务任务与 D139 的站点内部 system-task 混为同一资源。长期 pending/running 默认可见且永不按年龄 retention 删除；terminal retention、截断、ID gap、current 补齐和失败状态必须作为用户可见完整性说明。
