# fetch-bilibili

一个基于 Go 的后端服务，用于维护 B 站博主列表（UID 或名称），周期性拉取新视频并检查可用性，逐步累积“绝版视频库”。支持博主配置文件动态同步与 HTTP 接口添加。

## 功能特性
- 博主列表文件（YAML/JSON）动态加载与定期刷新
- 文件中移除的博主自动停用（status=paused）
- HTTP 接口添加博主（POST /creators）
- HTTP 运维接口：博主停用/恢复/移除、单视频查询、单视频重下、单视频检查、手动 cleanup
- B 站真实 API 接入（WBI 签名、可用性检查、名称解析）
- Cookie/SESSDATA 支持 + 有效性检查
- 调度器 + 工作池
- MySQL 存储 + 启动自动迁移
- Docker / Compose 部署

## 当前范围与边界
- 已实现：拉取、检查、下载、清理、启动恢复、重复任务保护、下载链路数据一致性修复、基础运维接口
- 未实现：多平台扩展

## 快速开始（Docker Compose）
1) 复制配置文件
```bash
cp .env.example .env
cp configs/config.example.yaml configs/config.yaml
cp configs/creators.example.yaml configs/creators.yaml
```

2) 修改 `configs/config.yaml`
```yaml
storage:
  root_dir: "/data/bilibili"
mysql:
  dsn: "fetch:fetchpass@tcp(mysql:3306)/fetch?charset=utf8mb4&parseTime=true&loc=Local"
  auto_migrate: true
creators:
  file: "/app/creators.yaml"
  reload_interval: "1m"
```

3) 构建前端并启动（构建 + 运行）
```bash
cd frontend
npm install --registry=https://registry.npmmirror.com
npm run test:state
npm run test:vite-config
npm run test:smoke
npm run build

cd ..
docker compose up -d --build
```

4) 确认自动迁移完成
```bash
docker compose logs app --tail=200
```

注意：
- `configs/config.example.yaml` 只是模板；实际启动配置请使用 `configs/config.yaml`。
- 默认 `docker-compose.yml` 将 MySQL 映射到宿主机 `3307:3306`（避免 3306 冲突，可自行改回）。
- 默认 `docker-compose.yml` 同时启动前端容器，访问地址为 `http://localhost:5173`。
- 前端容器会挂载 `frontend/dist` 静态产物，因此在首次 `docker compose up -d --build` 前，需要先执行一次 `frontend` 本地构建。
- 当前前端默认以 `API 模式` 启动，会自动请求 `http://localhost:8080`。
- `docker-compose.yml` 已将 `./configs/config.yaml` 以可写方式挂载到容器内 `/app/config.yaml`，并为 `app` 服务开启了 `restart: unless-stopped`。
- 因此前端设置页保存配置后，后端容器会自动重启并重新加载最新配置；重启窗口内接口可能短暂返回失败，刷新或重新同步即可。
- 后端容器启动时会自动执行 `migrations/00001_init.sql`，日志中出现「数据库迁移完成」即可。
- 项目默认通过 `.env` 中的国内镜像前缀拉取 `mysql`、`golang`、`alpine`。如果要切换镜像源，直接修改 `.env` 即可。
- 如需容器内使用博主文件，请挂载并在配置中指向容器路径：
  - `./configs/creators.yaml:/app/creators.yaml:ro`
  - `creators.file: "/app/creators.yaml"`

## Docker 国内镜像源
项目已经内置国内镜像默认值：

```dotenv
MYSQL_IMAGE=m.daocloud.io/docker.io/library/mysql:8.0
GO_IMAGE=m.daocloud.io/docker.io/library/golang:1.22-alpine
ALPINE_IMAGE=m.daocloud.io/docker.io/library/alpine:3.20
APP_IMAGE=fetch-bilibili-app
FRONTEND_IMAGE=m.daocloud.io/docker.io/library/nginx:1.27-alpine
```

这些变量定义在 `/Users/ws/Desktop/chat/fetch-bilibili/.env.example`。复制为 `.env` 后，`docker compose` 会自动读取。

如果你希望宿主机层面也统一走国内加速，建议再配置 Docker Daemon。

Docker Desktop（Mac / Windows）：
1. 打开 `Settings -> Docker Engine`
2. 将配置改成下面这样

```json
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io"
  ]
}
```

3. 点击 `Apply & Restart`

Linux：

```bash
sudo mkdir -p /etc/docker
sudo tee /etc/docker/daemon.json >/dev/null <<'EOF'
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io"
  ]
}
EOF
sudo systemctl daemon-reload
sudo systemctl restart docker
```

说明：
- 项目内置的镜像前缀方案和 Docker Daemon 加速可以同时使用。
- 如果公共镜像在某个时段不稳定，只需要改 `.env` 里的镜像地址，不用改代码。
- 建议固定具体 tag，不要使用 `latest`。
- 前端容器默认使用独立的 `nginx:alpine` 静态服务镜像，不再复用后端镜像入口点。

## 本地运行（非容器）
1) 创建数据库
```sql
CREATE DATABASE fetch DEFAULT CHARSET utf8mb4;
```

2) 启动服务（自动迁移会在启动阶段执行）
```bash
go run ./cmd/server
```

3) 指定配置文件
```bash
FETCH_CONFIG=/path/to/config.yaml go run ./cmd/server
```

## 配置说明
完整说明见 `docs/config.md`。常用关键字段：
- `storage.root_dir`：本地存储根目录
- `mysql.dsn`：MySQL 连接串
- `mysql.auto_migrate`：是否自动执行内置迁移，默认开启
- `creators.file` / `creators.reload_interval`：博主文件路径与刷新周期
- `storage.cleanup_retention_hours`：视频下载成功后，至少保留多少小时才允许被 cleanup 删除，默认 168 小时
- `scheduler.check_stable_days`：稳定阈值（默认 30 天）
- `scheduler.check_interval`：下架检查周期（默认 24h）
- `bilibili.fetch_page_size`：单次拉取单个博主的最新视频数量，默认 5
- `bilibili.cookie` / `bilibili.sessdata`

## 博主配置文件
支持两种写法（列表 / 包裹字段）：
```yaml
creators:
  - uid: "123456"
    name: "示例博主"
    platform: "bilibili"
    status: "active"
  - name: "仅名称"
    platform: "bilibili"
```

说明：
- 支持 YAML/JSON
- 文件移除的博主会被自动停用（status=paused）
- 如果通过 HTTP `DELETE /creators/{id}` 手工移除博主，服务会将其标记为 `removed`，后续文件同步不会自动恢复。
- 如需恢复已手工移除的博主，可再次通过 `POST /creators` 添加相同 UID。

## HTTP API
### 添加博主
```
POST /creators
Content-Type: application/json

{"uid":"123456","name":"示例博主","platform":"bilibili"}
```

响应示例：
```json
{
  "id": 1,
  "uid": "123456",
  "name": "示例博主",
  "platform": "bilibili",
  "status": "active"
}
```

### 健康检查
- `GET /healthz`
- `GET /readyz`

### 驾驶舱查询接口
- `GET /jobs`
- `GET /videos`
- `GET /system/status`
- `GET /storage/stats`
- `GET /system/config`
- `GET /events/stream`

其中 `GET /system/status` 现已补充：
- Cookie 来源（配置 / Cookie 文件 / SESSDATA 文件）
- 最近一次认证检查与配置刷新结果
- 风控退避剩余秒数、最近命中时间与原因
- 认证监控是否启用

其中 `GET /events/stream` 用于驾驶舱实时事件流：
- 建连后立即返回 `hello`
- 后续增量推送 `job.changed`、`video.changed`、`creator.changed`、`storage.changed`、`system.changed`
- 前端断线后会自动重连，重连成功后自动补一次快照

### 系统配置编辑接口
- `GET /system/config`：读取当前运行配置文件内容与路径
- `PUT /system/config`：保存配置文件；若内容有变化，会先执行 YAML 与业务配置校验，通过后写回文件，并触发后端重启

请求示例：
```http
PUT /system/config
Content-Type: application/json

{"content":"server:\n  http_addr: \":8080\"\n"}
```

响应示例：
```json
{
  "changed": true,
  "restart_scheduled": true,
  "path": "/app/config.yaml"
}
```

## 前端控制台（独立）
前端已切换为独立框架工程，目录为 `frontend/`，默认与 Go 后端解耦部署。

本地启动：
```bash
cd frontend
npm install
npm run dev
```

说明：
- 默认访问地址通常为 `http://localhost:5173`
- 后端地址默认填写 `http://localhost:8080`
- 当前仅保留 `API 模式`，会自动同步真实后端数据
- 页面会先加载一次快照，再连接 `/events/stream` 接收增量事件
- 顶部会展示实时连接状态：连接中 / 实时同步中 / 重连中 / 连接中断
- SSE 正常时，任务、视频、博主、存储和系统状态会自动更新
- 仍保留低频对账：`GET /system/status` 每 30 秒一次，整页快照（含 `storage/stats`）每 60 秒一次
- SSE 重连成功后会自动补一次快照，无需手动刷新
- 当前已接入：
  - 读取博主列表
  - 添加博主
  - 触发任务（fetch/check/cleanup）
  - 查询最近任务
  - 查询最近视频
  - 查询系统状态
  - 查询存储统计
  - 在线读取 `config.yaml`
  - 在设置页编辑配置文件
  - 保存前查看差异预览
  - 展示配置校验结果详情
  - 保存成功后提示后端重启
- 使用 `docker compose up -d --build` 时，会同时启动前端容器
- `web/` 旧静态原型已废弃，不再维护

### 前端配置编辑说明
在页面「设置」区域可以直接维护后端配置：

1. 点击「重新加载」，读取后端当前配置文件内容。
2. 在「配置文件编辑」文本框中修改 YAML。
3. 查看「保存前差异预览」，确认本次变更。
4. 点击「保存配置」。
5. 若校验通过，页面会提示「配置已保存，后端正在重启」。
6. 若校验失败，「校验结果详情」会展示错误原因，配置文件不会被写回。

说明：
- 未发生内容变化时，后端不会重启。
- 在 Docker Compose 部署下，保存成功依赖 `config.yaml` 可写挂载和 `app` 服务自动重启。
- 后端重启后，建议点击一次「同步数据」或「重新加载」确认新配置已生效。

前端测试：
```bash
cd frontend
npm run test:vite-config
npm run test:state
npm run test:smoke
npm run test:e2e
```

浏览器级 E2E：
- 默认 `mock` 模式会自动启动：
  - mock API：`http://127.0.0.1:43180`
  - 前端 dev server：`http://127.0.0.1:43173`
- 覆盖：
  - 打开页面并同步接口数据
  - 添加博主
  - 编辑配置并查看差异预览
  - 配置校验失败时展示错误详情
  - 触发任务并查看任务详情

真实联调可使用 `live` 模式：
```bash
cd frontend
E2E_MODE=live \
E2E_BASE_URL=http://127.0.0.1:5173 \
E2E_API_BASE=http://127.0.0.1:8080 \
npm run test:e2e
```

说明：
- `mock` 模式用于仓库内可重复执行的浏览器测试。
- `live` 模式用于对你本机已启动的真实前后端栈做复验。
- `live` 模式中的“添加博主”用例会显式提交 `uid + name`，避免把外部名称解析能力波动误判成前后端联动失败。

如果网络环境下 `npm install` 较慢，可使用镜像：
```bash
cd frontend
npm install --registry=https://registry.npmmirror.com
```

容器化前端：
```bash
cd frontend
npm run test:state
npm run build

cd ..
docker compose up -d --build frontend
```

## B 站适配说明
当前使用的接口：
- 投稿列表：`GET /x/space/wbi/arc/search`（WBI 签名）
- 可用性检查：`GET /x/web-interface/view`
- 登录状态检查：`GET /x/web-interface/nav`
- 名称解析：`GET /x/web-interface/search/type`

Cookie 支持：
- `bilibili.cookie`：直接填写完整 Cookie
- `bilibili.sessdata`：只填写 `SESSDATA`，服务会自动拼成 Cookie 头
- 认证检查：`auth_check_interval`

## 调度与流程
- 调度器周期性入队 `fetch` / `check` / `cleanup` 任务
- Worker 处理流程：
  - `fetch`：拉取博主视频列表并入库
  - `check`：更新视频状态（OUT_OF_PRINT / STABLE）
  - `cleanup`：当本地存储超过安全阈值时，先跳过仍处于最短保留期内的下载文件，再按“是否绝版 -> 博主粉丝量 -> 播放量 -> 收藏量”顺序清理低价值文件，并优先保留绝版视频
- 稳定阈值：`scheduler.check_stable_days`（默认 30 天）

## 数据模型
结构说明见 `docs/mysql-schema.md`，实际执行来源见 `migrations/00001_init.sql`。

## 测试与覆盖率
```bash
go test ./... -cover
```
覆盖率基线：>= 85%（见 `docs/dev-standards.md`）。

## 常见问题
- MySQL 端口冲突：修改 `docker-compose.yml` 中的宿主机端口映射。
- 表不存在：先确认 `mysql.auto_migrate` 是否保持为 `true`，再检查 `docker compose logs app` 或本地启动日志中是否有迁移报错。
- 403/412：可能触发风控或权限限制，建议配置 Cookie 并降低频率。
- 风控风险：大量请求可能触发限制，建议调小 `limits.global_qps` / `limits.per_creator_qps`，并结合 `bilibili.risk_backoff_*` 退避配置。
- 配置保存后暂时无法联动：
  - 先等待后端容器完成自动重启。
  - 再执行一次页面中的「同步数据」或「重新加载」。
  - 如仍失败，检查 `docker compose logs app --tail=200` 是否存在配置校验或启动报错。
- Docker 拉取慢或超时：
  - 先确认项目根目录存在 `.env`，并且镜像地址仍然可用。
  - 再确认 Docker Desktop 或 `/etc/docker/daemon.json` 已配置 `registry-mirrors`。
  - 如仍失败，可把 `.env` 里的镜像前缀切换为你自己的企业镜像仓库。
- Docker 构建时访问镜像站被 `127.0.0.1:7890` 之类本地代理拦截：
  - 可直接执行 `env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY docker compose up -d --build`
  - 或先临时清理当前 shell 的代理环境变量后再执行 Compose

## 路线图
- 实现视频下载与清理任务
- 增加博主更新/停用/恢复等 HTTP 接口
- 多平台适配（抖音/快手/小红书）
