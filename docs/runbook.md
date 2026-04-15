# 运行与部署说明（本地版）

## 1. 依赖
- Go 1.20+
- MySQL 8.0+
- 本地磁盘空间（建议 ≥ 2TB，按需求调整）

## 2. 目录结构约定
- 主存储根目录：`storage.root_dir`
- 主存储真实文件：`storage.root_dir/store/{platform}/{video_id}.mp4`
- 人工浏览目录：`storage.root_dir/library/{platform}/creators/{uid}_{safe_name}/`
- 日志：默认 stdout，可改为文件输出
- 配置文件：`configs/config.yaml`

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
```sql
CREATE DATABASE fetch DEFAULT CHARSET utf8mb4;
```
- 表结构迁移由服务启动自动执行，默认开启 `mysql.auto_migrate: true`。
- 当前权威 schema 来源：`migrations/00001_init.sql`。
- 若关闭 `mysql.auto_migrate`，需确保外部发布流程已完成同版本迁移后再启动服务。

## 4. 配置文件
复制并修改：
```
configs/config.example.yaml -> configs/config.yaml
```
关键字段：
- `storage.root_dir`
- `mysql.dsn`
- `mysql.auto_migrate`（默认 `true`，建议保持开启）
- `scheduler.check_stable_days`（默认 30 天）
- `scheduler.check_interval`（默认 24h）
- `creators.file`（博主列表文件路径，可用 `configs/creators.example.yaml` 参考）
- 从文件移除的博主会被自动停用（`status=paused`）。
- 如通过 `DELETE /creators/{id}` 手工移除博主，服务会标记为 `removed`，后续文件同步不会自动恢复。
- 通过前端设置页保存配置时，如果内容发生变化，后端会自动重启；重启窗口内接口短暂不可用属于正常现象。

## 5. 启动服务（示例）
```bash
go run ./cmd/server
```

预期日志：
- 出现「数据库迁移完成」表示 schema 已完成自动收敛。
- 首次启动后无需再手工执行建表 SQL。
- 启动阶段会执行一次浏览目录全量重建；若失败，日志会出现「浏览目录启动重建失败」。

## 6. 运行检查
- 服务启动日志无错误。
- 访问健康检查：`/healthz`、`/readyz`。
- 数据库连接正常。
- 若已有本地库存视频，应能在 `storage.root_dir/library/` 下看到按博主分类的浏览目录。
- `creator.json` / `index.json` 会随下载、下架、cleanup 和博主改名实时更新。

建议执行：
```bash
curl -sf http://127.0.0.1:8080/healthz
find /data/bilibili/store -maxdepth 2 -type f | head
find /data/bilibili/library -maxdepth 4 -type f | head
```

## 7. 常见问题
- 无法下载：检查网络与请求频率限制配置。
- 存储过快增长：检查清理策略与 `storage.max_bytes`。
- 任务堆积：提高并发或降低采集频率。
- 浏览目录为空但数据库里有已下载视频：
  - 先看日志里是否有「浏览目录启动重建失败」或「重建浏览目录失败」。
  - 确认 `video_files.path` 指向的真实文件仍存在于 `store/`。
  - 可直接删除 `storage.root_dir/library/` 后重启服务，让投影层全量重建。
- `index.json` 和目录内容不一致：
  - 浏览目录是派生产物，不要手工修 `json`。
  - 等待自动对账，或重启服务触发一次启动重建。
- 设置页保存配置后长期未恢复：
  - 先等待页面提示从「等待后端恢复」切换到「后端已恢复并重新加载配置」。
  - 若超过 45 秒仍未恢复，执行 `docker compose logs app --tail=200` 检查启动日志。
  - 同时确认 `./configs/config.yaml` 挂载路径可写，且 `app` 服务的 `restart` 策略仍然生效。

## 8. 建议的下一步
- 接入监控（Prometheus/Grafana）。
- 任务失败告警（邮件/IM）。
- 增加本地索引与文件校验。

## 9. 容器化部署
如需容器化，请参考：
- `docs/container-deploy.md`
