# 容器化部署说明（Docker / Compose）

## 1. 目标
- 通过 Docker 构建服务镜像。
- 通过 Docker Compose 启动 MySQL + 后端应用 + 前端控制台。

## 2. 文件说明
- `Dockerfile`：多阶段构建 Go 二进制。
- `docker-compose.yml`：编排 MySQL、后端应用与前端控制台。
- `.env.example`：镜像源与镜像标签示例。
- `configs/config.example.yaml`：配置模板。
- `configs/config.yaml`：容器默认运行配置。

## 3. 构建镜像
```bash
cp .env.example .env
cp configs/config.example.yaml configs/config.yaml
docker build -t fetch-bilibili:dev .
```

## 4. 使用 Compose 启动
```bash
docker compose up -d --build
```

启动后默认访问地址：
- 前端控制台：`http://localhost:5173`
- 后端服务：`http://localhost:8080`
- MySQL：`localhost:3307`

## 5. 配置与挂载
- 默认要求先准备 `configs/config.yaml`；`configs/config.example.yaml` 只作为模板，不直接挂载运行。
- 应用配置：默认映射 `/app/config.yaml`。
- 视频存储目录：`./data/bilibili` → `/data/bilibili`。
- MySQL 数据目录：`./data/mysql` → `/var/lib/mysql`。
- 配置文件路径可通过环境变量 `FETCH_CONFIG` 覆盖。
- 前端容器默认通过挂载 `frontend/dist` 发布静态站点。
- 前端容器使用独立的 `nginx:alpine` 镜像提供静态页面，不再复用后端镜像。
- 因此前端在执行 `docker compose up -d --build` 之前，需要先在宿主机完成一次前端构建。
- 当前前端默认以 `API 模式` 启动，默认访问后端地址 `http://localhost:8080`。
- 在前端设置页保存配置后，如内容有变化，`app` 容器会自动重启；页面会显示「重启中 / 已恢复」状态，重启窗口内短暂不可用属于正常现象。

### 5.1 前端构建后再启动容器
```bash
cp .env.example .env
cp configs/config.example.yaml configs/config.yaml

cd frontend
npm install --registry=https://registry.npmmirror.com
npm run test:state
npm run test:vite-config
npm run test:smoke
npm run build

cd ..
docker compose up -d --build
```

### 5.2 博主列表文件挂载示例
```yaml
  app:
    volumes:
      - ./configs/config.yaml:/app/config.yaml
      - ./configs/creators.yaml:/app/creators.yaml:ro
```

并在配置文件中指向容器内路径：
```yaml
creators:
  file: "/app/creators.yaml"
  reload_interval: "1m"
```

## 6. 国内镜像源配置
项目默认使用一组已在当前环境验证可用的华为云 SWR Docker Hub 同步地址：

```dotenv
MYSQL_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/mysql:8.0
GO_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/golang:1.22-alpine
ALPINE_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/alpine:3.20
APP_IMAGE=fetch-bilibili-app
FRONTEND_IMAGE=swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/nginx:1.27-alpine
```

`docker compose` 会自动读取项目根目录的 `.env`。如果需要更换镜像源，只需要修改 `.env`，不需要改 `Dockerfile` 或 `docker-compose.yml`。
例如，你也可以按需切到官方源或私有仓库：

```dotenv
MYSQL_IMAGE=mysql:8.0
GO_IMAGE=golang:1.22-alpine
ALPINE_IMAGE=alpine:3.20
FRONTEND_IMAGE=nginx:1.27-alpine
```

### 6.1 Docker Desktop
在 `Settings -> Docker Engine` 中加入：

```json
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io"
  ]
}
```

保存后执行 `Apply & Restart`。

### 6.2 Linux daemon.json
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

### 6.3 使用建议
- 保持镜像 tag 固定，不要使用 `latest`。
- 如果公共镜像在高峰期不稳定，优先切换 `.env` 到你的企业私有仓库或云厂商镜像仓库。
- 如果 `docker compose build app` 在拉取基础镜像阶段失败，优先排查本机代理、Docker Daemon 镜像加速器和当前 `.env` 指向的镜像源，而不是先怀疑项目构建逻辑。
- 前端容器默认使用独立的 `nginx:alpine` 静态服务镜像，避免受后端镜像入口点影响。

## 7. 初始化数据库
首次启动只需要保证数据库已创建，应用容器会在启动阶段自动执行内置迁移：
- 默认配置：`mysql.auto_migrate: true`
- 权威 schema 来源：`migrations/00001_init.sql`
- 成功标志：`docker compose logs app` 中出现「数据库迁移完成」

如果你在生产环境中由外部变更平台统一管理数据库，可将 `mysql.auto_migrate` 关闭；但必须确保对应版本迁移已经先执行完成。

## 8. 启动后验证

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/system/status
open http://localhost:5173
```

预期：
- `GET /healthz` 返回 `ok`
- `GET /system/status` 返回 JSON
- 前端页面可打开，并在 `API 模式` 下自动拉取真实数据
- `docker compose logs app` 可看到中文迁移日志，不再需要手工贴 SQL

## 9. 常见问题
- 启动后无法连接 MySQL：
  - 确认 `configs/config.yaml` 中的 DSN 与 `docker-compose.yml` 一致。
- 前端页面打不开：
  - 确认 `frontend` 服务已启动并监听 `5173` 端口。
- 前端能打开但无法联动：
  - 确认后端 `app` 服务正常运行，并在前端页面中使用 `http://localhost:8080` 作为 API 地址。
- 拉取镜像超时：
  - 先执行 `cp .env.example .env`
  - 再确认 Docker Daemon 已配置 `registry-mirrors`
  - 最后执行 `docker compose config` 检查实际生效的镜像地址
- Docker 构建时被本地代理拦截：
  - `./scripts/deploy.sh` 与 `powershell -File .\scripts\deploy.ps1` 会在识别到 `127.0.0.1:7890` 一类 BuildKit 本地代理残留错误时，自动改用 `DOCKER_BUILDKIT=0` 重试
  - 如需手工绕过，可直接执行 `DOCKER_BUILDKIT=0 docker compose up -d --build`
  - 典型现象是日志里出现 `127.0.0.1:7890`、`connection reset by peer` 或镜像鉴权失败

## 10. 下一步建议
- 生成 Go 工程骨架后再执行容器构建。
- 增加 `healthcheck` 与 `/healthz` 接口。
