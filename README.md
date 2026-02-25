# fetch-bilibili

一个基于 Go 的后端服务，用于维护 B 站博主列表（UID 或名称），周期性拉取新视频并检查可用性，逐步累积“绝版视频库”。支持博主配置文件动态同步与 HTTP 接口添加。

## 功能特性
- 博主列表文件（YAML/JSON）动态加载与定期刷新
- 文件中移除的博主自动停用（status=paused）
- HTTP 接口添加博主（POST /creators）
- B 站真实 API 接入（WBI 签名、可用性检查、名称解析）
- Cookie/SESSDATA 支持 + 自动刷新/有效性检查
- 调度器 + 工作池
- MySQL 存储
- Docker / Compose 部署

## 当前范围与边界
- 已实现：拉取、检查与下载流程
- 未实现：清理任务（当前为占位 no-op）

## 快速开始（Docker Compose）
1) 复制配置文件
```bash
cp configs/config.example.yaml configs/config.yaml
cp configs/creators.example.yaml configs/creators.yaml
```

2) 修改 `configs/config.yaml`
```yaml
storage:
  root_dir: "/data/bilibili"
mysql:
  dsn: "fetch:fetchpass@tcp(mysql:3306)/fetch?charset=utf8mb4&parseTime=true&loc=Local"
creators:
  file: "/app/creators.yaml"
  reload_interval: "1m"
```

3) 启动（构建 + 运行）
```bash
docker compose up -d --build
```

4) 初始化数据库表
```bash
awk '/^```sql/{flag=1;next}/^```/{flag=0}flag{print}' docs/mysql-schema.md \
  | docker compose exec -T mysql mysql -uroot -prootpass fetch
```

注意：
- 默认 `docker-compose.yml` 将 MySQL 映射到宿主机 `3307:3306`（避免 3306 冲突，可自行改回）。
- 如需容器内使用博主文件，请挂载并在配置中指向容器路径：
  - `./configs/creators.yaml:/app/creators.yaml:ro`
  - `creators.file: "/app/creators.yaml"`

## 本地运行（非容器）
1) 创建数据库
```sql
CREATE DATABASE fetch DEFAULT CHARSET utf8mb4;
```

2) 启动服务
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
- `creators.file` / `creators.reload_interval`：博主文件路径与刷新周期
- `scheduler.check_stable_days`：稳定阈值（默认 30 天）
- `scheduler.check_interval`：下架检查周期（默认 24h）
- `bilibili.cookie` / `bilibili.sessdata` 或 `bilibili.cookie_file`

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

## B 站适配说明
当前使用的接口：
- 投稿列表：`GET /x/space/wbi/arc/search`（WBI 签名）
- 可用性检查：`GET /x/web-interface/view`
- 登录状态检查：`GET /x/web-interface/nav`
- 名称解析：`GET /x/web-interface/search/type`

Cookie 支持：
- `bilibili.cookie` / `bilibili.sessdata`
- `bilibili.cookie_file` / `bilibili.sessdata_file`
- 自动刷新/检查：`auth_reload_interval` / `auth_check_interval`

## 调度与流程
- 调度器周期性入队 `fetch` / `check` / `cleanup` 任务
- Worker 处理流程：
  - `fetch`：拉取博主视频列表并入库
  - `check`：更新视频状态（OUT_OF_PRINT / STABLE）
- 稳定阈值：`scheduler.check_stable_days`（默认 30 天）

## 数据模型
见 `docs/mysql-schema.md`。

## 测试与覆盖率
```bash
go test ./... -cover
```
覆盖率基线：>= 85%（见 `docs/dev-standards.md`）。

## 常见问题
- MySQL 端口冲突：修改 `docker-compose.yml` 中的宿主机端口映射。
- 表不存在：重新执行 `docs/mysql-schema.md` 中的建表 SQL。
- 403/412：可能触发风控或权限限制，建议配置 Cookie 并降低频率。
- 风控风险：大量请求可能触发限制，建议调小 `limits.global_qps` / `limits.per_creator_qps`，并结合 `bilibili.risk_backoff_*` 退避配置。

## 路线图
- 实现视频下载与清理任务
- 增加博主更新/停用/恢复等 HTTP 接口
- 多平台适配（抖音/快手/小红书）
