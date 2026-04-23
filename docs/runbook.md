# 运行与部署说明（本地版）

本文档只描述当前仓库的本地运行与排障方式。容器化部署请另见
`docs/container-deploy.md`。

## 1. 依赖

- Go 1.22+
- MySQL 8.0+
- `ffmpeg`
  - 当前下载链路在命中 DASH 音视频流时，会调用本机 `ffmpeg` 合并。
- 本地磁盘空间
  - 体量取决于你的 `storage.max_bytes` 配置；`2 TB` 只是常见目标，不是固定门
    槛。

## 2. 关键路径

- 默认配置文件：`configs/config.yaml`
- 可选配置覆盖：`FETCH_CONFIG=/path/to/config.yaml`
- 主存储目录：`storage.root_dir/store/{platform}/{video_id}.mp4`
- 浏览投影目录：
  `storage.root_dir/library/{platform}/creators/{uid}_{safe_name}/`
- 日志输出：默认 stdout

目录示例：

```text
/data/bilibili/
  store/
    bilibili/
      BV1xxx.mp4
  library/
    bilibili/
      creators/
        352981594_某博主/
          _meta/
            creator.json
            index.json
          videos/
          rare/
```

## 3. MySQL 初始化

先创建数据库：

```sql
CREATE DATABASE fetch DEFAULT CHARSET utf8mb4;
```

说明：

- 服务启动时会在 `mysql.auto_migrate: true` 下自动执行 `migrations/*.sql`。
- 当前权威 schema 以迁移链为准，不是单个 `00001_init.sql`。
- 若关闭 `mysql.auto_migrate`，必须先由外部流程完成对应版本迁移，再启动服
  务。

## 4. 配置准备

最小步骤：

```bash
cp configs/config.example.yaml configs/config.yaml
```

如需启用博主文件同步，再额外准备：

```bash
cp configs/creators.example.yaml configs/creators.yaml
```

关键字段：

- `storage.root_dir`
- `mysql.dsn`
- `mysql.auto_migrate`
- `creators.file`
- `creators.reload_interval`
- `scheduler.check_interval`
- `scheduler.check_stable_days`
- `discovery.enabled`
- `discovery.interval`
- `bilibili.cookie` / `bilibili.sessdata`

补充说明：

- `creators.file` 为空时，不启动文件同步。
- 从文件移除的博主会自动停用为 `paused`。
- 通过 `DELETE /creators/{id}` 手工移除的博主会变为 `removed`，后续文件同步
  不会自动恢复。
- 当前配置写回是整份文件写回；内容发生变化时，服务会在当前进程内触发一次
  重启并重新加载配置，不依赖容器 `restart` 策略。

## 5. 启动服务

使用默认配置路径：

```bash
go run ./cmd/server
```

使用自定义配置路径：

```bash
FETCH_CONFIG=/path/to/config.yaml go run ./cmd/server
```

常见启动日志：

- `数据库迁移完成`
  - 表示迁移链已执行完成。
- `启动恢复：已重新入队 ... 个运行中任务`
  - 表示启动恢复把残留 `running` 任务重新改回 `queued`。
- `启动恢复：已修复 ... 个 DOWNLOADING 视频状态`
  - 表示无活动下载支撑的 `DOWNLOADING` 视频已修正为 `NEW` 或 `DOWNLOADED`。
- `启动恢复：已修复 ... 个 DOWNLOADED 缺失文件视频状态`
  - 表示缺失真实文件的 `DOWNLOADED` 视频已回退为 `NEW`。
- `浏览目录启动重建失败: ...`
  - 表示浏览投影初始化失败；不一定阻塞主服务启动，但需要排查。
- `配置已更新，正在重启服务…`
  - 表示配置保存成功后，当前进程正在重启并重新加载配置。

## 6. 运行检查

最小检查命令：

```bash
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/readyz
curl -sf http://127.0.0.1:8080/system/status
```

若 `storage.root_dir=/data/bilibili`，还可以继续确认本地文件与浏览投影：

```bash
find /data/bilibili/store -maxdepth 2 -type f | head
find /data/bilibili/library -maxdepth 4 -type f | head
```

运行中应满足：

- `/healthz` 返回 `ok`
- `/readyz` 返回 `ready`
- `/system/status` 返回 JSON 快照
- 若已有库存文件，`library/` 下能看到按博主组织的浏览投影
- `creator.json` / `index.json` 会随着下载、下架、cleanup 和博主变更被更新

## 7. 常见问题

- 无法下载
  - 先检查网络、Cookie、风控退避状态和 `ffmpeg` 是否可用。
- 存储增长过快
  - 核对 `storage.max_bytes`、`storage.safe_bytes`、
    `storage.cleanup_retention_hours` 与 cleanup 执行情况。
- 任务堆积
  - 优先检查 MySQL、B 站风控状态和 `limits.download_concurrency` /
    `limits.global_qps`。
- 浏览目录为空但数据库里已有已下载视频
  - 先查看是否出现 `浏览目录启动重建失败` 或 `重建浏览目录失败` 日志。
  - 再确认 `video_files.path` 指向的真实文件仍存在于 `store/`。
  - `library/` 是派生产物，可删除 `storage.root_dir/library/` 后重启服务重
    建。
- `index.json` 与目录内容不一致
  - 不要手工改 `_meta/*.json`。
  - 等待投影层自动对账，或重启服务触发一次启动重建。
- 设置页保存配置后长期未恢复
  - 本地直跑时，先看当前终端是否出现 `配置已更新，正在重启服务…` 或新的启
    动报错。
  - Docker Compose 下，再看 `docker compose logs app --tail=200`。
  - 若配置文件未实际变化，后端不会重启；这时应返回 `changed=false`。
  - 若配置文件有变化但无法写回，优先检查 `configs/config.yaml` 或
    `FETCH_CONFIG` 指向文件的写权限。

## 8. 相关文档

- 配置项说明：`docs/config.md`
- API 与 SSE 契约：`docs/api.md`
- 存储与 cleanup 规则：`docs/storage-policy.md`
- 浏览投影与数据模型：`docs/data-model.md`
- 容器化部署：`docs/container-deploy.md`
