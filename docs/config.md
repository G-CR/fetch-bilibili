# 配置设计（Go 服务）

本配置用于单机后端服务，当前通过配置文件加载，支持 YAML/JSON；可通过 `FETCH_CONFIG` 指定配置文件路径。

## 1. 配置原则
- 所有时间以秒/分钟/天为单位，统一换算为 `duration`。
- 重要参数提供默认值，便于快速启动。
- 清理权重可配置，默认比例 10:8:6。
- `storage.root_dir` 只配置一次，服务会在该目录下自动维护 `store/` 和 `library/` 两套子目录。

## 2. 配置示例（YAML）
```yaml
server:
  addr: ":8080"
  read_timeout: "10s"
  write_timeout: "30s"

storage:
  root_dir: "/data/bilibili"
  max_bytes: 2199023255552      # 2TB
  safe_bytes: 1979120929996     # 90% of 2TB
  keep_out_of_print: true
  cleanup_retention_hours: 168
  delete_weights:
    out_of_print_penalty: -100
    stable_bonus: 30
    downloaded_bonus: 20
    follower_weight: -10        # 默认 10
    view_weight: -8             # 默认 8
    favorite_weight: -6         # 默认 6
    age_30d_bonus: 5
    size_gb_bonus: 3
    last_access_30d_bonus: 5

scheduler:
  fetch_interval: "45m"
  check_interval: "24h"
  cleanup_interval: "24h"
  check_stable_days: 30

discovery:
  enabled: true
  interval: "24h"
  max_keywords_per_run: 20
  max_pages_per_keyword: 2
  max_candidates_per_run: 100
  max_related_per_creator: 10
  auto_enqueue_fetch_on_approve: true
  score_version: "v1"
  keywords:
    - "影视剪辑"
    - "直播切片"
    - "补档"
    - "重传"
    - "演唱会"
    - "MV"
  score_weights:
    keyword_risk:
      max: 40
    activity_30d:
      low: 5
      medium: 10
      high: 15
    similarity:
      weak: 5
      medium: 10
      strong: 20
    deletion_trace:
      single: 10
      max: 20
    account_size:
      small_bonus: 10
      oversize_penalty: -5
    feedback:
      ignore_penalty: -15

limits:
  global_qps: 2
  per_creator_qps: 1
  download_concurrency: 4
  check_concurrency: 8

creators:
  file: "configs/creators.yaml"
  reload_interval: "1m"

bilibili:
  resolve_name_cache_ttl: "24h"
  request_timeout: "10m"
  user_agent: "fetch-bilibili/1.0"
  fetch_page_size: 5
  cookie: ""         # 可直接粘贴 Cookie
  sessdata: ""       # 或只配置 SESSDATA
  auth_check_interval: "12h"
  auth_reload_interval: "30m"
  risk_backoff_base: "2s"
  risk_backoff_max: "30s"
  risk_backoff_jitter: 0.3

mysql:
  dsn: "user:pass@tcp(127.0.0.1:3306)/fetch?charset=utf8mb4&parseTime=true&loc=Local"
  auto_migrate: true
  max_open_conns: 20
  max_idle_conns: 10
  conn_max_lifetime: "30m"

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

## 3. 字段说明
### 3.1 server
- `addr`：HTTP 监听地址。
- `read_timeout` / `write_timeout`：请求超时时间。

### 3.2 storage
- `root_dir`：本地存储根目录。
- 服务会在其下自动维护：
  - `store/`：真实文件目录
  - `library/`：人工浏览投影目录
- `max_bytes`：最大容量。
- `safe_bytes`：清理后安全容量，建议为 `max_bytes * 0.9`。
- `keep_out_of_print`：是否强制保留绝版。
- `cleanup_retention_hours`：视频下载成功后，真实文件在 `store/` 中至少保留多少小时才允许被 cleanup 删除，默认 168 小时。
- `delete_weights`：清理评分权重（可配置）。

### 3.3 scheduler
- `fetch_interval`：博主视频列表拉取周期。
- `check_interval`：下架检测周期（默认 24h）。
- `cleanup_interval`：清理任务周期。
- `check_stable_days`：稳定阈值天数（默认 30）。

### 3.4 discovery
- `enabled`：是否启用自动发现调度。默认 `false`。
- `interval`：discover 任务自动调度周期，默认 `24h`。
- `max_keywords_per_run`：单次运行最多使用多少个关键词。
- `max_pages_per_keyword`：每个关键词最多抓取多少页搜索结果。
- `max_candidates_per_run`：单次 discover 最多写入多少个候选。
- `max_related_per_creator`：每个已追踪博主最多扩散出多少个关联候选。
- `auto_enqueue_fetch_on_approve`：批准候选后，是否自动为该博主创建一次定向 fetch。
- `score_version`：候选评分版本号，便于后续演进。
- `keywords`：关键词发现入口。
- `score_weights`：候选评分权重。

补充说明：
- 当前一期只支持 B 站。
- 自动发现分两层：
  - 关键词发现：直接搜索作者 / 视频。
  - 一跳关系扩散：从已追踪博主的最近公开视频出发，基于标题关键词与相似度做一次扩散。
- 关系扩散严格只做一跳，不会递归发现“候选的候选”。
- 候选不会自动转正，必须人工审核。
- `auto_enqueue_fetch_on_approve=true` 时，批准后只会定向拉取该博主，不会触发全量 fetch。

### 3.5 limits
- `global_qps`：全局请求速率限制。
- `per_creator_qps`：单博主请求速率限制。
- `download_concurrency`：下载并发数。
- `check_concurrency`：检查并发数。

### 3.6 creators
- `file`：博主列表文件路径（YAML/JSON）。
- `reload_interval`：动态刷新周期，设为 `0` 表示仅启动时加载。
- 可参考 `configs/creators.example.yaml`。
- 文件格式示例：
```yaml
creators:
  - uid: "123456"
    name: "示例博主"
    platform: "bilibili"
  - name: "通过名称解析"
    platform: "bilibili"
```
- 文件只负责新增/更新，不会自动删除数据库已有博主。
- 从文件中移除的博主会被自动停用（status=paused）。
- 如果某个博主已通过 HTTP 删除接口被手工移除（status=removed），文件同步不会将其自动恢复。
- 如需恢复已手工移除的博主，请再次通过 `POST /creators` 添加相同 UID。

### 3.6.1 浏览目录同步
- 浏览目录不是单独配置项，默认复用 `storage.root_dir`。
- 启动时会先做一次全量重建。
- 运行中通过 `creator.changed` / `video.changed` 事件按博主增量重建。
- 当前版本内置每 6 小时一次全量对账，用于修复投影偏差；该周期暂未暴露为配置项。

### 3.7 bilibili
- `resolve_name_cache_ttl`：名称解析为 UID 的缓存时间。
- `request_timeout`：请求超时。
- `user_agent`：请求 UA。
- `fetch_page_size`：每次拉取单个博主投稿列表时请求的最新视频数量，默认 5。
- `cookie`：完整 Cookie 字符串（优先使用）。
- `sessdata`：仅提供 SESSDATA 时自动拼成 Cookie。
- `auth_check_interval`：Cookie 登录状态检查周期（默认 12h）。
- `auth_reload_interval`：认证观察任务的刷新周期字段（默认 30m）；当前不再支持从文件自动刷新 Cookie。
- `risk_backoff_base`：触发风控时的基础退避时长。
- `risk_backoff_max`：退避最大时长（指数增长上限）。
- `risk_backoff_jitter`：退避抖动比例（0~1）。

### 3.8 mysql
- `dsn`：MySQL 连接串。
- `auto_migrate`：是否在服务启动时自动执行内置迁移，默认开启；仅在外部迁移平台已接管 schema 变更时关闭。
- `max_open_conns` / `max_idle_conns` / `conn_max_lifetime`：连接池配置。

### 3.9 logging
- `level`：日志级别。
- `format`：日志格式（json/text）。
- `output`：输出位置（stdout/file）。

## 4. 配置加载建议
- 默认配置文件路径是 `configs/config.yaml`。
- 可通过 `FETCH_CONFIG=/path/to/config.yaml` 指定其它配置文件。
- 当前不支持逐字段环境变量覆盖；需要修改运行配置时，请直接编辑配置文件或调用 `PUT /system/config`。
- 已废弃的 `bilibili.cookie_file` / `bilibili.sessdata_file` 会在启动时直接报错。
- 关键配置缺失时（如 `storage.root_dir`、`mysql.dsn`）启动失败并给出提示。
