# New API Pilot

New API Pilot 是独立部署的多站点运营管理平台，用于统一管理多个 new-api
站点的授权、客户、账户、数据采集、统计、导出、告警和运维状态。

相邻的 `new-api` 仓库只作为上游接口、工程工具和视觉规范的参考；本项目拥有
独立的 Go Module、数据库、前端和发布流程。

> 项目是否具备发布资格，以
> [`docs/acceptance/manifest.yaml`](./docs/acceptance/manifest.yaml) 为准。
> 仓库中存在实现或测试，不等于所有受控验收已经完成。

## 快速开始

本地开发只要求安装 Docker Desktop。开发栈同时运行 MySQL、Redis、Go API 和
Bun/Rsbuild 前端，不需要宿主机 Go 或 Bun。

```powershell
docker compose -f docker-compose.dev.yml up -d --build api web
```

启动完成后访问：

- 前端：<http://localhost:5173>
- API 健康检查：<http://localhost:3000/healthz>
- MySQL：`localhost:3307`

开发环境初始管理员为 `admin`，首次密码来自
`docker-compose.dev.yml` 的 `PLATFORM_BOOTSTRAP_ADMIN_PASSWORD`，首次登录后必须
修改。开发配置不读取 `.env.example`。

查看状态和日志：

```powershell
docker compose -f docker-compose.dev.yml ps
docker compose -f docker-compose.dev.yml logs -f api web mysql redis
```

停止开发栈但保留数据：

```powershell
docker compose -f docker-compose.dev.yml down
```

## 开发栈

| 服务 | 容器 | 作用 | 更新方式 |
|---|---|---|---|
| `web` | `new-api-pilot-dev-web` | Bun/Rsbuild 前端，端口 5173 | 源码 bind mount，HMR 自动更新 |
| `api` | `new-api-pilot-dev-api` | Go 后端 API，端口 3000 | 修改后重建开发镜像 |
| `mysql` | `new-api-pilot-dev-mysql` | MySQL 8.4，端口 3307 | 使用持久化开发卷 |
| `redis` | `new-api-pilot-dev-redis` | Redis 7 | 使用持久化开发卷 |

开发 Compose 使用独立项目名和稳定数据卷，不会与生产 Compose 互相重建。前端
将 `/api` 代理到容器网络内的 `api:3000`。

前端源码修改不需要重建 API：

```powershell
docker compose -f docker-compose.dev.yml up -d web
```

后端、依赖、Dockerfile、Compose 或运行配置修改后执行：

```powershell
docker compose -f docker-compose.dev.yml up -d --build api web
```

## AI Agent 开发约定

本项目日常以 AI Agent 开发为主。强制工作流定义在
[`AGENTS.md`](./AGENTS.md)，其核心要求是：

1. 先判断是功能/契约变更还是保持契约的缺陷修复；
2. 功能或外部契约变更必须先更新权威详细设计；
3. 文档、实现、测试、配置和暴露入口必须形成完整纵向切片；
4. 根据影响范围执行后端、前端和文档验证；
5. 完成后只刷新 `docker-compose.dev.yml` 开发栈；
6. API 和 Web 均健康，且 <http://localhost:5173> 可访问后才能交付。

README 用于项目介绍和可复制操作；`AGENTS.md` 是 AI Agent 的权威执行规则；
`docs/` 是产品、架构、API 和运维契约的权威来源。三者不得互相替代。

## 技术栈

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
config/       启动配置与校验
controller/   HTTP 请求解析和响应映射
dto/          API 请求与响应契约
model/        GORM 模型、仓储、migration 和 seed
router/       Gin 路由与中间件组合
service/      业务规则和编排
worker/       调度器与后台任务
web/          React 前端、单元测试和 Playwright E2E
docs/         权威详细设计、运维基线和验收清单
```

## 配置与镜像

- `docker-compose.dev.yml`：本地开发栈，不读取正式秘密；
- `Dockerfile.dev`：只构建 Go 后端，前端由 `web` 服务提供；
- `docker-compose.yml`：正式部署编排；
- `Dockerfile`：构建包含前端静态资源的正式镜像；
- `.env.example`：正式部署配置模板，不会自动成为实际配置；
- `.env`：服务器实际部署配置，不应提交 Git。

默认依赖源：

```dotenv
BUN_IMAGE=docker.m.daocloud.io/oven/bun:1.3.13-alpine
GO_IMAGE=docker.m.daocloud.io/library/golang:1.25-alpine
RUNTIME_IMAGE=docker.m.daocloud.io/library/alpine:3.22
MYSQL_IMAGE=docker.m.daocloud.io/library/mysql:8.4
REDIS_IMAGE=docker.m.daocloud.io/library/redis:7-alpine
GO_MODULE_PROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct
GO_SUM_DATABASE=sum.golang.google.cn
ALPINE_MIRROR=https://mirrors.aliyun.com/alpine
```

## 质量检查

前端：

```powershell
Set-Location web
bun run check
bun run test:unit
```

后端、文档和发布门禁统一通过 Docker 执行：

```bash
make test-api-docker
make docs-check
make acceptance
```

如果当前系统没有 GNU Make，AI Agent 必须执行 Makefile 中等价的 Docker 命令，
不能改用宿主机 Go，也不能把开发数据库作为测试数据库。集成测试只能使用隔离的
`new_api_pilot_test_*` 数据库。

## 权威文档与验收

- 产品、架构、API 和运维约束：[`docs/`](./docs/)
- AI Agent 执行规则：[`AGENTS.md`](./AGENTS.md)
- 验收清单：[`docs/acceptance/manifest.yaml`](./docs/acceptance/manifest.yaml)
- 受控演练：[`docs/acceptance/runbooks/`](./docs/acceptance/runbooks/)
- 验收证据：`artifacts/acceptance/Axx/<run-id>/`

不得根据单个测试通过或已有证据目录宣称项目已经发布完成；只有 required 门禁和
受控验收全部满足后，才能更新发布结论。
