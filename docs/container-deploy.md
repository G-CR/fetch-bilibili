# 容器化部署说明（Docker / Compose）

本文档只描述当前仓库已经存在的容器化部署方式：`Dockerfile`、
`docker-compose.yml`、`scripts/deploy.sh` 与 `scripts/deploy.ps1`。

## 1. 部署形态

- `docker-compose.yml` 当前编排 3 个服务：
  - `mysql`
  - `app`
  - `frontend`
- `app` 镜像由仓库根目录 `Dockerfile` 多阶段构建。
- `frontend` 当前不在容器内构建，而是直接挂载宿主机的 `frontend/dist` 静态产
  物，由独立 `nginx:alpine` 容器提供页面。
- 因此，`docker compose up -d --build` 之前，必须先在宿主机完成一次前端构
  建。

## 2. 关键文件

- `Dockerfile`
  - 构建 Go 二进制，并在运行镜像中安装 `ffmpeg`。
- `docker-compose.yml`
  - 编排 MySQL、后端应用和前端静态站点。
- `.env.example`
  - 镜像仓库与标签模板。
- `configs/config.example.yaml`
  - 配置模板，不直接作为运行时挂载文件。
- `configs/config.yaml`
  - 默认容器运行配置。
- `scripts/deploy.sh`
  - Shell 部署入口。
- `scripts/deploy.ps1`
  - PowerShell 部署入口。

## 3. 准备步骤

最小准备命令：

```bash
cp .env.example .env
cp configs/config.example.yaml configs/config.yaml
```

如需使用博主文件同步，再准备：

```bash
cp configs/creators.example.yaml configs/creators.yaml
```

前端构建步骤：

```bash
cd frontend
npm install --registry=https://registry.npmmirror.com
npm run test:state
npm run test:vite-config
npm run test:smoke
npm run build
cd ..
```

## 4. 当前 Compose 行为

当前默认端口：

- 前端控制台：`http://localhost:5173`
- 后端服务：`http://localhost:8080`
- MySQL：`localhost:3307`

当前默认挂载：

- `./data/mysql` -> `/var/lib/mysql`
- `./data/bilibili` -> `/data/bilibili`
- `./configs/config.yaml` -> `/app/config.yaml`
- `./frontend/dist` -> `/usr/share/nginx/html`

可选挂载：

- `./configs/creators.yaml:/app/creators.yaml:ro`

若启用博主文件挂载，配置中应写：

```yaml
creators:
  file: "/app/creators.yaml"
  reload_interval: "1m"
```

补充说明：

- `FETCH_CONFIG` 在容器内当前固定指向 `/app/config.yaml`。
- `app` 服务配置了 `restart: unless-stopped`。
- 但前端设置页保存配置后的“重启”当前主要依赖后端进程内自重启，不依赖容
  器被 Docker 重新拉起。
- 保存配置成功后，页面会出现「重启中 / 已恢复」状态；这个窗口内接口短暂
  不可用属于当前承认的正常现象。

## 5. 构建与启动

直接使用 Compose：

```bash
docker compose up -d --build
```

若只想更新后端容器：

```bash
docker compose up -d --build app
```

仓库内现有部署脚本：

- `bash scripts/deploy.sh deploy-all`
  - 执行后端测试、前端快速测试、前端构建，再启动全部容器。
- `bash scripts/deploy.sh deploy-app`
  - 只验证并部署 `app`。
- `bash scripts/deploy.sh restart`
  - 重启 `app` 与 `frontend`。
- `bash scripts/deploy.sh status`
  - 查看当前服务状态。
- `pwsh -NoProfile -File scripts/deploy.ps1 <command>`
  - PowerShell 侧提供与 Shell 版等价的命令语义。

## 6. 镜像源

项目默认通过 `.env` 提供一组可替换的镜像变量：

```dotenv
MYSQL_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/mysql:8.0
GO_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/golang:1.22-alpine
ALPINE_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/alpine:3.20
APP_IMAGE=fetch-bilibili-app
FRONTEND_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/nginx:1.27-alpine
```

说明：

- `docker compose` 会自动读取项目根目录的 `.env`。
- 若需要切换官方源、私有仓库或其他国内源，只需改 `.env`，不必改
  `Dockerfile` 或 `docker-compose.yml`。
- 建议固定具体 tag，不要使用 `latest`。

如需宿主机层面的镜像加速，可额外配置 Docker Daemon 的
`registry-mirrors`。

## 7. 数据库与启动检查

首次部署前只需要保证数据库可创建；应用容器启动后会在
`mysql.auto_migrate: true` 下自动执行 `migrations/*.sql`。

成功标志：

- `docker compose logs app --tail=200` 中出现 `数据库迁移完成`
- `docker compose ps` 中 `mysql` 与 `app` 均已启动
- 前端页面可访问

推荐检查命令：

```bash
docker compose config
docker compose ps
docker compose logs app --tail=200
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/system/status
```

预期：

- `GET /healthz` 返回 `ok`
- `GET /system/status` 返回 JSON
- 前端页面能打开并自动请求 `http://localhost:8080`

## 8. 常见问题

- 启动后无法连接 MySQL
  - 先核对 `configs/config.yaml` 中 DSN 与 `docker-compose.yml` 是否一致。
- 前端页面打不开
  - 确认已经先构建 `frontend/dist`，且 `frontend` 容器处于运行状态。
- 前端能打开但无法联动
  - 确认 `app` 服务正常运行，并在页面中使用 `http://localhost:8080` 作为
    API 地址。
- 保存配置后长期未恢复
  - 先看前端是否仍停留在「等待后端恢复」。
  - 再看 `docker compose logs app --tail=200` 是否存在配置校验或启动报错。
  - 若配置文件没有实际变化，后端不会重启；这时接口应返回
    `changed=false`。
- 拉取镜像超时
  - 先确认项目根目录存在 `.env`，且镜像地址仍可用。
  - 再确认 Docker Daemon 已配置 `registry-mirrors`（如你依赖镜像加速）。
  - 最后执行 `docker compose config` 检查最终生效的镜像地址。
- Docker 构建被本地代理残留拦截
  - 当前 `scripts/deploy.sh` 与 `scripts/deploy.ps1` 都会在识别到
    `127.0.0.1:7890` 一类 BuildKit 本地代理残留错误时，自动改用
    `DOCKER_BUILDKIT=0` 重试。
  - 如需手工绕过，可直接执行：

```bash
DOCKER_BUILDKIT=0 docker compose up -d --build
```

## 9. 相关文档

- 本地直跑与排障：`docs/runbook.md`
- 配置项说明：`docs/config.md`
- API 与前端联动：`docs/api.md`
- 部署与验证规范：`AGENTS.md`
