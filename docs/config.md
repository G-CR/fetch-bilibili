# 配置设计（Go 服务）

本配置用于单机后端服务，支持环境变量或配置文件（YAML/JSON/TOML）加载。

## 1. 配置原则
- 所有时间以秒/分钟/天为单位，统一换算为 `duration`。
- 重要参数提供默认值，便于快速启动。
- 清理权重可配置，默认比例 10:8:6。

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
  request_timeout: "10s"
  user_agent: "fetch-bilibili/1.0"
  cookie: ""         # 可直接粘贴 Cookie
  sessdata: ""       # 或只配置 SESSDATA
  cookie_file: ""    # 读取完整 Cookie 或 SESSDATA 文件
  sessdata_file: ""  # 仅 SESSDATA 文件
  auth_check_interval: "12h"
  auth_reload_interval: "30m"
  risk_backoff_base: "2s"
  risk_backoff_max: "30s"
  risk_backoff_jitter: 0.3

mysql:
  dsn: "user:pass@tcp(127.0.0.1:3306)/fetch?charset=utf8mb4&parseTime=true&loc=Local"
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
- `max_bytes`：最大容量。
- `safe_bytes`：清理后安全容量，建议为 `max_bytes * 0.9`。
- `keep_out_of_print`：是否强制保留绝版。
- `delete_weights`：清理评分权重（可配置）。

### 3.3 scheduler
- `fetch_interval`：博主视频列表拉取周期。
- `check_interval`：下架检测周期（默认 24h）。
- `cleanup_interval`：清理任务周期。
- `check_stable_days`：稳定阈值天数（默认 30）。

### 3.4 limits
- `global_qps`：全局请求速率限制。
- `per_creator_qps`：单博主请求速率限制。
- `download_concurrency`：下载并发数。
- `check_concurrency`：检查并发数。

### 3.5 creators
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

### 3.6 bilibili
- `resolve_name_cache_ttl`：名称解析为 UID 的缓存时间。
- `request_timeout`：请求超时。
- `user_agent`：请求 UA。
- `cookie`：完整 Cookie 字符串（优先使用）。
- `sessdata`：仅提供 SESSDATA 时自动拼成 Cookie。
- `cookie_file`：Cookie 文件路径；内容含 `=` 视为完整 Cookie，否则视为 SESSDATA。
- `sessdata_file`：SESSDATA 文件路径（仅包含 token）。
- `auth_check_interval`：Cookie 登录状态检查周期（默认 12h）。
- `auth_reload_interval`：从文件刷新 Cookie 的周期（默认 30m）。
- `cookie_file` 与 `sessdata_file` 同时配置时优先使用 `cookie_file`，刷新成功后覆盖当前 Cookie。
- `risk_backoff_base`：触发风控时的基础退避时长。
- `risk_backoff_max`：退避最大时长（指数增长上限）。
- `risk_backoff_jitter`：退避抖动比例（0~1）。

### 3.7 mysql
- `dsn`：MySQL 连接串。
- `max_open_conns` / `max_idle_conns` / `conn_max_lifetime`：连接池配置。

### 3.8 logging
- `level`：日志级别。
- `format`：日志格式（json/text）。
- `output`：输出位置（stdout/file）。

## 4. 配置加载建议
- 默认支持 `config.yaml`，并允许环境变量覆盖（如 `FETCH_SERVER_ADDR`）。
- 关键配置缺失时（如 `storage.root_dir`、`mysql.dsn`）启动失败并给出提示。
