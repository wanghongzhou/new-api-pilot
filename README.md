# New API Pilot

## Container Validation And Local Refresh

- Run backend tests through `make test-api-docker`; the command uses a Docker
  Go image and the isolated `new_api_pilot_test` database.
- After every change, rebuild and restart the local API with
  `docker compose up -d --build api`, then verify that the `api` service is
  healthy.

New API Pilot 是独立部署的多站点运营管理平台。它集中管理多个 new-api
站点的授权、客户和账户、数据采集、统计、导出、告警与运维状态。相邻的
`new-api` 仓库仅用于参考上游接口和工程约定；本项目拥有独立的 Go Module、
数据库、前端构建和发布流程。

项目代码已经包含主要业务域，但尚未通过全部发布验收。当前完成度和发布
结论必须以 [`docs/acceptance/manifest.yaml`](./docs/acceptance/manifest.yaml)
为准；其中仍标记为 `planned:` 的测试或证据，以及依赖受控环境或外部确认的
验收项，都会阻断最终发布。

## 已实现范围

- 平台登录、Session、平台用户和角色权限；
- 站点接入、预检、授权、生命周期、能力检查和实例资源状态；
- 客户、托管账户、远端用户同步和恢复流程；
- 小时采集、历史回填、完整性窗口、任务调度和失败恢复；
- 全局、站点、客户、账户、模型和通道统计，以及 Dashboard；
- CSV/XLSX 导出任务、文件下载和容量保护；
- 告警规则、告警事件、钉钉投递和系统设置；
- migration、健康/就绪检查、Prometheus 指标、备份和恢复校验工具。

前端产品语言固定为简体中文 `zh-CN`，不提供语言检测、语言切换或其他
locale。所有用户可见文案仍通过 i18next 管理并接受静态检查。

## 技术栈与结构

- 后端：Go 1.25、Gin、GORM、MySQL 8；
- 前端：React 19、TypeScript、Rsbuild、TanStack Router/Query；
- UI：Base UI、shadcn base-nova、Tailwind CSS v4、Hugeicons；
- 工具：Bun、tsgo、oxlint、oxfmt、Playwright、Docker Compose。

后端依赖保持单向流动：

```text
router -> controller -> service -> model
```

主要目录：

```text
common/       通用基础设施、响应、加密、Session 和指标
config/       环境配置与校验
controller/   HTTP 请求解析和响应映射
dto/          API 请求与响应契约
model/        GORM 模型、仓储和 migration
router/       Gin 路由与中间件组合
service/      业务规则和编排
worker/       调度器与后台任务
web/          React 前端、单元测试和 Playwright E2E
docs/         权威需求、详细设计和验收清单
```

## 本地启动

环境要求：Docker Desktop、Bun 1.3.13。后端验证统一在 Docker 测试镜像内执行，不要求或接受宿主机 Go 结果作为发布证据。

```powershell
Copy-Item .env.example .env
docker compose up -d --build
```

Dockerfile 默认使用 Docker Hub/上游官方镜像，不绑定单一镜像站。受限网络可在
`.env` 中覆盖 `BUN_IMAGE`、`GO_IMAGE`、`RUNTIME_IMAGE`、`MYSQL_IMAGE`、
`REDIS_IMAGE`、`GO_MODULE_PROXY`、`GO_SUM_DATABASE` 和 `ALPINE_MIRROR`，无需修改
受版本控制的构建文件。例如使用国内镜像时可配置：

```dotenv
BUN_IMAGE=docker.m.daocloud.io/oven/bun:1.3.13-alpine
GO_IMAGE=docker.m.daocloud.io/library/golang:1.25-alpine
RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.22
MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:8.4
GO_MODULE_PROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
GO_SUM_DATABASE=sum.golang.google.cn
ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
```

完全离线时仅导入基础镜像并不足以重新构建，因为 Go Module、Bun 包和 Alpine
软件包也需要缓存。推荐在联网环境构建并用 `docker save new-api-pilot-dev:local`
导出应用镜像，目标主机 `docker load` 后执行
`docker compose up -d --no-build api`。需要离线重新构建时，必须同时转移精确基础
镜像与已预热的 BuildKit 缓存；Dockerfile 会复用 Go Module/build cache。无论哪种
方式都应保持 `.env` tag 一致，不应通过临时编辑 Dockerfile 切换来源。

默认地址：

- 应用/API：<http://localhost:3000>
- MySQL：`localhost:3307`

容器启动会执行数据库初始化。需要单独执行 migration 门禁时使用：

```powershell
docker compose run --rm --entrypoint /usr/local/bin/new-api-pilot api migrate
```

前端热更新开发服务器运行在 <http://localhost:5173>，并将 `/api` 代理到后端：

```powershell
Set-Location web
bun install --frozen-lockfile
bun run dev
```

使用 GNU Make 时，对应入口为 `make dev-api`、`make dev-web`、`make logs` 和
`make down`。

## 质量检查

前端检查和测试：

```powershell
Set-Location web
bun run check
bun run test:unit
bun run test:e2e
```

后端、文档和监控规则：

```powershell
make test-api-docker
make docs-check
docker run --rm --entrypoint /bin/promtool -v "${PWD}:/workspace:ro" prom/prometheus:v3.5.0 check rules /workspace/deploy/prometheus/recording-rules.yaml /workspace/deploy/prometheus/alert-rules.yaml
```

GNU Make 提供相同的聚合入口：

```bash
make check-web
make test-api-docker
make test-support
make check-prometheus
make contract-generate
make docs-check
```

完整发布门禁需要 Docker Compose 中独立的 `new_api_pilot_test` 集成测试数据库和 Redis；不得使用开发数据库或宿主机 Go：

```bash
make acceptance TEST_DATABASE_DSN='user:password@tcp(127.0.0.1:3306)/pilot_test?charset=utf8mb4&parseTime=True&loc=Asia%2FShanghai'
```

`make acceptance` 会先运行 final 文档检查；任何 `planned:` 路径、缺失或过期
证据、required 用例跳过、受控演练失败都会使命令失败。容量、生产站点清单、
部署回滚和 PITR 等项目还需要对应的资源、外部确认或隔离演练，仓库内存在
实现或模板不等于这些验收已经通过。

## 需求与证据

- 权威需求和约束位于 [`docs`](./docs/)；
- 验收编号、fixture、责任角色、测试路径和证据路径以
  [`docs/acceptance/manifest.yaml`](./docs/acceptance/manifest.yaml) 为准；
- 受控演练说明位于 [`docs/acceptance/runbooks`](./docs/acceptance/runbooks/)；
- 每次验收证据写入独立的 `artifacts/acceptance/Axx/<run-id>/` 目录。

不要根据单个测试通过或已有证据目录宣称整个项目完成；只有 A01-A88 的
required 门禁全部关闭后，才可以更新发布结论。
