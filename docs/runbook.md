# 运行与部署说明（本地版）

## 1. 依赖
- Go 1.20+
- MySQL 8.0+
- 本地磁盘空间（建议 ≥ 2TB，按需求调整）

## 2. 目录结构约定
- 视频文件目录：`storage.root_dir` 指定（默认示例 `/data/bilibili`）
- 日志：默认 stdout，可改为文件输出
- 配置文件：`configs/config.yaml`

## 3. MySQL 初始化
```sql
CREATE DATABASE fetch DEFAULT CHARSET utf8mb4;
```
- 执行迁移：使用 `docs/mysql-schema.md` 中的建表 SQL（临时）
- 后续建议用迁移工具统一管理（见 `docs/mysql-schema.md`）

## 4. 配置文件
复制并修改：
```
configs/config.example.yaml -> configs/config.yaml
```
关键字段：
- `storage.root_dir`
- `mysql.dsn`
- `scheduler.check_stable_days`（默认 30 天）
- `scheduler.check_interval`（默认 24h）
- `creators.file`（博主列表文件路径，可用 `configs/creators.example.yaml` 参考）
- 从文件移除的博主会被自动停用（`status=paused`）。

## 5. 启动服务（示例）
```bash
go run ./cmd/server
```

## 6. 运行检查
- 服务启动日志无错误。
- 访问健康检查（建议实现 `/healthz`）。
- 数据库连接正常。

## 7. 常见问题
- 无法下载：检查网络与请求频率限制配置。
- 存储过快增长：检查清理策略与 `storage.max_bytes`。
- 任务堆积：提高并发或降低采集频率。

## 8. 建议的下一步
- 接入监控（Prometheus/Grafana）。
- 任务失败告警（邮件/IM）。
- 增加本地索引与文件校验。

## 9. 容器化部署
如需容器化，请参考：
- `docs/container-deploy.md`
