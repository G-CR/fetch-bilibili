# 容器化部署说明（Docker / Compose）

## 1. 目标
- 通过 Docker 构建服务镜像。
- 通过 Docker Compose 启动 MySQL + 应用。

## 2. 文件说明
- `Dockerfile`：多阶段构建 Go 二进制。
- `docker-compose.yml`：编排 MySQL 与应用。
- `configs/config.example.yaml`：容器默认配置示例。

## 3. 构建镜像
```bash
docker build -t fetch-bilibili:dev .
```

## 4. 使用 Compose 启动
```bash
docker compose up -d
```

## 5. 配置与挂载
- 应用配置：默认映射 `/app/config.yaml`。
- 视频存储目录：`./data/bilibili` → `/data/bilibili`。
- MySQL 数据目录：`./data/mysql` → `/var/lib/mysql`。
- 配置文件路径可通过环境变量 `FETCH_CONFIG` 覆盖。

### 5.1 Cookie 文件挂载示例
当使用 `bilibili.cookie_file` / `bilibili.sessdata_file` 时，建议通过只读挂载提供文件：
```yaml
  app:
    volumes:
      - ./configs/config.example.yaml:/app/config.yaml:ro
      - ./secrets/bilibili_cookie.txt:/app/secrets/bilibili_cookie.txt:ro
    environment:
      FETCH_CONFIG: /app/config.yaml
```
并在配置文件中指向容器内路径：
```yaml
bilibili:
  cookie_file: "/app/secrets/bilibili_cookie.txt"
```

### 5.2 博主列表文件挂载示例
```yaml
  app:
    volumes:
      - ./configs/config.example.yaml:/app/config.yaml:ro
      - ./configs/creators.example.yaml:/app/creators.yaml:ro
```
并在配置文件中指向容器内路径：
```yaml
creators:
  file: "/app/creators.yaml"
  reload_interval: "1m"
```

## 6. 初始化数据库
首次启动后需要执行建表 SQL：
- 使用 `docs/mysql-schema.md` 中的 SQL 创建表。
- 后续建议用迁移工具（golang-migrate 或 Goose）。

## 7. 常见问题
- 启动后无法连接 MySQL：
  - 确认 `configs/config.example.yaml` 中的 DSN 与 `docker-compose.yml` 一致。

## 8. 下一步建议
- 生成 Go 工程骨架后再执行容器构建。
- 增加 `healthcheck` 与 `/healthz` 接口。
