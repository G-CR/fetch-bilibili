# 配置设计（当前实现）

本文件描述当前 Go 服务实际支持的配置项、默认值、校验规则与生效语义。
权威实现位于 `internal/config/config.go`；默认运行配置路径为
`configs/config.yaml`，也可通过 `FETCH_CONFIG` 覆盖。

## 1. 配置入口与生效方式

- 默认配置文件路径是 `configs/config.yaml`。
- 可通过 `FETCH_CONFIG=/path/to/config.yaml` 指定其他配置文件路径。
- 解析入口为 `config.Load` / `config.Parse`，当前支持 YAML，也兼容 JSON。
- 当前不支持逐字段环境变量覆盖；运行参数以配置文件内容为准。
- `GET /system/config` 返回当前配置文件路径与全文内容。
- `PUT /system/config` 以“整份文档写回”的方式保存配置：
  - 保存前先执行 YAML 解析与业务校验。
  - 内容未变化时，不写盘，也不触发重启。
  - 内容有变化时，写回原路径，并异步请求后端重启。

## 2. 必填项与统一校验

- `storage.root_dir` 必填。
- `mysql.dsn` 必填。
- `storage.cleanup_retention_hours` 不能小于 `0`。
- `bilibili.fetch_page_size` 当前只拒绝负数；当值为 `0` 时，会回落为默认值
  `5`。
- 已废弃字段 `bilibili.cookie_file` 和 `bilibili.sessdata_file` 只要配置了
  非空值，就会直接报错。
- 当 `discovery.enabled=true` 时，必须同时满足：
  - `discovery.interval > 0`
  - `discovery.max_keywords_per_run > 0`
  - `discovery.max_pages_per_keyword > 0`
  - `discovery.max_candidates_per_run > 0`
  - `discovery.keywords` 至少 1 项

## 3. 配置项说明

### 3.1 `server`

- `addr`：默认 `:8080`。HTTP 监听地址。
- `read_timeout`：默认 `10s`。服务端读取请求的超时时间。
- `write_timeout`：默认 `30s`。服务端写响应的超时时间。

### 3.2 `storage`

- `root_dir`：必填。本地存储根目录。
- 服务会在 `root_dir` 下维护两套子目录：
  - `store/`：真实文件目录。
  - `library/`：便于人工浏览的投影目录。
- `max_bytes`：默认 `2199023255552`（`2 TB`）。总容量上限。
- `safe_bytes`：默认 `max_bytes * 0.9`。cleanup 目标安全容量。
- `keep_out_of_print`：默认 `true`。是否优先保留下架视频。
- `cleanup_retention_hours`：默认 `168`。视频下载成功后，真实文件至少保留
  `168` 小时才允许被 cleanup 删除。

#### `storage.delete_weights`

以下字段在配置结构与默认值中存在：

| 字段 | 默认值 |
| :--- | :--- |
| `out_of_print_penalty` | `-100` |
| `stable_bonus` | `30` |
| `downloaded_bonus` | `20` |
| `follower_weight` | `-10` |
| `view_weight` | `-8` |
| `favorite_weight` | `-6` |
| `age_30d_bonus` | `5` |
| `size_gb_bonus` | `3` |
| `last_access_30d_bonus` | `5` |

当前 cleanup 主流程尚未按这些权重做动态打分；它们目前属于已保留的配置
结构，而不是已经完全接线的运行时开关。

### 3.3 `scheduler`

- `fetch_interval`：默认 `45m`。定期拉取博主视频列表的周期。
- `check_interval`：默认 `24h`。定期检查视频状态的周期。
- `cleanup_interval`：默认 `24h`。定期执行 cleanup 的周期。
- `check_stable_days`：默认 `30`。视频进入稳定期的天数阈值。

### 3.4 `discovery`

- `enabled`：默认 `false`。只控制自动 discover 调度是否开启，不影响手动触
  发的候选发现接口。
- `interval`：默认 `24h`。自动 discover 调度周期。
- `max_keywords_per_run`：默认 `20`。单次 discover 最多使用的关键词数量。
- `max_pages_per_keyword`：默认 `2`。单个关键词最多抓取的搜索结果页数。
- `max_candidates_per_run`：默认 `100`。单次 discover 最多写入的候选数。
- `max_related_per_creator`：默认 `10`。单个已追踪博主最多扩散出的关联候选
  数。
- `auto_enqueue_fetch_on_approve`：默认 `true`。候选批准后，是否自动补一次
  定向 fetch。
- `score_version`：默认 `v1`。当前候选评分版本标记。
- `keywords`：默认空。只有在 `enabled=true` 时才要求至少提供 1 个关键词。

#### `discovery.score_weights`

| 字段 | 默认值 |
| :--- | :--- |
| `keyword_risk.max` | `40` |
| `activity_30d.low` | `5` |
| `activity_30d.medium` | `10` |
| `activity_30d.high` | `15` |
| `similarity.weak` | `5` |
| `similarity.medium` | `10` |
| `similarity.strong` | `20` |
| `deletion_trace.single` | `10` |
| `deletion_trace.max` | `20` |
| `account_size.small_bonus` | `10` |
| `account_size.oversize_penalty` | `-5` |
| `feedback.ignore_penalty` | `-15` |

### 3.5 `limits`

- `global_qps`：默认 `2`。全局请求速率限制。
- `per_creator_qps`：默认 `1`。单博主请求速率限制。
- `download_concurrency`：默认 `4`。当前实际控制整个 worker 池大小。
- `check_concurrency`：默认 `8`。当前主要体现在系统状态输出中，尚未真正接
  入执行并发控制。

### 3.6 `creators`

- `file`：默认空字符串。为空时，不启用文件同步。
- `reload_interval`：默认 `1m`。当 `file` 已配置时，服务启动后会先执行一
  次同步；只有当该值大于 `0` 时，才继续按周期轮询文件变化。
- 文件格式支持 YAML / JSON，可参考 `configs/creators.example.yaml`。
- 文件同步当前是“upsert + pause missing”模型：
  - 文件中存在的条目会尝试新增或更新。
  - 从文件中移除的活跃博主会被改成 `paused`。
  - 已经通过接口手工移除为 `removed` 的博主，不会因为文件再次出现而自
    动恢复。

### 3.7 `bilibili`

- `resolve_name_cache_ttl`：默认 `24h`。名称解析到 UID 的缓存时间。
- `request_timeout`：默认 `10s`。B 站客户端的请求超时。
- `user_agent`：默认 `fetch-bilibili/1.0`。请求头中的 `User-Agent`。
- `fetch_page_size`：默认 `5`。单次拉取单个博主最新投稿列表时请求的视频数
  量。
- `cookie`：默认空。优先使用完整 Cookie 字符串。
- `sessdata`：默认空。仅提供 `SESSDATA` 时，客户端会自动拼接成 Cookie。
- `bilibili.auth_check_interval`：默认 `12h`。认证观察器的检查周期；仅当值
  大于 `0` 时，启动时会先做一次检查，并开启后续定时检查。
- `bilibili.auth_reload_interval`：默认 `30m`。认证观察器的 reload 周期；启
  动时会先执行一次 reload，当值大于 `0` 时再开启后续定时 reload。
  当前 `ReloadAuth()` 仍只会记录一次 `no_change`，不代表已经支持独立的
  Cookie 热加载链路。
- `risk_backoff_base`：默认 `2s`。风控退避基础时长。
- `risk_backoff_max`：默认 `30s`。风控退避上限。
- `risk_backoff_jitter`：默认 `0.3`。风控退避抖动比例。

### 3.8 `mysql`

- `dsn`：必填。MySQL 连接串。
- `auto_migrate`：默认 `true`。服务启动时自动执行内置迁移。
- `max_open_conns`：默认 `20`。连接池最大打开连接数。
- `max_idle_conns`：默认 `10`。连接池最大空闲连接数。
- `conn_max_lifetime`：默认 `30m`。连接最大复用时长。

### 3.9 `logging`

- `level`：默认 `info`。
- `format`：默认 `json`。
- `output`：默认 `stdout`。

当前代码会解析并回填这 3 个字段，但大多数日志路径仍直接使用标准库
`log`，尚未按 `logging.level`、`logging.format`、`logging.output`
切换日志实现。

## 4. 当前尚未完全接线的配置项

- `storage.delete_weights`：已定义默认值，但当前 cleanup 主流程未按配置动态
  算分。
- `limits.check_concurrency`：已进入配置结构与状态输出，但尚未接入实际 worker
  并发控制。
- `logging.*`：已进入配置结构与默认值，但尚未驱动统一日志后端行为。

## 5. 最小可运行示例

以下配置足以让服务按默认值启动，其余字段会由代码回填默认值：

```yaml
storage:
  root_dir: "/data/bilibili"

mysql:
  dsn: "user:pass@tcp(127.0.0.1:3306)/fetch?charset=utf8mb4&parseTime=true&loc=Local"
```

若需要携带认证访问 B 站、启用文件同步或自动 discover，可在此基础上追加：

```yaml
bilibili:
  cookie: "SESSDATA=your-token"

creators:
  file: "configs/creators.yaml"
  reload_interval: "1m"

discovery:
  enabled: true
  interval: "24h"
  keywords:
    - "影视剪辑"
```
